#!/bin/bash
# Parse SCREEN_RESOLUTION (e.g. "1920x1080x24") into Xvnc flags
RES="${SCREEN_RESOLUTION:-1920x1080x24}"
WIDTH=$(echo "$RES" | cut -d'x' -f1)
HEIGHT=$(echo "$RES" | cut -d'x' -f2)
DEPTH=$(echo "$RES" | cut -d'x' -f3)
DEPTH=${DEPTH:-24}

# Extract display number from DISPLAY (e.g. ":99" -> "99")
DISPLAY_NUM="${DISPLAY#:}"
X11_SOCKET="/tmp/.X11-unix/X${DISPLAY_NUM}"

# Check if another X server already owns this display (e.g. linuxserver images).
# If the socket exists, use x11vnc to attach to the existing display.
if [ -e "$X11_SOCKET" ]; then
  echo "Detected existing X server on ${DISPLAY}, using x11vnc"
  exec /usr/bin/x11vnc \
    -display "${DISPLAY}" \
    -rfbport "${VNC_PORT}" \
    -nopw \
    -shared \
    -forever \
    -noxdamage
fi

# Try to start Xvnc. If it fails because another X server grabbed the display
# while we were checking, fall back to x11vnc.
echo "Starting Xvnc on ${DISPLAY}"
/usr/bin/Xvnc "${DISPLAY}" \
  -geometry "${WIDTH}x${HEIGHT}" \
  -depth "${DEPTH}" \
  -rfbport "${VNC_PORT}" \
  -SecurityTypes None \
  -AlwaysShared \
  -ac \
  -pn \
  -SendCutText \
  -AcceptCutText &
XVNC_PID=$!

# Give Xvnc a moment to start or fail
sleep 1

if kill -0 "$XVNC_PID" 2>/dev/null; then
  # Xvnc started successfully — wait for it
  wait "$XVNC_PID"
else
  # Xvnc failed — likely an X server appeared on the display during startup.
  # Wait for the socket to be ready, then attach with x11vnc.
  echo "Xvnc failed, waiting for external X server on ${DISPLAY}..."
  for i in $(seq 1 30); do
    if [ -e "$X11_SOCKET" ]; then
      echo "External X server ready, starting x11vnc"
      exec /usr/bin/x11vnc \
        -display "${DISPLAY}" \
        -rfbport "${VNC_PORT}" \
        -nopw \
        -shared \
        -forever \
        -noxdamage
    fi
    sleep 1
  done
  echo "ERROR: No X server found on ${DISPLAY} after 30s"
  exit 1
fi
