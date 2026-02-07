package sessions

import (
	"testing"

	"github.com/rjsadow/launchpad/internal/db"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     db.SessionStatus
		to       db.SessionStatus
		expected bool
	}{
		// Valid transitions from creating
		{"creating to running", db.SessionStatusCreating, db.SessionStatusRunning, true},
		{"creating to failed", db.SessionStatusCreating, db.SessionStatusFailed, true},

		// Valid transitions from running
		{"running to stopped", db.SessionStatusRunning, db.SessionStatusStopped, true},
		{"running to expired", db.SessionStatusRunning, db.SessionStatusExpired, true},
		{"running to failed", db.SessionStatusRunning, db.SessionStatusFailed, true},

		// Invalid transitions from creating
		{"creating to stopped", db.SessionStatusCreating, db.SessionStatusStopped, false},
		{"creating to expired", db.SessionStatusCreating, db.SessionStatusExpired, false},

		// Invalid transitions from running
		{"running to creating", db.SessionStatusRunning, db.SessionStatusCreating, false},

		// Stopped sessions can be restarted
		{"stopped to creating", db.SessionStatusStopped, db.SessionStatusCreating, true},
		{"stopped to running", db.SessionStatusStopped, db.SessionStatusRunning, false},

		// No transitions from terminal states
		{"expired to running", db.SessionStatusExpired, db.SessionStatusRunning, false},
		{"expired to creating", db.SessionStatusExpired, db.SessionStatusCreating, false},
		{"failed to running", db.SessionStatusFailed, db.SessionStatusRunning, false},
		{"failed to creating", db.SessionStatusFailed, db.SessionStatusCreating, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanTransition(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		status   db.SessionStatus
		expected bool
	}{
		{db.SessionStatusCreating, false},
		{db.SessionStatusRunning, false},
		{db.SessionStatusStopped, false},
		{db.SessionStatusExpired, true},
		{db.SessionStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := IsTerminalState(tt.status)
			if result != tt.expected {
				t.Errorf("IsTerminalState(%s) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}

func TestValidateAndLogTransition(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		from      db.SessionStatus
		to        db.SessionStatus
		reason    string
		wantErr   bool
	}{
		{
			name:      "valid transition creating to running",
			sessionID: "test-session-1",
			from:      db.SessionStatusCreating,
			to:        db.SessionStatusRunning,
			reason:    "pod ready",
			wantErr:   false,
		},
		{
			name:      "valid transition running to stopped",
			sessionID: "test-session-2",
			from:      db.SessionStatusRunning,
			to:        db.SessionStatusStopped,
			reason:    "user requested",
			wantErr:   false,
		},
		{
			name:      "valid transition running to expired",
			sessionID: "test-session-3",
			from:      db.SessionStatusRunning,
			to:        db.SessionStatusExpired,
			reason:    "session timeout",
			wantErr:   false,
		},
		{
			name:      "invalid transition creating to stopped",
			sessionID: "test-session-4",
			from:      db.SessionStatusCreating,
			to:        db.SessionStatusStopped,
			reason:    "",
			wantErr:   true,
		},
		{
			name:      "valid transition stopped to creating (restart)",
			sessionID: "test-session-5",
			from:      db.SessionStatusStopped,
			to:        db.SessionStatusCreating,
			reason:    "user restarted",
			wantErr:   false,
		},
		{
			name:      "invalid transition stopped to running",
			sessionID: "test-session-6",
			from:      db.SessionStatusStopped,
			to:        db.SessionStatusRunning,
			reason:    "",
			wantErr:   true,
		},
		{
			name:      "invalid transition from terminal state",
			sessionID: "test-session-7",
			from:      db.SessionStatusExpired,
			to:        db.SessionStatusRunning,
			reason:    "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAndLogTransition(tt.sessionID, tt.from, tt.to, tt.reason)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAndLogTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				// Verify it's a TransitionError
				if _, ok := err.(*TransitionError); !ok {
					t.Errorf("Expected TransitionError, got %T", err)
				}
			}
		})
	}
}

func TestTransitionError(t *testing.T) {
	err := &TransitionError{
		SessionID: "test-123",
		From:      db.SessionStatusCreating,
		To:        db.SessionStatusStopped,
	}

	expected := "invalid session state transition: creating -> stopped (session: test-123)"
	if err.Error() != expected {
		t.Errorf("TransitionError.Error() = %q, want %q", err.Error(), expected)
	}
}
