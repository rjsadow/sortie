package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Tenant represents a tenant in the multi-tenant system
type Tenant struct {
	bun.BaseModel `bun:"table:tenants"`

	ID        string         `json:"id" bun:"id,pk"`
	Name      string         `json:"name" bun:"name,notnull"`
	Slug      string         `json:"slug" bun:"slug,unique,notnull"`
	Settings  TenantSettings `json:"settings" bun:"-"`
	Quotas    TenantQuotas   `json:"quotas" bun:"-"`
	CreatedAt time.Time      `json:"created_at" bun:"created_at,nullzero,notnull,default:current_timestamp"`
	UpdatedAt time.Time      `json:"updated_at" bun:"updated_at,nullzero,notnull,default:current_timestamp"`

	// JSON-serialized DB columns
	SettingsJSON string `json:"-" bun:"settings"`
	QuotasJSON   string `json:"-" bun:"quotas"`
}

// TenantSettings holds tenant-specific configuration
type TenantSettings struct {
	PrimaryColor   string `json:"primary_color,omitempty"`
	SecondaryColor string `json:"secondary_color,omitempty"`
	LogoURL        string `json:"logo_url,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
}

// TenantQuotas holds per-tenant resource quotas
type TenantQuotas struct {
	MaxSessionsPerUser int    `json:"max_sessions_per_user,omitempty"` // 0 = use global default
	MaxTotalSessions   int    `json:"max_total_sessions,omitempty"`   // 0 = unlimited
	MaxUsers           int    `json:"max_users,omitempty"`            // 0 = unlimited
	MaxApps            int    `json:"max_apps,omitempty"`             // 0 = unlimited
	DefaultCPURequest  string `json:"default_cpu_request,omitempty"`
	DefaultCPULimit    string `json:"default_cpu_limit,omitempty"`
	DefaultMemRequest  string `json:"default_mem_request,omitempty"`
	DefaultMemLimit    string `json:"default_mem_limit,omitempty"`
}

// DefaultTenantID is the ID of the default tenant used for backwards compatibility
const DefaultTenantID = "default"

// CreateTenant inserts a new tenant
func (db *DB) CreateTenant(tenant Tenant) error {
	settingsJSON, err := json.Marshal(tenant.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	quotasJSON, err := json.Marshal(tenant.Quotas)
	if err != nil {
		return fmt.Errorf("failed to marshal quotas: %w", err)
	}

	now := time.Now()
	_, err = db.conn.Exec(
		"INSERT INTO tenants (id, name, slug, settings, quotas, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		tenant.ID, tenant.Name, tenant.Slug, string(settingsJSON), string(quotasJSON), now, now,
	)
	return err
}

// GetTenant retrieves a tenant by ID
func (db *DB) GetTenant(id string) (*Tenant, error) {
	return db.scanTenant(
		"SELECT id, name, slug, settings, quotas, created_at, updated_at FROM tenants WHERE id = ?",
		id,
	)
}

// GetTenantBySlug retrieves a tenant by slug
func (db *DB) GetTenantBySlug(slug string) (*Tenant, error) {
	return db.scanTenant(
		"SELECT id, name, slug, settings, quotas, created_at, updated_at FROM tenants WHERE slug = ?",
		slug,
	)
}

func (db *DB) scanTenant(query string, args ...any) (*Tenant, error) {
	var t Tenant
	var settingsJSON, quotasJSON string
	err := db.conn.QueryRow(query, args...).Scan(
		&t.ID, &t.Name, &t.Slug, &settingsJSON, &quotasJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if settingsJSON != "" && settingsJSON != "{}" {
		json.Unmarshal([]byte(settingsJSON), &t.Settings)
	}
	if quotasJSON != "" && quotasJSON != "{}" {
		json.Unmarshal([]byte(quotasJSON), &t.Quotas)
	}

	return &t, nil
}

// ListTenants returns all tenants
func (db *DB) ListTenants() ([]Tenant, error) {
	rows, err := db.conn.Query(
		"SELECT id, name, slug, settings, quotas, created_at, updated_at FROM tenants ORDER BY name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []Tenant
	for rows.Next() {
		var t Tenant
		var settingsJSON, quotasJSON string
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &settingsJSON, &quotasJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if settingsJSON != "" && settingsJSON != "{}" {
			json.Unmarshal([]byte(settingsJSON), &t.Settings)
		}
		if quotasJSON != "" && quotasJSON != "{}" {
			json.Unmarshal([]byte(quotasJSON), &t.Quotas)
		}
		tenants = append(tenants, t)
	}

	return tenants, rows.Err()
}

// UpdateTenant updates an existing tenant
func (db *DB) UpdateTenant(tenant Tenant) error {
	settingsJSON, err := json.Marshal(tenant.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	quotasJSON, err := json.Marshal(tenant.Quotas)
	if err != nil {
		return fmt.Errorf("failed to marshal quotas: %w", err)
	}

	result, err := db.conn.Exec(
		"UPDATE tenants SET name = ?, slug = ?, settings = ?, quotas = ?, updated_at = ? WHERE id = ?",
		tenant.Name, tenant.Slug, string(settingsJSON), string(quotasJSON), time.Now(), tenant.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteTenant removes a tenant by ID
func (db *DB) DeleteTenant(id string) error {
	if id == DefaultTenantID {
		return fmt.Errorf("cannot delete the default tenant")
	}

	result, err := db.conn.Exec("DELETE FROM tenants WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Tenant-scoped query methods

// ListAppsByTenant returns all applications belonging to a tenant
func (db *DB) ListAppsByTenant(tenantID string) ([]Application, error) {
	rows, err := db.conn.Query(
		"SELECT id, name, description, url, icon, category, visibility, launch_type, os_type, container_image, container_port, container_args, cpu_request, cpu_limit, memory_request, memory_limit, egress_policy FROM applications WHERE tenant_id = ? ORDER BY category, name",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanApps(rows)
}

// ListUsersByTenant returns all users belonging to a tenant
func (db *DB) ListUsersByTenant(tenantID string) ([]User, error) {
	rows, err := db.conn.Query(
		"SELECT id, username, email, display_name, password_hash, roles, auth_provider, auth_provider_id, tenant_id, tenant_roles, created_at, updated_at FROM users WHERE tenant_id = ? ORDER BY created_at DESC",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}

// ListSessionsByTenant returns all active sessions belonging to a tenant
func (db *DB) ListSessionsByTenant(tenantID string) ([]Session, error) {
	rows, err := db.conn.Query(
		"SELECT id, user_id, app_id, pod_name, pod_ip, status, idle_timeout, tenant_id, created_at, updated_at FROM sessions WHERE tenant_id = ? AND status NOT IN ('terminated', 'failed') ORDER BY created_at DESC",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSessions(rows)
}

// CountActiveSessionsByTenant returns the number of active sessions for a tenant
func (db *DB) CountActiveSessionsByTenant(tenantID string) (int, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE tenant_id = ? AND status IN ('creating', 'running')",
		tenantID,
	).Scan(&count)
	return count, err
}

// CountUsersByTenant returns the number of users in a tenant
func (db *DB) CountUsersByTenant(tenantID string) (int, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM users WHERE tenant_id = ?", tenantID).Scan(&count)
	return count, err
}

// CountAppsByTenant returns the number of apps in a tenant
func (db *DB) CountAppsByTenant(tenantID string) (int, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM applications WHERE tenant_id = ?", tenantID).Scan(&count)
	return count, err
}

// LogAuditWithTenant creates a tenant-scoped audit log entry
func (db *DB) LogAuditWithTenant(tenantID, user, action, details string) error {
	_, err := db.conn.Exec(
		"INSERT INTO audit_log (tenant_id, user, action, details) VALUES (?, ?, ?, ?)",
		tenantID, user, action, details,
	)
	return err
}

// QueryAuditLogsByTenant returns audit logs for a specific tenant
func (db *DB) QueryAuditLogsByTenant(tenantID string, filter AuditLogFilter) (*AuditLogPage, error) {
	where := "WHERE tenant_id = ?"
	args := []any{tenantID}

	if filter.User != "" {
		where += " AND user = ?"
		args = append(args, filter.User)
	}
	if filter.Action != "" {
		where += " AND action = ?"
		args = append(args, filter.Action)
	}
	if !filter.From.IsZero() {
		where += " AND timestamp >= ?"
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		where += " AND timestamp <= ?"
		args = append(args, filter.To)
	}

	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM audit_log "+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count audit logs: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := max(filter.Offset, 0)

	query := "SELECT id, timestamp, user, action, details FROM audit_log " + where + " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, limit, offset)

	rows, err := db.conn.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.User, &l.Action, &l.Details); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &AuditLogPage{Logs: logs, Total: total}, nil
}

// helper to scan application rows (shared between ListApps and ListAppsByTenant)
func scanApps(rows *sql.Rows) ([]Application, error) {
	var apps []Application
	for rows.Next() {
		var app Application
		var visibility string
		var launchType, osType, containerImage string
		var containerPort int
		var containerArgsJSON string
		var cpuRequest, cpuLimit, memoryRequest, memoryLimit string
		var egressPolicyJSON string
		if err := rows.Scan(&app.ID, &app.Name, &app.Description, &app.URL, &app.Icon, &app.Category, &visibility, &launchType, &osType, &containerImage, &containerPort, &containerArgsJSON, &cpuRequest, &cpuLimit, &memoryRequest, &memoryLimit, &egressPolicyJSON); err != nil {
			return nil, err
		}
		app.Visibility = CategoryVisibility(visibility)
		if app.Visibility == "" {
			app.Visibility = CategoryVisibilityPublic
		}
		app.LaunchType = LaunchType(launchType)
		if app.LaunchType == "" {
			app.LaunchType = LaunchTypeURL
		}
		app.OsType = osType
		if app.OsType == "" {
			app.OsType = "linux"
		}
		app.ContainerImage = containerImage
		app.ContainerPort = containerPort
		if containerArgsJSON != "" && containerArgsJSON != "[]" {
			json.Unmarshal([]byte(containerArgsJSON), &app.ContainerArgs)
		}
		if cpuRequest != "" || cpuLimit != "" || memoryRequest != "" || memoryLimit != "" {
			app.ResourceLimits = &ResourceLimits{
				CPURequest:    cpuRequest,
				CPULimit:      cpuLimit,
				MemoryRequest: memoryRequest,
				MemoryLimit:   memoryLimit,
			}
		}
		if egressPolicyJSON != "" {
			var ep EgressPolicy
			if json.Unmarshal([]byte(egressPolicyJSON), &ep) == nil && ep.Mode != "" {
				app.EgressPolicy = &ep
			}
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

// helper to scan user rows
func scanUsers(rows *sql.Rows) ([]User, error) {
	var users []User
	for rows.Next() {
		var user User
		var rolesJSON string
		var authProvider, authProviderID sql.NullString
		var tenantID sql.NullString
		var tenantRolesJSON sql.NullString
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash, &rolesJSON, &authProvider, &authProviderID, &tenantID, &tenantRolesJSON, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(rolesJSON), &user.Roles); err != nil {
			user.Roles = []string{"user"}
		}
		if authProvider.Valid {
			user.AuthProvider = authProvider.String
		}
		if authProviderID.Valid {
			user.AuthProviderID = authProviderID.String
		}
		if tenantID.Valid {
			user.TenantID = tenantID.String
		}
		if tenantRolesJSON.Valid && tenantRolesJSON.String != "" && tenantRolesJSON.String != "[]" {
			json.Unmarshal([]byte(tenantRolesJSON.String), &user.TenantRoles)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// helper to scan session rows
func scanSessions(rows *sql.Rows) ([]Session, error) {
	var sessions []Session
	for rows.Next() {
		var session Session
		var status string
		var tenantID sql.NullString
		if err := rows.Scan(&session.ID, &session.UserID, &session.AppID, &session.PodName, &session.PodIP, &status, &session.IdleTimeout, &tenantID, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		session.Status = SessionStatus(status)
		if tenantID.Valid {
			session.TenantID = tenantID.String
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
