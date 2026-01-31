# VNC Sidecar Container for Launchpad
# Provides X11 display via Xvfb, VNC server via x11vnc, and WebSocket via websockify
FROM ubuntu:22.04

LABEL org.opencontainers.image.source="https://github.com/rjsadow/launchpad"
LABEL org.opencontainers.image.description="VNC sidecar for streaming thick-client applications"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# Avoid prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install required packages
RUN apt-get update && apt-get install -y --no-install-recommends \
    xvfb \
    x11vnc \
    websockify \
    supervisor \
    procps \
    net-tools \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -m -u 1000 -s /bin/bash appuser

# Create necessary directories
RUN mkdir -p /tmp/.X11-unix /var/log/supervisor /run/supervisor && \
    chown -R appuser:appuser /tmp/.X11-unix /var/log/supervisor /run/supervisor && \
    chmod 1777 /tmp/.X11-unix

# Copy supervisor configuration
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Set environment variables
ENV DISPLAY=:99
ENV VNC_PORT=5900
ENV WEBSOCKET_PORT=6080
ENV SCREEN_RESOLUTION=1920x1080x24

# Switch to non-root user
USER appuser
WORKDIR /home/appuser

# Expose VNC and WebSocket ports
EXPOSE 5900 6080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD netstat -tlnp | grep -q ":6080" || exit 1

# Start supervisor
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
