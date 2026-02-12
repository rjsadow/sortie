package db

import (
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// --- Category CRUD ---

// CreateCategory inserts a new category
func (db *DB) CreateCategory(cat Category) error {
	if cat.TenantID == "" {
		cat.TenantID = DefaultTenantID
	}
	now := time.Now()
	_, err := db.conn.Exec(
		"INSERT INTO categories (id, name, description, tenant_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		cat.ID, cat.Name, cat.Description, cat.TenantID, now, now,
	)
	return err
}

// GetCategory retrieves a category by ID
func (db *DB) GetCategory(id string) (*Category, error) {
	var cat Category
	err := db.conn.QueryRow(
		"SELECT id, name, description, tenant_id, created_at, updated_at FROM categories WHERE id = ?",
		id,
	).Scan(&cat.ID, &cat.Name, &cat.Description, &cat.TenantID, &cat.CreatedAt, &cat.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

// GetCategoryByName retrieves a category by name
func (db *DB) GetCategoryByName(name string) (*Category, error) {
	var cat Category
	err := db.conn.QueryRow(
		"SELECT id, name, description, tenant_id, created_at, updated_at FROM categories WHERE name = ?",
		name,
	).Scan(&cat.ID, &cat.Name, &cat.Description, &cat.TenantID, &cat.CreatedAt, &cat.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

// ListCategories returns all categories
func (db *DB) ListCategories() ([]Category, error) {
	rows, err := db.conn.Query(
		"SELECT id, name, description, tenant_id, created_at, updated_at FROM categories ORDER BY name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCategories(rows)
}

// ListCategoriesByTenant returns categories for a specific tenant
func (db *DB) ListCategoriesByTenant(tenantID string) ([]Category, error) {
	rows, err := db.conn.Query(
		"SELECT id, name, description, tenant_id, created_at, updated_at FROM categories WHERE tenant_id = ? ORDER BY name",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCategories(rows)
}

// UpdateCategory updates an existing category
func (db *DB) UpdateCategory(cat Category) error {
	result, err := db.conn.Exec(
		"UPDATE categories SET name = ?, description = ?, updated_at = ? WHERE id = ?",
		cat.Name, cat.Description, time.Now(), cat.ID,
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

// DeleteCategory removes a category by ID
func (db *DB) DeleteCategory(id string) error {
	result, err := db.conn.Exec("DELETE FROM categories WHERE id = ?", id)
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
	// Clean up junction tables
	db.conn.Exec("DELETE FROM category_admins WHERE category_id = ?", id)
	db.conn.Exec("DELETE FROM category_approved_users WHERE category_id = ?", id)
	return nil
}

// --- Category admin management ---

// AddCategoryAdmin adds a user as admin of a category
func (db *DB) AddCategoryAdmin(categoryID, userID string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO category_admins (category_id, user_id) VALUES (?, ?)",
		categoryID, userID,
	)
	return err
}

// RemoveCategoryAdmin removes a user as admin of a category
func (db *DB) RemoveCategoryAdmin(categoryID, userID string) error {
	result, err := db.conn.Exec(
		"DELETE FROM category_admins WHERE category_id = ? AND user_id = ?",
		categoryID, userID,
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

// ListCategoryAdmins returns all admin user IDs for a category
func (db *DB) ListCategoryAdmins(categoryID string) ([]string, error) {
	rows, err := db.conn.Query(
		"SELECT user_id FROM category_admins WHERE category_id = ? ORDER BY user_id",
		categoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// IsCategoryAdmin checks if a user is an admin of a category
func (db *DB) IsCategoryAdmin(userID, categoryID string) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM category_admins WHERE category_id = ? AND user_id = ?",
		categoryID, userID,
	).Scan(&count)
	return count > 0, err
}

// GetCategoriesAdminedByUser returns category IDs that a user admins
func (db *DB) GetCategoriesAdminedByUser(userID string) ([]string, error) {
	rows, err := db.conn.Query(
		"SELECT category_id FROM category_admins WHERE user_id = ? ORDER BY category_id",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// --- Approved user management ---

// AddCategoryApprovedUser adds a user to a category's approved list
func (db *DB) AddCategoryApprovedUser(categoryID, userID string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO category_approved_users (category_id, user_id) VALUES (?, ?)",
		categoryID, userID,
	)
	return err
}

// RemoveCategoryApprovedUser removes a user from a category's approved list
func (db *DB) RemoveCategoryApprovedUser(categoryID, userID string) error {
	result, err := db.conn.Exec(
		"DELETE FROM category_approved_users WHERE category_id = ? AND user_id = ?",
		categoryID, userID,
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

// ListCategoryApprovedUsers returns all approved user IDs for a category
func (db *DB) ListCategoryApprovedUsers(categoryID string) ([]string, error) {
	rows, err := db.conn.Query(
		"SELECT user_id FROM category_approved_users WHERE category_id = ? ORDER BY user_id",
		categoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// IsCategoryApprovedUser checks if a user is approved for a category
func (db *DB) IsCategoryApprovedUser(userID, categoryID string) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM category_approved_users WHERE category_id = ? AND user_id = ?",
		categoryID, userID,
	).Scan(&count)
	return count > 0, err
}

// --- Visibility-filtered queries ---

// ListVisibleCategoriesForUser returns categories that contain at least one
// app visible to the user. System admins see all categories.
func (db *DB) ListVisibleCategoriesForUser(userID string, userRoles []string, tenantID string) ([]Category, error) {
	// System admins see everything
	if slices.Contains(userRoles, "admin") {
		return db.ListCategoriesByTenant(tenantID)
	}

	rows, err := db.conn.Query(`
		SELECT DISTINCT c.id, c.name, c.description, c.tenant_id, c.created_at, c.updated_at
		FROM categories c
		INNER JOIN applications a ON a.category = c.name AND a.tenant_id = ?
		LEFT JOIN category_admins ca ON c.id = ca.category_id AND ca.user_id = ?
		LEFT JOIN category_approved_users cau ON c.id = cau.category_id AND cau.user_id = ?
		WHERE c.tenant_id = ?
		AND (a.visibility = 'public'
		     OR (a.visibility = 'approved' AND (ca.user_id IS NOT NULL OR cau.user_id IS NOT NULL))
		     OR (a.visibility = 'admin_only' AND ca.user_id IS NOT NULL))
		ORDER BY c.name`,
		tenantID, userID, userID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCategories(rows)
}

// ListAppsForUser returns apps filtered by app-level visibility.
// System admins get all apps for the tenant.
func (db *DB) ListAppsForUser(userID string, userRoles []string, tenantID string) ([]Application, error) {
	// System admins see everything
	if slices.Contains(userRoles, "admin") {
		return db.ListAppsByTenant(tenantID)
	}

	rows, err := db.conn.Query(`
		SELECT a.id, a.name, a.description, a.url, a.icon, a.category,
		       a.visibility, a.launch_type, a.os_type, a.container_image, a.container_port,
		       a.container_args, a.cpu_request, a.cpu_limit, a.memory_request,
		       a.memory_limit, a.egress_policy
		FROM applications a
		LEFT JOIN categories c ON a.category = c.name AND c.tenant_id = ?
		LEFT JOIN category_admins ca ON c.id = ca.category_id AND ca.user_id = ?
		LEFT JOIN category_approved_users cau ON c.id = cau.category_id AND cau.user_id = ?
		WHERE a.tenant_id = ?
		AND (a.visibility = 'public'
		     OR (a.visibility = 'approved' AND (ca.user_id IS NOT NULL OR cau.user_id IS NOT NULL))
		     OR (a.visibility = 'admin_only' AND ca.user_id IS NOT NULL))
		ORDER BY a.category, a.name`,
		tenantID, userID, userID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApps(rows)
}

// EnsureCategoryExists creates a category for the given name if it doesn't exist.
// Returns the category ID. Used for backwards-compatible auto-creation.
func (db *DB) EnsureCategoryExists(name, tenantID string) (string, error) {
	if name == "" {
		return "", nil
	}
	existing, err := db.GetCategoryByName(name)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return existing.ID, nil
	}

	id := fmt.Sprintf("cat-%s-%d", name, time.Now().UnixNano())
	cat := Category{
		ID:       id,
		Name:     name,
		TenantID: tenantID,
	}
	if err := db.CreateCategory(cat); err != nil {
		return "", err
	}
	return id, nil
}

// --- helpers ---

func scanCategories(rows *sql.Rows) ([]Category, error) {
	var cats []Category
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Description, &cat.TenantID, &cat.CreatedAt, &cat.UpdatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}

func scanStringColumn(rows *sql.Rows) ([]string, error) {
	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
