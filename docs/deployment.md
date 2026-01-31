# High Availability Deployment Guide

This guide covers deploying Launchpad in a highly available
configuration using Kubernetes.

## Prerequisites

- Kubernetes cluster (1.21+)
- kubectl configured
- Container registry access
- Persistent volume provisioner (for SQLite storage)

## Building the Container Image

```dockerfile
# Dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN npm install --prefix web && npm run build --prefix web
RUN CGO_ENABLED=0 go build -o launchpad .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/launchpad .
EXPOSE 8080
CMD ["./launchpad", "-db", "/data/launchpad.db"]
```

Build and push:

```bash
docker build -t your-registry/launchpad:latest .
docker push your-registry/launchpad:latest
```

## Kubernetes Manifests

### Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: launchpad
```

### ConfigMap for Branding

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: launchpad-config
  namespace: launchpad
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
  name: launchpad-data
  namespace: launchpad
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
  name: launchpad
  namespace: launchpad
spec:
  replicas: 1  # Single replica for SQLite; see HA notes below
  selector:
    matchLabels:
      app: launchpad
  template:
    metadata:
      labels:
        app: launchpad
    spec:
      containers:
        - name: launchpad
          image: your-registry/launchpad:latest
          ports:
            - containerPort: 8080
          env:
            - name: LAUNCHPAD_CONFIG
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
            claimName: launchpad-data
        - name: config
          configMap:
            name: launchpad-config
```

### Service

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: launchpad
  namespace: launchpad
spec:
  selector:
    app: launchpad
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
  name: launchpad
  namespace: launchpad
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
    - hosts:
        - launchpad.example.com
      secretName: launchpad-tls
  rules:
    - host: launchpad.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: launchpad
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

### SQLite Limitations

SQLite is suitable for single-instance deployments. For true HA with
multiple replicas, consider:

1. **PostgreSQL Migration**: Replace SQLite with PostgreSQL for multi-replica support
2. **Read Replicas**: Use SQLite with read replicas (Litestream) for read scaling
3. **Shared Storage**: Use a shared filesystem (NFS, EFS) with file locking

### Load Balancing

The Service and Ingress configuration handles load balancing.
For external load balancers:

```yaml
# service-lb.yaml
apiVersion: v1
kind: Service
metadata:
  name: launchpad-lb
  namespace: launchpad
spec:
  type: LoadBalancer
  selector:
    app: launchpad
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
  name: launchpad-pdb
  namespace: launchpad
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: launchpad
```

### Horizontal Pod Autoscaler

For automatic scaling (requires PostgreSQL for multi-replica):

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: launchpad-hpa
  namespace: launchpad
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: launchpad
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
  name: launchpad
  namespace: launchpad
spec:
  selector:
    matchLabels:
      app: launchpad
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

### Database Backup

```bash
# Backup SQLite database
kubectl exec -n launchpad deployment/launchpad -- \
  sqlite3 /data/launchpad.db ".backup '/data/backup.db'"

# Copy backup locally
kubectl cp launchpad/launchpad-pod:/data/backup.db ./backup.db
```

### Disaster Recovery

1. Regular PVC snapshots (if supported by storage class)
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
