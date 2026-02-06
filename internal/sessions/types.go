package sessions

import (
	"time"

	"github.com/rjsadow/launchpad/internal/db"
)

// CreateSessionRequest represents a request to create a new session
type CreateSessionRequest struct {
	AppID        string `json:"app_id"`
	UserID       string `json:"user_id"`
	ScreenWidth  int    `json:"screen_width,omitempty"`
	ScreenHeight int    `json:"screen_height,omitempty"`
}

// SessionResponse represents a session in API responses
type SessionResponse struct {
	ID           string           `json:"id"`
	UserID       string           `json:"user_id"`
	AppID        string           `json:"app_id"`
	AppName      string           `json:"app_name,omitempty"`
	PodName      string           `json:"pod_name"`
	Status       db.SessionStatus `json:"status"`
	WebSocketURL string           `json:"websocket_url,omitempty"` // For container apps (VNC)
	ProxyURL     string           `json:"proxy_url,omitempty"`     // For web_proxy apps
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// SessionFromDB converts a database session to an API response
func SessionFromDB(session *db.Session, appName string, wsURL string, proxyURL string) *SessionResponse {
	return &SessionResponse{
		ID:           session.ID,
		UserID:       session.UserID,
		AppID:        session.AppID,
		AppName:      appName,
		PodName:      session.PodName,
		Status:       session.Status,
		WebSocketURL: wsURL,
		ProxyURL:     proxyURL,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
	}
}
