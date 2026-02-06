# Browser Sidecar Container for Launchpad
# Provides X11 display and VNC server via TigerVNC Xvnc, WebSocket, and Firefox browser
# Used for web_proxy apps - browser connects to localhost web service
FROM ubuntu:22.04

LABEL org.opencontainers.image.source="https://github.com/rjsadow/launchpad"
LABEL org.opencontainers.image.description="Browser sidecar for viewing web applications via VNC"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# Avoid prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install required packages and Firefox from Mozilla PPA (not snap)
RUN apt-get update && apt-get install -y --no-install-recommends \
    tigervnc-standalone-server \
    websockify \
    supervisor \
    procps \
    net-tools \
    netcat-openbsd \
    # Window manager (lightweight)
    openbox \
    # Fonts for proper rendering
    fonts-liberation \
    fonts-dejavu-core \
    # D-Bus for Firefox
    dbus-x11 \
    # For adding PPA
    software-properties-common \
    gpg-agent \
    && rm -rf /var/lib/apt/lists/*

# Add Mozilla PPA and install real Firefox (not snap)
RUN add-apt-repository -y ppa:mozillateam/ppa && \
    echo 'Package: *\nPin: release o=LP-PPA-mozillateam\nPin-Priority: 1001' > /etc/apt/preferences.d/mozilla-firefox && \
    apt-get update && \
    apt-get install -y --no-install-recommends firefox && \
    rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -m -u 1000 -s /bin/bash appuser

# Create necessary directories
RUN mkdir -p /tmp/.X11-unix /var/log/supervisor /run/supervisor \
    /home/appuser/.config/openbox /home/appuser/.mozilla && \
    chown -R appuser:appuser /tmp/.X11-unix /var/log/supervisor /run/supervisor \
    /home/appuser/.config /home/appuser/.mozilla && \
    chmod 1777 /tmp/.X11-unix

# Copy configuration files
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY openbox-autostart /home/appuser/.config/openbox/autostart
COPY start-browser.sh /usr/local/bin/start-browser.sh
COPY start-xvnc.sh /usr/local/bin/start-xvnc.sh

# Set permissions
RUN chmod +x /usr/local/bin/start-browser.sh /usr/local/bin/start-xvnc.sh && \
    chown appuser:appuser /home/appuser/.config/openbox/autostart

# Set environment variables
ENV DISPLAY=:99
ENV VNC_PORT=5900
ENV WEBSOCKET_PORT=6080
ENV SCREEN_RESOLUTION=1920x1080x24
# URL to open in the browser (set by pod spec)
ENV BROWSER_URL=http://localhost:8080

# Switch to non-root user
USER appuser
WORKDIR /home/appuser

# Expose VNC and WebSocket ports
EXPOSE 5900 6080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD netstat -tlnp | grep -q ":6080" || exit 1

# Start supervisor
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
