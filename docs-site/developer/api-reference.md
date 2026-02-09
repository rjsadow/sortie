# API Reference

Sortie exposes a REST API for managing applications, sessions, templates, and users.
The full OpenAPI specification is available at `openapi.yaml` in the repository root.

## Authentication

All API requests (except `/api/auth/*` and `/api/config`) require a valid JWT token.

Include the token in the `Authorization` header:

```
Authorization: Bearer <token>
```

### Login

```
POST /api/auth/login
Content-Type: application/json

{"username": "admin", "password": "changeme"}
```

Returns access and refresh tokens.

### Refresh Token

```
POST /api/auth/refresh
Content-Type: application/json

{"refresh_token": "<refresh_token>"}
```

## Applications

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/apps` | List all applications |
| POST | `/api/apps` | Create application |
| GET | `/api/apps/:id` | Get application by ID |
| PUT | `/api/apps/:id` | Update application |
| DELETE | `/api/apps/:id` | Delete application |

## Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/sessions` | List sessions |
| POST | `/api/sessions` | Create session |
| GET | `/api/sessions/:id` | Get session by ID |
| DELETE | `/api/sessions/:id` | Terminate session |

## Templates

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/templates` | List all templates |
| GET | `/api/templates/:id` | Get template by ID |

## Admin Endpoints

These endpoints require the `admin` role.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/admin/users` | List users |
| GET | `/api/admin/sessions` | List all sessions (admin view) |
| GET/PUT | `/api/admin/settings` | Manage settings |
| GET | `/api/admin/templates` | Manage templates |
| GET | `/api/admin/diagnostics` | Download diagnostics bundle |
| GET | `/api/admin/health` | Detailed health check |

## WebSocket Endpoints

| Path | Protocol | Description |
|------|----------|-------------|
| `/ws/sessions/:id` | VNC (binary WebSocket) | Linux desktop streaming |
| `/ws/guac/sessions/:id` | Guacamole (text WebSocket) | Windows desktop streaming |

WebSocket connections require JWT authentication via query parameter (`?token=<jwt>`),
cookie, or Authorization header.

## Observability

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check |
| GET | `/api/load` | Current load status |
| GET | `/debug/vars` | expvar metrics |
