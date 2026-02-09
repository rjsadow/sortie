package websocket

import (
	"log"
	"net/http"
	"strings"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/sessions"
)

// Handler handles WebSocket connections for sessions
type Handler struct {
	sessionManager *sessions.Manager
}

// NewHandler creates a new WebSocket handler
func NewHandler(sessionManager *sessions.Manager) *Handler {
	return &Handler{
		sessionManager: sessionManager,
	}
}

// ServeHTTP handles WebSocket upgrade requests for session connections
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /ws/sessions/{id}
	path := strings.TrimPrefix(r.URL.Path, "/ws/sessions/")
	sessionID := strings.TrimSuffix(path, "/")

	if sessionID == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	// Get the session
	session, err := h.sessionManager.GetSession(r.Context(), sessionID)
	if err != nil {
		log.Printf("Error getting session %s: %v", sessionID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Check session status
	if session.Status != db.SessionStatusRunning {
		http.Error(w, "Session is not running", http.StatusBadRequest)
		return
	}

	if session.PodIP == "" {
		http.Error(w, "Session pod IP not available", http.StatusServiceUnavailable)
		return
	}

	// Create proxy to the VNC sidecar's websockify endpoint
	targetURL := h.sessionManager.GetPodWebSocketEndpoint(session)
	if targetURL == "" {
		http.Error(w, "Unable to determine WebSocket endpoint", http.StatusServiceUnavailable)
		return
	}

	log.Printf("Proxying WebSocket for session %s to %s", sessionID, targetURL)

	// Create and serve the proxy
	proxy := NewProxy(targetURL)
	proxy.ServeHTTP(w, r)
}
