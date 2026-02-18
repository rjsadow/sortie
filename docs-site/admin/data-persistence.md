# Data Persistence Strategy

This document defines what data persists in Sortie, where it is stored,
and how to manage it across deployments.

## Overview

Sortie uses a layered persistence strategy:

| Data Type        | Storage                       | Persistence | Survives Restart |
| ---------------- | ----------------------------- | ----------- | ---------------- |
| User Settings    | Browser localStorage          | Per-browser | Yes (client)     |
| App Specs        | SQLite or PostgreSQL           | Server-side | Yes              |
| Session Metadata | Database + in-memory cache     | Server-side | Partial          |
| Workspace Volume | Kubernetes emptyDir            | Pod-local   | No               |
| Audit/Analytics  | SQLite or PostgreSQL           | Server-side | Yes              |
| Recordings       | Local filesystem or S3         | Server-side | Yes (with PVC or S3) |

## Database Backend

Sortie supports two database backends: **SQLite** (default) and **PostgreSQL**.
Set the backend via the `SORTIE_DB_TYPE` environment variable.

### SQLite (default)

SQLite is the zero-configuration default. The database is a single file
created automatically on first run.

```bash
SORTIE_DB_TYPE=sqlite        # or omit entirely (sqlite is default)
SORTIE_DB=sortie.db          # file path
```

SQLite is suitable for single-instance deployments. For multi-replica
or high-availability setups, use PostgreSQL.

### PostgreSQL

PostgreSQL enables multi-replica deployments, horizontal scaling, and
standard database tooling for backups and monitoring.

Configure via a full DSN or individual parameters:

```bash
SORTIE_DB_TYPE=postgres

# Option 1: Full DSN (takes precedence)
SORTIE_DB_DSN=postgres://sortie:password@db.example.com:5432/sortie?sslmode=require

# Option 2: Individual parameters
SORTIE_DB_HOST=db.example.com
SORTIE_DB_PORT=5432
SORTIE_DB_NAME=sortie
SORTIE_DB_USER=sortie
SORTIE_DB_PASSWORD=password
SORTIE_DB_SSLMODE=require
```

### Helm Chart Configuration

The Helm chart supports both backends via `values.yaml`:

```yaml
# SQLite (default)
database:
  type: sqlite
  sqlite:
    path: "/data/sortie.db"

# PostgreSQL
database:
  type: postgres
  postgres:
    host: "db.example.com"
    port: 5432
    database: "sortie"
    user: "sortie"
    password: ""           # or use existingSecret
    sslMode: "require"
    # existingSecret: my-pg-secret   # references a K8s Secret with "password" key
    # dsn: ""                        # full DSN (overrides individual params)
```

### Migrations

Sortie uses [golang-migrate](https://github.com/golang-migrate/migrate)
with embedded SQL files for each dialect. Migrations run automatically on
startup. Each backend has its own migration files in
`internal/db/migrations/{sqlite,postgres}/`.

When upgrading from a pre-migration database, Sortie detects existing tables
and baselines the migration version automatically.

## User Settings

User preferences are stored client-side in the browser's localStorage.

### What Persists

| Key                   | Value                  | Purpose                  |
| --------------------- | ---------------------- | ------------------------ |
| `sortie-theme`     | `'dark'` or `'light'`  | Color scheme preference  |
| `sortie-collapsed` | JSON array of names    | Collapsed categories     |

### Storage Location

```javascript
// web/src/App.tsx
localStorage.getItem('sortie-theme')
localStorage.getItem('sortie-collapsed')
```

### Characteristics

- **Scope**: Per-browser, per-origin
- **Capacity**: ~5MB per origin (browser limit)
- **Durability**: Survives browser restarts, cleared if user clears site data
- **Cross-device**: Not synchronized; each browser has independent settings

### Future Considerations

For server-side user preferences (Phase 3), store in SQLite with user ID:

```sql
CREATE TABLE user_preferences (
    user_id TEXT PRIMARY KEY,
    theme TEXT DEFAULT 'light',
    collapsed_categories TEXT,  -- JSON array
    favorites TEXT,             -- JSON array of app IDs
    updated_at TIMESTAMP
);
```

## App Specs (Application Definitions)

Application metadata is the core data model, stored in the configured database.

### Schema

```sql
CREATE TABLE applications (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    url TEXT,
    icon TEXT,
    category TEXT,
    visibility TEXT NOT NULL DEFAULT 'public',  -- 'public', 'approved', or 'admin_only'
    launch_type TEXT DEFAULT 'url',             -- 'url' or 'container'
    container_image TEXT                        -- Docker image for container apps
);
```

### Data Model

```go
// internal/db/db.go
type Application struct {
    ID             string             `json:"id"`
    Name           string             `json:"name"`
    Description    string             `json:"description"`
    URL            string             `json:"url"`
    Icon           string             `json:"icon"`
    Category       string             `json:"category"`
    Visibility     CategoryVisibility `json:"visibility"`
    LaunchType     string             `json:"launch_type"`
    ContainerImage string             `json:"container_image,omitempty"`
}
```

The `visibility` field controls who can see the application. See the
[Access Control guide](/guide/access-control) for details.

### Seeding

On first run, if the `applications` table is empty, Sortie seeds from JSON:

```bash
# Via environment variable
SORTIE_SEED=/path/to/apps.json

# Or command-line flag
./sortie -seed /path/to/apps.json
```

Example seed file (`examples/apps-with-containers.json`):

```json
{
  "applications": [
    {
      "id": "browser",
      "name": "Web Browser",
      "description": "Chromium browser in container",
      "category": "Tools",
      "visibility": "public",
      "launch_type": "container",
      "container_image": "ghcr.io/example/chromium:latest"
    }
  ]
}
```

### CRUD Operations

| Endpoint         | Method | Description            |
| ---------------- | ------ | ---------------------- |
| `/api/apps`      | GET    | List all applications  |
| `/api/apps`      | POST   | Create application     |
| `/api/apps/{id}` | GET    | Get single application |
| `/api/apps/{id}` | PUT    | Update application     |
| `/api/apps/{id}` | DELETE | Delete application     |

All mutations are logged to the audit table.

## Session Metadata

Sessions track active container-based application instances.

### Session Schema

```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    app_id TEXT NOT NULL,
    pod_name TEXT,
    pod_ip TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_status ON sessions(status);
```

### Session States

```text
┌──────────┐     success     ┌─────────┐
│ creating │ ───────────────▶│ running │
└────┬─────┘                 └────┬────┘
     │                            │
     │ failure                    │ user action / timeout / failure
     ▼                            ▼
┌──────────┐              ┌─────────────────┐
│  failed  │              │ stopped/expired │
└──────────┘              └─────────────────┘
```

Valid state transitions (`internal/sessions/state.go`):

| From       | To                             |
| ---------- | ------------------------------ |
| `creating` | `running`, `failed`            |
| `running`  | `stopped`, `expired`, `failed` |
| `stopped`  | (terminal)                     |
| `expired`  | (terminal)                     |
| `failed`   | (terminal)                     |

### In-Memory Cache

The Session Manager maintains an in-memory map for fast lookups:

```go
// internal/sessions/manager.go
type Manager struct {
    sessions map[string]*db.Session  // Cached sessions
    mu       sync.RWMutex
    db       *db.DB
    k8s      *kubernetes.Clientset
}
```

Cache is populated on-demand from the database and invalidated on session
termination.

### Session Lifecycle

1. **Creation**: User launches container app → session created with `creating` status
2. **Pod Startup**: Kubernetes pod starts → goroutine waits for readiness
3. **Running**: Pod ready → status updated to `running`, pod IP captured
4. **Active Use**: User interacts via VNC WebSocket proxy
5. **Cleanup**: Either:
   - User terminates → `stopped`
   - Timeout (default 2h) → `expired`
   - Error → `failed`
6. **Deletion**: Kubernetes pod deleted, session removed from cache

### Configuration

```bash
SORTIE_SESSION_TIMEOUT=120          # Minutes until expiry
SORTIE_SESSION_CLEANUP_INTERVAL=5   # Minutes between cleanup
SORTIE_POD_READY_TIMEOUT=120        # Seconds to wait for pod
```

## Workspace Volume

Workspace volumes provide temporary storage for container sessions.

### Current Configuration

In `deploy/kubernetes/deployment.yaml`, the Sortie server uses ephemeral storage:

```yaml
volumeMounts:
  - name: data
    mountPath: /data
  - name: tmp
    mountPath: /tmp

volumes:
  - name: data
    emptyDir: {}    # Ephemeral - lost on pod restart
  - name: tmp
    emptyDir: {}
```

### Session Pod Volumes

Container sessions (spawned by Session Manager) also use ephemeral storage:

```yaml
# Pod created by internal/sessions/manager.go
volumes:
  - name: workspace
    emptyDir: {}
  - name: x11
    emptyDir: {}    # Shared X11 socket between app and VNC sidecar
```

### Persistence Options

For persistent workspace data, configure a PersistentVolumeClaim:

```yaml
# Per-session PVC (dynamic provisioning)
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: session-${SESSION_ID}
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 5Gi
  storageClassName: fast-ssd
```

Mount in session pod:

```yaml
volumeMounts:
  - name: workspace
    mountPath: /home/user/workspace
volumes:
  - name: workspace
    persistentVolumeClaim:
      claimName: session-${SESSION_ID}
```

### Resource Quotas

Default limits in `deploy/kubernetes/resource-quota.yaml`:

```yaml
ResourceQuota:
  persistentvolumeclaims: "10"    # Max PVCs in namespace
  requests.storage: "100Gi"       # Total storage requests
```

## Audit Logs and Analytics

### Audit Log Schema

```sql
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TIMESTAMP NOT NULL,
    user TEXT NOT NULL,
    action TEXT NOT NULL,
    details TEXT
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp);
```

### Tracked Actions

| Action              | Trigger                   | Details         |
| ------------------- | ------------------------- | --------------- |
| `CREATE_APP`        | POST /api/apps            | App name, ID    |
| `UPDATE_APP`        | PUT /api/apps/{id}        | App ID, changes |
| `DELETE_APP`        | DELETE /api/apps/{id}     | App ID          |
| `CREATE_SESSION`    | POST /api/sessions        | User ID, App ID |
| `TERMINATE_SESSION` | DELETE /api/sessions/{id} | Session ID      |

### Analytics Schema

```sql
CREATE TABLE analytics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL
);

CREATE INDEX idx_analytics_app_id ON analytics(app_id);
CREATE INDEX idx_analytics_timestamp ON analytics(timestamp);
```

### API Endpoints

| Endpoint          | Method | Description             |
| ----------------- | ------ | ----------------------- |
| `/api/audit`      | GET    | Last 100 audit entries  |
| `/api/analytics`  | GET    | App launch statistics   |

## Backups

### SQLite Backup

#### Manual Backup

```bash
# From within the cluster
kubectl exec -n sortie deployment/sortie -- \
  sqlite3 /data/sortie.db ".backup '/data/backup.db'"

# Copy to local machine
POD=$(kubectl get pod -n sortie -l app=sortie \
  -o jsonpath='{.items[0].metadata.name}')
kubectl cp sortie/$POD:/data/backup.db \
  ./sortie-backup-$(date +%Y%m%d).db
```

#### Automated Backup CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: sortie-backup
  namespace: sortie
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: alpine:3.19
              command:
                - /bin/sh
                - -c
                - |
                  apk add --no-cache sqlite
                  BACKUP="/backup/sortie-$(date +%Y%m%d).db"
                  sqlite3 /data/sortie.db ".backup '$BACKUP'"
              volumeMounts:
                - name: data
                  mountPath: /data
                  readOnly: true
                - name: backup
                  mountPath: /backup
          volumes:
            - name: data
              persistentVolumeClaim:
                claimName: sortie-data
            - name: backup
              persistentVolumeClaim:
                claimName: sortie-backup
          restartPolicy: OnFailure
```

### PostgreSQL Backup

For PostgreSQL deployments, use standard `pg_dump` tooling:

```bash
# Manual backup
pg_dump -h db.example.com -U sortie -d sortie > sortie-backup-$(date +%Y%m%d).sql

# Compressed backup
pg_dump -h db.example.com -U sortie -d sortie -Fc > sortie-backup-$(date +%Y%m%d).dump

# Restore
pg_restore -h db.example.com -U sortie -d sortie --clean sortie-backup.dump
```

Most managed PostgreSQL services (AWS RDS, Cloud SQL, Azure Database)
provide automated backups. For self-hosted Postgres, configure
`pg_dump` via CronJob or use a tool like
[pgBackRest](https://pgbackrest.org/).

### Backup Storage

Create a separate PVC for backups:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: sortie-backup
  namespace: sortie
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard
```

### Restore Procedure

1. Stop the Sortie deployment:

   ```bash
   kubectl scale deployment sortie -n sortie --replicas=0
   ```

2. Copy backup to the data volume:

   ```bash
   kubectl cp ./sortie-backup.db sortie/restore-pod:/data/sortie.db
   ```

3. Restart the deployment:

   ```bash
   kubectl scale deployment sortie -n sortie --replicas=1
   ```

### Disaster Recovery

| Scenario           | Recovery Method                       |
| ------------------ | ------------------------------------- |
| Pod restart        | PVC data intact, automatic recovery   |
| PVC corruption     | Restore from backup                   |
| Namespace deletion | Restore manifests + backup database   |
| Cluster loss       | Re-deploy from GitOps + off-cluster   |

### Off-Cluster Backup

For disaster recovery, sync backups to external storage:

```bash
# Example: sync to S3-compatible storage
kubectl exec -n sortie deployment/sortie -- \
  sqlite3 /data/sortie.db ".backup '/tmp/backup.db'"

kubectl cp sortie/sortie-pod:/tmp/backup.db - | \
  aws s3 cp - s3://backups/sortie/$(date +%Y%m%d).db
```

## Configuration Persistence

### Environment Variables

Primary configuration via environment (`.env.example`):

```bash
SORTIE_PORT=8080
SORTIE_DB_TYPE=sqlite             # or "postgres"
SORTIE_DB=sortie.db               # SQLite file path
# SORTIE_DB_DSN=postgres://...    # PostgreSQL connection string
SORTIE_SEED=examples/apps.json
SORTIE_CONFIG=branding.json
SORTIE_NAMESPACE=sortie
```

### Branding Configuration

Optional JSON file for branding overrides:

```json
{
  "logo_url": "https://cdn.example.com/logo.png",
  "primary_color": "#398D9B",
  "secondary_color": "#4AB7C3",
  "tenant_name": "Acme Corp"
}
```

Mounted via ConfigMap in Kubernetes deployments.

## Summary

| What             | Where                            | How to Backup          |
| ---------------- | -------------------------------- | ---------------------- |
| User preferences | Browser localStorage             | N/A (client-side)      |
| Applications     | Database `applications` table    | sqlite3 .backup / pg_dump |
| Sessions         | Database `sessions` + memory     | DB backup (no live)    |
| Audit logs       | Database `audit_log` table       | Database backup        |
| Analytics        | Database `analytics` table       | Database backup        |
| Branding config  | ConfigMap / JSON file            | GitOps                 |
| Recordings       | Local filesystem or S3           | PVC / S3 replication   |
| K8s manifests    | Git repository                   | GitOps                 |

For production deployments:

1. **SQLite**: use PersistentVolumeClaim for database storage; limited to single replica
2. **PostgreSQL**: recommended for multi-replica, HA, and horizontal scaling
3. Configure automated daily backups (CronJob for SQLite, `pg_dump` or managed backups for Postgres)
4. Test restore procedures regularly
