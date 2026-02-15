# Session Video Recording Feature Plan

## Context

Users need the ability to record their Sortie sessions (or portions of them) as downloadable video files. This enables training content creation, documentation, session review, and compliance auditing. The feature should work for both VNC (Linux) and RDP/Guacamole (Windows) container sessions, with both user-initiated and admin-policy-driven recording.

## Approach: Client-Side MediaRecorder

Both VNC (noVNC) and Guacamole render to HTML5 `<canvas>` elements in the browser. The browser-native `canvas.captureStream()` + `MediaRecorder` API captures exactly what the user sees with zero backend CPU overhead. The recorded WebM video blob is uploaded to the server after the user stops recording.

**Why not server-side?** Decoding VNC RFB or Guacamole protocol into video frames server-side would require FFmpeg or similar, adding significant complexity and CPU load. Client-side is the right starting point; server-side can be added later for admin-mandated recording that persists even if the user closes the browser.

---

## Phase 1: VNC Recording + Backend Infrastructure

### 1.1 Database: `recordings` table

**File:** `internal/db/db.go`

Add schema, `Recording` struct, status constants, and CRUD methods:

```sql
CREATE TABLE IF NOT EXISTS recordings (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    size_bytes INTEGER DEFAULT 0,
    duration_seconds REAL DEFAULT 0,
    format TEXT NOT NULL DEFAULT 'webm',
    storage_backend TEXT NOT NULL DEFAULT 'local',
    storage_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'recording',
    tenant_id TEXT DEFAULT 'default',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
```

Status values: `recording` | `uploading` | `ready` | `failed`

CRUD: `CreateRecording`, `GetRecording`, `UpdateRecordingComplete`, `ListRecordingsByUser`, `ListRecordingsBySession`, `ListAllRecordings` (admin), `DeleteRecording`

### 1.2 Storage abstraction

**New files:**
- `internal/recordings/storage.go` - `RecordingStore` interface (`Save`, `Get`, `Delete`)
- `internal/recordings/storage_local.go` - Local filesystem impl, files at `{baseDir}/{year}/{month}/{id}.webm`

### 1.3 Recording HTTP handler

**New file:** `internal/recordings/handler.go`

Follow the pattern from `internal/files/handler.go` for session validation and auth.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/sessions/{id}/recording/start` | Create recording record (status=recording), returns recording ID |
| POST | `/api/sessions/{id}/recording/stop` | Mark recording as uploading |
| POST | `/api/sessions/{id}/recording/upload` | Multipart upload of WebM blob, sets status=ready |
| GET | `/api/recordings` | List current user's recordings |
| GET | `/api/recordings/{id}/download` | Stream recording file |
| DELETE | `/api/recordings/{id}` | Delete recording + stored file |
| GET | `/api/admin/recordings` | Admin: list all recordings |

### 1.4 Configuration

**File:** `internal/config/config.go`

New fields:
- `VideoRecordingEnabled` (bool, env: `SORTIE_VIDEO_RECORDING_ENABLED`)
- `RecordingStorageBackend` (string: "local"|"s3", env: `SORTIE_RECORDING_STORAGE_BACKEND`)
- `RecordingStoragePath` (string, default: "/data/recordings", env: `SORTIE_RECORDING_STORAGE_PATH`)
- `RecordingMaxSizeMB` (int, default: 500, env: `SORTIE_RECORDING_MAX_SIZE_MB`)
- `RecordingRetentionDays` (int, default: 0 = keep forever, env: `SORTIE_RECORDING_RETENTION_DAYS`)

### 1.5 Wire up in main.go and server.go

**Files:** `main.go`, `internal/server/server.go`

- Initialize `RecordingStore` based on config backend
- Create `recordings.Handler`, register routes with `withTenant` middleware
- Add `RecordingStore` and handler to `App` struct

### 1.6 Frontend: `useRecording` hook

**New file:** `web/src/hooks/useRecording.ts`

```typescript
interface UseRecordingReturn {
  isRecording: boolean;
  duration: number; // seconds elapsed
  startRecording: (canvas: HTMLCanvasElement, sessionId: string) => Promise<void>;
  stopRecording: () => Promise<void>;
  error: string | null;
}
```

Flow:
1. `startRecording` → `POST /api/sessions/{id}/recording/start` → get `recording_id`
2. `canvas.captureStream(10)` → `new MediaRecorder(stream, {mimeType: 'video/webm;codecs=vp9'})` (fallback: vp8)
3. Collect chunks via `ondataavailable` (1s intervals), track duration
4. `stopRecording` → `mediaRecorder.stop()` → assemble blob → `POST .../recording/upload` as multipart

### 1.7 Frontend: VNCViewer canvas callback

**File:** `web/src/components/VNCViewer.tsx`

Add prop `onCanvasReady?: (canvas: HTMLCanvasElement) => void`. Use the existing MutationObserver pattern (lines 200-228) that already finds the canvas for FPS monitoring — extend it to also call `onCanvasReady` when the canvas is discovered.

### 1.8 Frontend: Recording button in toolbar

**File:** `web/src/components/SessionViewer.tsx`

- Accept canvas via `onCanvasReady` from VNCViewer
- Add record button to floating toolbar (line 178) next to fullscreen/stats/clipboard buttons
- When recording: red pulsing dot + duration counter
- When not recording: circle icon (standard record symbol)

### 1.9 Frontend types

**File:** `web/src/types.ts`

Add `Recording` interface and `RecordingStatus` type.

---

## Phase 2: Guacamole/RDP Recording

### 2.1 Guacamole canvas access

**File:** `web/src/guacamole-common-js.d.ts`

Add type for `getDefaultLayer().getCanvas()` which returns the underlying `HTMLCanvasElement`.

### 2.2 GuacamoleViewer canvas callback

**File:** `web/src/components/GuacamoleViewer.tsx`

Add `onCanvasReady` prop (same pattern as VNCViewer). After client connects, extract canvas via:
```typescript
client.getDisplay().getDefaultLayer().getCanvas()
```

### 2.3 SessionViewer unification

**File:** `web/src/components/SessionViewer.tsx`

Wire `onCanvasReady` to both VNCViewer and GuacamoleViewer. The recording button appears for both session types with no difference in behavior.

---

## Phase 3: Admin Controls & Recordings UI

### 3.1 Auto-recording admin policy

- Admin setting `recording_auto_record` stored via existing settings table
- `SessionResponse` gets new field `recording_policy` ("optional" | "required")
- Frontend auto-starts recording when policy is "required" and canvas is available

### 3.2 Recordings list page

**New file:** `web/src/components/RecordingsList.tsx`

Table showing user's recordings: app name, date, duration, size, status, download/delete actions.

### 3.3 Admin recordings panel

**File:** `web/src/components/Admin.tsx`

New "Recordings" tab listing all recordings across users with management controls.

### 3.4 Navigation

**File:** `web/src/App.tsx`

Add "Recordings" link in navigation.

---

## Phase 4: S3 Storage & Retention Cleanup

### 4.1 S3 storage backend

**New file:** `internal/recordings/storage_s3.go`

Uses AWS SDK v2. Config fields: `RecordingS3Bucket`, `RecordingS3Region`, `RecordingS3Endpoint` (for MinIO), `RecordingS3Prefix`.

### 4.2 Cleanup goroutine

**New file:** `internal/recordings/cleanup.go`

Runs hourly, deletes recordings older than `RecordingRetentionDays`. Similar to session expiry cleanup.

### 4.3 Helm chart

**Files:** `charts/sortie/values.yaml`, `charts/sortie/templates/deployment.yaml`

Add `recording.*` values and corresponding env vars in the deployment template.

---

## Files Summary

**New files (8):**
| File | Phase |
|------|-------|
| `internal/recordings/storage.go` | 1 |
| `internal/recordings/storage_local.go` | 1 |
| `internal/recordings/handler.go` | 1 |
| `web/src/hooks/useRecording.ts` | 1 |
| `web/src/components/RecordingsList.tsx` | 3 |
| `internal/recordings/storage_s3.go` | 4 |
| `internal/recordings/cleanup.go` | 4 |

**Modified files (11):**
| File | Phase | Changes |
|------|-------|---------|
| `internal/db/db.go` | 1 | recordings table + CRUD |
| `internal/config/config.go` | 1 | recording config fields |
| `internal/server/server.go` | 1 | route registration |
| `main.go` | 1 | store initialization |
| `web/src/types.ts` | 1 | Recording types |
| `web/src/components/VNCViewer.tsx` | 1 | onCanvasReady callback |
| `web/src/components/SessionViewer.tsx` | 1 | recording button in toolbar |
| `web/src/guacamole-common-js.d.ts` | 2 | getCanvas() type |
| `web/src/components/GuacamoleViewer.tsx` | 2 | onCanvasReady callback |
| `web/src/components/Admin.tsx` | 3 | recordings admin tab |
| `web/src/App.tsx` | 3 | navigation entry |
| `charts/sortie/values.yaml` | 4 | recording config |

---

## Verification

1. **Unit tests**: Recording handler CRUD, storage local read/write/delete
2. **Manual test (VNC)**: Launch Linux container app → click record → interact → stop → verify upload → download and play WebM
3. **Manual test (RDP)**: Same flow with Windows container app
4. **Edge cases**: Close browser mid-recording (recording is lost, status stays "recording" — add a cleanup for stale recordings), very long recordings (monitor blob memory), unsupported browser (MediaRecorder not available — hide button)
5. **Build**: `go build ./...` and `npm --prefix web run build` pass without errors
