#!/usr/bin/env bash
#
# Kind cluster setup script for Launchpad local development
#
# Usage:
#   ./scripts/kind-setup.sh          # Create cluster and deploy
#   ./scripts/kind-setup.sh windows  # Create cluster with Windows RDP test support
#   ./scripts/kind-setup.sh teardown # Delete cluster
#
set -euo pipefail

CLUSTER_NAME="launchpad"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

check_dependencies() {
    local missing=()
    for cmd in kind kubectl helm docker; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done
    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing dependencies: ${missing[*]}"
        exit 1
    fi
}

cluster_exists() {
    kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"
}

create_cluster() {
    if cluster_exists; then
        log_warn "Cluster '${CLUSTER_NAME}' already exists"
        return 0
    fi

    log_info "Creating Kind cluster '${CLUSTER_NAME}'..."
    kind create cluster --name "$CLUSTER_NAME" --config "${ROOT_DIR}/deploy/kind/kind-config.yaml"

    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
}

load_images() {
    log_info "Building and loading images into Kind..."

    # Build launchpad image if Dockerfile exists
    if [ -f "${ROOT_DIR}/Dockerfile" ]; then
        log_info "Building launchpad image..."
        docker build -t ghcr.io/rjsadow/launchpad:latest "${ROOT_DIR}"
        kind load docker-image ghcr.io/rjsadow/launchpad:latest --name "$CLUSTER_NAME"
    fi

    # Build VNC sidecar if it exists
    if [ -d "${ROOT_DIR}/docker/vnc-sidecar" ]; then
        log_info "Building VNC sidecar image..."
        docker build -t ghcr.io/rjsadow/launchpad-vnc-sidecar:latest "${ROOT_DIR}/docker/vnc-sidecar"
        kind load docker-image ghcr.io/rjsadow/launchpad-vnc-sidecar:latest --name "$CLUSTER_NAME"
    fi

    # Build VNC browser sidecar if it exists
    if [ -d "${ROOT_DIR}/docker/browser-sidecar" ]; then
        log_info "Building browser sidecar image..."
        docker build -t ghcr.io/rjsadow/launchpad-browser-sidecar:latest "${ROOT_DIR}/docker/browser-sidecar"
        kind load docker-image ghcr.io/rjsadow/launchpad-browser-sidecar:latest --name "$CLUSTER_NAME"
    fi

    # Build guacd sidecar if it exists (for Windows RDP support)
    if [ -d "${ROOT_DIR}/docker/guacd-sidecar" ]; then
        log_info "Building guacd sidecar image..."
        docker build -t ghcr.io/rjsadow/launchpad-guacd-sidecar:latest "${ROOT_DIR}/docker/guacd-sidecar"
        kind load docker-image ghcr.io/rjsadow/launchpad-guacd-sidecar:latest --name "$CLUSTER_NAME"
    fi
}

load_windows_images() {
    log_info "Building and loading Windows RDP test images into Kind..."

    # Build xrdp test image (Linux with xrdp for testing the RDP pipeline)
    if [ -d "${ROOT_DIR}/docker/xrdp-test" ]; then
        log_info "Building xrdp test image..."
        docker build -t ghcr.io/rjsadow/launchpad-xrdp-test:latest "${ROOT_DIR}/docker/xrdp-test"
        kind load docker-image ghcr.io/rjsadow/launchpad-xrdp-test:latest --name "$CLUSTER_NAME"
    fi

    # Also load the official guacd image (used as default sidecar if not overridden)
    log_info "Pulling and loading guacamole/guacd:1.5.5..."
    docker pull guacamole/guacd:1.5.5 2>/dev/null || true
    kind load docker-image guacamole/guacd:1.5.5 --name "$CLUSTER_NAME"
}

deploy_helm() {
    local extra_args=("$@")
    log_info "Deploying Launchpad via Helm..."

    # Install or upgrade the release
    helm upgrade --install launchpad "${ROOT_DIR}/charts/launchpad" \
        --namespace launchpad \
        --create-namespace \
        --set image.pullPolicy=Never \
        "${extra_args[@]}" \
        --wait --timeout 120s

    log_info "Deployment complete!"
}

show_access_info() {
    log_info "Launchpad is deployed!"
    echo ""
    echo "To access Launchpad:"
    echo "  kubectl port-forward -n launchpad svc/launchpad 8080:80"
    echo "  Then open http://localhost:8080"
    echo ""
    echo "To view logs:"
    echo "  kubectl logs -n launchpad -l app.kubernetes.io/name=launchpad -f"
    echo ""
    echo "To teardown:"
    echo "  ./scripts/kind-setup.sh teardown"
}

teardown() {
    if cluster_exists; then
        log_info "Deleting Kind cluster '${CLUSTER_NAME}'..."
        kind delete cluster --name "$CLUSTER_NAME"
        log_info "Cluster deleted."
    else
        log_warn "Cluster '${CLUSTER_NAME}' does not exist."
    fi
}

seed_windows_app() {
    log_info "Seeding Windows RDP test app via API..."

    # Wait for the service to be ready
    kubectl wait --for=condition=Available deployment/launchpad \
        -n launchpad --timeout=120s 2>/dev/null || true

    # Port-forward in background
    kubectl port-forward -n launchpad svc/launchpad 18080:80 &>/dev/null &
    local pf_pid=$!
    sleep 3

    # Login to get a token
    local token
    token=$(curl -sf http://localhost:18080/api/auth/login \
        -H 'Content-Type: application/json' \
        -d '{"username":"admin","password":"admin123"}' | \
        python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null) || true

    if [ -n "$token" ]; then
        # Create the Windows test app
        local http_code
        http_code=$(curl -sf -o /dev/null -w '%{http_code}' http://localhost:18080/api/apps \
            -H 'Content-Type: application/json' \
            -H "Authorization: Bearer ${token}" \
            -d '{
                "id": "windows-desktop",
                "name": "Windows Desktop (Test)",
                "description": "Test desktop via RDP (xrdp + XFCE)",
                "url": "",
                "icon": "https://upload.wikimedia.org/wikipedia/commons/thumb/5/5f/Windows_logo_-_2012.svg/88px-Windows_logo_-_2012.svg.png",
                "category": "Development",
                "launch_type": "container",
                "os_type": "windows",
                "container_image": "ghcr.io/rjsadow/launchpad-xrdp-test:latest",
                "resource_limits": {
                    "cpu_request": "500m",
                    "cpu_limit": "2",
                    "memory_request": "512Mi",
                    "memory_limit": "2Gi"
                }
            }') || true

        if [ "$http_code" = "201" ]; then
            log_info "Windows test app seeded successfully."
        elif [ "$http_code" = "409" ]; then
            log_warn "Windows test app already exists, skipping."
        else
            log_warn "Failed to seed Windows test app (HTTP $http_code). You can add it manually via the admin UI."
        fi
    else
        log_warn "Could not authenticate to seed Windows test app. Add it manually via the admin UI."
    fi

    # Clean up port-forward
    kill "$pf_pid" 2>/dev/null || true
    wait "$pf_pid" 2>/dev/null || true
}

main() {
    cd "$ROOT_DIR"

    case "${1:-}" in
        teardown|delete|destroy)
            teardown
            ;;
        windows)
            check_dependencies
            create_cluster
            load_images
            load_windows_images
            deploy_helm
            seed_windows_app
            show_access_info
            echo ""
            log_info "Windows RDP support enabled with test xrdp desktop app."
            echo "  Login and launch 'Windows Desktop (Test)' to test RDP via Guacamole."
            ;;
        *)
            check_dependencies
            create_cluster
            load_images
            deploy_helm
            show_access_info
            ;;
    esac
}

main "$@"
