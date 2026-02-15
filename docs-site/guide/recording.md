# Session Recording

Sortie can record your container sessions as video files. Recordings are
captured client-side in the browser as `.webm` files and uploaded to the
server when you stop recording.

## Recording a Session

Once your container session is running, you can start recording from the
session viewer:

1. Click the **Record** button in the session toolbar
2. The button turns red to indicate recording is active
3. Click the button again to stop recording
4. The recording is automatically uploaded to the server

If your administrator has enabled **auto-record**, recording starts
automatically when the session launches. You can still stop it manually.

## Managing Recordings

Click **Recordings** in the header to view your recordings. From the
recordings list you can:

- See all your past recordings with session details and duration
- Download a recording as a `.webm` file
- Delete recordings you no longer need

## Recording Status

Each recording progresses through the following states:

| Status | Meaning |
|--------|---------|
| `recording` | Capture is in progress |
| `uploading` | Recording is being uploaded to the server |
| `ready` | Upload complete, available for download |
| `failed` | Upload or processing failed |

Recordings in the `ready` state can be downloaded or deleted. Failed
recordings can be deleted but not downloaded.

## Storage and Retention

Your administrator controls where recordings are stored and how long they
are kept. Recordings may be automatically deleted after a retention period.
See the [admin recording guide](/admin/recording) for configuration details.
