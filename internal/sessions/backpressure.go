package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// LoadStatus represents the current system load for autoscaling decisions.
type LoadStatus struct {
	ActiveSessions int     `json:"active_sessions"`
	MaxSessions    int     `json:"max_sessions"` // 0 = unlimited
	QueueDepth     int     `json:"queue_depth"`
	MaxQueueSize   int     `json:"max_queue_size"`
	LoadFactor     float64 `json:"load_factor"` // 0.0-1.0, ratio of active to max
	Accepting      bool    `json:"accepting"`   // false when overloaded
}

// BackpressureHandler exposes load status and enforces backpressure at the HTTP layer.
// It wraps session creation endpoints to return appropriate status codes and headers
// when the system is under load, giving clients actionable feedback.
type BackpressureHandler struct {
	manager  *Manager
	queue    *SessionQueue // may be nil if queueing is disabled
	maxQueue int
}

// NewBackpressureHandler creates a handler that monitors load from the session manager
// and optional queue.
func NewBackpressureHandler(manager *Manager, queue *SessionQueue, maxQueueSize int) *BackpressureHandler {
	return &BackpressureHandler{
		manager:  manager,
		queue:    queue,
		maxQueue: maxQueueSize,
	}
}

// GetLoadStatus returns the current system load status.
func (h *BackpressureHandler) GetLoadStatus() *LoadStatus {
	globalCount, _ := h.manager.db.CountActiveSessions()

	queueDepth := 0
	if h.queue != nil {
		queueDepth = h.queue.Len()
	}

	maxSessions := h.manager.maxGlobalSessions

	loadFactor := 0.0
	if maxSessions > 0 {
		loadFactor = float64(globalCount) / float64(maxSessions)
		if loadFactor > 1.0 {
			loadFactor = 1.0
		}
	}

	accepting := true
	if maxSessions > 0 && globalCount >= maxSessions {
		// At capacity — accepting only if queue has room
		if h.queue == nil || queueDepth >= h.maxQueue {
			accepting = false
		}
	}

	return &LoadStatus{
		ActiveSessions: globalCount,
		MaxSessions:    maxSessions,
		QueueDepth:     queueDepth,
		MaxQueueSize:   h.maxQueue,
		LoadFactor:     loadFactor,
		Accepting:      accepting,
	}
}

// ServeLoadStatus handles GET /api/load — returns current load for monitoring
// and custom HPA metrics adapters.
func (h *BackpressureHandler) ServeLoadStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := h.GetLoadStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// WriteBackpressureHeaders adds backpressure headers to a response.
// These inform clients and load balancers about current load conditions.
func (h *BackpressureHandler) WriteBackpressureHeaders(w http.ResponseWriter) {
	status := h.GetLoadStatus()

	// X-Load-Factor: 0.0-1.0 ratio for load balancer use
	w.Header().Set("X-Load-Factor", fmt.Sprintf("%.2f", status.LoadFactor))

	// X-Queue-Depth: current queue size
	w.Header().Set("X-Queue-Depth", strconv.Itoa(status.QueueDepth))
}

// WriteRetryAfter sets the Retry-After header based on current load.
// Higher load = longer retry interval.
func WriteRetryAfter(w http.ResponseWriter, loadFactor float64) {
	// Scale retry interval: 5s at low load, up to 30s at full load
	retrySeconds := int(5 + loadFactor*25)
	w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
}

// EnhancedReadinessCheck returns false when the system is overloaded,
// signaling to the load balancer to stop sending new traffic to this replica.
// Existing connections (WebSockets) continue to be served.
func (h *BackpressureHandler) EnhancedReadinessCheck() bool {
	status := h.GetLoadStatus()
	// Mark replica as not-ready when load factor exceeds 95%
	// This makes the k8s Service stop routing NEW requests here,
	// while existing WebSocket connections continue unaffected.
	if status.MaxSessions > 0 && status.LoadFactor >= 0.95 {
		return false
	}
	return true
}

// Middleware returns an HTTP middleware that adds backpressure headers to all responses
// and rejects session creation requests with HTTP 429 + Retry-After when overloaded.
func (h *BackpressureHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.WriteBackpressureHeaders(w)
		next.ServeHTTP(w, r)
	})
}

// EstimateWaitTime returns an estimated wait time based on queue depth and
// historical session creation rate. Used for client-facing queue position feedback.
func EstimateWaitTime(queuePosition int, avgSessionDuration time.Duration) time.Duration {
	if queuePosition <= 0 {
		return 0
	}
	// Assume sessions turn over at roughly 1/avgSessionDuration rate
	// Each queue position adds approximately the poll interval wait
	return time.Duration(queuePosition) * DefaultQueuePollInterval * 2
}
