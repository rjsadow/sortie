# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code)
when working with code in this repository.

## Project Overview

Sortie is a web-based application launcher that runs
containerized desktop environments via Kubernetes. Users
launch apps that become pods with VNC/RDP/browser sidecars,
accessed through the browser. Go backend with embedded React
frontend, SQLite database.

Go module path: `github.com/rjsadow/sortie`

## Build Commands

```bash
make build              # Full production build (frontend → docs → Go binary)
make frontend           # Build React frontend to web/dist
make backend            # Build Go binary (requires frontend + docs built first)
make docs               # Build VitePress docs to docs-site/dist
make dev                # Run frontend (Vite HMR :5173) + backend (:8080) concurrently
make clean              # Remove all build artifacts and databases
```

Frontend commands use `npm --prefix web` since shell `cd`
doesn't persist between Bash tool calls.

## Testing

```bash
make test               # Go unit tests (excludes /tests/ directory)
make test-integration   # API integration tests with mock runner (no K8s needed)
make test-all           # Unit + integration combined
make test-playwright    # Playwright browser E2E tests (port 3847, mock runner)
make test-e2e           # Full E2E against live Kind cluster

# Single test examples
go test -v -run TestSessionFromDB ./internal/sessions/
go test -v -run TestAdmin_AutoRecordSetting ./tests/integration/
npx --prefix web playwright test web/e2e/auth.spec.ts
```

Integration tests (`tests/integration/`) use
`testutil.NewTestServer(t)` which spins up a full HTTP
server with mock runner and temp SQLite database. Playwright
E2E tests run against a real server binary with
`--mock-runner` flag.

## Linting

```bash
make lint                           # Frontend ESLint
npx --prefix web tsc -b --noEmit    # TypeScript type checking
golangci-lint run ./...             # Go linting (standard linters, errcheck disabled)
```

## Architecture

### Backend

The Go binary embeds the compiled frontend
(`//go:embed all:web/dist`) and docs site
(`//go:embed all:docs-site/dist`), serving everything from
a single binary. Entry point is `main.go`.

**Three app launch types** determine pod architecture:

- `url` — opens an external URL (no pod)
- `container` — spawns a pod with VNC sidecar (Linux) or
  guacd+xrdp sidecars (Windows, `os_type: "windows"`)
- `web_proxy` — spawns a pod with a headless browser
  sidecar, proxied through the backend

**Key packages:**

- `internal/server/` — HTTP handler composition, all route
  handlers in `handlers.go`
- `internal/sessions/` — Session lifecycle (manager, state
  machine, queue, backpressure)
- `internal/k8s/` — Pod spec builders (`BuildPodSpec`,
  `BuildWebProxyPodSpec`, `BuildWindowsPodSpec`)
- `internal/runner/` — Pluggable workload backend
  (Kubernetes, mock for testing)
- `internal/db/` — SQLite schema, CRUD operations,
  migrations
- `internal/recordings/` — Session video recording
  (handler, storage)
- `internal/plugins/auth/` — Auth providers (JWT local,
  OIDC/SSO, noop for testing)
- `internal/middleware/` — Auth, tenant, RBAC, rate
  limiting, security headers

**Routing:** Public routes (health, auth, config) then
JWT-protected routes (apps, sessions, admin) then SPA
catch-all. Admin routes use `requireAdmin()` middleware.

**Database:** SQLite via `modernc.org/sqlite` (pure Go, no
CGO). Migrations in `/migrations/` use
`ALTER TABLE ADD COLUMN` with error suppression for
idempotency. Admin settings use a generic key-value store
(`GetSetting`/`SetSetting`).

### Frontend

React 19 + TypeScript + Vite + Tailwind CSS. In
development, Vite proxies API calls to the backend.

- `web/src/App.tsx` — Main component, routing, auth state
- `web/src/components/Admin.tsx` — Admin panel (settings,
  users, categories, apps, templates, sessions, recordings)
- `web/src/services/auth.ts` — API client functions with
  JWT token management
- `web/src/types.ts` — TypeScript interfaces matching
  backend JSON responses
- `web/src/components/SessionViewer.tsx` — Container
  session viewer (switches between VNC and Guacamole)

Libraries `noVNC` and `guacamole-common-js` lack TypeScript
types — custom `.d.ts` files are in `web/src/`.

### Key Patterns

- `SessionFromDB()` converts DB session records to API
  responses — takes `wsURL`, `guacURL`, `proxyURL`,
  `recordingPolicy` params
- Sidecar images configured via `k8s.Configure*()`
  functions and `SORTIE_*_SIDECAR_IMAGE` env vars
- Session manager routes pod creation by `app.OsType` and
  `app.LaunchType`
- Frontend modals follow the same pattern (fixed overlay,
  max-width container) — see `RecordingsList`, `AuditLog`,
  `SessionManager`
- Frontend dynamically imports viewer libraries to avoid
  bundling unused code

## Configuration

Primary configuration is via environment variables (see
`.env.example` for full list). Key ones:

- `SORTIE_PORT` (default 8080), `SORTIE_DB`
  (default sortie.db), `SORTIE_JWT_SECRET`
  (required, min 32 chars)
- `SORTIE_NAMESPACE` — Kubernetes namespace for session
  pods
- `SORTIE_VIDEO_RECORDING_ENABLED` — enables recording
  routes and handler
- `SORTIE_OIDC_*` — SSO/OIDC provider configuration

Helm chart in `charts/sortie/` for Kubernetes deployment.
Local development uses `make kind` for Kind cluster setup.
