# Kubernetes Pod Orchestration

Launchpad supports launching containerized thick-client applications in
Kubernetes pods with VNC streaming to users via WebSocket.

## Architecture

```text
Browser → Launchpad API → Kubernetes → Pod (App + VNC Sidecar) → WebSocket
```

When a user launches a container application:

1. Launchpad creates a session and spawns a Kubernetes pod
2. The pod contains two containers:
   - **App container**: Runs the actual application
   - **VNC sidecar**: Provides Xvfb, x11vnc, and websockify
3. The VNC sidecar streams the application's display via WebSocket
4. Launchpad proxies the WebSocket connection to the user's browser
5. noVNC in the browser renders the VNC stream

## Prerequisites

- Kubernetes cluster (1.24+)
- kubectl configured with cluster access
- Container images for your applications

## Deployment

### 1. Create the namespace and RBAC

```bash
kubectl apply -f deploy/kubernetes/rbac.yaml
```

### 2. Apply resource quotas (optional but recommended)

```bash
kubectl apply -f deploy/kubernetes/resource-quota.yaml
```

### 3. Apply network policies (optional but recommended)

```bash
kubectl apply -f deploy/kubernetes/network-policy.yaml
```

### 4. Deploy Launchpad

```bash
kubectl apply -f deploy/kubernetes/deployment.yaml
```

### 5. Build and push the VNC sidecar image

```bash
docker build -t ghcr.io/rjsadow/launchpad-vnc-sidecar:latest docker/vnc-sidecar/
docker push ghcr.io/rjsadow/launchpad-vnc-sidecar:latest
```

## Configuration

### Environment Variables

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `LAUNCHPAD_NAMESPACE` | `default` | Kubernetes namespace for session pods |
| `SESSION_TIMEOUT` | `120` | Session timeout in minutes |
| `SESSION_CLEANUP_INTERVAL` | `5` | Cleanup interval in minutes |
| `POD_READY_TIMEOUT` | `120` | Pod ready timeout in seconds |
| `LAUNCHPAD_VNC_SIDECAR_IMAGE` | (see below) | VNC sidecar container image |
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig (out-of-cluster) |

Default VNC sidecar image: `ghcr.io/rjsadow/launchpad-vnc-sidecar:latest`

### Adding Container Applications

Add applications with `launch_type: "container"` to your apps.json:

```json
{
  "applications": [
    {
      "id": "firefox",
      "name": "Firefox Browser",
      "description": "Firefox web browser in a container",
      "url": "",
      "icon": "https://example.com/firefox.png",
      "category": "Browsers",
      "launch_type": "container",
      "container_image": "ghcr.io/yourorg/firefox-desktop:latest"
    },
    {
      "id": "vscode",
      "name": "VS Code",
      "description": "Visual Studio Code IDE",
      "url": "",
      "icon": "https://example.com/vscode.png",
      "category": "Development",
      "launch_type": "container",
      "container_image": "codercom/code-server:latest"
    }
  ]
}
```

### Per-Application Resource Limits

You can specify custom CPU and memory resource limits for container applications.
If not specified, default values are used (CPU: 500m request / 2 limit,
Memory: 512Mi request / 2Gi limit).

```json
{
  "id": "libreoffice",
  "name": "LibreOffice",
  "description": "Full office suite in a container",
  "url": "",
  "icon": "https://example.com/libreoffice.png",
  "category": "Productivity",
  "launch_type": "container",
  "container_image": "jlesage/libreoffice:latest",
  "resource_limits": {
    "cpu_request": "500m",
    "cpu_limit": "2",
    "memory_request": "1Gi",
    "memory_limit": "4Gi"
  }
}
```

Resource limits use Kubernetes resource quantity notation:

- **CPU**: millicores (e.g., "100m" = 0.1 CPU, "2" = 2 CPUs)
- **Memory**: bytes with suffix (e.g., "256Mi", "1Gi", "2Gi")

| Field | Description | Default |
| ----- | ----------- | ------- |
| `cpu_request` | Minimum CPU guaranteed | `500m` |
| `cpu_limit` | Maximum CPU allowed | `2` |
| `memory_request` | Minimum memory guaranteed | `512Mi` |
| `memory_limit` | Maximum memory allowed | `2Gi` |

**Note**: Resource limits are enforced by Kubernetes. Pods exceeding memory
limits will be OOM-killed. CPU limits are throttled but not killed.

### Building Application Images

Application container images must:

1. Run a GUI application that uses the `DISPLAY` environment variable
2. Be compatible with the non-root user (UID 1000)

Example Dockerfile for a GUI app:

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    firefox \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1000 appuser
USER appuser

ENV DISPLAY=:99
CMD ["firefox"]
```

## API Reference

### Sessions API

#### Create Session

```http
POST /api/sessions
Content-Type: application/json

{
  "app_id": "firefox",
  "user_id": "optional-user-id"
}
```

Response:

```json
{
  "id": "session-uuid",
  "user_id": "user-id",
  "app_id": "firefox",
  "app_name": "Firefox Browser",
  "pod_name": "launchpad-session-uuid",
  "status": "creating",
  "websocket_url": "/ws/sessions/session-uuid",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

#### Get Session

```http
GET /api/sessions/{id}
```

#### List Sessions

```http
GET /api/sessions
GET /api/sessions?user_id=user-123
```

#### Terminate Session

```http
DELETE /api/sessions/{id}
```

### WebSocket Connection

Connect to the VNC stream:

```javascript
const ws = new WebSocket(
  'wss://launchpad.example.com/ws/sessions/{id}'
);
```

The WebSocket connection proxies the noVNC/websockify protocol.

## Security Considerations

1. **RBAC**: The Launchpad service account has minimal permissions
   (pod CRUD only)
2. **Network Policies**: Session pods are isolated and can only communicate
   with the Launchpad server
3. **Resource Quotas**: Limit the number of pods and resources in
   the namespace
4. **Pod Security**: Pods run as non-root with dropped capabilities
5. **Session Timeout**: Stale sessions are automatically cleaned up

## Troubleshooting

### Pod not starting

Check pod events:

```bash
kubectl describe pod -n launchpad launchpad-session-xxx
```

### VNC connection fails

1. Verify the pod is running: `kubectl get pods -n launchpad`
2. Check VNC sidecar logs:
   `kubectl logs -n launchpad launchpad-session-xxx -c vnc-sidecar`
3. Check network policies allow traffic

### Session stuck in "creating"

1. Increase `POD_READY_TIMEOUT` if images are large
2. Check image pull status: `kubectl describe pod`
3. Verify the VNC sidecar image is accessible

## Local Development

For local development with kind or minikube:

```bash
# Create a kind cluster
kind create cluster --name launchpad

# Load images into kind
kind load docker-image ghcr.io/rjsadow/launchpad:latest --name launchpad
kind load docker-image ghcr.io/rjsadow/launchpad-vnc-sidecar:latest \
  --name launchpad

# Apply manifests
kubectl apply -f deploy/kubernetes/

# Port forward to access locally
kubectl port-forward -n launchpad svc/launchpad 8080:80
```
