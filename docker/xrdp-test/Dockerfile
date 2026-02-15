# Test image for Windows RDP support
# Uses xrdp on Linux to simulate an RDP server for testing the guacd pipeline
FROM ubuntu:22.04@sha256:c7eb020043d8fc2ae0793fb35a37bff1cf33f156d4d4b12ccc7f3ef8706c38b1

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    xrdp \
    xorgxrdp \
    xauth \
    xfce4 \
    xfce4-terminal \
    dbus-x11 \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Configure xrdp
RUN sed -i 's/^port=3389/port=3389/' /etc/xrdp/xrdp.ini && \
    sed -i 's/^security_layer=.*/security_layer=rdp/' /etc/xrdp/xrdp.ini && \
    sed -i 's/^crypt_level=.*/crypt_level=none/' /etc/xrdp/xrdp.ini && \
    sed -i 's/^max_bpp=.*/max_bpp=24/' /etc/xrdp/xrdp.ini && \
    # Auto-login: skip the xrdp login screen when credentials are provided via RDP
    # Handle both commented (#autorun=) and uncommented (autorun=) lines
    sed -i 's/^#\?autorun=.*/autorun=Xorg/' /etc/xrdp/xrdp.ini && \
    # If autorun line doesn't exist at all, add it after [Globals]
    grep -q '^autorun=' /etc/xrdp/xrdp.ini || sed -i '/^\[Globals\]/a autorun=Xorg' /etc/xrdp/xrdp.ini

# Create a test user
RUN useradd -m -s /bin/bash testuser && \
    echo "testuser:testpass" | chpasswd && \
    adduser testuser sudo

# Set up xrdp to use xfce4
RUN echo "xfce4-session" > /home/testuser/.xsession && \
    chown testuser:testuser /home/testuser/.xsession

# Start script
COPY start-xrdp.sh /start-xrdp.sh
RUN chmod +x /start-xrdp.sh

EXPOSE 3389

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD nc -z localhost 3389 || exit 1

CMD ["/start-xrdp.sh"]
