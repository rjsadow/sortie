package guacamole

import (
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/sessions"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"guacamole"},
}

// Handler handles Guacamole WebSocket connections for Windows RDP sessions
type Handler struct {
	sessionManager *sessions.Manager
}

// NewHandler creates a new Guacamole WebSocket handler
func NewHandler(sessionManager *sessions.Manager) *Handler {
	return &Handler{
		sessionManager: sessionManager,
	}
}

// ServeHTTP handles WebSocket upgrade requests for Guacamole sessions
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /ws/guac/sessions/{id}
	path := strings.TrimPrefix(r.URL.Path, "/ws/guac/sessions/")
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

	if session.Status != db.SessionStatusRunning {
		http.Error(w, "Session is not running", http.StatusBadRequest)
		return
	}

	if session.PodIP == "" {
		http.Error(w, "Session pod IP not available", http.StatusServiceUnavailable)
		return
	}

	// Upgrade to WebSocket
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade Guacamole WebSocket for session %s: %v", sessionID, err)
		return
	}
	defer clientConn.Close()

	// Get screen dimensions from query params (optional, fallback to defaults)
	width := r.URL.Query().Get("width")
	if width == "" {
		width = "1920"
	}
	height := r.URL.Query().Get("height")
	if height == "" {
		height = "1080"
	}

	// Create guacd proxy
	// guacd runs as a sidecar in the same pod, accessible via pod IP on port 4822
	// The RDP server runs in the app container, accessible via localhost:3389 from guacd's perspective
	guacdAddr := session.PodIP + ":4822"
	proxy := NewGuacdProxy(guacdAddr, "127.0.0.1", "3389", "testuser", "testpass", width, height)

	log.Printf("Starting Guacamole proxy for session %s to guacd at %s", sessionID, guacdAddr)

	if err := proxy.Serve(clientConn); err != nil {
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
			log.Printf("Guacamole proxy error for session %s: %v", sessionID, err)
		}
	}
}
