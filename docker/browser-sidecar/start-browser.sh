#!/bin/bash
# Start browser script for browser-sidecar
# Waits for the web service to be ready, then launches Firefox

set -e

# Default URL if not set
BROWSER_URL="${BROWSER_URL:-http://localhost:8080}"

echo "Browser sidecar starting..."
echo "Target URL: $BROWSER_URL"

# Extract host and port from URL for health check
# Remove protocol prefix, then extract host (before : or /)
URL_HOST=$(echo "$BROWSER_URL" | sed -E 's|^https?://||' | sed -E 's|[:/].*||')

# Extract port - look for :PORT pattern after host
URL_PORT=$(echo "$BROWSER_URL" | sed -E 's|^https?://[^:/]+:([0-9]+).*|\1|')
# If no port was extracted (pattern didn't match), use default based on protocol
if [ "$URL_PORT" = "$BROWSER_URL" ] || [ -z "$URL_PORT" ]; then
    if [[ "$BROWSER_URL" == https://* ]]; then
        URL_PORT=443
    else
        URL_PORT=80
    fi
fi

echo "Waiting for web service at $URL_HOST:$URL_PORT..."

# Wait for the web service to be available (max 120 seconds)
MAX_WAIT=120
WAIT_COUNT=0
while ! nc -z "$URL_HOST" "$URL_PORT" 2>/dev/null; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
        echo "Timeout waiting for web service at $URL_HOST:$URL_PORT"
        echo "Starting browser anyway..."
        break
    fi
    if [ $((WAIT_COUNT % 10)) -eq 0 ]; then
        echo "Still waiting for $URL_HOST:$URL_PORT... ($WAIT_COUNT seconds)"
    fi
done

if [ $WAIT_COUNT -lt $MAX_WAIT ]; then
    echo "Web service is available!"
    # Give it a moment to fully initialize
    sleep 2
fi

echo "Launching Firefox..."

# Launch Firefox in kiosk-like mode
# --no-remote: Don't connect to existing Firefox instance
# --new-instance: Start a new instance
# Maximize window via openbox
exec firefox \
    --no-remote \
    --new-instance \
    "$BROWSER_URL"
