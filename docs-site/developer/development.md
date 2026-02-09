# Local Development Guide

This guide covers setting up and running Sortie for local development.

## Prerequisites

| Tool    | Version | Check Command       |
|---------|---------|---------------------|
| Go      | 1.24+   | `go version`        |
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

| Service  | Port | URL                       | Description                    |
|----------|------|---------------------------|--------------------------------|
| Frontend | 5173 | `http://localhost:5173`   | Vite dev server with HMR       |
| Backend  | 8080 | `http://localhost:8080`   | Go API server                  |
| Database | -    | `./sortie.db`          | SQLite file (created on start) |

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

Sortie uses SQLite for persistence. The database file (`sortie.db`)
is created automatically on first run in the current directory.

### Seeding Data

```bash
./sortie -seed apps.json
```

Or during development, the sample apps are loaded automatically.

### Reset Database

```bash
rm sortie.db
make dev
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

Reset the database:

```bash
rm sortie.db
```

The database is recreated on next server start.
