# Sortie

A self-hosted application launcher that gives your organization
one portal to access every internal tool. Users browse a catalog,
click launch, and get a running desktop app streamed to their
browser — no local install required.

## The Problem

Teams juggle dozens of internal tools across scattered bookmarks,
wikis, and Slack messages. Installing desktop software on every
workstation is slow and hard to manage. Sortie solves both: a
single web page lists every app, and containerized apps run in
Kubernetes so users just need a browser.

## Core Concepts

| Concept     | What it is |
|-------------|-----------|
| **App**     | An entry in the catalog. Can be a simple URL link, a Linux container streamed via VNC, or a Windows container streamed via RDP. |
| **Session** | A running instance of a container app. Sortie creates a Kubernetes pod, streams the desktop to the browser, and cleans up when the user is done. |
| **User**    | An authenticated person with a role (`admin` or `user`). Admins manage the catalog, users launch apps. |
| **Template**| A pre-configured app definition from the built-in marketplace. Admins can add apps to the catalog in one click. |

## Quickstart

### Prerequisites

- Go 1.24+, Node.js 22+, Make

### Run locally (no Kubernetes needed for URL-type apps)

```bash
git clone https://github.com/rjsadow/sortie.git
cd sortie
cp .env.example .env          # edit to set SORTIE_JWT_SECRET
make dev                       # starts frontend (:5173) + backend (:8080)
```

Open `http://localhost:5173`, log in with the admin credentials
from your `.env`, and start adding apps.

### Run with Kubernetes (container apps)

```bash
# Local cluster with Kind
make kind

# Or deploy with Helm
helm install sortie charts/sortie \
  --namespace sortie --create-namespace \
  --set ingress.host=sortie.example.com
```

### Docker

```bash
docker build -t sortie .
docker run -p 8080:8080 \
  -e SORTIE_JWT_SECRET=$(openssl rand -base64 32) \
  -e SORTIE_ADMIN_PASSWORD=changeme \
  sortie
```

## How It Works

```text
Browser ──▶ Sortie (Go server) ──▶ Kubernetes API
  │              │                       │
  │  static UI   │  REST API             │  creates pod +
  │  (React)     │  + WebSocket          │  VNC/RDP sidecar
  │              │                       │
  └──── noVNC / Guacamole stream ◀───────┘
```

1. User picks an app from the dashboard.
2. For **URL apps**, Sortie opens the link in a new tab.
3. For **container apps**, Sortie creates a Kubernetes pod with
   the app image and a VNC (Linux) or Guacamole/RDP (Windows)
   sidecar, then streams the desktop to the browser over
   WebSocket.
4. Sessions auto-expire after a configurable timeout
   (default 2 hours).

## Tech Stack

| Layer      | Technology |
|------------|-----------|
| Backend    | Go, `net/http`, SQLite |
| Frontend   | React, TypeScript, Tailwind CSS, Vite |
| Streaming  | noVNC (Linux), Apache Guacamole (Windows) |
| Orchestration | Kubernetes, Helm |
| Auth       | JWT (access + refresh tokens), bcrypt |

## Configuration

Copy `.env.example` and set the values for your environment.
Key options:

| Variable | Purpose | Default |
|----------|---------|---------|
| `SORTIE_PORT` | Server port | `8080` |
| `SORTIE_JWT_SECRET` | Signing key for auth tokens | *(required)* |
| `SORTIE_ADMIN_PASSWORD` | Initial admin password | *(required)* |
| `SORTIE_NAMESPACE` | Kubernetes namespace for pods | `sortie` |
| `SORTIE_SESSION_TIMEOUT` | Session lifetime | `120m` |

See `.env.example` for the full list.

## Project Layout

```text
main.go              Server entry point and HTTP routing
internal/
  config/            Configuration loading
  db/                SQLite database layer
  sessions/          Session lifecycle management
  k8s/               Kubernetes pod orchestration
  websocket/         VNC WebSocket proxy
  guacamole/         Guacamole WebSocket proxy
  middleware/        Auth and security middleware
  plugins/           Extensible plugin system
web/                 React frontend (Vite + TypeScript)
charts/sortie/    Helm chart
deploy/              Kubernetes manifests and Kind config
```

## Documentation

- [Development Guide](docs/DEVELOPMENT.md)
- [Kubernetes Deployment](docs/KUBERNETES.md)
- [Reverse Proxy Setup](docs/REVERSE_PROXY.md)
  (NGINX, Traefik, Caddy)
- [Data Persistence & Backup](docs/DATA_PERSISTENCE.md)
- [Disaster Recovery](docs/DISASTER_RECOVERY.md)
- [Plugin System](docs/PLUGIN_SYSTEM.md)
- [Template Marketplace](docs/TEMPLATES.md)
