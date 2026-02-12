package db

import (
	"database/sql"
	"testing"
)

func TestCategoryCRUD(t *testing.T) {
	db := setupTestDB(t)

	t.Run("create and get category", func(t *testing.T) {
		cat := Category{
			ID:          "cat-dev-1",
			Name:        "Development",
			Description: "Dev tools",
			TenantID:    "default",
		}
		if err := db.CreateCategory(cat); err != nil {
			t.Fatalf("CreateCategory() error = %v", err)
		}

		got, err := db.GetCategory("cat-dev-1")
		if err != nil {
			t.Fatalf("GetCategory() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetCategory() returned nil")
		}
		if got.Name != "Development" {
			t.Errorf("got Name = %s, want Development", got.Name)
		}
		if got.TenantID != "default" {
			t.Errorf("got TenantID = %s, want default", got.TenantID)
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
	})

	t.Run("get by name", func(t *testing.T) {
		got, err := db.GetCategoryByName("Development")
		if err != nil {
			t.Fatalf("GetCategoryByName() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetCategoryByName() returned nil")
		}
		if got.ID != "cat-dev-1" {
			t.Errorf("got ID = %s, want cat-dev-1", got.ID)
		}
	})

	t.Run("get nonexistent category", func(t *testing.T) {
		got, err := db.GetCategory("nonexistent")
		if err != nil {
			t.Fatalf("GetCategory() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("create second category", func(t *testing.T) {
		cat := Category{
			ID:       "cat-ops-1",
			Name:     "Operations",
			TenantID: "default",
		}
		if err := db.CreateCategory(cat); err != nil {
			t.Fatalf("CreateCategory() error = %v", err)
		}
	})

	t.Run("list categories", func(t *testing.T) {
		cats, err := db.ListCategories()
		if err != nil {
			t.Fatalf("ListCategories() error = %v", err)
		}
		if len(cats) != 2 {
			t.Fatalf("ListCategories() returned %d, want 2", len(cats))
		}
		// Sorted by name
		if cats[0].Name != "Development" {
			t.Errorf("first category = %s, want Development", cats[0].Name)
		}
		if cats[1].Name != "Operations" {
			t.Errorf("second category = %s, want Operations", cats[1].Name)
		}
	})

	t.Run("list by tenant", func(t *testing.T) {
		cats, err := db.ListCategoriesByTenant("default")
		if err != nil {
			t.Fatalf("ListCategoriesByTenant() error = %v", err)
		}
		if len(cats) != 2 {
			t.Fatalf("got %d, want 2", len(cats))
		}

		cats, err = db.ListCategoriesByTenant("other-tenant")
		if err != nil {
			t.Fatalf("ListCategoriesByTenant() error = %v", err)
		}
		if len(cats) != 0 {
			t.Fatalf("got %d, want 0", len(cats))
		}
	})

	t.Run("update category", func(t *testing.T) {
		cat := Category{
			ID:          "cat-dev-1",
			Name:        "Dev Tools",
			Description: "Updated",
		}
		if err := db.UpdateCategory(cat); err != nil {
			t.Fatalf("UpdateCategory() error = %v", err)
		}
		got, _ := db.GetCategory("cat-dev-1")
		if got.Name != "Dev Tools" {
			t.Errorf("got Name = %s, want Dev Tools", got.Name)
		}
		if got.Description != "Updated" {
			t.Errorf("got Description = %s, want Updated", got.Description)
		}
	})

	t.Run("update nonexistent category", func(t *testing.T) {
		err := db.UpdateCategory(Category{ID: "nonexistent", Name: "x"})
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete category", func(t *testing.T) {
		if err := db.DeleteCategory("cat-ops-1"); err != nil {
			t.Fatalf("DeleteCategory() error = %v", err)
		}
		got, _ := db.GetCategory("cat-ops-1")
		if got != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete nonexistent category", func(t *testing.T) {
		err := db.DeleteCategory("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestCategoryAdminJunction(t *testing.T) {
	db := setupTestDB(t)

	// Create test category
	db.CreateCategory(Category{ID: "cat-1", Name: "Cat1", TenantID: "default"})

	t.Run("add and check admin", func(t *testing.T) {
		if err := db.AddCategoryAdmin("cat-1", "user-a"); err != nil {
			t.Fatalf("AddCategoryAdmin() error = %v", err)
		}

		isAdmin, err := db.IsCategoryAdmin("user-a", "cat-1")
		if err != nil {
			t.Fatalf("IsCategoryAdmin() error = %v", err)
		}
		if !isAdmin {
			t.Error("expected user-a to be admin of cat-1")
		}

		isAdmin, _ = db.IsCategoryAdmin("user-b", "cat-1")
		if isAdmin {
			t.Error("expected user-b NOT to be admin of cat-1")
		}
	})

	t.Run("add duplicate admin is idempotent", func(t *testing.T) {
		if err := db.AddCategoryAdmin("cat-1", "user-a"); err != nil {
			t.Fatalf("duplicate AddCategoryAdmin() error = %v", err)
		}
	})

	t.Run("list admins", func(t *testing.T) {
		db.AddCategoryAdmin("cat-1", "user-b")
		admins, err := db.ListCategoryAdmins("cat-1")
		if err != nil {
			t.Fatalf("ListCategoryAdmins() error = %v", err)
		}
		if len(admins) != 2 {
			t.Fatalf("got %d admins, want 2", len(admins))
		}
	})

	t.Run("get categories admined by user", func(t *testing.T) {
		db.CreateCategory(Category{ID: "cat-2", Name: "Cat2", TenantID: "default"})
		db.AddCategoryAdmin("cat-2", "user-a")

		catIDs, err := db.GetCategoriesAdminedByUser("user-a")
		if err != nil {
			t.Fatalf("GetCategoriesAdminedByUser() error = %v", err)
		}
		if len(catIDs) != 2 {
			t.Fatalf("got %d categories, want 2", len(catIDs))
		}
	})

	t.Run("remove admin", func(t *testing.T) {
		if err := db.RemoveCategoryAdmin("cat-1", "user-b"); err != nil {
			t.Fatalf("RemoveCategoryAdmin() error = %v", err)
		}
		isAdmin, _ := db.IsCategoryAdmin("user-b", "cat-1")
		if isAdmin {
			t.Error("expected user-b NOT to be admin after removal")
		}
	})

	t.Run("remove nonexistent admin", func(t *testing.T) {
		err := db.RemoveCategoryAdmin("cat-1", "user-z")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestCategoryApprovedUserJunction(t *testing.T) {
	db := setupTestDB(t)

	db.CreateCategory(Category{ID: "cat-1", Name: "Cat1", TenantID: "default"})

	t.Run("add and check approved user", func(t *testing.T) {
		if err := db.AddCategoryApprovedUser("cat-1", "user-a"); err != nil {
			t.Fatalf("AddCategoryApprovedUser() error = %v", err)
		}

		isApproved, err := db.IsCategoryApprovedUser("user-a", "cat-1")
		if err != nil {
			t.Fatalf("IsCategoryApprovedUser() error = %v", err)
		}
		if !isApproved {
			t.Error("expected user-a to be approved for cat-1")
		}

		isApproved, _ = db.IsCategoryApprovedUser("user-b", "cat-1")
		if isApproved {
			t.Error("expected user-b NOT to be approved for cat-1")
		}
	})

	t.Run("list approved users", func(t *testing.T) {
		db.AddCategoryApprovedUser("cat-1", "user-b")
		users, err := db.ListCategoryApprovedUsers("cat-1")
		if err != nil {
			t.Fatalf("ListCategoryApprovedUsers() error = %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
	})

	t.Run("remove approved user", func(t *testing.T) {
		if err := db.RemoveCategoryApprovedUser("cat-1", "user-b"); err != nil {
			t.Fatalf("RemoveCategoryApprovedUser() error = %v", err)
		}
		isApproved, _ := db.IsCategoryApprovedUser("user-b", "cat-1")
		if isApproved {
			t.Error("expected user-b NOT to be approved after removal")
		}
	})

	t.Run("remove nonexistent approved user", func(t *testing.T) {
		err := db.RemoveCategoryApprovedUser("cat-1", "user-z")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestListVisibleCategoriesForUser(t *testing.T) {
	db := setupTestDB(t)

	// Create categories (no visibility on categories now)
	db.CreateCategory(Category{ID: "cat-public", Name: "Public Cat", TenantID: "default"})
	db.CreateCategory(Category{ID: "cat-approved", Name: "Approved Cat", TenantID: "default"})
	db.CreateCategory(Category{ID: "cat-admin", Name: "Admin Cat", TenantID: "default"})

	// Create apps with visibility in each category
	for _, app := range []Application{
		{ID: "app-pub", Name: "Public App", Description: "d", URL: "http://x", Icon: "i", Category: "Public Cat", Visibility: CategoryVisibilityPublic, TenantID: "default"},
		{ID: "app-appr", Name: "Approved App", Description: "d", URL: "http://x", Icon: "i", Category: "Approved Cat", Visibility: CategoryVisibilityApproved, TenantID: "default"},
		{ID: "app-admin", Name: "Admin App", Description: "d", URL: "http://x", Icon: "i", Category: "Admin Cat", Visibility: CategoryVisibilityAdminOnly, TenantID: "default"},
	} {
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp(%s) error = %v", app.ID, err)
		}
	}

	// user-a is admin of cat-admin
	db.AddCategoryAdmin("cat-admin", "user-a")
	// user-b is approved for cat-approved
	db.AddCategoryApprovedUser("cat-approved", "user-b")
	// user-c is admin of cat-approved
	db.AddCategoryAdmin("cat-approved", "user-c")

	t.Run("system admin sees all categories", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("sys-admin", []string{"admin", "user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(cats) != 3 {
			t.Errorf("admin sees %d categories, want 3", len(cats))
		}
	})

	t.Run("regular user sees only categories with public apps", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("regular-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(cats) != 1 {
			t.Fatalf("regular user sees %d categories, want 1", len(cats))
		}
		if cats[0].ID != "cat-public" {
			t.Errorf("got %s, want cat-public", cats[0].ID)
		}
	})

	t.Run("category admin sees admin_only categories they admin", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("user-a", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(cats) != 2 {
			t.Fatalf("category admin sees %d categories, want 2 (public + admin_only)", len(cats))
		}
	})

	t.Run("approved user sees approved categories", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("user-b", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(cats) != 2 {
			t.Fatalf("approved user sees %d categories, want 2 (public + approved)", len(cats))
		}
	})

	t.Run("category admin of approved category sees it", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("user-c", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(cats) != 2 {
			t.Fatalf("cat admin sees %d categories, want 2 (public + approved)", len(cats))
		}
	})

	t.Run("user does NOT see admin_only categories they are not admin of", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("user-b", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		for _, c := range cats {
			if c.ID == "cat-admin" {
				t.Error("user-b should NOT see cat-admin")
			}
		}
	})

	t.Run("user does NOT see approved categories they are not approved for", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("regular-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		for _, c := range cats {
			if c.ID == "cat-approved" {
				t.Error("regular-user should NOT see cat-approved")
			}
		}
	})
}

func TestListAppsForUser(t *testing.T) {
	db := setupTestDB(t)

	// Create categories (visibility is on apps now)
	db.CreateCategory(Category{ID: "cat-public", Name: "Public", TenantID: "default"})
	db.CreateCategory(Category{ID: "cat-approved", Name: "Approved", TenantID: "default"})
	db.CreateCategory(Category{ID: "cat-admin", Name: "Admin Only", TenantID: "default"})

	// Create apps with app-level visibility
	for _, app := range []Application{
		{ID: "app-pub", Name: "Public App", Description: "d", URL: "http://x", Icon: "i", Category: "Public", Visibility: CategoryVisibilityPublic, TenantID: "default"},
		{ID: "app-appr", Name: "Approved App", Description: "d", URL: "http://x", Icon: "i", Category: "Approved", Visibility: CategoryVisibilityApproved, TenantID: "default"},
		{ID: "app-admin", Name: "Admin App", Description: "d", URL: "http://x", Icon: "i", Category: "Admin Only", Visibility: CategoryVisibilityAdminOnly, TenantID: "default"},
	} {
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp(%s) error = %v", app.ID, err)
		}
	}

	db.AddCategoryAdmin("cat-admin", "cat-admin-user")
	db.AddCategoryApprovedUser("cat-approved", "approved-user")

	t.Run("admin sees all apps", func(t *testing.T) {
		apps, err := db.ListAppsForUser("admin-user", []string{"admin"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(apps) != 3 {
			t.Errorf("admin sees %d apps, want 3", len(apps))
		}
	})

	t.Run("regular user sees only public apps", func(t *testing.T) {
		apps, err := db.ListAppsForUser("regular-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(apps) != 1 {
			t.Fatalf("regular user sees %d apps, want 1", len(apps))
		}
		if apps[0].ID != "app-pub" {
			t.Errorf("got %s, want app-pub", apps[0].ID)
		}
	})

	t.Run("category admin sees their admin_only apps", func(t *testing.T) {
		apps, err := db.ListAppsForUser("cat-admin-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(apps) != 2 {
			t.Errorf("cat admin sees %d apps, want 2 (public + admin_only)", len(apps))
		}
	})

	t.Run("approved user sees approved apps", func(t *testing.T) {
		apps, err := db.ListAppsForUser("approved-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(apps) != 2 {
			t.Errorf("approved user sees %d apps, want 2 (public + approved)", len(apps))
		}
	})
}

func TestMixedVisibilityInCategory(t *testing.T) {
	db := setupTestDB(t)

	// One category with apps of different visibilities
	db.CreateCategory(Category{ID: "cat-mixed", Name: "Mixed", TenantID: "default"})

	for _, app := range []Application{
		{ID: "app-mixed-pub", Name: "Public App", Description: "d", URL: "http://x", Icon: "i", Category: "Mixed", Visibility: CategoryVisibilityPublic, TenantID: "default"},
		{ID: "app-mixed-admin", Name: "Admin App", Description: "d", URL: "http://x", Icon: "i", Category: "Mixed", Visibility: CategoryVisibilityAdminOnly, TenantID: "default"},
	} {
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp(%s) error = %v", app.ID, err)
		}
	}

	t.Run("regular user sees only public app in mixed category", func(t *testing.T) {
		apps, err := db.ListAppsForUser("regular-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(apps) != 1 {
			t.Fatalf("regular user sees %d apps, want 1", len(apps))
		}
		if apps[0].ID != "app-mixed-pub" {
			t.Errorf("got %s, want app-mixed-pub", apps[0].ID)
		}
	})

	t.Run("category sees category with at least one visible app", func(t *testing.T) {
		cats, err := db.ListVisibleCategoriesForUser("regular-user", []string{"user"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(cats) != 1 {
			t.Fatalf("got %d categories, want 1", len(cats))
		}
		if cats[0].Name != "Mixed" {
			t.Errorf("got %s, want Mixed", cats[0].Name)
		}
	})

	t.Run("admin sees both apps", func(t *testing.T) {
		apps, err := db.ListAppsForUser("admin-user", []string{"admin"}, "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if len(apps) != 2 {
			t.Errorf("admin sees %d apps, want 2", len(apps))
		}
	})
}

func TestDataMigrationExistingCategories(t *testing.T) {
	db := setupTestDB(t)

	// Create apps with categories (simulating existing data)
	for _, app := range []Application{
		{ID: "m-1", Name: "App1", Description: "d", URL: "http://x", Icon: "i", Category: "DevTools"},
		{ID: "m-2", Name: "App2", Description: "d", URL: "http://x", Icon: "i", Category: "DevTools"},
		{ID: "m-3", Name: "App3", Description: "d", URL: "http://x", Icon: "i", Category: "Security"},
	} {
		db.CreateApp(app)
	}

	// Run migration again (simulates reopening DB)
	db.migrateExistingCategories()

	// Check that categories were auto-created
	cat, err := db.GetCategoryByName("DevTools")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if cat == nil {
		t.Fatal("expected DevTools category to be auto-created")
	}

	cat, err = db.GetCategoryByName("Security")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if cat == nil {
		t.Fatal("expected Security category to be auto-created")
	}
}

func TestEnsureCategoryExists(t *testing.T) {
	db := setupTestDB(t)

	t.Run("creates new category", func(t *testing.T) {
		id, err := db.EnsureCategoryExists("NewCat", "default")
		if err != nil {
			t.Fatalf("EnsureCategoryExists() error = %v", err)
		}
		if id == "" {
			t.Fatal("expected non-empty ID")
		}

		cat, _ := db.GetCategoryByName("NewCat")
		if cat == nil {
			t.Fatal("category not created")
		}
	})

	t.Run("returns existing category", func(t *testing.T) {
		id1, _ := db.EnsureCategoryExists("NewCat", "default")
		id2, _ := db.EnsureCategoryExists("NewCat", "default")
		if id1 != id2 {
			t.Errorf("expected same ID, got %s and %s", id1, id2)
		}
	})

	t.Run("empty name returns empty", func(t *testing.T) {
		id, err := db.EnsureCategoryExists("", "default")
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if id != "" {
			t.Errorf("expected empty ID, got %s", id)
		}
	})
}

func TestDeleteCategoryCleansUpJunctions(t *testing.T) {
	db := setupTestDB(t)

	db.CreateCategory(Category{ID: "cat-del", Name: "Deletable", TenantID: "default"})
	db.AddCategoryAdmin("cat-del", "user-a")
	db.AddCategoryApprovedUser("cat-del", "user-b")

	if err := db.DeleteCategory("cat-del"); err != nil {
		t.Fatalf("DeleteCategory() error = %v", err)
	}

	// Verify junction table entries are cleaned up
	admins, _ := db.ListCategoryAdmins("cat-del")
	if len(admins) != 0 {
		t.Errorf("expected 0 admins after delete, got %d", len(admins))
	}
	users, _ := db.ListCategoryApprovedUsers("cat-del")
	if len(users) != 0 {
		t.Errorf("expected 0 approved users after delete, got %d", len(users))
	}
}
