# Session Recording

Session recording captures container sessions as `.webm` video files.
Recordings are captured client-side in the browser and uploaded to a
configurable storage backend (local filesystem or S3-compatible storage).

## Enabling Recording

Set the `SORTIE_VIDEO_RECORDING_ENABLED` environment variable to enable the
recording feature:

```bash
SORTIE_VIDEO_RECORDING_ENABLED=true
```

When disabled (the default), recording API routes are not registered and the
recording UI controls are hidden.

## Storage Backends

Sortie supports two storage backends for recording files.

### Local Storage

The default backend stores recordings on the local filesystem:

```bash
SORTIE_RECORDING_STORAGE_BACKEND=local
SORTIE_RECORDING_STORAGE_PATH=/data/recordings
```

When using local storage with Kubernetes, mount a PersistentVolumeClaim at
the storage path so recordings survive pod restarts.

### S3 Storage

For production deployments, use S3-compatible storage (AWS S3, MinIO, etc.):

```bash
SORTIE_RECORDING_STORAGE_BACKEND=s3
SORTIE_RECORDING_S3_BUCKET=my-recordings-bucket
SORTIE_RECORDING_S3_REGION=us-east-1
SORTIE_RECORDING_S3_PREFIX=recordings/
```

For self-hosted S3-compatible storage like MinIO, set a custom endpoint:

```bash
SORTIE_RECORDING_S3_ENDPOINT=https://minio.example.com
```

AWS credentials are read from the standard AWS SDK credential chain
(environment variables, shared credentials file, IAM role, etc.).

### Environment Variable Reference

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SORTIE_VIDEO_RECORDING_ENABLED` | bool | `false` | Enable session recording |
| `SORTIE_RECORDING_STORAGE_BACKEND` | string | `local` | Storage backend: `local` or `s3` |
| `SORTIE_RECORDING_STORAGE_PATH` | string | `/data/recordings` | Local storage directory |
| `SORTIE_RECORDING_MAX_SIZE_MB` | int | `500` | Maximum upload size in MB |
| `SORTIE_RECORDING_RETENTION_DAYS` | int | `0` | Days to retain recordings (0 = forever) |
| `SORTIE_RECORDING_S3_BUCKET` | string | | S3 bucket name (required for S3 backend) |
| `SORTIE_RECORDING_S3_REGION` | string | `us-east-1` | AWS region |
| `SORTIE_RECORDING_S3_ENDPOINT` | string | | Custom S3 endpoint for MinIO |
| `SORTIE_RECORDING_S3_PREFIX` | string | `recordings/` | Key prefix within the bucket |

## Auto-Record

Administrators can enable automatic recording for all sessions from the
**Settings** tab in the admin panel. When auto-record is active, recording
starts as soon as a container session enters the running state. Users can
still stop recording manually.

This setting is stored in the database and can be toggled at any time
without restarting the server.

## Retention Policy

Configure automatic cleanup of old recordings with
`SORTIE_RECORDING_RETENTION_DAYS`:

```bash
SORTIE_RECORDING_RETENTION_DAYS=30
```

When set to a value greater than zero, a cleanup job runs hourly and deletes
recordings in the `ready` state that are older than the configured number of
days. Recordings in other states (recording, uploading, failed) are not
affected by retention cleanup.

Set to `0` (the default) to keep recordings indefinitely.

## Upload Limits

The maximum recording upload size is controlled by
`SORTIE_RECORDING_MAX_SIZE_MB` (default: 500 MB). Uploads exceeding this
limit are rejected with a `413` status code. Adjust this based on expected
session duration and video quality.

## Helm Configuration

The Helm chart exposes recording settings under the `recording` key in
`values.yaml`:

```yaml
recording:
  enabled: false
  storageBackend: "local"    # "local" or "s3"
  storagePath: "/data/recordings"
  maxSizeMB: 500
  retentionDays: 0           # 0 = keep forever
  s3:
    bucket: ""
    region: "us-east-1"
    endpoint: ""             # Custom S3 endpoint (MinIO)
    prefix: "recordings/"
```

Set `recording.enabled: true` and configure the storage backend for your
environment.

## Managing Recordings

Administrators can view and manage all recordings across all users from the
**Recordings** tab in the admin panel. The admin view shows:

- Recording ID and associated session
- User who created the recording
- Recording duration and file size
- Current status

Administrators can download or delete any recording regardless of ownership.
Regular users can only manage their own recordings.
