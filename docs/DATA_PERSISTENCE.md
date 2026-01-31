# Data Persistence Strategy

This document defines what data persists in Launchpad, where it is stored,
and how to manage it across deployments.

## Overview

Launchpad uses a layered persistence strategy:

| Data Type        | Storage                  | Persistence | Survives Restart |
| ---------------- | ------------------------ | ----------- | ---------------- |
| User Settings    | Browser localStorage     | Per-browser | Yes (client)     |
| App Specs        | SQLite database          | Server-side | Yes (with PVC)   |
| Session Metadata | SQLite + in-memory cache | Server-side | Partial          |
| Workspace Volume | Kubernetes emptyDir      | Pod-local   | No               |
| Audit/Analytics  | SQLite database          | Server-side | Yes (with PVC)   |

## User Settings

User preferences are stored client-side in the browser's localStorage.

### What Persists

| Key                   | Value                  | Purpose                  |
| --------------------- | ---------------------- | ------------------------ |
| `launchpad-theme`     | `'dark'` or `'light'`  | Color scheme preference  |
| `launchpad-collapsed` | JSON array of names    | Collapsed categories     |

### Storage Location

```javascript
// web/src/App.tsx
localStorage.getItem('launchpad-theme')
localStorage.getItem('launchpad-collapsed')
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

Application metadata is the core data model, stored in SQLite.

### Schema

```sql
CREATE TABLE applications (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    url TEXT,
    icon TEXT,
    category TEXT,
    launch_type TEXT DEFAULT 'url',      -- 'url' or 'container'
    container_image TEXT                  -- Docker image for container apps
);
```

### Data Model

```go
// internal/db/db.go
type Application struct {
    ID             string `json:"id"`
    Name           string `json:"name"`
    Description    string `json:"description"`
    URL            string `json:"url"`
    Icon           string `json:"icon"`
    Category       string `json:"category"`
    LaunchType     string `json:"launch_type"`
    ContainerImage string `json:"container_image,omitempty"`
}
```

### Seeding

On first run, if the `applications` table is empty, Launchpad seeds from JSON:

```bash
# Via environment variable
LAUNCHPAD_SEED=/path/to/apps.json

# Or command-line flag
./launchpad -seed /path/to/apps.json
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
LAUNCHPAD_SESSION_TIMEOUT=120          # Minutes until expiry
LAUNCHPAD_SESSION_CLEANUP_INTERVAL=5   # Minutes between cleanup
LAUNCHPAD_POD_READY_TIMEOUT=120        # Seconds to wait for pod
```

## Workspace Volume

Workspace volumes provide temporary storage for container sessions.

### Current Configuration

In `deploy/kubernetes/deployment.yaml`, the Launchpad server uses ephemeral storage:

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

### SQLite Database Backup

#### Manual Backup

```bash
# From within the cluster
kubectl exec -n launchpad deployment/launchpad -- \
  sqlite3 /data/launchpad.db ".backup '/data/backup.db'"

# Copy to local machine
POD=$(kubectl get pod -n launchpad -l app=launchpad \
  -o jsonpath='{.items[0].metadata.name}')
kubectl cp launchpad/$POD:/data/backup.db \
  ./launchpad-backup-$(date +%Y%m%d).db
```

#### Automated Backup CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: launchpad-backup
  namespace: launchpad
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
                  BACKUP="/backup/launchpad-$(date +%Y%m%d).db"
                  sqlite3 /data/launchpad.db ".backup '$BACKUP'"
              volumeMounts:
                - name: data
                  mountPath: /data
                  readOnly: true
                - name: backup
                  mountPath: /backup
          volumes:
            - name: data
              persistentVolumeClaim:
                claimName: launchpad-data
            - name: backup
              persistentVolumeClaim:
                claimName: launchpad-backup
          restartPolicy: OnFailure
```

### Backup Storage

Create a separate PVC for backups:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: launchpad-backup
  namespace: launchpad
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard
```

### Restore Procedure

1. Stop the Launchpad deployment:

   ```bash
   kubectl scale deployment launchpad -n launchpad --replicas=0
   ```

2. Copy backup to the data volume:

   ```bash
   kubectl cp ./launchpad-backup.db launchpad/restore-pod:/data/launchpad.db
   ```

3. Restart the deployment:

   ```bash
   kubectl scale deployment launchpad -n launchpad --replicas=1
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
kubectl exec -n launchpad deployment/launchpad -- \
  sqlite3 /data/launchpad.db ".backup '/tmp/backup.db'"

kubectl cp launchpad/launchpad-pod:/tmp/backup.db - | \
  aws s3 cp - s3://backups/launchpad/$(date +%Y%m%d).db
```

## Configuration Persistence

### Environment Variables

Primary configuration via environment (`.env.example`):

```bash
LAUNCHPAD_PORT=8080
LAUNCHPAD_DB=launchpad.db
LAUNCHPAD_SEED=examples/apps.json
LAUNCHPAD_CONFIG=branding.json
LAUNCHPAD_NAMESPACE=launchpad
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

| What             | Where                         | How to Backup       |
| ---------------- | ----------------------------- | ------------------- |
| User preferences | Browser localStorage          | N/A (client-side)   |
| Applications     | SQLite `applications` table   | Database backup     |
| Sessions         | SQLite `sessions` + memory    | DB backup (no live) |
| Audit logs       | SQLite `audit_log` table      | Database backup     |
| Analytics        | SQLite `analytics` table      | Database backup     |
| Branding config  | ConfigMap / JSON file         | GitOps              |
| K8s manifests    | Git repository                | GitOps              |

For production deployments:

1. Use PersistentVolumeClaim for database storage
2. Configure automated daily backups
3. Test restore procedures regularly
4. Consider PostgreSQL migration for high-availability requirements
