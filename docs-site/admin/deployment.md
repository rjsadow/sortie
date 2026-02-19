# High Availability Deployment Guide

This guide covers deploying Sortie in a highly available
configuration using Kubernetes.

## Prerequisites

- Kubernetes cluster (1.21+)
- kubectl configured
- Container registry access
- Persistent volume provisioner (for SQLite storage), or a PostgreSQL instance

## Building the Container Image

```dockerfile
# Dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN npm install --prefix web && npm run build --prefix web
RUN CGO_ENABLED=0 go build -o sortie .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/sortie .
EXPOSE 8080
CMD ["./sortie", "-db", "/data/sortie.db"]
```

Build and push:

```bash
docker build -t your-registry/sortie:latest .
docker push your-registry/sortie:latest
```

## Kubernetes Manifests

### Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: sortie
```

### ConfigMap for Branding

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sortie-config
  namespace: sortie
data:
  branding.json: |
    {
      "logo_url": "https://your-cdn.com/logo.png",
      "primary_color": "#398D9B",
      "secondary_color": "#4AB7C3",
      "tenant_name": "Your Organization"
    }
```

### PersistentVolumeClaim

```yaml
# pvc.yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: sortie-data
  namespace: sortie
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: standard
```

### Deployment

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sortie
  namespace: sortie
spec:
  replicas: 1  # Single replica for SQLite; see HA notes below
  selector:
    matchLabels:
      app: sortie
  template:
    metadata:
      labels:
        app: sortie
    spec:
      containers:
        - name: sortie
          image: your-registry/sortie:latest
          ports:
            - containerPort: 8080
          env:
            - name: SORTIE_CONFIG
              value: /config/branding.json
          volumeMounts:
            - name: data
              mountPath: /data
            - name: config
              mountPath: /config
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
          livenessProbe:
            httpGet:
              path: /api/apps
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /api/apps
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: sortie-data
        - name: config
          configMap:
            name: sortie-config
```

### Service

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: sortie
  namespace: sortie
spec:
  selector:
    app: sortie
  ports:
    - port: 80
      targetPort: 8080
  type: ClusterIP
```

### Ingress with TLS

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: sortie
  namespace: sortie
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
    - hosts:
        - sortie.example.com
      secretName: sortie-tls
  rules:
    - host: sortie.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: sortie
                port:
                  number: 80
```

## Deploying

```bash
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f pvc.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
```

## High Availability Considerations

### Choosing a Database Backend

Sortie supports SQLite and PostgreSQL. Choose based on your scaling needs:

| Criteria           | SQLite                        | PostgreSQL                    |
| ------------------ | ----------------------------- | ----------------------------- |
| Setup complexity   | Zero config (file-based)      | Requires a Postgres instance  |
| Replicas           | Single instance only           | Multiple replicas supported   |
| Backups            | sqlite3 `.backup` / Litestream | pg_dump / managed backups     |
| Best for           | Dev, single-node, small teams  | Production, HA, scaling       |

### Deploying with PostgreSQL

Set `SORTIE_DB_TYPE=postgres` and provide connection parameters. With the
Helm chart:

```yaml
# values.yaml
database:
  type: postgres
  postgres:
    host: "db.example.com"
    port: 5432
    database: "sortie"
    user: "sortie"
    sslMode: "require"
    existingSecret: sortie-pg-password  # K8s Secret with "password" key
```

Create the password Secret:

```bash
kubectl create secret generic sortie-pg-password \
  --namespace sortie \
  --from-literal=password='your-secure-password'
```

Deploy:

```bash
helm upgrade --install sortie charts/sortie \
  --namespace sortie --create-namespace \
  -f values.yaml
```

When using PostgreSQL, the PVC for SQLite storage is automatically
skipped, and `replicaCount` can be increased for horizontal scaling.

### SQLite with Persistence

For single-instance SQLite deployments, enable persistence to survive
pod restarts:

```yaml
database:
  type: sqlite
  sqlite:
    path: "/data/sortie.db"
persistence:
  enabled: true
  size: 1Gi
  storageClassName: standard
```

### Load Balancing

The Service and Ingress configuration handles load balancing.
For external load balancers:

```yaml
# service-lb.yaml
apiVersion: v1
kind: Service
metadata:
  name: sortie-lb
  namespace: sortie
spec:
  type: LoadBalancer
  selector:
    app: sortie
  ports:
    - port: 443
      targetPort: 8080
```

### Health Checks

The deployment includes liveness and readiness probes on `/api/apps`.
Kubernetes will:

- Restart pods that fail liveness checks
- Remove pods from service endpoints that fail readiness checks

### Pod Disruption Budget

For maintenance operations:

```yaml
# pdb.yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: sortie-pdb
  namespace: sortie
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: sortie
```

### Horizontal Pod Autoscaler

For automatic scaling (requires PostgreSQL — see above):

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: sortie-hpa
  namespace: sortie
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: sortie
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

## Monitoring

### Prometheus Metrics

Add metrics endpoint to the application for monitoring:

```yaml
# servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: sortie
  namespace: sortie
spec:
  selector:
    matchLabels:
      app: sortie
  endpoints:
    - port: http
      path: /metrics
```

### Logging

Configure centralized logging:

```yaml
# fluent-bit sidecar or cluster-level logging
```

## Backup and Recovery

### SQLite Backup

```bash
kubectl exec -n sortie deployment/sortie -- \
  sqlite3 /data/sortie.db ".backup '/data/backup.db'"

kubectl cp sortie/sortie-pod:/data/backup.db ./backup.db
```

### PostgreSQL Backup

```bash
pg_dump -h db.example.com -U sortie -d sortie -Fc > sortie-backup.dump
```

For managed PostgreSQL (RDS, Cloud SQL), use the provider's automated
backup and point-in-time recovery features.

### Disaster Recovery

1. Regular PVC snapshots (SQLite) or managed DB backups (PostgreSQL)
2. Off-cluster backup of database
3. GitOps for manifest recovery

## Security Considerations

1. **Network Policies**: Restrict pod-to-pod communication
2. **RBAC**: Limit service account permissions
3. **Secrets Management**: Use external secrets for sensitive config
4. **Image Scanning**: Scan container images for vulnerabilities
5. **Pod Security Standards**: Apply restricted security context

```yaml
# security-context in deployment
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  readOnlyRootFilesystem: true
```
