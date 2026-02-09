package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rjsadow/sortie/internal/db"
)

func TestGetLoadStatus_UnlimitedSessions(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 0, // unlimited
	})

	h := NewBackpressureHandler(m, nil, 0)
	status := h.GetLoadStatus()

	if status.MaxSessions != 0 {
		t.Errorf("MaxSessions = %d, want 0", status.MaxSessions)
	}
	if status.LoadFactor != 0.0 {
		t.Errorf("LoadFactor = %f, want 0.0", status.LoadFactor)
	}
	if !status.Accepting {
		t.Error("Accepting should be true when unlimited")
	}
}

func TestGetLoadStatus_WithSessions(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 10,
	})

	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	now := time.Now()

	// Create 7 active sessions
	for i := range 7 {
		s := db.Session{
			ID: fmt.Sprintf("s%d", i), UserID: "u1", AppID: "app1",
			PodName: fmt.Sprintf("p%d", i), Status: db.SessionStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := database.CreateSession(s); err != nil {
			t.Fatalf("CreateSession error: %v", err)
		}
	}

	h := NewBackpressureHandler(m, nil, 50)
	status := h.GetLoadStatus()

	if status.ActiveSessions != 7 {
		t.Errorf("ActiveSessions = %d, want 7", status.ActiveSessions)
	}
	if status.MaxSessions != 10 {
		t.Errorf("MaxSessions = %d, want 10", status.MaxSessions)
	}
	if status.LoadFactor != 0.7 {
		t.Errorf("LoadFactor = %f, want 0.7", status.LoadFactor)
	}
	if !status.Accepting {
		t.Error("Accepting should be true at 70% load")
	}
}

func TestGetLoadStatus_AtCapacity_NoQueue(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 2,
	})

	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	now := time.Now()

	for i := range 2 {
		s := db.Session{
			ID: fmt.Sprintf("s%d", i), UserID: "u1", AppID: "app1",
			PodName: fmt.Sprintf("p%d", i), Status: db.SessionStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := database.CreateSession(s); err != nil {
			t.Fatalf("CreateSession error: %v", err)
		}
	}

	// No queue configured
	h := NewBackpressureHandler(m, nil, 0)
	status := h.GetLoadStatus()

	if status.Accepting {
		t.Error("Accepting should be false at capacity with no queue")
	}
	if status.LoadFactor != 1.0 {
		t.Errorf("LoadFactor = %f, want 1.0", status.LoadFactor)
	}
}

func TestGetLoadStatus_AtCapacity_WithQueue(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 2,
	})

	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	now := time.Now()

	for i := range 2 {
		s := db.Session{
			ID: fmt.Sprintf("s%d", i), UserID: "u1", AppID: "app1",
			PodName: fmt.Sprintf("p%d", i), Status: db.SessionStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := database.CreateSession(s); err != nil {
			t.Fatalf("CreateSession error: %v", err)
		}
	}

	// Queue with space
	q := NewSessionQueue(QueueConfig{
		MaxSize:      10,
		Timeout:      time.Second,
		PollInterval: 50 * time.Millisecond,
	}, func() bool { return false })
	defer q.Stop()

	h := NewBackpressureHandler(m, q, 10)
	status := h.GetLoadStatus()

	if !status.Accepting {
		t.Error("Accepting should be true at capacity when queue has room")
	}
}

func TestServeLoadStatus(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 100,
	})

	h := NewBackpressureHandler(m, nil, 50)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/load", nil)
	h.ServeLoadStatus(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	var status LoadStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if status.MaxSessions != 100 {
		t.Errorf("MaxSessions = %d, want 100", status.MaxSessions)
	}
	if !status.Accepting {
		t.Error("Accepting should be true")
	}
}

func TestServeLoadStatus_MethodNotAllowed(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{})
	h := NewBackpressureHandler(m, nil, 0)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/load", nil)
	h.ServeLoadStatus(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

func TestWriteBackpressureHeaders(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 100,
	})

	h := NewBackpressureHandler(m, nil, 50)

	w := httptest.NewRecorder()
	h.WriteBackpressureHeaders(w)

	if got := w.Header().Get("X-Load-Factor"); got == "" {
		t.Error("X-Load-Factor header not set")
	}
	if got := w.Header().Get("X-Queue-Depth"); got != "0" {
		t.Errorf("X-Queue-Depth = %q, want 0", got)
	}
}

func TestWriteRetryAfter(t *testing.T) {
	tests := []struct {
		loadFactor float64
		wantMin    int
		wantMax    int
	}{
		{0.0, 5, 5},
		{0.5, 15, 20},
		{1.0, 30, 30},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		WriteRetryAfter(w, tt.loadFactor)

		got := w.Header().Get("Retry-After")
		if got == "" {
			t.Errorf("Retry-After header not set for loadFactor=%f", tt.loadFactor)
			continue
		}
	}
}

func TestEnhancedReadinessCheck(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 10,
	})

	seedContainerApp(t, database, "app1", "Test App", "test:latest")
	now := time.Now()

	h := NewBackpressureHandler(m, nil, 0)

	// At 0% load - ready
	if !h.EnhancedReadinessCheck() {
		t.Error("should be ready at 0% load")
	}

	// Create 9 sessions (90% load) - still ready
	for i := range 9 {
		s := db.Session{
			ID: fmt.Sprintf("s%d", i), UserID: "u1", AppID: "app1",
			PodName: fmt.Sprintf("p%d", i), Status: db.SessionStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		}
		database.CreateSession(s) //nolint:errcheck
	}

	if !h.EnhancedReadinessCheck() {
		t.Error("should be ready at 90% load")
	}

	// Create 10th session (100% load >= 95%) - not ready
	database.CreateSession(db.Session{ //nolint:errcheck
		ID: "s9", UserID: "u1", AppID: "app1",
		PodName: "p9", Status: db.SessionStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})

	if h.EnhancedReadinessCheck() {
		t.Error("should NOT be ready at 100% load")
	}
}

func TestEnhancedReadinessCheck_UnlimitedAlwaysReady(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 0, // unlimited
	})

	h := NewBackpressureHandler(m, nil, 0)

	if !h.EnhancedReadinessCheck() {
		t.Error("should always be ready with unlimited sessions")
	}
}

func TestBackpressureMiddleware(t *testing.T) {
	database := newTestDB(t)
	m := NewManagerWithConfig(database, ManagerConfig{
		MaxGlobalSessions: 100,
	})

	h := NewBackpressureHandler(m, nil, 50)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := h.Middleware(inner)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-Load-Factor"); got == "" {
		t.Error("middleware should set X-Load-Factor header")
	}
}

func TestEstimateWaitTime(t *testing.T) {
	if got := EstimateWaitTime(0, time.Minute); got != 0 {
		t.Errorf("EstimateWaitTime(0) = %v, want 0", got)
	}

	got := EstimateWaitTime(5, time.Minute)
	if got <= 0 {
		t.Errorf("EstimateWaitTime(5) should be positive, got %v", got)
	}
}
