# Local Development Guide

This guide covers setting up and running Sortie for local development.

## Prerequisites

| Tool    | Version | Check Command       |
|---------|---------|---------------------|
| Go      | 1.25+   | `go version`        |
| Node.js | 22+     | `node --version`    |
| npm     | 10+     | `npm --version`     |
| Make    | any     | `make --version`    |

## Quick Start

```bash
# Clone the repository
git clone https://github.com/rjsadow/sortie.git
cd sortie

# Start the development environment
make dev
```

This starts both the backend and frontend servers with a single command.

## Port Reference

| Service  | Port | URL                       | Description                        |
|----------|------|---------------------------|------------------------------------|
| Frontend | 5173 | `http://localhost:5173`   | Vite dev server with HMR           |
| Backend  | 8080 | `http://localhost:8080`   | Go API server                      |
| Database | -    | `./sortie.db`          | SQLite file (default) or PostgreSQL |

**Use `http://localhost:5173`** for development. The Vite dev server proxies
API requests (`/api/*`, `/ws/*`, `/apps.json`) to the backend automatically.

## Development Commands

### Start Development Environment

```bash
make dev
```

Starts both frontend (Vite with HMR) and backend (Go) servers. Output shows
both server logs. Press `Ctrl+C` to stop all servers.

### Run Servers Separately

```bash
# Terminal 1: Backend only
make dev-backend

# Terminal 2: Frontend only
make dev-frontend
```

Useful when you need to restart one server without affecting the other.

### Build for Production

```bash
make build
```

Builds the frontend, then compiles the Go binary with embedded assets.
Output: `./sortie`

### Run Production Build

```bash
make run
```

Builds and runs the production server on port 8080.

### Other Commands

```bash
make deps      # Install frontend dependencies
make frontend  # Build frontend only
make backend   # Build Go binary (requires frontend built first)
make lint      # Run frontend linter
make test      # Run Go tests
make clean     # Remove build artifacts and database
```

## Project Structure

```text
sortie/
├── main.go              # Go server entry point
├── internal/            # Go packages
│   ├── db/              # Database operations
│   ├── sessions/        # Session management
│   └── websocket/       # WebSocket handler
├── web/                 # Frontend (React + Vite)
│   ├── src/             # React components
│   ├── public/          # Static assets
│   ├── dist/            # Build output (generated)
│   └── package.json     # Frontend dependencies
├── docs/                # Documentation
├── Makefile             # Build automation
└── apps.json            # Sample application data
```

## Database

Sortie supports **SQLite** (default) and **PostgreSQL** as database backends.

### SQLite (default)

The database file (`sortie.db`) is created automatically on first run:

```bash
make dev   # uses SQLite by default
```

Reset: `rm sortie.db && make dev`

### PostgreSQL (local development)

Start a local Postgres instance and point Sortie at it:

```bash
# Start Postgres via Docker
docker run --rm -d --name sortie-pg \
  -e POSTGRES_USER=sortie -e POSTGRES_PASSWORD=sortie -e POSTGRES_DB=sortie \
  -p 5432:5432 postgres:16

# Run Sortie with Postgres
export SORTIE_DB_TYPE=postgres
export SORTIE_DB_DSN='postgres://sortie:sortie@localhost:5432/sortie?sslmode=disable'
make dev
```

Migrations run automatically on startup for both backends.

### Seeding Data

```bash
./sortie -seed apps.json
```

Or during development, the sample apps are loaded automatically.

## Testing

### Go Tests

```bash
make test               # Unit tests (SQLite, excludes /tests/ directory)
make test-integration   # API integration tests with mock runner
make test-all           # Unit + integration combined
```

### Running Tests Against PostgreSQL

All Go tests support dual-backend execution. Set environment variables
to run against Postgres instead of SQLite:

```bash
# Start a local Postgres (if not already running)
docker run --rm -d --name sortie-test-pg \
  -e POSTGRES_USER=sortie_test -e POSTGRES_PASSWORD=sortie_test \
  -e POSTGRES_DB=sortie_test -p 5432:5432 postgres:16

# Run unit tests against Postgres
SORTIE_TEST_DB_TYPE=postgres \
SORTIE_TEST_POSTGRES_DSN='postgres://sortie_test:sortie_test@localhost:5432/sortie_test?sslmode=disable' \
  go test -v -race -p 1 -count=1 ./internal/db/...

# Run integration tests against Postgres
SORTIE_TEST_DB_TYPE=postgres \
SORTIE_TEST_POSTGRES_DSN='postgres://sortie_test:sortie_test@localhost:5432/sortie_test?sslmode=disable' \
  go test -v -race -p 1 -count=1 -timeout 5m ./tests/integration/...
```

The `-p 1` flag is required for Postgres tests since they share a
single database instance and must run serially.

Without `SORTIE_TEST_POSTGRES_DSN`, all Postgres-specific tests
automatically skip.

### Playwright E2E Tests

```bash
make test-playwright    # Browser E2E tests (port 3847, mock runner)
```

### Linting

```bash
make lint                           # Frontend ESLint
npx --prefix web tsc -b --noEmit    # TypeScript type checking
golangci-lint run ./...             # Go linting
```

## API Endpoints

| Method | Endpoint              | Description              |
|--------|-----------------------|--------------------------|
| GET    | /api/apps             | List all applications    |
| POST   | /api/apps             | Create application       |
| GET    | /api/apps/:id         | Get application by ID    |
| PUT    | /api/apps/:id         | Update application       |
| DELETE | /api/apps/:id         | Delete application       |
| GET    | /api/audit            | Get audit logs           |
| POST   | /api/analytics/launch | Record app launch        |
| GET    | /api/analytics/stats  | Get analytics stats      |
| GET    | /api/config           | Get branding config      |
| GET    | /api/sessions         | List sessions            |
| POST   | /api/sessions         | Create session           |
| DELETE | /api/sessions/:id     | Terminate session        |

## Troubleshooting

### "web/dist/*: no matching files found"

This error occurs when Go tries to embed frontend assets before they exist.

**Fix:** Run `make dev` or `make frontend` to build the frontend first.

### Port already in use

Check what's using the port:

```bash
lsof -i :8080  # Check backend port
lsof -i :5173  # Check frontend port
```

Kill the process or use a different port.

### Frontend changes not reflecting

The Vite dev server supports hot module replacement (HMR). If changes
don't appear:

1. Check the terminal for errors
2. Hard refresh the browser (`Ctrl+Shift+R`)
3. Restart `make dev`

### Database issues

For SQLite, reset by deleting the file:

```bash
rm sortie.db
```

For PostgreSQL, drop and recreate the database:

```bash
dropdb -h localhost -U sortie_test sortie_test
createdb -h localhost -U sortie_test sortie_test
```

The schema is recreated automatically on next server start.
