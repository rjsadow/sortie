package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rjsadow/sortie/internal/middleware"
	"github.com/rjsadow/sortie/internal/plugins"
	"github.com/rjsadow/sortie/internal/sessions"
)

// Authenticator validates a JWT token and returns the authenticated user.
// This is satisfied by *auth.JWTAuthProvider (via plugins.AuthProvider).
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*plugins.AuthResult, error)
}

const (
	// clientBufSize is the per-client event channel buffer.
	// If the client falls behind, events are dropped (caught up via polling).
	clientBufSize = 32

	// heartbeatInterval keeps the connection alive through proxies.
	heartbeatInterval = 30 * time.Second
)

// sseEvent is the payload written to each client's channel.
type sseEvent struct {
	Event string // SSE event type (e.g. "session")
	Data  []byte // JSON-encoded payload
}

// client represents a single connected EventSource.
type client struct {
	userID string
	ch     chan sseEvent
}

// Hub implements both sessions.SessionRecorder (receives lifecycle events)
// and http.Handler (serves the SSE endpoint). It fans out events to
// connected browsers filtered by user ID.
type Hub struct {
	authProvider Authenticator

	mu      sync.RWMutex
	clients map[*client]struct{}
}

// NewHub creates an SSE hub.
func NewHub(authProvider Authenticator) *Hub {
	return &Hub{
		authProvider: authProvider,
		clients:      make(map[*client]struct{}),
	}
}

// OnEvent implements sessions.SessionRecorder. It encodes the event as JSON
// and performs a non-blocking fan-out to every client whose userID matches.
func (h *Hub) OnEvent(_ context.Context, event sessions.SessionEventData) {
	payload := struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Status    string `json:"status"`
	}{
		Type:      string(event.Event),
		SessionID: event.SessionID,
		Status:    eventToStatus(event.Event),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("sse: failed to marshal event", "error", err)
		return
	}

	msg := sseEvent{Event: "session", Data: data}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c.userID != event.UserID {
			continue
		}
		select {
		case c.ch <- msg:
		default:
			// Client buffer full — skip, they'll catch up via polling.
		}
	}
}

// ServeHTTP serves the GET /api/sessions/events SSE endpoint.
// Authentication uses the same triple-fallback as the gateway:
// query param "token", cookie, or Authorization header.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.authenticate(r)
	if err != nil || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx proxy buffering

	// Register client.
	c := &client{
		userID: user.ID,
		ch:     make(chan sseEvent, clientBufSize),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()

	// Send connected event so the frontend knows SSE is live.
	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// authenticate extracts and validates a JWT from the request.
// Same triple-fallback as gateway: query param, cookie, Authorization header.
func (h *Hub) authenticate(r *http.Request) (*plugins.User, error) {
	token := ""

	if t := r.URL.Query().Get("token"); t != "" {
		token = t
	}

	if token == "" {
		if c, err := r.Cookie(middleware.AccessTokenCookieName); err == nil {
			token = c.Value
		}
	}

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

// eventToStatus maps a session lifecycle event to a human-readable status
// string for the SSE payload.
func eventToStatus(event sessions.SessionEvent) string {
	switch event {
	case sessions.EventSessionCreated:
		return "creating"
	case sessions.EventSessionReady:
		return "running"
	case sessions.EventSessionFailed:
		return "failed"
	case sessions.EventSessionStopped:
		return "stopped"
	case sessions.EventSessionRestarted:
		return "creating"
	case sessions.EventSessionExpired:
		return "expired"
	case sessions.EventSessionTerminated:
		return "stopped"
	default:
		return "unknown"
	}
}

// ClientCount returns the number of connected SSE clients (for testing/diagnostics).
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
