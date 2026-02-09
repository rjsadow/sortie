#!/usr/bin/env bash
#
# CI E2E test infrastructure for Sortie.
#
# Usage:
#   ./scripts/ci-e2e.sh setup          # Create Kind cluster, build/load images, deploy via Helm
#   ./scripts/ci-e2e.sh collect-logs   # Collect pod logs and events (for CI artifact upload)
#   ./scripts/ci-e2e.sh teardown       # Delete Kind cluster
#
set -euo pipefail

CLUSTER_NAME="sortie-e2e"
NAMESPACE="sortie"
PORT_FORWARD_PORT=18080
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
LOG_DIR="${LOG_DIR:-/tmp/e2e-logs}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

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

setup() {
    check_dependencies

    log_info "Creating Kind cluster '${CLUSTER_NAME}'..."
    kind create cluster --name "$CLUSTER_NAME" \
        --config "${ROOT_DIR}/deploy/kind/kind-config.yaml" \
        --wait 120s

    log_info "Waiting for nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s

    log_info "Building and loading images..."

    # Build main app image
    if [ -f "${ROOT_DIR}/Dockerfile" ]; then
        docker build -t ghcr.io/rjsadow/sortie:e2e "${ROOT_DIR}"
        kind load docker-image ghcr.io/rjsadow/sortie:e2e --name "$CLUSTER_NAME"
    fi

    # Build VNC sidecar
    if [ -d "${ROOT_DIR}/docker/vnc-sidecar" ]; then
        docker build -t ghcr.io/rjsadow/sortie-vnc-sidecar:e2e "${ROOT_DIR}/docker/vnc-sidecar"
        kind load docker-image ghcr.io/rjsadow/sortie-vnc-sidecar:e2e --name "$CLUSTER_NAME"
    fi

    # Build browser sidecar
    if [ -d "${ROOT_DIR}/docker/browser-sidecar" ]; then
        docker build -t ghcr.io/rjsadow/sortie-browser-sidecar:e2e "${ROOT_DIR}/docker/browser-sidecar"
        kind load docker-image ghcr.io/rjsadow/sortie-browser-sidecar:e2e --name "$CLUSTER_NAME"
    fi

    log_info "Deploying Sortie via Helm..."
    helm upgrade --install sortie "${ROOT_DIR}/charts/sortie" \
        --namespace "$NAMESPACE" \
        --create-namespace \
        --set image.repository=ghcr.io/rjsadow/sortie \
        --set image.tag=e2e \
        --set image.pullPolicy=Never \
        --wait --timeout 180s

    log_info "Starting port-forward on localhost:${PORT_FORWARD_PORT}..."
    kubectl port-forward -n "$NAMESPACE" svc/sortie "${PORT_FORWARD_PORT}:80" &
    local pf_pid=$!
    echo "$pf_pid" > /tmp/e2e-port-forward.pid

    log_info "Waiting for /readyz to return 200..."
    local retries=60
    while [ $retries -gt 0 ]; do
        if curl -sf "http://localhost:${PORT_FORWARD_PORT}/readyz" > /dev/null 2>&1; then
            log_info "Sortie is ready!"
            return 0
        fi
        retries=$((retries - 1))
        sleep 2
    done

    log_error "Timed out waiting for Sortie to be ready"
    collect_logs
    exit 1
}

collect_logs() {
    mkdir -p "$LOG_DIR"
    log_info "Collecting logs to ${LOG_DIR}..."

    kubectl get pods -n "$NAMESPACE" -o wide > "${LOG_DIR}/pods.txt" 2>&1 || true
    kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' > "${LOG_DIR}/events.txt" 2>&1 || true
    kubectl describe pods -n "$NAMESPACE" > "${LOG_DIR}/pod-describe.txt" 2>&1 || true

    # Collect logs from all pods in the namespace
    for pod in $(kubectl get pods -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
        for container in $(kubectl get pod "$pod" -n "$NAMESPACE" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null); do
            kubectl logs "$pod" -n "$NAMESPACE" -c "$container" > "${LOG_DIR}/${pod}_${container}.log" 2>&1 || true
        done
    done

    # Collect session pod logs (they run in the same namespace with specific labels)
    for pod in $(kubectl get pods -n "$NAMESPACE" -l managed-by=sortie -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
        for container in $(kubectl get pod "$pod" -n "$NAMESPACE" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null); do
            kubectl logs "$pod" -n "$NAMESPACE" -c "$container" > "${LOG_DIR}/session_${pod}_${container}.log" 2>&1 || true
        done
    done

    log_info "Logs collected in ${LOG_DIR}"
}

teardown() {
    # Kill port-forward if running
    if [ -f /tmp/e2e-port-forward.pid ]; then
        local pf_pid
        pf_pid=$(cat /tmp/e2e-port-forward.pid)
        kill "$pf_pid" 2>/dev/null || true
        rm -f /tmp/e2e-port-forward.pid
    fi

    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_info "Deleting Kind cluster '${CLUSTER_NAME}'..."
        kind delete cluster --name "$CLUSTER_NAME"
        log_info "Cluster deleted."
    else
        log_warn "Cluster '${CLUSTER_NAME}' does not exist."
    fi
}

case "${1:-}" in
    setup)
        setup
        ;;
    collect-logs)
        collect_logs
        ;;
    teardown)
        teardown
        ;;
    *)
        echo "Usage: $0 {setup|collect-logs|teardown}"
        exit 1
        ;;
esac
