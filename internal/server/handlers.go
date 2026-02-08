package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/middleware"
	"github.com/rjsadow/launchpad/internal/plugins"
	"github.com/rjsadow/launchpad/internal/plugins/auth"
	"github.com/rjsadow/launchpad/internal/sessions"
)

// handlers binds HTTP handler methods to an App's dependencies.
type handlers struct {
	app *App
}

// --- Health endpoints ---

func (h *handlers) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *handlers) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ready := true
	checks := make(map[string]interface{})

	if err := h.app.DB.Ping(); err != nil {
		ready = false
		checks["database"] = map[string]string{"status": "unhealthy", "error": err.Error()}
	} else {
		checks["database"] = map[string]string{"status": "healthy"}
	}

	pluginStatuses := plugins.Global().HealthCheck(r.Context())
	pluginChecks := make([]map[string]interface{}, 0, len(pluginStatuses))
	for _, ps := range pluginStatuses {
		entry := map[string]interface{}{
			"name":    ps.PluginName,
			"type":    ps.PluginType,
			"healthy": ps.Healthy,
			"message": ps.Message,
		}
		if !ps.Healthy {
			ready = false
		}
		pluginChecks = append(pluginChecks, entry)
	}
	checks["plugins"] = pluginChecks

	bp := h.app.BackpressureHandler
	if bp != nil && !bp.EnhancedReadinessCheck() {
		ready = false
		checks["sessions"] = map[string]string{"status": "overloaded"}
	} else if bp != nil {
		loadStatus := bp.GetLoadStatus()
		checks["sessions"] = map[string]interface{}{
			"status":          "ok",
			"active_sessions": loadStatus.ActiveSessions,
			"max_sessions":    loadStatus.MaxSessions,
			"load_factor":     loadStatus.LoadFactor,
			"queue_depth":     loadStatus.QueueDepth,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		checks["status"] = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		checks["status"] = "not_ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(checks)
}

// --- Auth endpoints ---

func (h *handlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.app.JWTAuth == nil || h.app.Config.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	result, err := h.app.JWTAuth.LoginWithCredentials(r.Context(), req.Username, req.Password)
	if err != nil {
		slog.Warn("login failed", "username", req.Username, "error", err)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	h.app.DB.LogAudit(req.Username, "LOGIN", "User logged in")

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    result.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(result.ExpiresIn),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.app.JWTAuth == nil || h.app.Config.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.RefreshToken == "" {
		http.Error(w, "Refresh token is required", http.StatusBadRequest)
		return
	}

	result, err := h.app.JWTAuth.RefreshAccessToken(r.Context(), req.RefreshToken)
	if err != nil && h.app.OIDCAuth != nil {
		result, err = h.app.OIDCAuth.RefreshAccessToken(r.Context(), req.RefreshToken)
	}
	if err != nil {
		slog.Warn("token refresh failed", "error", err)
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    result.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(result.ExpiresIn),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *handlers) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.app.JWTAuth == nil || h.app.Config.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
		return
	}

	token := parts[1]
	result, err := h.app.JWTAuth.Authenticate(r.Context(), token)
	if (err != nil || !result.Authenticated) && h.app.OIDCAuth != nil {
		result, err = h.app.OIDCAuth.Authenticate(r.Context(), token)
	}
	if err != nil || !result.Authenticated {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result.User)
}

func (h *handlers) isRegistrationAllowed() bool {
	if dbSetting, err := h.app.DB.GetSetting("allow_registration"); err == nil && dbSetting != "" {
		return strings.EqualFold(dbSetting, "true") || dbSetting == "1"
	}
	return h.app.Config.AllowRegistration
}

func (h *handlers) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.app.JWTAuth == nil || h.app.Config.JWTSecret == "" {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	if !h.isRegistrationAllowed() {
		http.Error(w, "Registration is not enabled", http.StatusForbidden)
		return
	}

	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	existing, err := h.app.DB.GetUserByUsername(req.Username)
	if err != nil {
		slog.Error("error checking username", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		http.Error(w, "Username already taken", http.StatusConflict)
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("error hashing password", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	user := db.User{
		ID:           fmt.Sprintf("user-%s-%d", req.Username, time.Now().UnixNano()),
		Username:     req.Username,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: passwordHash,
		Roles:        []string{"user"},
	}

	if err := h.app.DB.CreateUser(user); err != nil {
		slog.Error("error creating user", "error", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	h.app.DB.LogAudit(req.Username, "REGISTER", "User registered")

	result, err := h.app.JWTAuth.LoginWithCredentials(r.Context(), req.Username, req.Password)
	if err != nil {
		slog.Error("error generating tokens after registration", "error", err)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "Registration successful, please login"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// --- OIDC endpoints ---

func (h *handlers) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.app.OIDCAuth == nil {
		http.Error(w, "SSO is not configured", http.StatusServiceUnavailable)
		return
	}

	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL == "" {
		redirectURL = "/"
	}

	loginURL := h.app.OIDCAuth.GetLoginURL(redirectURL)
	if loginURL == "" {
		http.Error(w, "Failed to generate login URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, loginURL, http.StatusFound)
}

func (h *handlers) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.app.OIDCAuth == nil {
		http.Error(w, "SSO is not configured", http.StatusServiceUnavailable)
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("OIDC callback error", "error", errParam, "description", errDesc)
		http.Error(w, fmt.Sprintf("SSO error: %s", errDesc), http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	result, err := h.app.OIDCAuth.HandleCallback(r.Context(), code, state)
	if err != nil {
		slog.Error("OIDC callback failed", "error", err)
		http.Error(w, "SSO authentication failed", http.StatusUnauthorized)
		return
	}

	if !result.Authenticated || result.User == nil {
		http.Error(w, "SSO authentication failed", http.StatusUnauthorized)
		return
	}

	accessToken := result.Token
	refreshToken := result.Message

	h.app.DB.LogAudit(result.User.Username, "SSO_LOGIN", "User logged in via OIDC SSO")

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.app.Config.JWTAccessExpiry.Seconds()),
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>SSO Login</title></head>
<body>
<script>
localStorage.setItem('launchpad-access-token', %q);
localStorage.setItem('launchpad-refresh-token', %q);
localStorage.setItem('launchpad-user', JSON.stringify(%s));
window.location.href = '/';
</script>
<noscript><p>Login successful. <a href="/">Click here</a> to continue.</p></noscript>
</body>
</html>`, accessToken, refreshToken, mustJSON(result.User))
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// --- Config endpoint ---

// BrandingConfig represents tenant branding configuration.
type BrandingConfig struct {
	LogoURL           string `json:"logo_url"`
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	TenantName        string `json:"tenant_name"`
	AllowRegistration bool   `json:"allow_registration"`
	SSOEnabled        bool   `json:"sso_enabled"`
}

func (h *handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	brandingCfg := BrandingConfig{
		LogoURL:           h.app.Config.LogoURL,
		PrimaryColor:      h.app.Config.PrimaryColor,
		SecondaryColor:    h.app.Config.SecondaryColor,
		TenantName:        h.app.Config.TenantName,
		AllowRegistration: h.isRegistrationAllowed(),
	}

	if data, err := os.ReadFile(h.app.Config.BrandingConfigPath); err == nil {
		json.Unmarshal(data, &brandingCfg)
	}

	brandingCfg.AllowRegistration = h.isRegistrationAllowed()
	brandingCfg.SSOEnabled = h.app.OIDCAuth != nil

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(brandingCfg)
}

// --- App CRUD ---

func (h *handlers) handleApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		apps, err := h.app.DB.ListApps()
		if err != nil {
			slog.Error("error listing apps", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if apps == nil {
			apps = []db.Application{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apps)

	case http.MethodPost:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var app db.Application
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &app); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if app.ID == "" || app.Name == "" {
			http.Error(w, "Missing required fields: id, name", http.StatusBadRequest)
			return
		}

		if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
			if app.ContainerImage == "" {
				http.Error(w, "Missing required field for container/web_proxy app: container_image", http.StatusBadRequest)
				return
			}
		} else if app.URL == "" {
			http.Error(w, "Missing required field: url", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.CreateApp(app); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "Application with this ID already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		details := fmt.Sprintf("Created app: %s (%s)", app.Name, app.ID)
		h.app.DB.LogAudit(user.Username, "CREATE_APP", details)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(app)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleAppByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/apps/")
	if id == "" {
		http.Error(w, "Missing app ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		app, err := h.app.DB.GetApp(id)
		if err != nil {
			slog.Error("error getting app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if app == nil {
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)

	case http.MethodPut:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var app db.Application
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &app); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		app.ID = id

		if app.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}

		if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
			if app.ContainerImage == "" {
				http.Error(w, "Missing required field for container/web_proxy app: container_image", http.StatusBadRequest)
				return
			}
		} else if app.URL == "" {
			http.Error(w, "Missing required field: url", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.UpdateApp(app); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "Application not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		details := fmt.Sprintf("Updated app: %s (%s)", app.Name, app.ID)
		h.app.DB.LogAudit(user.Username, "UPDATE_APP", details)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app)

	case http.MethodDelete:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		app, err := h.app.DB.GetApp(id)
		if err != nil {
			slog.Error("error getting app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if app == nil {
			http.Error(w, "Application not found", http.StatusNotFound)
			return
		}

		if err := h.app.DB.DeleteApp(id); err != nil {
			slog.Error("error deleting app", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		details := fmt.Sprintf("Deleted app: %s (%s)", app.Name, id)
		h.app.DB.LogAudit(user.Username, "DELETE_APP", details)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- AppSpec CRUD ---

func (h *handlers) handleAppSpecs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		specs, err := h.app.DB.ListAppSpecs()
		if err != nil {
			slog.Error("error listing app specs", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if specs == nil {
			specs = []db.AppSpec{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(specs)

	case http.MethodPost:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var spec db.AppSpec
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &spec); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if spec.ID == "" || spec.Name == "" || spec.Image == "" {
			http.Error(w, "Missing required fields: id, name, image", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.CreateAppSpec(spec); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "AppSpec with this ID already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "CREATE_APPSPEC", fmt.Sprintf("Created app spec: %s (%s)", spec.Name, spec.ID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spec)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleAppSpecByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/appspecs/")
	if id == "" {
		http.Error(w, "Missing app spec ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		spec, err := h.app.DB.GetAppSpec(id)
		if err != nil {
			slog.Error("error getting app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if spec == nil {
			http.Error(w, "AppSpec not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)

	case http.MethodPut:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var spec db.AppSpec
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &spec); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		spec.ID = id

		if spec.Name == "" || spec.Image == "" {
			http.Error(w, "Missing required fields: name, image", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.UpdateAppSpec(spec); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "AppSpec not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "UPDATE_APPSPEC", fmt.Sprintf("Updated app spec: %s (%s)", spec.Name, spec.ID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)

	case http.MethodDelete:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		spec, err := h.app.DB.GetAppSpec(id)
		if err != nil {
			slog.Error("error getting app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if spec == nil {
			http.Error(w, "AppSpec not found", http.StatusNotFound)
			return
		}

		if err := h.app.DB.DeleteAppSpec(id); err != nil {
			slog.Error("error deleting app spec", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "DELETE_APPSPEC", fmt.Sprintf("Deleted app spec: %s (%s)", spec.Name, id))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Session endpoints ---

func (h *handlers) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID := r.URL.Query().Get("user_id")
		var sessionList []db.Session
		var err error

		if userID != "" {
			sessionList, err = h.app.SessionManager.ListSessionsByUser(r.Context(), userID)
		} else {
			sessionList, err = h.app.SessionManager.ListSessions(r.Context())
		}

		if err != nil {
			slog.Error("error listing sessions", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if sessionList == nil {
			sessionList = []db.Session{}
		}

		responses := make([]sessions.SessionResponse, len(sessionList))
		for i, s := range sessionList {
			app, _ := h.app.DB.GetApp(s.AppID)
			appName := ""
			wsURL := ""
			guacURL := ""
			proxyURL := ""
			if app != nil {
				appName = app.Name
				if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
					if app.OsType == "windows" {
						guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(&s)
					} else {
						wsURL = h.app.SessionManager.GetSessionWebSocketURL(&s)
					}
				}
			}
			responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, guacURL, proxyURL)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responses)

	case http.MethodPost:
		var req sessions.CreateSessionRequest
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.AppID == "" {
			http.Error(w, "Missing required field: app_id", http.StatusBadRequest)
			return
		}

		if req.UserID == "" {
			req.UserID = "anonymous"
		}

		session, err := h.app.SessionManager.CreateSession(r.Context(), &req)
		if err != nil {
			switch err.(type) {
			case *sessions.QuotaExceededError:
				loadStatus := h.app.BackpressureHandler.GetLoadStatus()
				sessions.WriteRetryAfter(w, loadStatus.LoadFactor)
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			case *sessions.QueueFullError:
				loadStatus := h.app.BackpressureHandler.GetLoadStatus()
				sessions.WriteRetryAfter(w, loadStatus.LoadFactor)
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			case *sessions.QueueTimeoutError:
				sessions.WriteRetryAfter(w, 1.0)
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			default:
				slog.Error("error creating session", "error", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		app, _ := h.app.DB.GetApp(session.AppID)
		appName := ""
		wsURL := ""
		guacURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				if app.OsType == "windows" {
					guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(session)
				} else {
					wsURL = h.app.SessionManager.GetSessionWebSocketURL(session)
				}
			}
		}

		response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL)

		details := fmt.Sprintf("Created session %s for app %s", session.ID, session.AppID)
		h.app.DB.LogAudit(req.UserID, "CREATE_SESSION", details)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	remainder := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if remainder == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(remainder, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "stop":
		h.handleSessionStop(w, r, id)
		return
	case action == "restart":
		h.handleSessionRestart(w, r, id)
		return
	case action == "files" || strings.HasPrefix(action, "files/"):
		h.app.FileHandler.ServeHTTP(w, r)
		return
	case action == "":
		// Fall through
	default:
		http.Error(w, "Unknown session action", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		session, err := h.app.SessionManager.GetSession(r.Context(), id)
		if err != nil {
			slog.Error("error getting session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		app, _ := h.app.DB.GetApp(session.AppID)
		appName := ""
		wsURL := ""
		guacURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				if app.OsType == "windows" {
					guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(session)
				} else {
					wsURL = h.app.SessionManager.GetSessionWebSocketURL(session)
				}
			}
		}

		response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodDelete:
		session, err := h.app.SessionManager.GetSession(r.Context(), id)
		if err != nil {
			slog.Error("error getting session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		if err := h.app.SessionManager.TerminateSession(r.Context(), id); err != nil {
			slog.Error("error terminating session", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		details := fmt.Sprintf("Terminated session %s", id)
		h.app.DB.LogAudit("admin", "TERMINATE_SESSION", details)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleSessionStop(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.app.SessionManager.StopSession(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "invalid session state") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		slog.Error("error stopping session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session, err := h.app.SessionManager.GetSession(r.Context(), id)
	if err != nil {
		slog.Error("error getting session after stop", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	app, _ := h.app.DB.GetApp(session.AppID)
	appName := ""
	if app != nil {
		appName = app.Name
	}
	response := sessions.SessionFromDB(session, appName, "", "", "")

	h.app.DB.LogAudit("user", "STOP_SESSION", fmt.Sprintf("Stopped session %s", id))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *handlers) handleSessionRestart(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := h.app.SessionManager.RestartSession(r.Context(), id)
	if err != nil {
		if _, ok := err.(*sessions.QuotaExceededError); ok {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "must be stopped") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		slog.Error("error restarting session", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	app, _ := h.app.DB.GetApp(session.AppID)
	appName := ""
	wsURL := ""
	guacURL := ""
	proxyURL := ""
	if app != nil {
		appName = app.Name
		if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
			if app.OsType == "windows" {
				guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(session)
			} else {
				wsURL = h.app.SessionManager.GetSessionWebSocketURL(session)
			}
		}
	}

	response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL)

	h.app.DB.LogAudit("user", "RESTART_SESSION", fmt.Sprintf("Restarted session %s", id))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// --- Quota endpoint ---

func (h *handlers) handleQuotas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	userID := "anonymous"
	if user != nil {
		userID = user.ID
	}

	status, err := h.app.SessionManager.GetQuotaStatus(userID)
	if err != nil {
		slog.Error("error getting quota status", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// --- Audit endpoints ---

func (h *handlers) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter, err := parseAuditFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page, err := h.app.DB.QueryAuditLogs(filter)
	if err != nil {
		slog.Error("error querying audit logs", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if page.Logs == nil {
		page.Logs = []db.AuditLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

func (h *handlers) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter, err := parseAuditFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if filter.Limit <= 0 || filter.Limit > 10000 {
		filter.Limit = 10000
	}
	filter.Offset = 0

	page, err := h.app.DB.QueryAuditLogs(filter)
	if err != nil {
		slog.Error("error exporting audit logs", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if page.Logs == nil {
		page.Logs = []db.AuditLog{}
	}

	format := r.URL.Query().Get("format")
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit_log.csv")
		writeAuditCSV(w, page.Logs)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit_log.json")
	json.NewEncoder(w).Encode(page.Logs)
}

func (h *handlers) handleAuditFilters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	actions, err := h.app.DB.GetAuditLogActions()
	if err != nil {
		slog.Error("error getting audit actions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	users, err := h.app.DB.GetAuditLogUsers()
	if err != nil {
		slog.Error("error getting audit users", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if actions == nil {
		actions = []string{}
	}
	if users == nil {
		users = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{
		"actions": actions,
		"users":   users,
	})
}

func parseAuditFilter(r *http.Request) (db.AuditLogFilter, error) {
	q := r.URL.Query()
	filter := db.AuditLogFilter{
		User:   q.Get("user"),
		Action: q.Get("action"),
	}

	if from := q.Get("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return filter, fmt.Errorf("invalid 'from' date: %w", err)
		}
		filter.From = t
	}
	if to := q.Get("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return filter, fmt.Errorf("invalid 'to' date: %w", err)
		}
		filter.To = t
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return filter, fmt.Errorf("invalid 'limit': %w", err)
		}
		filter.Limit = limit
	}
	if offsetStr := q.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return filter, fmt.Errorf("invalid 'offset': %w", err)
		}
		filter.Offset = offset
	}

	return filter, nil
}

func writeAuditCSV(w io.Writer, logs []db.AuditLog) {
	fmt.Fprintf(w, "ID,Timestamp,User,Action,Details\n")
	for _, log := range logs {
		details := strings.ReplaceAll(log.Details, "\"", "\"\"")
		fmt.Fprintf(w, "%d,%s,%s,%s,\"%s\"\n",
			log.ID,
			log.Timestamp.Format(time.RFC3339),
			log.User,
			log.Action,
			details,
		)
	}
}

// --- Analytics endpoints ---

func (h *handlers) handleAnalyticsLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppID string `json:"app_id"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.AppID == "" {
		http.Error(w, "Missing required field: app_id", http.StatusBadRequest)
		return
	}

	if err := h.app.DB.RecordLaunch(req.AppID); err != nil {
		slog.Error("error recording launch", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

func (h *handlers) handleAnalyticsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := h.app.DB.GetAnalyticsStats()
	if err != nil {
		slog.Error("error getting analytics stats", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// --- Admin endpoints ---

func (h *handlers) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.app.DB.GetAllSettings()
		if err != nil {
			slog.Error("error getting settings", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"allow_registration":     h.isRegistrationAllowed(),
			"max_sessions_per_user":  h.app.Config.MaxSessionsPerUser,
			"max_global_sessions":    h.app.Config.MaxGlobalSessions,
			"default_cpu_request":    h.app.Config.DefaultCPURequest,
			"default_cpu_limit":      h.app.Config.DefaultCPULimit,
			"default_memory_request": h.app.Config.DefaultMemRequest,
			"default_memory_limit":   h.app.Config.DefaultMemLimit,
		}

		for k, v := range settings {
			response[k] = v
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodPut:
		var req map[string]string
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		for key, value := range req {
			if err := h.app.DB.SetSetting(key, value); err != nil {
				slog.Error("error updating setting", "key", key, "error", err)
				http.Error(w, "Failed to update settings", http.StatusInternalServerError)
				return
			}
		}

		user := middleware.GetUserFromContext(r.Context())
		h.app.DB.LogAudit(user.Username, "UPDATE_SETTINGS", fmt.Sprintf("Updated settings: %v", req))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionList, err := h.app.SessionManager.ListSessions(r.Context())
	if err != nil {
		slog.Error("error listing sessions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if sessionList == nil {
		sessionList = []db.Session{}
	}

	responses := make([]sessions.SessionResponse, len(sessionList))
	for i, s := range sessionList {
		app, _ := h.app.DB.GetApp(s.AppID)
		appName := ""
		wsURL := ""
		guacURL := ""
		proxyURL := ""
		if app != nil {
			appName = app.Name
			if app.LaunchType == db.LaunchTypeContainer || app.LaunchType == db.LaunchTypeWebProxy {
				if app.OsType == "windows" {
					guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(&s)
				} else {
					wsURL = h.app.SessionManager.GetSessionWebSocketURL(&s)
				}
			}
		}
		responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, guacURL, proxyURL)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

func (h *handlers) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		users, err := h.app.DB.ListUsers()
		if err != nil {
			slog.Error("error listing users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if users == nil {
			users = []db.User{}
		}

		type userResponse struct {
			ID          string    `json:"id"`
			Username    string    `json:"username"`
			Email       string    `json:"email,omitempty"`
			DisplayName string    `json:"display_name,omitempty"`
			Roles       []string  `json:"roles"`
			CreatedAt   time.Time `json:"created_at"`
		}

		response := make([]userResponse, len(users))
		for i, u := range users {
			response[i] = userResponse{
				ID:          u.ID,
				Username:    u.Username,
				Email:       u.Email,
				DisplayName: u.DisplayName,
				Roles:       u.Roles,
				CreatedAt:   u.CreatedAt,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodPost:
		var req struct {
			Username    string   `json:"username"`
			Password    string   `json:"password"`
			Email       string   `json:"email"`
			DisplayName string   `json:"display_name"`
			Roles       []string `json:"roles"`
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Username == "" || req.Password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		existing, err := h.app.DB.GetUserByUsername(req.Username)
		if err != nil {
			slog.Error("error checking username", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			http.Error(w, "Username already exists", http.StatusConflict)
			return
		}

		passwordHash, err := auth.HashPassword(req.Password)
		if err != nil {
			slog.Error("error hashing password", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		roles := req.Roles
		if len(roles) == 0 {
			roles = []string{"user"}
		}

		user := db.User{
			ID:           fmt.Sprintf("user-%s-%d", req.Username, time.Now().UnixNano()),
			Username:     req.Username,
			Email:        req.Email,
			DisplayName:  req.DisplayName,
			PasswordHash: passwordHash,
			Roles:        roles,
		}

		if err := h.app.DB.CreateUser(user); err != nil {
			slog.Error("error creating user", "error", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		adminUser := middleware.GetUserFromContext(r.Context())
		h.app.DB.LogAudit(adminUser.Username, "CREATE_USER", fmt.Sprintf("Created user: %s", req.Username))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"id":       user.ID,
			"username": user.Username,
			"message":  "User created successfully",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleAdminUserByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	if id == "" {
		http.Error(w, "User ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		currentUser := middleware.GetUserFromContext(r.Context())
		if currentUser != nil && currentUser.ID == id {
			http.Error(w, "Cannot delete your own account", http.StatusBadRequest)
			return
		}

		user, err := h.app.DB.GetUserByID(id)
		if err != nil {
			slog.Error("error getting user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := h.app.DB.DeleteUser(id); err != nil {
			slog.Error("error deleting user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(currentUser.Username, "DELETE_USER", fmt.Sprintf("Deleted user: %s (%s)", user.Username, id))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Template endpoints ---

func (h *handlers) handleTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	templates, err := h.app.DB.ListTemplates()
	if err != nil {
		slog.Error("error listing templates", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if templates == nil {
		templates = []db.Template{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}

func (h *handlers) handleTemplateByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	templateID := strings.TrimPrefix(r.URL.Path, "/api/templates/")
	if templateID == "" {
		http.Error(w, "Template ID required", http.StatusBadRequest)
		return
	}

	template, err := h.app.DB.GetTemplate(templateID)
	if err != nil {
		slog.Error("error getting template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if template == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(template)
}

func (h *handlers) handleAdminTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := h.app.DB.ListTemplates()
		if err != nil {
			slog.Error("error listing templates", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if templates == nil {
			templates = []db.Template{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)

	case http.MethodPost:
		var template db.Template
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &template); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if template.TemplateID == "" {
			http.Error(w, "Missing required field: template_id", http.StatusBadRequest)
			return
		}
		if template.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}
		if template.TemplateCategory == "" {
			http.Error(w, "Missing required field: template_category", http.StatusBadRequest)
			return
		}
		if template.Category == "" {
			http.Error(w, "Missing required field: category", http.StatusBadRequest)
			return
		}

		if template.TemplateVersion == "" {
			template.TemplateVersion = "1.0.0"
		}
		if template.LaunchType == "" {
			template.LaunchType = "container"
		}

		if err := h.app.DB.CreateTemplate(template); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "Template with this ID already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user := middleware.GetUserFromContext(r.Context())
		h.app.DB.LogAudit(user.Username, "CREATE_TEMPLATE", fmt.Sprintf("Created template: %s (%s)", template.Name, template.TemplateID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(template)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleAdminTemplateByID(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimPrefix(r.URL.Path, "/api/admin/templates/")
	if templateID == "" {
		http.Error(w, "Template ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		template, err := h.app.DB.GetTemplate(templateID)
		if err != nil {
			slog.Error("error getting template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if template == nil {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)

	case http.MethodPut:
		var template db.Template
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &template); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		template.TemplateID = templateID

		if template.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.UpdateTemplate(template); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "Template not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user := middleware.GetUserFromContext(r.Context())
		h.app.DB.LogAudit(user.Username, "UPDATE_TEMPLATE", fmt.Sprintf("Updated template: %s (%s)", template.Name, templateID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)

	case http.MethodDelete:
		template, err := h.app.DB.GetTemplate(templateID)
		if err != nil {
			slog.Error("error getting template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if template == nil {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}

		if err := h.app.DB.DeleteTemplate(templateID); err != nil {
			slog.Error("error deleting template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		user := middleware.GetUserFromContext(r.Context())
		h.app.DB.LogAudit(user.Username, "DELETE_TEMPLATE", fmt.Sprintf("Deleted template: %s (%s)", template.Name, templateID))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Diagnostics / Health / Support ---

func (h *handlers) handleDiagnosticsBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	h.app.DB.LogAudit(user.Username, "GENERATE_DIAGNOSTICS", "Generated diagnostics bundle")

	if r.Header.Get("Accept") == "application/gzip" {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=diagnostics-%s.tar.gz", time.Now().UTC().Format("20060102-150405")))
		if err := h.app.DiagCollector.WriteTarGz(r.Context(), w); err != nil {
			slog.Error("failed to generate diagnostics archive", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	bundle, err := h.app.DiagCollector.Collect(r.Context())
	if err != nil {
		slog.Error("failed to collect diagnostics", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bundle)
}

func (h *handlers) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbHealthy := true
	dbMessage := "OK"
	if err := h.app.DB.Ping(); err != nil {
		dbHealthy = false
		dbMessage = err.Error()
	}

	activeSessionCount, _ := h.app.DB.CountActiveSessions()
	apps, _ := h.app.DB.ListApps()
	users, _ := h.app.DB.ListUsers()

	pluginStatuses := plugins.Global().HealthCheck(r.Context())
	allHealthy := dbHealthy
	for _, ps := range pluginStatuses {
		if !ps.Healthy {
			allHealthy = false
		}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	health := map[string]any{
		"status": "healthy",
		"components": map[string]any{
			"database": map[string]any{
				"healthy": dbHealthy,
				"message": dbMessage,
			},
			"plugins": pluginStatuses,
		},
		"stats": map[string]any{
			"active_sessions": activeSessionCount,
			"total_apps":      len(apps),
			"total_users":     len(users),
			"goroutines":      runtime.NumGoroutine(),
			"memory_alloc_mb": float64(memStats.Alloc) / 1024 / 1024,
		},
	}

	if !allHealthy {
		health["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (h *handlers) handleSupportInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := map[string]any{
		"application": "launchpad",
		"go_version":  runtime.Version(),
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"config": map[string]any{
			"auth_enabled":          h.app.Config.JWTSecret != "",
			"oidc_enabled":          h.app.Config.OIDCEnabled(),
			"namespace":             h.app.Config.Namespace,
			"max_sessions_per_user": h.app.Config.MaxSessionsPerUser,
			"max_global_sessions":   h.app.Config.MaxGlobalSessions,
			"recording_enabled":     h.app.Config.RecordingEnabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// --- Tenant admin endpoints ---

func (h *handlers) handleAdminTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenants, err := h.app.DB.ListTenants()
		if err != nil {
			slog.Error("error listing tenants", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if tenants == nil {
			tenants = []db.Tenant{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tenants)

	case http.MethodPost:
		var req struct {
			Name     string            `json:"name"`
			Slug     string            `json:"slug"`
			Settings db.TenantSettings `json:"settings"`
			Quotas   db.TenantQuotas   `json:"quotas"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Name == "" || req.Slug == "" {
			http.Error(w, "name and slug are required", http.StatusBadRequest)
			return
		}

		existing, err := h.app.DB.GetTenantBySlug(req.Slug)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			http.Error(w, "Tenant slug already exists", http.StatusConflict)
			return
		}

		tenant := db.Tenant{
			ID:        tenantUUID(),
			Name:      req.Name,
			Slug:      req.Slug,
			Settings:  req.Settings,
			Quotas:    req.Quotas,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := h.app.DB.CreateTenant(tenant); err != nil {
			slog.Error("error creating tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit("admin", "CREATE_TENANT", "Created tenant: "+tenant.Name)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tenant)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleAdminTenantByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/tenants/"), "/")
	tenantID := parts[0]
	if tenantID == "" {
		http.Error(w, "Tenant ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tenant, err := h.app.DB.GetTenant(tenantID)
		if err != nil {
			slog.Error("error getting tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if tenant == nil {
			http.Error(w, "Tenant not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tenant)

	case http.MethodPut:
		var req struct {
			Name     string            `json:"name"`
			Slug     string            `json:"slug"`
			Settings db.TenantSettings `json:"settings"`
			Quotas   db.TenantQuotas   `json:"quotas"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		tenant, err := h.app.DB.GetTenant(tenantID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if tenant == nil {
			http.Error(w, "Tenant not found", http.StatusNotFound)
			return
		}

		if req.Name != "" {
			tenant.Name = req.Name
		}
		if req.Slug != "" {
			tenant.Slug = req.Slug
		}
		tenant.Settings = req.Settings
		tenant.Quotas = req.Quotas

		if err := h.app.DB.UpdateTenant(*tenant); err != nil {
			slog.Error("error updating tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit("admin", "UPDATE_TENANT", "Updated tenant: "+tenant.Name)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tenant)

	case http.MethodDelete:
		if tenantID == db.DefaultTenantID {
			http.Error(w, "Cannot delete the default tenant", http.StatusForbidden)
			return
		}

		if err := h.app.DB.DeleteTenant(tenantID); err != nil {
			slog.Error("error deleting tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit("admin", "DELETE_TENANT", "Deleted tenant: "+tenantID)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Legacy apps.json ---

func (h *handlers) handleAppsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	apps, err := h.app.DB.ListApps()
	if err != nil {
		slog.Error("error listing apps", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := db.AppConfig{Applications: apps}
	if response.Applications == nil {
		response.Applications = []db.Application{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// --- Static file serving ---

func (h *handlers) staticHandler(fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		if strings.HasSuffix(path, ".html") || path == "/" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		} else if strings.HasPrefix(path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		if _, err := fs.Stat(h.app.StaticFS, path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}
}

// tenantUUID generates a UUID for new tenants.
func tenantUUID() string {
	// Use a simple format; production uses google/uuid
	return fmt.Sprintf("tenant-%d", time.Now().UnixNano())
}
