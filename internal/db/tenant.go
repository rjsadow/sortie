package db

import (
	"database/sql"
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
	now := time.Now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	_, err := db.bun.NewInsert().Model(&tenant).Exec(ctx())
	return err
}

// GetTenant retrieves a tenant by ID
func (db *DB) GetTenant(id string) (*Tenant, error) {
	var t Tenant
	err := db.bun.NewSelect().Model(&t).Where("id = ?", id).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTenantBySlug retrieves a tenant by slug
func (db *DB) GetTenantBySlug(slug string) (*Tenant, error) {
	var t Tenant
	err := db.bun.NewSelect().Model(&t).Where("slug = ?", slug).Scan(ctx())
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTenants returns all tenants
func (db *DB) ListTenants() ([]Tenant, error) {
	var tenants []Tenant
	err := db.bun.NewSelect().Model(&tenants).OrderExpr("name").Scan(ctx())
	return tenants, err
}

// UpdateTenant updates an existing tenant
func (db *DB) UpdateTenant(tenant Tenant) error {
	tenant.UpdatedAt = time.Now()
	result, err := db.bun.NewUpdate().Model(&tenant).
		Column("name", "slug", "settings", "quotas", "updated_at").
		WherePK().
		Exec(ctx())
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

	result, err := db.bun.NewDelete().Model((*Tenant)(nil)).Where("id = ?", id).Exec(ctx())
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
	var apps []Application
	err := db.bun.NewSelect().Model(&apps).
		Where("tenant_id = ?", tenantID).
		OrderExpr("category, name").
		Scan(ctx())
	return apps, err
}

// ListUsersByTenant returns all users belonging to a tenant
func (db *DB) ListUsersByTenant(tenantID string) ([]User, error) {
	var users []User
	err := db.bun.NewSelect().Model(&users).
		Where("tenant_id = ?", tenantID).
		OrderExpr("created_at DESC").
		Scan(ctx())
	return users, err
}

// ListSessionsByTenant returns all active sessions belonging to a tenant
func (db *DB) ListSessionsByTenant(tenantID string) ([]Session, error) {
	var sessions []Session
	err := db.bun.NewSelect().Model(&sessions).
		Where("tenant_id = ?", tenantID).
		Where("status NOT IN (?, ?)", "terminated", "failed").
		OrderExpr("created_at DESC").
		Scan(ctx())
	return sessions, err
}

// CountActiveSessionsByTenant returns the number of active sessions for a tenant
func (db *DB) CountActiveSessionsByTenant(tenantID string) (int, error) {
	count, err := db.bun.NewSelect().Model((*Session)(nil)).
		Where("tenant_id = ?", tenantID).
		Where("status IN (?, ?)", "creating", "running").
		Count(ctx())
	return count, err
}

// CountUsersByTenant returns the number of users in a tenant
func (db *DB) CountUsersByTenant(tenantID string) (int, error) {
	count, err := db.bun.NewSelect().Model((*User)(nil)).
		Where("tenant_id = ?", tenantID).
		Count(ctx())
	return count, err
}

// CountAppsByTenant returns the number of apps in a tenant
func (db *DB) CountAppsByTenant(tenantID string) (int, error) {
	count, err := db.bun.NewSelect().Model((*Application)(nil)).
		Where("tenant_id = ?", tenantID).
		Count(ctx())
	return count, err
}

// LogAuditWithTenant creates a tenant-scoped audit log entry
func (db *DB) LogAuditWithTenant(tenantID, user, action, details string) error {
	log := AuditLog{
		TenantID: tenantID,
		User:     user,
		Action:   action,
		Details:  details,
	}
	_, err := db.bun.NewInsert().Model(&log).Exec(ctx())
	return err
}

// QueryAuditLogsByTenant returns audit logs for a specific tenant
func (db *DB) QueryAuditLogsByTenant(tenantID string, filter AuditLogFilter) (*AuditLogPage, error) {
	q := db.bun.NewSelect().Model((*AuditLog)(nil)).Where("tenant_id = ?", tenantID)

	if filter.User != "" {
		q = q.Where("\"user\" = ?", filter.User)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if !filter.From.IsZero() {
		q = q.Where("timestamp >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		q = q.Where("timestamp <= ?", filter.To)
	}

	total, err := q.Count(ctx())
	if err != nil {
		return nil, fmt.Errorf("failed to count audit logs: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	offset := max(filter.Offset, 0)

	var logs []AuditLog
	err = q.OrderExpr("timestamp DESC").Limit(limit).Offset(offset).Scan(ctx(), &logs)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}

	return &AuditLogPage{Logs: logs, Total: total}, nil
}
