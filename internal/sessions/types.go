package sessions

import (
	"fmt"
	"time"

	"github.com/rjsadow/sortie/internal/db"
)

// QuotaExceededError is returned when a session cannot be created due to quota limits.
type QuotaExceededError struct {
	Reason string
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("quota exceeded: %s", e.Reason)
}

// QuotaStatus represents the current quota usage for a user.
type QuotaStatus struct {
	UserSessions       int    `json:"user_sessions"`
	MaxSessionsPerUser int    `json:"max_sessions_per_user"` // 0 = unlimited
	GlobalSessions     int    `json:"global_sessions"`
	MaxGlobalSessions  int    `json:"max_global_sessions"` // 0 = unlimited
	TenantSessions     int    `json:"tenant_sessions,omitempty"`
	MaxTenantSessions  int    `json:"max_tenant_sessions,omitempty"` // 0 = unlimited
	DefaultCPURequest  string `json:"default_cpu_request,omitempty"`
	DefaultCPULimit    string `json:"default_cpu_limit,omitempty"`
	DefaultMemRequest  string `json:"default_mem_request,omitempty"`
	DefaultMemLimit    string `json:"default_mem_limit,omitempty"`
}

// CreateSessionRequest represents a request to create a new session
type CreateSessionRequest struct {
	AppID        string `json:"app_id"`
	UserID       string `json:"user_id"`
	ScreenWidth  int    `json:"screen_width,omitempty"`
	ScreenHeight int    `json:"screen_height,omitempty"`
	IdleTimeout  int64  `json:"idle_timeout,omitempty"` // Per-session idle timeout in seconds (0 = use global default)
}

// SessionResponse represents a session in API responses
type SessionResponse struct {
	ID           string           `json:"id"`
	UserID       string           `json:"user_id"`
	AppID        string           `json:"app_id"`
	AppName      string           `json:"app_name,omitempty"`
	PodName      string           `json:"pod_name"`
	Status       db.SessionStatus `json:"status"`
	IdleTimeout  int64            `json:"idle_timeout,omitempty"` // Per-session idle timeout in seconds (0 = global default)
	WebSocketURL string           `json:"websocket_url,omitempty"`  // For Linux container apps (VNC)
	GuacamoleURL string           `json:"guacamole_url,omitempty"`  // For Windows container apps (RDP via Guacamole)
	ProxyURL     string           `json:"proxy_url,omitempty"`      // For web_proxy apps
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// SessionFromDB converts a database session to an API response
func SessionFromDB(session *db.Session, appName string, wsURL string, guacURL string, proxyURL string) *SessionResponse {
	return &SessionResponse{
		ID:           session.ID,
		UserID:       session.UserID,
		AppID:        session.AppID,
		AppName:      appName,
		PodName:      session.PodName,
		Status:       session.Status,
		IdleTimeout:  session.IdleTimeout,
		WebSocketURL: wsURL,
		GuacamoleURL: guacURL,
		ProxyURL:     proxyURL,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
	}
}
