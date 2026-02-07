package files

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_PathParsing(t *testing.T) {
	// Test that the handler correctly parses session IDs and actions from paths
	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "missing session ID",
			path:       "/api/sessions/",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid path - no files segment",
			path:       "/api/sessions/abc/notfiles",
			wantStatus: http.StatusBadRequest,
		},
	}

	// Create a handler with nil dependencies - requests should fail before
	// hitting the session manager due to path validation
	h := &Handler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}
