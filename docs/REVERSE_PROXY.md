# Reverse Proxy Configuration Guide

This guide covers deploying Sortie behind a reverse proxy for production
environments. It includes TLS termination, WebSocket support, path routing,
and security headers.

## Overview

In production, Sortie should run behind a reverse proxy that handles:

- **TLS termination**: HTTPS encryption at the edge
- **WebSocket proxying**: For VNC streaming to containerized apps
- **Load balancing**: Distribute traffic across instances
- **Security headers**: Add protective HTTP headers
- **Path routing**: Route requests to appropriate backends

```text
┌─────────┐     HTTPS      ┌─────────────┐     HTTP      ┌───────────┐
│ Browser │ ──────────────▶│   NGINX     │ ────────────▶│ Sortie │
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

### Complete Production Example - NGINX

```nginx
# /etc/nginx/sites-available/sortie.conf

# Upstream for Sortie backend
upstream sortie_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    listen [::]:80;
    server_name sortie.example.com;

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
    server_name sortie.example.com;

    # ==========================================================
    # TLS Configuration
    # ==========================================================

    ssl_certificate /etc/letsencrypt/live/sortie.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/sortie.example.com/privkey.pem;

    # TLS settings (Mozilla Intermediate)
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;

    # OCSP Stapling
    ssl_stapling on;
    ssl_stapling_verify on;
    ssl_trusted_certificate /etc/letsencrypt/live/sortie.example.com/chain.pem;
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

    access_log /var/log/nginx/sortie_access.log;
    error_log /var/log/nginx/sortie_error.log;

    # ==========================================================
    # Path Routing
    # ==========================================================

    # API endpoints
    location /api/ {
        proxy_pass http://sortie_backend;
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
        proxy_pass http://sortie_backend;
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
        proxy_pass http://sortie_backend;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Cache static assets
        location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)$ {
            proxy_pass http://sortie_backend;
            proxy_cache_valid 200 7d;
            add_header Cache-Control "public, max-age=604800, immutable";
        }
    }

    # Health check endpoint (bypass auth if using external health monitors)
    location = /api/apps {
        proxy_pass http://sortie_backend;
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
sudo certbot --nginx -d sortie.example.com

# Or standalone (if NGINX not running yet)
sudo certbot certonly --standalone -d sortie.example.com

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
websocat wss://sortie.example.com/ws/sessions/test

# Using curl (check upgrade)
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: $(openssl rand -base64 16)" \
  https://sortie.example.com/ws/sessions/test
```

## Path Routing

### Route Summary

| Path Pattern         | Backend   | Notes                  |
|----------------------|-----------|------------------------|
| `/api/*`             | Sortie | REST API endpoints     |
| `/ws/*`              | Sortie | WebSocket (VNC streams)|
| `/`                  | Sortie | Frontend static files  |
| `/*.js, *.css, etc.` | Sortie | Cached static assets   |

### Multi-Backend Example

If running microservices or additional backends:

```nginx
upstream sortie_backend {
    server 127.0.0.1:8080;
}

upstream auth_backend {
    server 127.0.0.1:8081;
}

server {
    # ... TLS config ...

    location /api/ {
        proxy_pass http://sortie_backend;
        # ... proxy settings ...
    }

    location /auth/ {
        proxy_pass http://auth_backend;
        # ... proxy settings ...
    }

    location / {
        proxy_pass http://sortie_backend;
        # ... proxy settings ...
    }
}
```

## Headers Configuration

### Forwarded Headers

Sortie needs to know the original client information:

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
upstream sortie_backend {
    least_conn;  # Load balancing method

    server 10.0.0.1:8080 weight=5;
    server 10.0.0.2:8080 weight=5;
    server 10.0.0.3:8080 backup;

    keepalive 32;
}
```

### Health Checks (NGINX Plus or OpenResty)

```nginx
upstream sortie_backend {
    server 10.0.0.1:8080;
    server 10.0.0.2:8080;

    health_check interval=5s fails=3 passes=2;
}
```

For open-source NGINX, use passive health checks:

```nginx
upstream sortie_backend {
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
openssl s_client -connect sortie.example.com:443 -servername sortie.example.com

# Using SSL Labs (public sites)
# Visit: https://www.ssllabs.com/ssltest/
```

### Verify Headers

```bash
curl -I https://sortie.example.com
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
  https://sortie.example.com/ws/sessions/test
```

Should return `101 Switching Protocols` (when session exists).

## Troubleshooting

### Common Issues

#### 502 Bad Gateway

- Backend not running: `systemctl status sortie`
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
error_log /var/log/nginx/sortie_error.log debug;
```

Check logs:

```bash
tail -f /var/log/nginx/sortie_error.log
```

## Traefik Configuration

Traefik is a cloud-native reverse proxy with automatic service discovery.

### Docker Compose Example

```yaml
# docker-compose.yml
version: "3.8"

services:
  traefik:
    image: traefik:v3.0
    command:
      - "--api.dashboard=true"
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--entrypoints.web.http.redirections.entryPoint.to=websecure"
      - "--entrypoints.web.http.redirections.entryPoint.scheme=https"
      - "--certificatesresolvers.letsencrypt.acme.email=admin@example.com"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
      - "--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
      - "letsencrypt:/letsencrypt"
    networks:
      - sortie

  sortie:
    image: your-registry/sortie:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.sortie.rule=Host(`sortie.example.com`)"
      - "traefik.http.routers.sortie.entrypoints=websecure"
      - "traefik.http.routers.sortie.tls.certresolver=letsencrypt"
      - "traefik.http.services.sortie.loadbalancer.server.port=8080"
      # Security headers middleware
      - "traefik.http.middlewares.security-headers.headers.stsSeconds=63072000"
      - "traefik.http.middlewares.security-headers.headers.stsIncludeSubdomains=true"
      - "traefik.http.middlewares.security-headers.headers.stsPreload=true"
      - "traefik.http.middlewares.security-headers.headers.frameDeny=true"
      - "traefik.http.middlewares.security-headers.headers.contentTypeNosniff=true"
      - "traefik.http.middlewares.security-headers.headers.browserXssFilter=true"
      - "traefik.http.middlewares.security-headers.headers.referrerPolicy=strict-origin-when-cross-origin"
      - "traefik.http.middlewares.security-headers.headers.permissionsPolicy=geolocation=(), microphone=(), camera=()"
      - "traefik.http.routers.sortie.middlewares=security-headers"
    networks:
      - sortie

volumes:
  letsencrypt:

networks:
  sortie:
```

### File-Based Configuration

For production deployments without Docker, use file-based configuration:

```yaml
# /etc/traefik/traefik.yml
api:
  dashboard: true
  insecure: false

entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
  websecure:
    address: ":443"

providers:
  file:
    directory: /etc/traefik/conf.d
    watch: true

certificatesResolvers:
  letsencrypt:
    acme:
      email: admin@example.com
      storage: /etc/traefik/acme.json
      httpChallenge:
        entryPoint: web

log:
  level: INFO
  filePath: /var/log/traefik/traefik.log

accessLog:
  filePath: /var/log/traefik/access.log
```

```yaml
# /etc/traefik/conf.d/sortie.yml
http:
  routers:
    sortie:
      rule: "Host(`sortie.example.com`)"
      entryPoints:
        - websecure
      service: sortie
      tls:
        certResolver: letsencrypt
      middlewares:
        - security-headers
        - rate-limit

    sortie-ws:
      rule: "Host(`sortie.example.com`) && PathPrefix(`/ws/`)"
      entryPoints:
        - websecure
      service: sortie
      tls:
        certResolver: letsencrypt
      middlewares:
        - security-headers

  services:
    sortie:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8080"
        healthCheck:
          path: /api/apps
          interval: 10s
          timeout: 3s

  middlewares:
    security-headers:
      headers:
        stsSeconds: 63072000
        stsIncludeSubdomains: true
        stsPreload: true
        frameDeny: true
        contentTypeNosniff: true
        browserXssFilter: true
        referrerPolicy: strict-origin-when-cross-origin
        permissionsPolicy: "geolocation=(), microphone=(), camera=()"
        customResponseHeaders:
          X-Robots-Tag: "noindex, nofollow"

    rate-limit:
      rateLimit:
        average: 100
        burst: 50
```

### Using Existing TLS Certificates - traefik

```yaml
# /etc/traefik/conf.d/tls.yml
tls:
  certificates:
    - certFile: /etc/ssl/certs/sortie.example.com.crt
      keyFile: /etc/ssl/private/sortie.example.com.key

  options:
    default:
      minVersion: VersionTLS12
      cipherSuites:
        - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
        - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
        - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
        - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
        - TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305
        - TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305
```

### Kubernetes IngressRoute

For Traefik in Kubernetes using CRDs:

```yaml
# ingressroute.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: sortie
  namespace: sortie
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`sortie.example.com`)
      kind: Rule
      services:
        - name: sortie
          port: 80
      middlewares:
        - name: security-headers
          namespace: sortie
  tls:
    certResolver: letsencrypt
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: security-headers
  namespace: sortie
spec:
  headers:
    stsSeconds: 63072000
    stsIncludeSubdomains: true
    stsPreload: true
    frameDeny: true
    contentTypeNosniff: true
    browserXssFilter: true
    referrerPolicy: strict-origin-when-cross-origin
```

### WebSocket Configuration for Traefik

Traefik automatically handles WebSocket upgrades. No special configuration
is needed for the `/ws/` endpoints. For long-lived connections, ensure
your load balancer or cloud provider doesn't have lower timeouts.

### Testing Traefik Configuration

```bash
# Validate configuration
traefik --configfile=/etc/traefik/traefik.yml --check

# Check dashboard (if enabled)
curl http://localhost:8080/api/overview

# Verify TLS
openssl s_client -connect sortie.example.com:443 \
  -servername sortie.example.com
```

## Caddy Configuration

Caddy provides automatic HTTPS with minimal configuration.

### Complete Production Example

```text
# /etc/caddy/Caddyfile

# Global options
{
    email admin@example.com
    acme_ca https://acme-v02.api.letsencrypt.org/directory

    # Enable OCSP stapling
    ocsp_stapling on

    # Logging
    log {
        output file /var/log/caddy/access.log
        format json
    }
}

# Main site
sortie.example.com {
    # TLS configuration
    tls {
        protocols tls1.2 tls1.3
        ciphers TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384 TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384 TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305 TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305
    }

    # Security headers
    header {
        Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
        X-XSS-Protection "1; mode=block"
        Referrer-Policy "strict-origin-when-cross-origin"
        Permissions-Policy "geolocation=(), microphone=(), camera=()"
        Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' wss://sortie.example.com; frame-ancestors 'self';"
        -Server
    }

    # WebSocket endpoint - special handling for VNC streaming
    @websocket {
        path /ws/*
        header Connection *Upgrade*
        header Upgrade websocket
    }
    reverse_proxy @websocket localhost:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}

        # Flush immediately for real-time streaming
        flush_interval -1
    }

    # API endpoints
    handle /api/* {
        reverse_proxy localhost:8080 {
            header_up Host {host}
            header_up X-Real-IP {remote_host}
            header_up X-Forwarded-For {remote_host}
            header_up X-Forwarded-Proto {scheme}

            # Health check
            health_uri /api/apps
            health_interval 10s
            health_timeout 5s
        }
    }

    # Static assets with caching
    @static {
        path *.js *.css *.png *.jpg *.jpeg *.gif *.ico *.svg *.woff *.woff2
    }
    handle @static {
        reverse_proxy localhost:8080
        header Cache-Control "public, max-age=604800, immutable"
    }

    # Default handler for frontend
    handle {
        reverse_proxy localhost:8080 {
            header_up Host {host}
            header_up X-Real-IP {remote_host}
            header_up X-Forwarded-For {remote_host}
            header_up X-Forwarded-Proto {scheme}
        }
    }

    # Logging
    log {
        output file /var/log/caddy/sortie.log {
            roll_size 100mb
            roll_keep 10
        }
        format json
    }
}
```

### Using Existing TLS Certificates

```text
sortie.example.com {
    tls /etc/ssl/certs/sortie.example.com.crt /etc/ssl/private/sortie.example.com.key

    # ... rest of configuration
}
```

### Load Balancing with Caddy

```text
sortie.example.com {
    reverse_proxy localhost:8080 localhost:8081 localhost:8082 {
        lb_policy least_conn
        health_uri /api/apps
        health_interval 10s

        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

### Rate Limiting with Caddy

```text
sortie.example.com {
    rate_limit {
        zone static_zone {
            key static
            events 100
            window 1m
        }
    }

    # ... rest of configuration
}
```

### Docker Compose with Caddy

```yaml
# docker-compose.yml
version: "3.8"

services:
  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
      - ./logs:/var/log/caddy
    networks:
      - sortie
    restart: unless-stopped

  sortie:
    image: your-registry/sortie:latest
    expose:
      - "8080"
    networks:
      - sortie
    restart: unless-stopped

volumes:
  caddy_data:
  caddy_config:

networks:
  sortie:
```

### Testing Caddy Configuration

```bash
# Validate Caddyfile
caddy validate --config /etc/caddy/Caddyfile

# Format Caddyfile
caddy fmt --overwrite /etc/caddy/Caddyfile

# Reload configuration
caddy reload --config /etc/caddy/Caddyfile

# Test TLS
curl -I https://sortie.example.com
```

## Proxy Comparison

| Feature              | NGINX              | Traefik            | Caddy              |
|----------------------|--------------------|--------------------|---------------------|
| Auto TLS (Let's Encrypt) | Manual setup   | Built-in           | Built-in           |
| Config complexity    | Medium             | Low-Medium         | Low                |
| Hot reload           | `nginx -s reload`  | Automatic          | Automatic          |
| Docker integration   | Manual             | Native labels      | Manual             |
| Kubernetes support   | Ingress            | IngressRoute CRD   | Ingress            |
| WebSocket            | Manual config      | Automatic          | Automatic          |
| Performance          | Excellent          | Good               | Good               |
| Memory footprint     | Low                | Medium             | Low                |
| Learning curve       | Medium             | Low                | Low                |

### Recommendations

- **NGINX**: Best for high-traffic production environments with complex
  routing requirements and when you need maximum performance.

- **Traefik**: Ideal for containerized environments (Docker/Kubernetes)
  with automatic service discovery and built-in Let's Encrypt.

- **Caddy**: Best for simplicity when you want automatic HTTPS with
  minimal configuration. Great for smaller deployments.

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
