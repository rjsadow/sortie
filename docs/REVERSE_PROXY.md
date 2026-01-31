# Reverse Proxy Configuration Guide

This guide covers deploying Launchpad behind a reverse proxy for production
environments. It includes TLS termination, WebSocket support, path routing,
and security headers.

## Overview

In production, Launchpad should run behind a reverse proxy that handles:

- **TLS termination**: HTTPS encryption at the edge
- **WebSocket proxying**: For VNC streaming to containerized apps
- **Load balancing**: Distribute traffic across instances
- **Security headers**: Add protective HTTP headers
- **Path routing**: Route requests to appropriate backends

```text
┌─────────┐     HTTPS      ┌─────────────┐     HTTP      ┌───────────┐
│ Browser │ ──────────────▶│   NGINX     │ ────────────▶│ Launchpad │
└─────────┘                │ (TLS term)  │              │  :8080    │
                           └─────────────┘              └───────────┘
                                 │
                                 │ WebSocket upgrade
                                 ▼
                           ┌───────────┐
                           │ VNC Proxy │
                           └───────────┘
```

## NGINX Configuration

### Complete Production Example

```nginx
# /etc/nginx/sites-available/launchpad.conf

# Upstream for Launchpad backend
upstream launchpad_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    listen [::]:80;
    server_name launchpad.example.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$server_name$request_uri;
    }
}

# Main HTTPS server
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name launchpad.example.com;

    # ==========================================================
    # TLS Configuration
    # ==========================================================

    ssl_certificate /etc/letsencrypt/live/launchpad.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/launchpad.example.com/privkey.pem;

    # TLS settings (Mozilla Intermediate)
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;

    # OCSP Stapling
    ssl_stapling on;
    ssl_stapling_verify on;
    ssl_trusted_certificate /etc/letsencrypt/live/launchpad.example.com/chain.pem;
    resolver 8.8.8.8 8.8.4.4 valid=300s;
    resolver_timeout 5s;

    # SSL session settings
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:50m;
    ssl_session_tickets off;

    # ==========================================================
    # Security Headers
    # ==========================================================

    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;

    # Content Security Policy (adjust as needed for your deployment)
    add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' wss://$server_name; frame-ancestors 'self';" always;

    # ==========================================================
    # Logging
    # ==========================================================

    access_log /var/log/nginx/launchpad_access.log;
    error_log /var/log/nginx/launchpad_error.log;

    # ==========================================================
    # Path Routing
    # ==========================================================

    # API endpoints
    location /api/ {
        proxy_pass http://launchpad_backend;
        proxy_http_version 1.1;

        # Headers for backend
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Port $server_port;

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;

        # Buffering
        proxy_buffering on;
        proxy_buffer_size 4k;
        proxy_buffers 8 4k;
    }

    # WebSocket endpoint for VNC sessions
    location /ws/ {
        proxy_pass http://launchpad_backend;
        proxy_http_version 1.1;

        # WebSocket upgrade headers
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket-specific timeouts (longer for persistent connections)
        proxy_connect_timeout 60s;
        proxy_send_timeout 3600s;
        proxy_read_timeout 3600s;

        # Disable buffering for WebSocket
        proxy_buffering off;
    }

    # Static assets and frontend
    location / {
        proxy_pass http://launchpad_backend;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Cache static assets
        location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)$ {
            proxy_pass http://launchpad_backend;
            proxy_cache_valid 200 7d;
            add_header Cache-Control "public, max-age=604800, immutable";
        }
    }

    # Health check endpoint (bypass auth if using external health monitors)
    location = /api/apps {
        proxy_pass http://launchpad_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## TLS Termination

### Obtaining Certificates with Let's Encrypt

```bash
# Install certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificate (NGINX plugin)
sudo certbot --nginx -d launchpad.example.com

# Or standalone (if NGINX not running yet)
sudo certbot certonly --standalone -d launchpad.example.com

# Test renewal
sudo certbot renew --dry-run
```

### Using Existing Certificates

If you have certificates from another CA:

```nginx
ssl_certificate /path/to/your/certificate.crt;
ssl_certificate_key /path/to/your/private.key;
ssl_trusted_certificate /path/to/your/ca-bundle.crt;
```

### Certificate Auto-Renewal

Certbot sets up automatic renewal via systemd timer or cron. Verify:

```bash
# Check timer status
sudo systemctl status certbot.timer

# Manual renewal (if needed)
sudo certbot renew --post-hook "systemctl reload nginx"
```

## WebSocket Support

WebSocket connections are used for VNC streaming to containerized applications.
Key configuration points:

### Required Headers

```nginx
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

### Timeout Configuration

WebSocket connections are long-lived. Default NGINX timeouts (60s) will
disconnect idle sessions. Increase for VNC:

```nginx
proxy_read_timeout 3600s;   # 1 hour
proxy_send_timeout 3600s;
```

### Disable Buffering

Buffering adds latency to real-time streams:

```nginx
proxy_buffering off;
```

### Testing WebSocket Connectivity

```bash
# Using websocat
websocat wss://launchpad.example.com/ws/sessions/test

# Using curl (check upgrade)
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: $(openssl rand -base64 16)" \
  https://launchpad.example.com/ws/sessions/test
```

## Path Routing

### Route Summary

| Path Pattern         | Backend   | Notes                  |
|----------------------|-----------|------------------------|
| `/api/*`             | Launchpad | REST API endpoints     |
| `/ws/*`              | Launchpad | WebSocket (VNC streams)|
| `/`                  | Launchpad | Frontend static files  |
| `/*.js, *.css, etc.` | Launchpad | Cached static assets   |

### Multi-Backend Example

If running microservices or additional backends:

```nginx
upstream launchpad_backend {
    server 127.0.0.1:8080;
}

upstream auth_backend {
    server 127.0.0.1:8081;
}

server {
    # ... TLS config ...

    location /api/ {
        proxy_pass http://launchpad_backend;
        # ... proxy settings ...
    }

    location /auth/ {
        proxy_pass http://auth_backend;
        # ... proxy settings ...
    }

    location / {
        proxy_pass http://launchpad_backend;
        # ... proxy settings ...
    }
}
```

## Headers Configuration

### Forwarded Headers

Launchpad needs to know the original client information:

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Host $host;
proxy_set_header X-Forwarded-Port $server_port;
```

### Security Headers Reference

| Header                      | Value                                 | Purpose                 |
|-----------------------------|---------------------------------------|-------------------------|
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains` | Force HTTPS             |
| `X-Frame-Options`           | `SAMEORIGIN`                          | Prevent clickjacking    |
| `X-Content-Type-Options`    | `nosniff`                             | Prevent MIME sniffing   |
| `X-XSS-Protection`          | `1; mode=block`                       | XSS filter              |
| `Referrer-Policy`           | `strict-origin-when-cross-origin`     | Control referrer        |
| `Permissions-Policy`        | `geolocation=(), ...`                 | Disable browser features|
| `Content-Security-Policy`   | See config                            | Control resource loading|

### Custom Headers for Debugging

```nginx
# Add request ID for tracing
proxy_set_header X-Request-ID $request_id;

# Log request ID
log_format trace '$remote_addr - $request_id - "$request" $status';
```

## Load Balancing

### Multiple Backend Instances

```nginx
upstream launchpad_backend {
    least_conn;  # Load balancing method

    server 10.0.0.1:8080 weight=5;
    server 10.0.0.2:8080 weight=5;
    server 10.0.0.3:8080 backup;

    keepalive 32;
}
```

### Health Checks (NGINX Plus or OpenResty)

```nginx
upstream launchpad_backend {
    server 10.0.0.1:8080;
    server 10.0.0.2:8080;

    health_check interval=5s fails=3 passes=2;
}
```

For open-source NGINX, use passive health checks:

```nginx
upstream launchpad_backend {
    server 10.0.0.1:8080 max_fails=3 fail_timeout=30s;
    server 10.0.0.2:8080 max_fails=3 fail_timeout=30s;
}
```

## Testing the Configuration

### Validate NGINX Config

```bash
sudo nginx -t
```

### Check TLS Configuration

```bash
# Using OpenSSL
openssl s_client -connect launchpad.example.com:443 -servername launchpad.example.com

# Using SSL Labs (public sites)
# Visit: https://www.ssllabs.com/ssltest/
```

### Verify Headers

```bash
curl -I https://launchpad.example.com
```

Expected output includes:

```text
HTTP/2 200
strict-transport-security: max-age=63072000; includeSubDomains; preload
x-frame-options: SAMEORIGIN
x-content-type-options: nosniff
```

### Test WebSocket Upgrade

```bash
curl -I \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  https://launchpad.example.com/ws/sessions/test
```

Should return `101 Switching Protocols` (when session exists).

## Troubleshooting

### Common Issues

#### 502 Bad Gateway

- Backend not running: `systemctl status launchpad`
- Firewall blocking: `sudo ufw status`
- Wrong upstream address: Check `proxy_pass` directive

#### WebSocket disconnects

- Timeout too short: Increase `proxy_read_timeout`
- Load balancer in path: Ensure sticky sessions or direct connection

#### Mixed content warnings

- Missing `X-Forwarded-Proto`: Add header in proxy config
- Hardcoded HTTP URLs: Check application configuration

#### Certificate errors

- Wrong domain: Verify `server_name` matches certificate
- Expired certificate: Run `certbot renew`
- Missing chain: Include intermediate certificates

### Debug Logging

Enable debug logging temporarily:

```nginx
error_log /var/log/nginx/launchpad_error.log debug;
```

Check logs:

```bash
tail -f /var/log/nginx/launchpad_error.log
```

## Alternative: Caddy Configuration

For simpler setup, Caddy handles TLS automatically:

```text
# /etc/caddy/Caddyfile
launchpad.example.com {
    reverse_proxy /api/* localhost:8080
    reverse_proxy /ws/* localhost:8080

    @websocket {
        header Connection *Upgrade*
        header Upgrade websocket
    }
    reverse_proxy @websocket localhost:8080

    reverse_proxy localhost:8080

    header {
        Strict-Transport-Security "max-age=63072000; includeSubDomains"
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
    }
}
```

## Security Checklist

- [ ] TLS 1.2+ only (no SSLv3, TLS 1.0, 1.1)
- [ ] Strong cipher suites configured
- [ ] HSTS header enabled
- [ ] Security headers configured
- [ ] Certificate auto-renewal working
- [ ] Firewall allows only 80/443
- [ ] Backend only accessible from proxy
- [ ] Logs configured and rotating
- [ ] Rate limiting configured (if needed)
