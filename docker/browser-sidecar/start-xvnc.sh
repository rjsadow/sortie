#!/bin/bash
# Parse SCREEN_RESOLUTION (e.g. "1920x1080x24") into Xvnc flags
RES="${SCREEN_RESOLUTION:-1920x1080x24}"
WIDTH=$(echo "$RES" | cut -d'x' -f1)
HEIGHT=$(echo "$RES" | cut -d'x' -f2)
DEPTH=$(echo "$RES" | cut -d'x' -f3)
DEPTH=${DEPTH:-24}

exec /usr/bin/Xvnc "${DISPLAY}" \
  -geometry "${WIDTH}x${HEIGHT}" \
  -depth "${DEPTH}" \
  -rfbport "${VNC_PORT}" \
  -SecurityTypes None \
  -AlwaysShared \
  -ac \
  -pn \
  -SendCutText \
  -AcceptCutText
