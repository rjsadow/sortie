#!/usr/bin/env bash
#
# Kind cluster setup script for Launchpad local development
#
# Usage:
#   ./scripts/kind-setup.sh          # Create cluster and deploy
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
}

deploy_helm() {
    log_info "Deploying Launchpad via Helm..."

    # Install or upgrade the release
    helm upgrade --install launchpad "${ROOT_DIR}/charts/launchpad" \
        --namespace launchpad \
        --create-namespace \
        --set image.pullPolicy=Never \
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

main() {
    cd "$ROOT_DIR"

    case "${1:-}" in
        teardown|delete|destroy)
            teardown
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
