// Package server provides the HTTP handler assembly for the Sortie application.
// It accepts all dependencies as parameters so that both main() and tests
// can build the same handler chain without route drift.
package server

import (
	"io/fs"
	"net/http"

	"github.com/rjsadow/sortie/internal/config"
	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/diagnostics"
	"github.com/rjsadow/sortie/internal/files"
	"github.com/rjsadow/sortie/internal/gateway"
	"github.com/rjsadow/sortie/internal/middleware"
	"github.com/rjsadow/sortie/internal/plugins/auth"
	"github.com/rjsadow/sortie/internal/sessions"
)

// App holds all dependencies needed to build the HTTP handler.
type App struct {
	DB                  *db.DB
	SessionManager      *sessions.Manager
	JWTAuth             *auth.JWTAuthProvider
	OIDCAuth            *auth.OIDCAuthProvider
	GatewayHandler      *gateway.Handler
	BackpressureHandler *sessions.BackpressureHandler
	FileHandler         *files.Handler
	DiagCollector       *diagnostics.Collector
	Config              *config.Config
	StaticFS            fs.FS // web/dist content (nil disables static serving)
	DocsFS              fs.FS // docs-site/dist content (nil disables docs serving)
}

// Handler builds and returns the complete HTTP handler with all routes
// registered and middleware applied.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	// Bind the handlers package to this App's dependencies.
	h := &handlers{app: a}

	// Observability endpoints (public, no auth required)
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)
	mux.HandleFunc("/api/load", a.BackpressureHandler.ServeLoadStatus)

	// Auth routes (public)
	mux.HandleFunc("/api/auth/login", h.handleLogin)
	mux.HandleFunc("/api/auth/logout", h.handleLogout)
	mux.HandleFunc("/api/auth/refresh", h.handleRefreshToken)
	mux.HandleFunc("/api/auth/me", h.handleAuthMe)
	mux.HandleFunc("/api/auth/register", h.handleRegister)

	// OIDC/SSO routes (public)
	mux.HandleFunc("/api/auth/oidc/login", h.handleOIDCLogin)
	mux.HandleFunc("/api/auth/oidc/callback", h.handleOIDCCallback)

	// Config route (public)
	mux.HandleFunc("/api/config", h.handleConfig)

	// Protected API routes
	authMiddleware := middleware.AuthMiddleware(a.JWTAuth)
	tenantMiddleware := middleware.TenantMiddleware(a.DB)
	requireAdmin := middleware.RequireRole(middleware.RoleAdmin)

	withTenant := func(handler http.Handler) http.Handler {
		return authMiddleware(tenantMiddleware(handler))
	}

	// Admin routes (protected, admin-only)
	mux.Handle("/api/admin/settings", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminSettings))))
	mux.Handle("/api/admin/users", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminUsers))))
	mux.Handle("/api/admin/users/", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminUserByID))))
	mux.Handle("/api/admin/sessions", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminSessions))))
	mux.Handle("/api/admin/templates", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminTemplates))))
	mux.Handle("/api/admin/templates/", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminTemplateByID))))

	// Enterprise support endpoints (admin-only)
	mux.Handle("/api/admin/diagnostics", authMiddleware(requireAdmin(http.HandlerFunc(h.handleDiagnosticsBundle))))
	mux.Handle("/api/admin/health", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminHealth))))
	mux.Handle("/api/admin/support/info", authMiddleware(requireAdmin(http.HandlerFunc(h.handleSupportInfo))))

	// Tenant admin routes (protected, admin-only)
	mux.Handle("/api/admin/tenants", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminTenants))))
	mux.Handle("/api/admin/tenants/", authMiddleware(requireAdmin(http.HandlerFunc(h.handleAdminTenantByID))))

	// User list endpoint (auth-protected, non-admin)
	mux.Handle("/api/users", authMiddleware(http.HandlerFunc(h.handleUsersList)))

	// Public template endpoints
	mux.HandleFunc("/api/templates", h.handleTemplates)
	mux.HandleFunc("/api/templates/", h.handleTemplateByID)

	// Category routes: tenant-scoped
	mux.Handle("/api/categories", withTenant(http.HandlerFunc(h.handleCategories)))
	mux.Handle("/api/categories/", withTenant(http.HandlerFunc(h.handleCategoryByID)))

	// App and AppSpec routes: tenant-scoped
	mux.Handle("/api/appspecs", withTenant(http.HandlerFunc(h.handleAppSpecs)))
	mux.Handle("/api/appspecs/", withTenant(http.HandlerFunc(h.handleAppSpecByID)))
	mux.Handle("/api/apps", withTenant(http.HandlerFunc(h.handleApps)))
	mux.Handle("/api/apps/", withTenant(http.HandlerFunc(h.handleAppByID)))

	// Audit logs: admin only, tenant-scoped
	mux.Handle("/api/audit", withTenant(requireAdmin(http.HandlerFunc(h.handleAuditLogs))))
	mux.Handle("/api/audit/export", withTenant(requireAdmin(http.HandlerFunc(h.handleAuditExport))))
	mux.Handle("/api/audit/filters", withTenant(requireAdmin(http.HandlerFunc(h.handleAuditFilters))))

	// Analytics
	mux.Handle("/api/analytics/launch", withTenant(http.HandlerFunc(h.handleAnalyticsLaunch)))
	mux.Handle("/api/analytics/stats", withTenant(requireAdmin(http.HandlerFunc(h.handleAnalyticsStats))))

	// Session API routes
	mux.Handle("/api/sessions", withTenant(http.HandlerFunc(h.handleSessions)))
	mux.Handle("/api/sessions/", withTenant(http.HandlerFunc(h.handleSessionByID)))

	// Quota API route
	mux.Handle("/api/quotas", withTenant(http.HandlerFunc(h.handleQuotas)))

	// WebSocket routes
	if a.GatewayHandler != nil {
		mux.Handle("/ws/sessions/", a.GatewayHandler)
		mux.Handle("/ws/guac/sessions/", a.GatewayHandler)
	}

	// Legacy apps.json
	mux.HandleFunc("/apps.json", h.handleAppsJSON)

	// Documentation site (VitePress static files)
	if a.DocsFS != nil {
		mux.HandleFunc("/docs/", h.docsHandler())
		mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
		})
	}

	// Static file serving and SPA routing
	if a.StaticFS != nil {
		fileServer := http.FileServer(http.FS(a.StaticFS))
		mux.HandleFunc("/", h.staticHandler(fileServer))
	}

	// Wrap with middleware
	return middleware.SecurityHeaders(middleware.RequestID(mux))
}
