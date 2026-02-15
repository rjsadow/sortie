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

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/middleware"
	"github.com/rjsadow/sortie/internal/plugins"
	"github.com/rjsadow/sortie/internal/plugins/auth"
	"github.com/rjsadow/sortie/internal/sessions"
)

// handlers binds HTTP handler methods to an App's dependencies.
type handlers struct {
	app *App
}

// getRecordingPolicy reads the recording_auto_record setting and returns
// "auto" when enabled, or "" otherwise.
func (h *handlers) getRecordingPolicy() string {
	val, err := h.app.DB.GetSetting("recording_auto_record")
	if err == nil && val == "true" {
		return "auto"
	}
	return ""
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

	// Look up categories this user administers
	adminCats, _ := h.app.DB.GetCategoriesAdminedByUser(result.User.ID)

	type meResponse struct {
		ID              string            `json:"id"`
		Username        string            `json:"username"`
		Email           string            `json:"email,omitempty"`
		Name            string            `json:"name,omitempty"`
		Roles           []string          `json:"roles,omitempty"`
		Groups          []string          `json:"groups,omitempty"`
		Metadata        map[string]string `json:"metadata,omitempty"`
		AdminCategories []string          `json:"admin_categories,omitempty"`
	}

	resp := meResponse{
		ID:              result.User.ID,
		Username:        result.User.Username,
		Email:           result.User.Email,
		Name:            result.User.Name,
		Roles:           result.User.Roles,
		Groups:          result.User.Groups,
		Metadata:        result.User.Metadata,
		AdminCategories: adminCats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *handlers) handleUsersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users, err := h.app.DB.ListUsers()
	if err != nil {
		http.Error(w, "Failed to list users", http.StatusInternalServerError)
		return
	}

	type basicUser struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}

	result := make([]basicUser, len(users))
	for i, u := range users {
		result[i] = basicUser{ID: u.ID, Username: u.Username}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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
localStorage.setItem('sortie-access-token', %q);
localStorage.setItem('sortie-refresh-token', %q);
localStorage.setItem('sortie-user', JSON.stringify(%s));
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
		user := middleware.GetUserFromContext(r.Context())
		userID := ""
		var userRoles []string
		if user != nil {
			userID = user.ID
			userRoles = user.Roles
		}
		tenantID := middleware.GetTenantIDFromContext(r.Context())

		apps, err := h.app.DB.ListAppsForUser(userID, userRoles, tenantID)
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
		// Check category admin permission for the app's category
		isCatAdmin := false
		// We'll check after parsing the body so we know the category
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

		if app.Visibility == "" {
			app.Visibility = db.CategoryVisibilityPublic
		}

		if app.Category != "" {
			if cat, _ := h.app.DB.GetCategoryByName(app.Category); cat != nil {
				isCatAdmin, _ = h.app.DB.IsCategoryAdmin(user.ID, cat.ID)
			}
		}

		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) && !isCatAdmin {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
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

		// Auto-create category if it doesn't exist (backwards compat)
		if app.Category != "" {
			tenantID := middleware.GetTenantIDFromContext(r.Context())
			h.app.DB.EnsureCategoryExists(app.Category, tenantID)
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
		if app.Visibility == "" {
			app.Visibility = db.CategoryVisibilityPublic
		}

		// Check category admin for the app's category
		isCatAdmin := false
		catName := app.Category
		if catName == "" {
			// Check existing app's category
			if existing, _ := h.app.DB.GetApp(id); existing != nil {
				catName = existing.Category
			}
		}
		if catName != "" {
			if cat, _ := h.app.DB.GetCategoryByName(catName); cat != nil {
				isCatAdmin, _ = h.app.DB.IsCategoryAdmin(user.ID, cat.ID)
			}
		}

		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) && !isCatAdmin {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

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

		// Auto-create category if it doesn't exist
		if app.Category != "" {
			tenantID := middleware.GetTenantIDFromContext(r.Context())
			h.app.DB.EnsureCategoryExists(app.Category, tenantID)
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

		// Check category admin for the app's category
		isCatAdmin := false
		if app.Category != "" {
			if cat, _ := h.app.DB.GetCategoryByName(app.Category); cat != nil {
				isCatAdmin, _ = h.app.DB.IsCategoryAdmin(user.ID, cat.ID)
			}
		}

		if !middleware.HasRole(user.Roles, middleware.RoleAdmin, middleware.RoleAppAuthor) && !isCatAdmin {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
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
			// Default to the authenticated user's sessions (admins use /api/admin/sessions for all)
			user := middleware.GetUserFromContext(r.Context())
			if user != nil {
				sessionList, err = h.app.SessionManager.ListSessionsByUser(r.Context(), user.ID)
			} else {
				sessionList, err = h.app.SessionManager.ListSessions(r.Context())
			}
		}

		if err != nil {
			slog.Error("error listing sessions", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if sessionList == nil {
			sessionList = []db.Session{}
		}

		recPolicy := h.getRecordingPolicy()
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
			responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, guacURL, proxyURL, recPolicy)
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
			user := middleware.GetUserFromContext(r.Context())
			if user != nil {
				req.UserID = user.ID
			} else {
				req.UserID = "anonymous"
			}
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

		response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL, h.getRecordingPolicy())

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
	case action == "shares" || strings.HasPrefix(action, "shares/"):
		h.handleSessionShares(w, r, id, strings.TrimPrefix(action, "shares"))
		return
	case strings.HasPrefix(action, "recording/"):
		if h.app.RecordingHandler != nil {
			h.app.RecordingHandler.ServeHTTP(w, r)
		} else {
			http.Error(w, "Video recording not enabled", http.StatusNotFound)
		}
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

		response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL, h.getRecordingPolicy())

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
	response := sessions.SessionFromDB(session, appName, "", "", "", "")

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

	response := sessions.SessionFromDB(session, appName, wsURL, guacURL, proxyURL, h.getRecordingPolicy())

	h.app.DB.LogAudit("user", "RESTART_SESSION", fmt.Sprintf("Restarted session %s", id))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// --- Session sharing endpoints ---

func (h *handlers) handleSessionShares(w http.ResponseWriter, r *http.Request, sessionID string, subPath string) {
	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check that the session exists
	session, err := h.app.SessionManager.GetSession(r.Context(), sessionID)
	if err != nil {
		slog.Error("error getting session for shares", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Only the session owner can manage shares
	isOwner := session.UserID == user.ID

	// Handle DELETE /api/sessions/{id}/shares/{shareId}
	shareID := strings.TrimPrefix(subPath, "/")
	if shareID != "" {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isOwner && !middleware.HasRole(user.Roles, middleware.RoleAdmin) {
			http.Error(w, "Forbidden: only session owner can revoke shares", http.StatusForbidden)
			return
		}
		if err := h.app.DB.DeleteSessionShare(shareID); err != nil {
			http.Error(w, "Share not found", http.StatusNotFound)
			return
		}
		h.app.DB.LogAudit(user.Username, "REVOKE_SESSION_SHARE", fmt.Sprintf("Revoked share %s for session %s", shareID, sessionID))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !isOwner && !middleware.HasRole(user.Roles, middleware.RoleAdmin) {
			http.Error(w, "Forbidden: only session owner can list shares", http.StatusForbidden)
			return
		}
		shares, err := h.app.DB.ListSessionShares(sessionID)
		if err != nil {
			slog.Error("error listing session shares", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if shares == nil {
			shares = []db.SessionShare{}
		}

		responses := make([]sessions.ShareResponse, len(shares))
		for i, s := range shares {
			resp := sessions.ShareResponse{
				ID:         s.ID,
				SessionID:  s.SessionID,
				UserID:     s.UserID,
				Permission: string(s.Permission),
				CreatedAt:  s.CreatedAt.Format(time.RFC3339),
			}
			if s.UserID != "" {
				u, _ := h.app.DB.GetUserByID(s.UserID)
				if u != nil {
					resp.Username = u.Username
				}
			}
			if s.ShareToken != "" {
				resp.ShareURL = fmt.Sprintf("/session/%s?share_token=%s", sessionID, s.ShareToken)
			}
			responses[i] = resp
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responses)

	case http.MethodPost:
		if !isOwner && !middleware.HasRole(user.Roles, middleware.RoleAdmin) {
			http.Error(w, "Forbidden: only session owner can create shares", http.StatusForbidden)
			return
		}

		// Only container sessions can be shared
		app, _ := h.app.DB.GetApp(session.AppID)
		if app != nil && app.LaunchType != db.LaunchTypeContainer {
			http.Error(w, "Only container sessions can be shared", http.StatusBadRequest)
			return
		}

		var req sessions.CreateShareRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		perm := db.SharePermissionReadOnly
		if req.Permission == "read_write" {
			perm = db.SharePermissionReadWrite
		}

		share := db.SessionShare{
			ID:         fmt.Sprintf("share-%d", time.Now().UnixNano()),
			SessionID:  sessionID,
			Permission: perm,
			CreatedBy:  user.ID,
			CreatedAt:  time.Now(),
		}

		resp := sessions.ShareResponse{
			SessionID:  sessionID,
			Permission: string(perm),
		}

		if req.LinkShare {
			token := fmt.Sprintf("%d", time.Now().UnixNano())
			share.ShareToken = token
			resp.ShareURL = fmt.Sprintf("/session/%s?share_token=%s", sessionID, token)
		} else {
			// Resolve user by username or ID
			targetUserID := req.UserID
			if targetUserID == "" && req.Username != "" {
				u, err := h.app.DB.GetUserByUsername(req.Username)
				if err != nil || u == nil {
					http.Error(w, "User not found", http.StatusNotFound)
					return
				}
				targetUserID = u.ID
				resp.Username = u.Username
			}
			if targetUserID == "" {
				http.Error(w, "Either user_id, username, or link_share is required", http.StatusBadRequest)
				return
			}
			if targetUserID == user.ID {
				http.Error(w, "Cannot share a session with yourself", http.StatusBadRequest)
				return
			}
			share.UserID = targetUserID
		}

		if err := h.app.DB.CreateSessionShare(share); err != nil {
			slog.Error("error creating session share", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resp.ID = share.ID
		resp.UserID = share.UserID
		resp.CreatedAt = share.CreatedAt.Format(time.RFC3339)

		h.app.DB.LogAudit(user.Username, "CREATE_SESSION_SHARE", fmt.Sprintf("Shared session %s (permission=%s)", sessionID, perm))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleSharedSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.app.DB.ListSharedSessionsForUser(user.ID)
	if err != nil {
		slog.Error("error listing shared sessions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	responses := make([]sessions.SessionResponse, 0, len(rows))
	for _, r := range rows {
		app, _ := h.app.DB.GetApp(r.Session.AppID)
		wsURL := ""
		guacURL := ""
		if app != nil {
			if app.LaunchType == db.LaunchTypeContainer {
				if app.OsType == "windows" {
					guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(&r.Session)
				} else {
					wsURL = h.app.SessionManager.GetSessionWebSocketURL(&r.Session)
				}
			}
		}

		resp := *sessions.SessionFromDB(&r.Session, r.AppName, wsURL, guacURL, "", "")
		resp.IsShared = true
		resp.OwnerUsername = r.OwnerUsername
		resp.SharePermission = string(r.Permission)
		resp.ShareID = r.ShareID
		responses = append(responses, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

func (h *handlers) handleJoinShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req sessions.JoinShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	share, err := h.app.DB.GetSessionShareByToken(req.Token)
	if err != nil {
		slog.Error("error looking up share token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if share == nil {
		http.Error(w, "Invalid or expired share token", http.StatusNotFound)
		return
	}

	// If this is a link-share with no user assigned, assign this user
	if share.UserID == "" {
		h.app.DB.UpdateSessionShareUserID(share.ID, user.ID)
	} else if share.UserID != user.ID {
		// Link was already claimed by someone else; create a new share for this user
		newShare := db.SessionShare{
			ID:         fmt.Sprintf("share-%d", time.Now().UnixNano()),
			SessionID:  share.SessionID,
			UserID:     user.ID,
			Permission: share.Permission,
			CreatedBy:  share.CreatedBy,
			CreatedAt:  time.Now(),
		}
		if err := h.app.DB.CreateSessionShare(newShare); err != nil {
			slog.Error("error creating share for joining user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Return the session info
	session, err := h.app.SessionManager.GetSession(r.Context(), share.SessionID)
	if err != nil || session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	app, _ := h.app.DB.GetApp(session.AppID)
	appName := ""
	wsURL := ""
	guacURL := ""
	if app != nil {
		appName = app.Name
		if app.LaunchType == db.LaunchTypeContainer {
			if app.OsType == "windows" {
				guacURL = h.app.SessionManager.GetSessionGuacWebSocketURL(session)
			} else {
				wsURL = h.app.SessionManager.GetSessionWebSocketURL(session)
			}
		}
	}

	resp := *sessions.SessionFromDB(session, appName, wsURL, guacURL, "", "")
	resp.IsShared = true
	resp.SharePermission = string(share.Permission)

	// Get owner username
	owner, _ := h.app.DB.GetUserByID(session.UserID)
	if owner != nil {
		resp.OwnerUsername = owner.Username
	}

	h.app.DB.LogAudit(user.Username, "JOIN_SESSION_SHARE", fmt.Sprintf("Joined session %s via share token", share.SessionID))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	recPolicy := h.getRecordingPolicy()
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
		responses[i] = *sessions.SessionFromDB(&s, appName, wsURL, guacURL, proxyURL, recPolicy)
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
		"application": "sortie",
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

// --- Category endpoints ---

func (h *handlers) handleCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user := middleware.GetUserFromContext(r.Context())
		userID := ""
		var userRoles []string
		if user != nil {
			userID = user.ID
			userRoles = user.Roles
		}
		tenantID := middleware.GetTenantIDFromContext(r.Context())

		cats, err := h.app.DB.ListVisibleCategoriesForUser(userID, userRoles, tenantID)
		if err != nil {
			slog.Error("error listing categories", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if cats == nil {
			cats = []db.Category{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cats)

	case http.MethodPost:
		user := middleware.GetUserFromContext(r.Context())
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var cat db.Category
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &cat); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if cat.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}
		if cat.ID == "" {
			cat.ID = fmt.Sprintf("cat-%s-%d", cat.Name, time.Now().UnixNano())
		}
		tenantID := middleware.GetTenantIDFromContext(r.Context())
		if cat.TenantID == "" {
			cat.TenantID = tenantID
		}

		if err := h.app.DB.CreateCategory(cat); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				http.Error(w, "Category with this name already exists", http.StatusConflict)
				return
			}
			slog.Error("error creating category", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "CREATE_CATEGORY", fmt.Sprintf("Created category: %s (%s)", cat.Name, cat.ID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(cat)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleCategoryByID(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/categories/{id} or /api/categories/{id}/admins or /api/categories/{id}/approved-users
	remainder := strings.TrimPrefix(r.URL.Path, "/api/categories/")
	if remainder == "" {
		http.Error(w, "Missing category ID", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(remainder, "/", 2)
	catID := parts[0]
	subResource := ""
	if len(parts) > 1 {
		subResource = parts[1]
	}

	switch subResource {
	case "admins":
		h.handleCategoryAdmins(w, r, catID)
		return
	case "approved-users":
		h.handleCategoryApprovedUsers(w, r, catID)
		return
	case "":
		// fall through to category CRUD
	default:
		// Check for /admins/{userID} or /approved-users/{userID}
		subParts := strings.SplitN(subResource, "/", 2)
		if len(subParts) == 2 {
			switch subParts[0] {
			case "admins":
				h.handleCategoryAdminByUserID(w, r, catID, subParts[1])
				return
			case "approved-users":
				h.handleCategoryApprovedUserByUserID(w, r, catID, subParts[1])
				return
			}
		}
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	user := middleware.GetUserFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		cat, err := h.app.DB.GetCategory(catID)
		if err != nil {
			slog.Error("error getting category", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if cat == nil {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cat)

	case http.MethodPut:
		isCatAdmin, _ := h.app.DB.IsCategoryAdmin(user.ID, catID)
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin) && !isCatAdmin {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var cat db.Category
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &cat); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		cat.ID = catID

		if cat.Name == "" {
			http.Error(w, "Missing required field: name", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.UpdateCategory(cat); err != nil {
			if err.Error() == "sql: no rows in result set" {
				http.Error(w, "Category not found", http.StatusNotFound)
				return
			}
			slog.Error("error updating category", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "UPDATE_CATEGORY", fmt.Sprintf("Updated category: %s (%s)", cat.Name, catID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cat)

	case http.MethodDelete:
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin) {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		cat, err := h.app.DB.GetCategory(catID)
		if err != nil {
			slog.Error("error getting category", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if cat == nil {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}

		if err := h.app.DB.DeleteCategory(catID); err != nil {
			slog.Error("error deleting category", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "DELETE_CATEGORY", fmt.Sprintf("Deleted category: %s (%s)", cat.Name, catID))

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleCategoryAdmins(w http.ResponseWriter, r *http.Request, catID string) {
	user := middleware.GetUserFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		admins, err := h.app.DB.ListCategoryAdmins(catID)
		if err != nil {
			slog.Error("error listing category admins", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if admins == nil {
			admins = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(admins)

	case http.MethodPost:
		isCatAdmin, _ := h.app.DB.IsCategoryAdmin(user.ID, catID)
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin) && !isCatAdmin {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.UserID == "" {
			http.Error(w, "Missing required field: user_id", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.AddCategoryAdmin(catID, req.UserID); err != nil {
			slog.Error("error adding category admin", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "ADD_CATEGORY_ADMIN", fmt.Sprintf("Added admin %s to category %s", req.UserID, catID))

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleCategoryAdminByUserID(w http.ResponseWriter, r *http.Request, catID, userID string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentUser := middleware.GetUserFromContext(r.Context())
	isCatAdmin, _ := h.app.DB.IsCategoryAdmin(currentUser.ID, catID)
	if !middleware.HasRole(currentUser.Roles, middleware.RoleAdmin) && !isCatAdmin {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	if err := h.app.DB.RemoveCategoryAdmin(catID, userID); err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	h.app.DB.LogAudit(currentUser.Username, "REMOVE_CATEGORY_ADMIN", fmt.Sprintf("Removed admin %s from category %s", userID, catID))

	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) handleCategoryApprovedUsers(w http.ResponseWriter, r *http.Request, catID string) {
	user := middleware.GetUserFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		users, err := h.app.DB.ListCategoryApprovedUsers(catID)
		if err != nil {
			slog.Error("error listing category approved users", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if users == nil {
			users = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)

	case http.MethodPost:
		isCatAdmin, _ := h.app.DB.IsCategoryAdmin(user.ID, catID)
		if !middleware.HasRole(user.Roles, middleware.RoleAdmin) && !isCatAdmin {
			http.Error(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.UserID == "" {
			http.Error(w, "Missing required field: user_id", http.StatusBadRequest)
			return
		}

		if err := h.app.DB.AddCategoryApprovedUser(catID, req.UserID); err != nil {
			slog.Error("error adding category approved user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.app.DB.LogAudit(user.Username, "ADD_CATEGORY_APPROVED_USER", fmt.Sprintf("Added approved user %s to category %s", req.UserID, catID))

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handlers) handleCategoryApprovedUserByUserID(w http.ResponseWriter, r *http.Request, catID, userID string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentUser := middleware.GetUserFromContext(r.Context())
	isCatAdmin, _ := h.app.DB.IsCategoryAdmin(currentUser.ID, catID)
	if !middleware.HasRole(currentUser.Roles, middleware.RoleAdmin) && !isCatAdmin {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	if err := h.app.DB.RemoveCategoryApprovedUser(catID, userID); err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	h.app.DB.LogAudit(currentUser.Username, "REMOVE_CATEGORY_APPROVED_USER", fmt.Sprintf("Removed approved user %s from category %s", userID, catID))

	w.WriteHeader(http.StatusNoContent)
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

// docsHandler serves VitePress static files with clean URL support.
// VitePress generates .html files but uses clean URLs (no extension),
// so we try: exact path → path.html → path/index.html.
func (h *handlers) docsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Strip the /docs/ prefix to get the path within DocsFS
		path := strings.TrimPrefix(r.URL.Path, "/docs/")

		// Resolve the file path: try exact → .html → /index.html
		resolved := h.resolveDocsPath(path)
		if resolved == "" {
			http.NotFound(w, r)
			return
		}

		f, err := h.app.DocsFS.Open(resolved)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set content type for HTML files
		if strings.HasSuffix(resolved, ".html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		} else if strings.HasPrefix(resolved, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		http.ServeContent(w, r, resolved, stat.ModTime(), f.(io.ReadSeeker))
	}
}

// resolveDocsPath finds the actual file path in DocsFS for a clean URL.
func (h *handlers) resolveDocsPath(path string) string {
	// Root → index.html
	if path == "" || path == "/" {
		return "index.html"
	}
	path = strings.TrimSuffix(path, "/")

	// Exact match for non-directory files (assets like .js, .css, images)
	if info, err := fs.Stat(h.app.DocsFS, path); err == nil && !info.IsDir() {
		return path
	}
	// Clean URL → .html file
	if !strings.Contains(path, ".") {
		htmlPath := path + ".html"
		if _, err := fs.Stat(h.app.DocsFS, htmlPath); err == nil {
			return htmlPath
		}
	}
	// Directory → index.html
	indexPath := path + "/index.html"
	if _, err := fs.Stat(h.app.DocsFS, indexPath); err == nil {
		return indexPath
	}
	return ""
}

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
