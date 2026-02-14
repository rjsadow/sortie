# Sessions

Sessions represent running instances of container applications. When a user
launches a container or web proxy app, Sortie creates a Kubernetes pod and
streams the desktop to the browser.

## Launching a Session

1. Find the application on the dashboard
2. Click the app card to launch it
3. Sortie creates a pod and connects automatically
4. The desktop streams directly in your browser via noVNC (Linux) or
   Guacamole (Windows)

## Session Lifecycle

```text
creating ──▶ running ──▶ terminated
                │
                └──▶ timeout (auto-cleanup)
```

- **Creating**: Pod is being provisioned in Kubernetes
- **Running**: Pod is ready and the desktop is streaming
- **Terminated**: User closed the session or it timed out

## Managing Sessions

Click the **Sessions** button in the header to view all your active sessions.
From the session manager you can:

- See which sessions are currently running
- Reconnect to a running session
- [Share a session](/guide/session-sharing) with other users
- Terminate sessions you no longer need

## Session Timeout

Sessions expire after a configurable timeout (default: 2 hours). The timeout
is controlled by the `SORTIE_SESSION_TIMEOUT` environment variable.

## Resource Limits

Each session pod runs with resource limits configured per application:

| Setting | Description | Example |
|---------|-------------|---------|
| `cpu_request` | Minimum CPU guaranteed | `250m` |
| `cpu_limit` | Maximum CPU allowed | `1` |
| `memory_request` | Minimum memory guaranteed | `256Mi` |
| `memory_limit` | Maximum memory allowed | `1Gi` |

Administrators can adjust these per application or set global defaults via
environment variables.
