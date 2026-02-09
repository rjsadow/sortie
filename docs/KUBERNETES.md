# Kubernetes Pod Orchestration

Sortie supports launching containerized thick-client applications in
Kubernetes pods with VNC streaming to users via WebSocket.

## Architecture

```text
Browser → Sortie API → Kubernetes → Pod (App + VNC Sidecar) → WebSocket
```

When a user launches a container application:

1. Sortie creates a session and spawns a Kubernetes pod
2. The pod contains two containers:
   - **App container**: Runs the actual application
   - **VNC sidecar**: Provides Xvfb, x11vnc, and websockify
3. The VNC sidecar streams the application's display via WebSocket
4. Sortie proxies the WebSocket connection to the user's browser
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

### 4. Deploy Sortie

```bash
kubectl apply -f deploy/kubernetes/deployment.yaml
```

### 5. Build and push the VNC sidecar image

```bash
docker build -t ghcr.io/rjsadow/sortie-vnc-sidecar:latest docker/vnc-sidecar/
docker push ghcr.io/rjsadow/sortie-vnc-sidecar:latest
```

## Configuration

### Environment Variables

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `SORTIE_NAMESPACE` | `default` | Kubernetes namespace for session pods |
| `SESSION_TIMEOUT` | `120` | Session timeout in minutes |
| `SESSION_CLEANUP_INTERVAL` | `5` | Cleanup interval in minutes |
| `POD_READY_TIMEOUT` | `120` | Pod ready timeout in seconds |
| `SORTIE_VNC_SIDECAR_IMAGE` | (see below) | VNC sidecar container image |
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig (out-of-cluster) |

Default VNC sidecar image: `ghcr.io/rjsadow/sortie-vnc-sidecar:latest`

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
  "pod_name": "sortie-session-uuid",
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
  'wss://sortie.example.com/ws/sessions/{id}'
);
```

The WebSocket connection proxies the noVNC/websockify protocol.

## Security Considerations

1. **RBAC**: The Sortie service account has minimal permissions
   (pod CRUD only)
2. **Network Policies**: Session pods are isolated and can only communicate
   with the Sortie server
3. **Resource Quotas**: Limit the number of pods and resources in
   the namespace
4. **Pod Security**: Pods run as non-root with dropped capabilities
5. **Session Timeout**: Stale sessions are automatically cleaned up

## Troubleshooting

### Pod not starting

Check pod events:

```bash
kubectl describe pod -n sortie sortie-session-xxx
```

### VNC connection fails

1. Verify the pod is running: `kubectl get pods -n sortie`
2. Check VNC sidecar logs:
   `kubectl logs -n sortie sortie-session-xxx -c vnc-sidecar`
3. Check network policies allow traffic

### Session stuck in "creating"

1. Increase `POD_READY_TIMEOUT` if images are large
2. Check image pull status: `kubectl describe pod`
3. Verify the VNC sidecar image is accessible

## Helm Chart

A Helm chart is provided for easier deployment and customization.

### Quick Start with Helm

```bash
# Install with default values
helm install sortie charts/sortie --namespace sortie --create-namespace

# Install with custom values
helm install sortie charts/sortie \
  --namespace sortie \
  --create-namespace \
  --set ingress.enabled=true \
  --set ingress.host=sortie.mycompany.com

# Upgrade an existing installation
helm upgrade sortie charts/sortie --namespace sortie
```

### Helm Values

Key configuration options in `values.yaml`:

| Value | Default | Description |
| ----- | ------- | ----------- |
| `image.repository` | `ghcr.io/rjsadow/sortie` | Sortie image |
| `image.tag` | `latest` | Image tag |
| `replicaCount` | `1` | Number of replicas |
| `ingress.enabled` | `false` | Enable ingress |
| `ingress.host` | `sortie.example.com` | Ingress hostname |
| `networkPolicy.enabled` | `true` | Enable network policies |
| `resourceQuota.enabled` | `true` | Enable resource quotas |

## Local Development

### Quick Start with Kind

Use the provided setup script for one-command local development:

```bash
# Create Kind cluster and deploy Sortie
./scripts/kind-setup.sh

# Access Sortie
kubectl port-forward -n sortie svc/sortie 8080:80
# Open http://localhost:8080

# Teardown when done
./scripts/kind-setup.sh teardown
```

### Manual Kind Setup

For manual setup with kind or minikube:

```bash
# Create a kind cluster with port mappings
kind create cluster --name sortie --config deploy/kind/kind-config.yaml

# Build and load images into kind
docker build -t ghcr.io/rjsadow/sortie:latest .
kind load docker-image ghcr.io/rjsadow/sortie:latest --name sortie
kind load docker-image ghcr.io/rjsadow/sortie-vnc-sidecar:latest \
  --name sortie

# Deploy with Helm
helm install sortie charts/sortie \
  --namespace sortie \
  --create-namespace \
  --set image.pullPolicy=Never

# Or apply raw manifests
kubectl apply -f deploy/kubernetes/

# Port forward to access locally
kubectl port-forward -n sortie svc/sortie 8080:80
```
