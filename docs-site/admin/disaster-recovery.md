# Disaster Recovery

This document defines disaster recovery procedures, objectives, and runbooks
for Sortie deployments.

## Recovery Objectives

### RPO (Recovery Point Objective)

| Data Type     | RPO Target | Rationale                          |
| ------------- | ---------- | ---------------------------------- |
| Applications  | 24 hours   | Daily backups; config changes rare |
| Audit Logs    | 24 hours   | Daily backups sufficient           |
| Analytics     | 24 hours   | Historical data, not critical      |
| Sessions      | 0 (none)   | Ephemeral; recreated on demand     |
| User Settings | N/A        | Client-side localStorage           |

### RTO (Recovery Time Objective)

| Scenario             | RTO Target | Recovery Method                   |
| -------------------- | ---------- | --------------------------------- |
| Pod restart          | 2 minutes  | Automatic via Kubernetes          |
| Node failure         | 5 minutes  | Pod rescheduling                  |
| PVC corruption       | 30 minutes | Restore from backup               |
| Namespace deletion   | 1 hour     | Full restore from backup + GitOps |
| Complete cluster loss| 4 hours    | Re-deploy cluster + restore       |

## Backup Strategy

### What Gets Backed Up

| Component        | Backup Method          | Location                |
| ---------------- | ---------------------- | ----------------------- |
| SQLite database  | Daily CronJob snapshot | Backup PVC + off-cluster|
| K8s manifests    | GitOps repository      | Git remote              |
| ConfigMaps       | GitOps repository      | Git remote              |
| Container images | Registry               | Container registry      |

### What Does NOT Get Backed Up

- **Sessions**: Ephemeral container instances; users restart as needed
- **User settings**: Stored in browser localStorage; not server-managed
- **Workspace volumes**: Session-specific emptyDir; non-persistent by design

## Backup Procedures

### Automated Daily Backup

Deploy the backup CronJob to run daily at 2 AM:

```yaml
# deploy/kubernetes/backup-cronjob.yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: sortie-backup
  namespace: sortie
spec:
  schedule: "0 2 * * *"
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
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
                  set -e
                  apk add --no-cache sqlite
                  TIMESTAMP=$(date +%Y%m%d-%H%M%S)
                  BACKUP_FILE="/backup/sortie-${TIMESTAMP}.db"
                  sqlite3 /data/sortie.db ".backup '${BACKUP_FILE}'"
                  # Keep only last 7 daily backups
                  ls -t /backup/*.db | tail -n +8 | xargs -r rm
                  echo "Backup completed: ${BACKUP_FILE}"
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

### Manual Backup

Create an on-demand backup before maintenance or upgrades:

```bash
# 1. Get the pod name
POD=$(kubectl get pod -n sortie -l app=sortie \
  -o jsonpath='{.items[0].metadata.name}')

# 2. Create backup inside the pod
kubectl exec -n sortie $POD -- \
  sqlite3 /data/sortie.db ".backup '/data/manual-backup.db'"

# 3. Copy backup to local machine
kubectl cp sortie/$POD:/data/manual-backup.db \
  ./sortie-backup-$(date +%Y%m%d-%H%M%S).db

# 4. Verify backup integrity
sqlite3 ./sortie-backup-*.db "PRAGMA integrity_check;"
```

### Off-Cluster Backup

For disaster recovery, sync backups to external storage:

```bash
# Example: AWS S3
BACKUP_FILE="sortie-$(date +%Y%m%d).db"
POD=$(kubectl get pod -n sortie -l app=sortie \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -n sortie $POD -- \
  sqlite3 /data/sortie.db ".backup '/tmp/backup.db'"

kubectl cp sortie/$POD:/tmp/backup.db ./$BACKUP_FILE
aws s3 cp ./$BACKUP_FILE s3://your-backup-bucket/sortie/

# Example: GCS
gsutil cp ./$BACKUP_FILE gs://your-backup-bucket/sortie/

# Cleanup local file
rm ./$BACKUP_FILE
```

## Restore Procedures

### Restore from In-Cluster Backup

Use this procedure when the database is corrupted but the backup PVC is intact.

**Pre-requisites:**

- Backup PVC contains valid backup files
- kubectl access to the cluster

**Procedure:**

```bash
# 1. List available backups (requires running Sortie pod)
kubectl exec -n sortie deployment/sortie -- ls -la /backup/

# 2. Scale down Sortie to prevent writes
kubectl scale deployment sortie -n sortie --replicas=0

# 3. Wait for pod termination
kubectl wait --for=delete pod -l app=sortie -n sortie --timeout=60s

# 4. Run restore job
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: restore-job
  namespace: sortie
spec:
  containers:
    - name: restore
      image: alpine:3.19
      command:
        - /bin/sh
        - -c
        - |
          set -e
          apk add --no-cache sqlite
          # Use most recent backup (adjust filename as needed)
          BACKUP=\$(ls -t /backup/*.db | head -1)
          echo "Restoring from: \$BACKUP"
          cp "\$BACKUP" /data/sortie.db
          sqlite3 /data/sortie.db "PRAGMA integrity_check;"
          echo "Restore completed successfully"
      volumeMounts:
        - name: data
          mountPath: /data
        - name: backup
          mountPath: /backup
          readOnly: true
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: sortie-data
    - name: backup
      persistentVolumeClaim:
        claimName: sortie-backup
  restartPolicy: Never
EOF

# 5. Wait for restore to complete
kubectl wait --for=condition=Succeeded pod/restore-job -n sortie --timeout=300s
kubectl logs restore-job -n sortie

# 6. Cleanup restore pod
kubectl delete pod restore-job -n sortie

# 7. Scale up Sortie
kubectl scale deployment sortie -n sortie --replicas=1

# 8. Verify service is healthy
kubectl wait --for=condition=Ready pod -l app=sortie -n sortie --timeout=120s
kubectl exec -n sortie deployment/sortie -- \
  wget -qO- http://localhost:8080/api/apps | head -c 100
```

### Restore from Off-Cluster Backup

Use this procedure when both the database and backup PVC are lost.

**Pre-requisites:**

- Off-cluster backup file accessible (S3, GCS, local file)
- kubectl access to the cluster
- Namespace and PVCs exist

**Procedure:**

```bash
# 1. Download backup from off-cluster storage
aws s3 cp s3://your-backup-bucket/sortie/sortie-20260130.db ./restore.db
# Or: gsutil cp gs://your-backup-bucket/sortie/sortie-20260130.db ./restore.db

# 2. Verify backup integrity locally
sqlite3 ./restore.db "PRAGMA integrity_check;"
sqlite3 ./restore.db "SELECT COUNT(*) FROM applications;"

# 3. Scale down Sortie
kubectl scale deployment sortie -n sortie --replicas=0
kubectl wait --for=delete pod -l app=sortie -n sortie --timeout=60s

# 4. Create temporary pod for file transfer
kubectl run restore-pod --image=alpine:3.19 -n sortie \
  --overrides='{"spec":{"containers":[{"name":"restore","image":"alpine:3.19","command":["sleep","3600"],"volumeMounts":[{"name":"data","mountPath":"/data"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"sortie-data"}}]}}'

kubectl wait --for=condition=Ready pod/restore-pod -n sortie --timeout=60s

# 5. Copy backup to pod
kubectl cp ./restore.db sortie/restore-pod:/data/sortie.db

# 6. Verify copy
kubectl exec -n sortie restore-pod -- ls -la /data/

# 7. Cleanup and restart
kubectl delete pod restore-pod -n sortie
kubectl scale deployment sortie -n sortie --replicas=1

# 8. Verify service
kubectl wait --for=condition=Ready pod -l app=sortie -n sortie --timeout=120s
curl -s https://sortie.example.com/api/apps | head -c 100
```

### Full Namespace Recovery

Use this procedure when the entire namespace is deleted.

**Pre-requisites:**

- GitOps repository with Kubernetes manifests
- Off-cluster database backup
- Container images available in registry

**Procedure:**

```bash
# 1. Recreate namespace and core resources from GitOps
kubectl apply -f deploy/kubernetes/namespace.yaml
kubectl apply -f deploy/kubernetes/rbac.yaml
kubectl apply -f deploy/kubernetes/configmap.yaml
kubectl apply -f deploy/kubernetes/pvc.yaml
kubectl apply -f deploy/kubernetes/network-policy.yaml
kubectl apply -f deploy/kubernetes/resource-quota.yaml

# 2. Wait for PVC to be bound
kubectl wait --for=jsonpath='{.status.phase}'=Bound \
  pvc/sortie-data -n sortie --timeout=120s

# 3. Restore database (follow off-cluster restore procedure)
# ... (steps 4-6 from "Restore from Off-Cluster Backup")

# 4. Deploy Sortie
kubectl apply -f deploy/kubernetes/deployment.yaml
kubectl apply -f deploy/kubernetes/service.yaml
kubectl apply -f deploy/kubernetes/ingress.yaml

# 5. Verify deployment
kubectl wait --for=condition=Ready pod -l app=sortie -n sortie --timeout=120s
kubectl get all -n sortie
```

## Disaster Scenarios

### Scenario 1: Pod Crash

**Symptoms:** Application unavailable, pod in CrashLoopBackOff

**Recovery:**

1. Kubernetes automatically restarts the pod
2. If persistent, check logs: `kubectl logs -n sortie deployment/sortie --previous`
3. Check resource limits and node capacity
4. If database corruption suspected, follow restore procedure

**Expected RTO:** 2 minutes (automatic)

### Scenario 2: Node Failure

**Symptoms:** Pod evicted, application unavailable

**Recovery:**

1. Kubernetes reschedules pod to healthy node
2. PVC reattaches to new pod automatically (for RWO volumes)
3. No manual intervention required

**Expected RTO:** 5 minutes (automatic)

### Scenario 3: PVC Data Corruption

**Symptoms:** Application errors, database integrity failures

**Recovery:**

1. Confirm corruption:

   ```bash
   kubectl exec -n sortie deployment/sortie -- \
     sqlite3 /data/sortie.db "PRAGMA integrity_check;"
   ```

2. Follow "Restore from In-Cluster Backup" procedure
3. If no backup PVC, follow "Restore from Off-Cluster Backup"

**Expected RTO:** 30 minutes (manual)

### Scenario 4: Accidental Namespace Deletion

**Symptoms:** All Sortie resources gone

**Recovery:**

1. Follow "Full Namespace Recovery" procedure
2. Restore database from off-cluster backup

**Expected RTO:** 1 hour (manual)

### Scenario 5: Complete Cluster Loss

**Symptoms:** Entire Kubernetes cluster unavailable

**Recovery:**

1. Provision new Kubernetes cluster
2. Configure kubectl access
3. Follow "Full Namespace Recovery" procedure
4. Update DNS to point to new cluster ingress

**Expected RTO:** 4 hours (manual)

## Testing Restore Procedures

Regular testing validates that backups are usable and procedures are current.

### Monthly Restore Test

Run this test monthly in a non-production namespace:

```bash
# 1. Create test namespace
kubectl create namespace sortie-restore-test

# 2. Copy PVCs and manifests (adjust for your setup)
kubectl apply -f deploy/kubernetes/pvc.yaml -n sortie-restore-test

# 3. Get latest backup
BACKUP=$(kubectl exec -n sortie deployment/sortie -- \
  ls -t /backup/*.db 2>/dev/null | head -1)
kubectl exec -n sortie deployment/sortie -- \
  cat "$BACKUP" > /tmp/test-restore.db

# 4. Verify backup
sqlite3 /tmp/test-restore.db "PRAGMA integrity_check;"
sqlite3 /tmp/test-restore.db "SELECT COUNT(*) FROM applications;"

# 5. Deploy to test namespace with restored data
# ... (follow restore procedure targeting sortie-restore-test)

# 6. Validate functionality
curl -s http://sortie-restore-test.internal/api/apps

# 7. Cleanup
kubectl delete namespace sortie-restore-test
rm /tmp/test-restore.db
```

### Test Checklist

After each restore test, verify:

- [ ] Database integrity check passes
- [ ] Application count matches expected
- [ ] API endpoints respond correctly
- [ ] Web UI loads and displays applications
- [ ] Session creation works (if container orchestration enabled)

### Document Test Results

Record test results for audit purposes:

| Date       | Backup Used        | Restore Time | Issues Found    | Tester |
| ---------- | ------------------ | ------------ | --------------- | ------ |
| 2026-01-31 | sortie-20260130 | 12 min       | None            | -      |
| ...        | ...                | ...          | ...             | ...    |

## Runbook Quick Reference

### Emergency Contacts

| Role              | Contact Method      |
| ----------------- | ------------------- |
| Platform Team     | #platform-oncall    |
| Database Admin    | #dba-support        |
| Security Team     | #security-incidents |

### Quick Commands

```bash
# Check backup status
kubectl get cronjob sortie-backup -n sortie
kubectl get jobs -n sortie | grep backup

# List available backups
kubectl exec -n sortie deployment/sortie -- ls -la /backup/

# Check database health
kubectl exec -n sortie deployment/sortie -- \
  sqlite3 /data/sortie.db "PRAGMA integrity_check;"

# Check application health
kubectl exec -n sortie deployment/sortie -- \
  wget -qO- http://localhost:8080/api/apps | jq '. | length'

# View recent logs
kubectl logs -n sortie deployment/sortie --tail=100
```

## Related Documentation

- [Data Persistence Strategy](./data-persistence) - Storage architecture details
- [Kubernetes Deployment](./kubernetes) - Pod orchestration setup
- [High Availability Guide](./deployment) - HA configuration options
