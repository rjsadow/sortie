package sessions

import (
	"fmt"
	"log"

	"github.com/rjsadow/sortie/internal/db"
)

// ValidTransitions defines the allowed state transitions for sessions.
// Key is the current state, value is a slice of valid next states.
var ValidTransitions = map[db.SessionStatus][]db.SessionStatus{
	db.SessionStatusCreating: {
		db.SessionStatusRunning,
		db.SessionStatusFailed,
	},
	db.SessionStatusRunning: {
		db.SessionStatusStopped,
		db.SessionStatusExpired,
		db.SessionStatusFailed,
	},
	// Stopped sessions can be restarted
	db.SessionStatusStopped: {
		db.SessionStatusCreating, // restart
	},
	// Terminal states with no valid transitions
	db.SessionStatusExpired: {},
	db.SessionStatusFailed:  {},
}

// IsTerminalState returns true if the given status is a terminal state.
// Note: "stopped" is not terminal since sessions can be restarted.
func IsTerminalState(status db.SessionStatus) bool {
	switch status {
	case db.SessionStatusExpired, db.SessionStatusFailed:
		return true
	default:
		return false
	}
}

// CanTransition checks if a transition from one state to another is valid.
func CanTransition(from, to db.SessionStatus) bool {
	validTargets, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, target := range validTargets {
		if target == to {
			return true
		}
	}
	return false
}

// TransitionError represents an invalid state transition attempt.
type TransitionError struct {
	SessionID string
	From      db.SessionStatus
	To        db.SessionStatus
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid session state transition: %s -> %s (session: %s)", e.From, e.To, e.SessionID)
}

// LogTransition logs a state transition for audit purposes.
func LogTransition(sessionID string, from, to db.SessionStatus, reason string) {
	if reason != "" {
		log.Printf("Session %s: state transition %s -> %s (reason: %s)", sessionID, from, to, reason)
	} else {
		log.Printf("Session %s: state transition %s -> %s", sessionID, from, to)
	}
}

// ValidateAndLogTransition validates a transition and logs it if valid.
// Returns an error if the transition is invalid.
func ValidateAndLogTransition(sessionID string, from, to db.SessionStatus, reason string) error {
	if !CanTransition(from, to) {
		return &TransitionError{
			SessionID: sessionID,
			From:      from,
			To:        to,
		}
	}
	LogTransition(sessionID, from, to, reason)
	return nil
}
