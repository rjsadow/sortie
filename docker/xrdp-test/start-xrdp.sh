#!/bin/bash
set -e

# Start D-Bus
mkdir -p /run/dbus
dbus-daemon --system --fork 2>/dev/null || true

# Generate xrdp keys if missing
if [ ! -f /etc/xrdp/rsakeys.ini ]; then
    xrdp-keygen xrdp auto
fi

# Start xrdp-sesman (session manager)
xrdp-sesman --nodaemon &

# Start xrdp (RDP server) in foreground
exec xrdp --nodaemon
