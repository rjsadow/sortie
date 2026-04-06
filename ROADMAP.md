# Sortie Roadmap

A self-hosted application launcher that streams containerized desktop
environments to the browser via Kubernetes.

## Vision

Provide organizations a single, reliable portal to launch and manage
internal tools. Users browse a catalog, click launch, and get a running
desktop app streamed to their browser — no local install required.

## Completed

### Core Platform

- [x] Application catalog with grid/list view
- [x] Search and filter applications by name
- [x] Categories for application grouping
- [x] Responsive design (desktop + mobile)
- [x] Dark mode
- [x] User favorites (client-side)
- [x] Configurable branding (logo, colors, tenant name)

### App Launch Types

- [x] **URL apps** — open external links in a new tab
- [x] **Container apps (Linux)** — Kubernetes pod with VNC sidecar,
      streamed via noVNC
- [x] **Container apps (Windows)** — Kubernetes pod with guacd + xrdp
      sidecars, streamed via Apache Guacamole
- [x] **Web proxy apps** — Kubernetes pod with headless browser sidecar,
      proxied through the backend

### Session Management

- [x] Session lifecycle (create, ready, running, stopped, expired)
- [x] Configurable session timeout and cleanup
- [x] Session queue with backpressure
- [x] Per-user and global session quotas
- [x] Configurable resource limits (CPU, memory) per session
- [x] Session sharing (invite by username or share link, view/interact
      permissions)

### Authentication & Authorization

- [x] JWT authentication (access + refresh tokens)
- [x] OIDC/SSO integration (Auth0, Keycloak, Entra ID, Okta)
- [x] Role-based access control (admin, user)
- [x] Category-scoped admin roles
- [x] Application visibility levels (public, approved, admin-only)
- [x] User self-registration (admin-configurable)

### Admin & Operations

- [x] Admin panel (users, apps, categories, settings, sessions)
- [x] REST API for programmatic management
- [x] Audit logging
- [x] Template marketplace (pre-configured app definitions)
- [x] Session video recording (local and S3 storage)
- [x] Network egress policies per application
- [x] Billing/metering event collection

### Infrastructure

- [x] Single binary with embedded frontend and docs
- [x] SQLite and PostgreSQL database support
- [x] Database migrations (golang-migrate)
- [x] Helm chart with RBAC, NetworkPolicy, ResourceQuota
- [x] Multi-arch Docker image (amd64, arm64)
- [x] External secrets (Vault, AWS Secrets Manager, Kubernetes Secrets)
- [x] CI pipeline (lint, test, security scan, Playwright E2E)
- [x] Automated releases with SBOM and attestation

### Documentation

- [x] VitePress documentation site
- [x] Getting started guide
- [x] Kubernetes deployment guide
- [x] Reverse proxy setup (NGINX, Traefik, Caddy)
- [x] Data persistence and disaster recovery guides
- [x] Plugin system documentation
- [x] API reference

---

## In Progress

- [ ] Multi-database support — Postgres integration hardening
- [ ] Plugin system — custom launcher, auth, and storage plugins

---

## Planned

### Near-term

- [ ] Keyboard navigation and shortcuts
- [ ] Application health status indicators
- [ ] Server-side user favorites
- [ ] High availability deployment guide
- [ ] Published docs site (GitHub Pages)

### Future

- [ ] File transfer between browser and container sessions
- [ ] Clipboard sync for VNC/RDP sessions
- [ ] Session snapshots and restore
- [ ] Custom branding per department/tenant
- [ ] Usage analytics dashboard
- [ ] Webhook integrations for session lifecycle events
