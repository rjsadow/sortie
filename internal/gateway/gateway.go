package gateway

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/guacamole"
	"github.com/rjsadow/launchpad/internal/middleware"
	"github.com/rjsadow/launchpad/internal/plugins"
	"github.com/rjsadow/launchpad/internal/sessions"
	"github.com/rjsadow/launchpad/internal/websocket"
)

// Handler is the gateway entry point for all WebSocket stream connections.
// It authenticates the caller, enforces rate limits, validates session
// ownership, and delegates to the appropriate backend proxy.
type Handler struct {
	sessionManager *sessions.Manager
	authProvider   plugins.AuthProvider
	database       *db.DB
	limiter        *RateLimiter
	vncHandler     *websocket.Handler
	guacHandler    *guacamole.Handler
}

// Config holds configuration for the gateway handler.
type Config struct {
	SessionManager *sessions.Manager
	AuthProvider   plugins.AuthProvider
	Database       *db.DB
	RateLimiter    *RateLimiter
}

// NewHandler creates a new gateway handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{
		sessionManager: cfg.SessionManager,
		authProvider:   cfg.AuthProvider,
		database:       cfg.Database,
		limiter:        cfg.RateLimiter,
		vncHandler:     websocket.NewHandler(cfg.SessionManager),
		guacHandler:    guacamole.NewHandler(cfg.SessionManager),
	}
}

// ServeHTTP routes incoming WebSocket requests through auth and rate limiting
// before delegating to the appropriate stream proxy.
//
// Routes:
//
//	/ws/sessions/{id}      -> VNC proxy
//	/ws/guac/sessions/{id} -> Guacamole (RDP) proxy
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// --- Rate limiting ---
	if h.limiter != nil && !h.limiter.Allow(clientIP(r)) {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// --- Authentication ---
	user, err := h.authenticate(r)
	if err != nil || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// --- Extract session ID and determine backend ---
	sessionID, backend := h.parseRoute(r.URL.Path)
	if sessionID == "" || backend == "" {
		http.Error(w, "Invalid gateway path", http.StatusBadRequest)
		return
	}

	// --- Session validation & ownership ---
	session, err := h.sessionManager.GetSession(r.Context(), sessionID)
	if err != nil {
		slog.Error("gateway: error fetching session", "session_id", sessionID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if session.Status != db.SessionStatusRunning {
		http.Error(w, "Session is not running", http.StatusBadRequest)
		return
	}

	// Enforce ownership: non-admin users can only access their own sessions
	if !middleware.HasRole(user.Roles, middleware.RoleAdmin) && session.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// --- Audit ---
	h.database.LogAudit(user.Username, "GATEWAY_CONNECT", "session="+sessionID+" backend="+backend)

	// --- Delegate to backend ---
	switch backend {
	case "vnc":
		h.vncHandler.ServeHTTP(w, r)
	case "guac":
		h.guacHandler.ServeHTTP(w, r)
	default:
		http.Error(w, "Unknown backend", http.StatusBadRequest)
	}
}

// authenticate extracts and validates a JWT from the request.
// WebSocket clients cannot set custom headers, so the token is accepted from:
//  1. query parameter "token"
//  2. cookie (launchpad_access_token)
//  3. Authorization header (for non-browser clients)
func (h *Handler) authenticate(r *http.Request) (*plugins.User, error) {
	token := ""

	// 1. Query parameter (typical for browser WebSocket connections)
	if t := r.URL.Query().Get("token"); t != "" {
		token = t
	}

	// 2. Cookie
	if token == "" {
		if c, err := r.Cookie(middleware.AccessTokenCookieName); err == nil {
			token = c.Value
		}
	}

	// 3. Authorization header
	if token == "" {
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = parts[1]
			}
		}
	}

	if token == "" || h.authProvider == nil {
		return nil, nil
	}

	result, err := h.authProvider.Authenticate(r.Context(), token)
	if err != nil {
		return nil, err
	}
	if !result.Authenticated || result.User == nil {
		return nil, nil
	}
	return result.User, nil
}

// parseRoute determines the session ID and backend type from the URL path.
func (h *Handler) parseRoute(path string) (sessionID, backend string) {
	// /ws/guac/sessions/{id}
	if strings.HasPrefix(path, "/ws/guac/sessions/") {
		id := strings.TrimPrefix(path, "/ws/guac/sessions/")
		id = strings.TrimSuffix(id, "/")
		if id != "" {
			return id, "guac"
		}
	}

	// /ws/sessions/{id}
	if strings.HasPrefix(path, "/ws/sessions/") {
		id := strings.TrimPrefix(path, "/ws/sessions/")
		id = strings.TrimSuffix(id, "/")
		if id != "" {
			return id, "vnc"
		}
	}

	return "", ""
}
