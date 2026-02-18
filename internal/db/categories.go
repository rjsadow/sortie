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
	cat.CreatedAt = now
	cat.UpdatedAt = now
	_, err := db.bun.NewInsert().Model(&cat).Exec(ctx())
	return err
}

// GetCategory retrieves a category by ID
func (db *DB) GetCategory(id string) (*Category, error) {
	var cat Category
	err := db.bun.NewSelect().Model(&cat).Where("id = ?", id).Scan(ctx())
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
	err := db.bun.NewSelect().Model(&cat).Where("name = ?", name).Scan(ctx())
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
	var cats []Category
	err := db.bun.NewSelect().Model(&cats).OrderExpr("name").Scan(ctx())
	return cats, err
}

// ListCategoriesByTenant returns categories for a specific tenant
func (db *DB) ListCategoriesByTenant(tenantID string) ([]Category, error) {
	var cats []Category
	err := db.bun.NewSelect().Model(&cats).Where("tenant_id = ?", tenantID).OrderExpr("name").Scan(ctx())
	return cats, err
}

// UpdateCategory updates an existing category
func (db *DB) UpdateCategory(cat Category) error {
	result, err := db.bun.NewUpdate().Model((*Category)(nil)).
		Set("name = ?", cat.Name).
		Set("description = ?", cat.Description).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", cat.ID).
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

// DeleteCategory removes a category by ID
func (db *DB) DeleteCategory(id string) error {
	result, err := db.bun.NewDelete().Model((*Category)(nil)).Where("id = ?", id).Exec(ctx())
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
	db.bun.NewDelete().Model((*CategoryAdmin)(nil)).Where("category_id = ?", id).Exec(ctx())
	db.bun.NewDelete().Model((*CategoryApprovedUser)(nil)).Where("category_id = ?", id).Exec(ctx())
	return nil
}

// --- Category admin management ---

// AddCategoryAdmin adds a user as admin of a category
func (db *DB) AddCategoryAdmin(categoryID, userID string) error {
	admin := CategoryAdmin{CategoryID: categoryID, UserID: userID}
	_, err := db.bun.NewInsert().Model(&admin).On("CONFLICT DO NOTHING").Exec(ctx())
	return err
}

// RemoveCategoryAdmin removes a user as admin of a category
func (db *DB) RemoveCategoryAdmin(categoryID, userID string) error {
	result, err := db.bun.NewDelete().Model((*CategoryAdmin)(nil)).
		Where("category_id = ?", categoryID).
		Where("user_id = ?", userID).
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

// ListCategoryAdmins returns all admin user IDs for a category
func (db *DB) ListCategoryAdmins(categoryID string) ([]string, error) {
	var userIDs []string
	err := db.bun.NewSelect().Model((*CategoryAdmin)(nil)).
		Column("user_id").
		Where("category_id = ?", categoryID).
		OrderExpr("user_id").
		Scan(ctx(), &userIDs)
	return userIDs, err
}

// IsCategoryAdmin checks if a user is an admin of a category
func (db *DB) IsCategoryAdmin(userID, categoryID string) (bool, error) {
	count, err := db.bun.NewSelect().Model((*CategoryAdmin)(nil)).
		Where("category_id = ?", categoryID).
		Where("user_id = ?", userID).
		Count(ctx())
	return count > 0, err
}

// GetCategoriesAdminedByUser returns category IDs that a user admins
func (db *DB) GetCategoriesAdminedByUser(userID string) ([]string, error) {
	var categoryIDs []string
	err := db.bun.NewSelect().Model((*CategoryAdmin)(nil)).
		Column("category_id").
		Where("user_id = ?", userID).
		OrderExpr("category_id").
		Scan(ctx(), &categoryIDs)
	return categoryIDs, err
}

// --- Approved user management ---

// AddCategoryApprovedUser adds a user to a category's approved list
func (db *DB) AddCategoryApprovedUser(categoryID, userID string) error {
	approved := CategoryApprovedUser{CategoryID: categoryID, UserID: userID}
	_, err := db.bun.NewInsert().Model(&approved).On("CONFLICT DO NOTHING").Exec(ctx())
	return err
}

// RemoveCategoryApprovedUser removes a user from a category's approved list
func (db *DB) RemoveCategoryApprovedUser(categoryID, userID string) error {
	result, err := db.bun.NewDelete().Model((*CategoryApprovedUser)(nil)).
		Where("category_id = ?", categoryID).
		Where("user_id = ?", userID).
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

// ListCategoryApprovedUsers returns all approved user IDs for a category
func (db *DB) ListCategoryApprovedUsers(categoryID string) ([]string, error) {
	var userIDs []string
	err := db.bun.NewSelect().Model((*CategoryApprovedUser)(nil)).
		Column("user_id").
		Where("category_id = ?", categoryID).
		OrderExpr("user_id").
		Scan(ctx(), &userIDs)
	return userIDs, err
}

// IsCategoryApprovedUser checks if a user is approved for a category
func (db *DB) IsCategoryApprovedUser(userID, categoryID string) (bool, error) {
	count, err := db.bun.NewSelect().Model((*CategoryApprovedUser)(nil)).
		Where("category_id = ?", categoryID).
		Where("user_id = ?", userID).
		Count(ctx())
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

	var cats []Category
	err := db.bun.NewRaw(`
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
	).Scan(ctx(), &cats)
	return cats, err
}

// ListAppsForUser returns apps filtered by app-level visibility.
// System admins get all apps for the tenant.
func (db *DB) ListAppsForUser(userID string, userRoles []string, tenantID string) ([]Application, error) {
	// System admins see everything
	if slices.Contains(userRoles, "admin") {
		return db.ListAppsByTenant(tenantID)
	}

	var apps []Application
	err := db.bun.NewRaw(`
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
	).Scan(ctx(), &apps)
	return apps, err
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
