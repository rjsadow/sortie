package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rjsadow/launchpad/internal/db"
)

// handleAdminTenants handles GET (list) and POST (create) for /api/admin/tenants
func handleAdminTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenants, err := database.ListTenants()
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

		// Check slug uniqueness
		existing, err := database.GetTenantBySlug(req.Slug)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			http.Error(w, "Tenant slug already exists", http.StatusConflict)
			return
		}

		tenant := db.Tenant{
			ID:        uuid.New().String(),
			Name:      req.Name,
			Slug:      req.Slug,
			Settings:  req.Settings,
			Quotas:    req.Quotas,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := database.CreateTenant(tenant); err != nil {
			slog.Error("error creating tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		database.LogAudit("admin", "CREATE_TENANT", "Created tenant: "+tenant.Name)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tenant)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminTenantByID handles GET, PUT, DELETE for /api/admin/tenants/{id}
func handleAdminTenantByID(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from URL path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/tenants/"), "/")
	tenantID := parts[0]
	if tenantID == "" {
		http.Error(w, "Tenant ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tenant, err := database.GetTenant(tenantID)
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

		tenant, err := database.GetTenant(tenantID)
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

		if err := database.UpdateTenant(*tenant); err != nil {
			slog.Error("error updating tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		database.LogAudit("admin", "UPDATE_TENANT", "Updated tenant: "+tenant.Name)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tenant)

	case http.MethodDelete:
		if tenantID == db.DefaultTenantID {
			http.Error(w, "Cannot delete the default tenant", http.StatusForbidden)
			return
		}

		if err := database.DeleteTenant(tenantID); err != nil {
			slog.Error("error deleting tenant", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		database.LogAudit("admin", "DELETE_TENANT", "Deleted tenant: "+tenantID)

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
