package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func setupTenantTestDB(t *testing.T) *DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "tenant-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

func TestDefaultTenantSeeded(t *testing.T) {
	db := setupTenantTestDB(t)

	tenant, err := db.GetTenant(DefaultTenantID)
	if err != nil {
		t.Fatalf("failed to get default tenant: %v", err)
	}
	if tenant == nil {
		t.Fatal("default tenant should be seeded automatically")
	}
	if tenant.Slug != "default" {
		t.Errorf("expected slug 'default', got %q", tenant.Slug)
	}
}

func TestCreateAndGetTenant(t *testing.T) {
	db := setupTenantTestDB(t)

	tenant := Tenant{
		ID:   "tenant-1",
		Name: "Acme Corp",
		Slug: "acme",
		Settings: TenantSettings{
			PrimaryColor: "#FF0000",
			DisplayName:  "Acme",
		},
		Quotas: TenantQuotas{
			MaxTotalSessions:   50,
			MaxSessionsPerUser: 3,
			MaxUsers:           100,
		},
	}

	if err := db.CreateTenant(tenant); err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	// Get by ID
	got, err := db.GetTenant("tenant-1")
	if err != nil {
		t.Fatalf("failed to get tenant by ID: %v", err)
	}
	if got == nil {
		t.Fatal("tenant not found by ID")
	}
	if got.Name != "Acme Corp" {
		t.Errorf("expected name 'Acme Corp', got %q", got.Name)
	}
	if got.Settings.PrimaryColor != "#FF0000" {
		t.Errorf("expected primary color '#FF0000', got %q", got.Settings.PrimaryColor)
	}
	if got.Quotas.MaxTotalSessions != 50 {
		t.Errorf("expected max total sessions 50, got %d", got.Quotas.MaxTotalSessions)
	}
	if got.Quotas.MaxSessionsPerUser != 3 {
		t.Errorf("expected max sessions per user 3, got %d", got.Quotas.MaxSessionsPerUser)
	}

	// Get by slug
	gotBySlug, err := db.GetTenantBySlug("acme")
	if err != nil {
		t.Fatalf("failed to get tenant by slug: %v", err)
	}
	if gotBySlug == nil {
		t.Fatal("tenant not found by slug")
	}
	if gotBySlug.ID != "tenant-1" {
		t.Errorf("expected ID 'tenant-1', got %q", gotBySlug.ID)
	}
}

func TestListTenants(t *testing.T) {
	db := setupTenantTestDB(t)

	// Default tenant exists
	tenants, err := db.ListTenants()
	if err != nil {
		t.Fatalf("failed to list tenants: %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("expected 1 tenant (default), got %d", len(tenants))
	}

	// Add another
	if err := db.CreateTenant(Tenant{ID: "t2", Name: "Beta", Slug: "beta"}); err != nil {
		t.Fatal(err)
	}

	tenants, err = db.ListTenants()
	if err != nil {
		t.Fatal(err)
	}
	if len(tenants) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestUpdateTenant(t *testing.T) {
	db := setupTenantTestDB(t)

	tenant := Tenant{ID: "t1", Name: "Original", Slug: "orig"}
	if err := db.CreateTenant(tenant); err != nil {
		t.Fatal(err)
	}

	tenant.Name = "Updated"
	tenant.Quotas.MaxTotalSessions = 25
	if err := db.UpdateTenant(tenant); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetTenant("t1")
	if got.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", got.Name)
	}
	if got.Quotas.MaxTotalSessions != 25 {
		t.Errorf("expected max sessions 25, got %d", got.Quotas.MaxTotalSessions)
	}
}

func TestDeleteTenant(t *testing.T) {
	db := setupTenantTestDB(t)

	if err := db.CreateTenant(Tenant{ID: "t1", Name: "Test", Slug: "test"}); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteTenant("t1"); err != nil {
		t.Fatalf("failed to delete tenant: %v", err)
	}

	got, _ := db.GetTenant("t1")
	if got != nil {
		t.Error("tenant should be deleted")
	}
}

func TestCannotDeleteDefaultTenant(t *testing.T) {
	db := setupTenantTestDB(t)

	if err := db.DeleteTenant(DefaultTenantID); err == nil {
		t.Error("should not be able to delete default tenant")
	}
}

func TestTenantScopedSessionCount(t *testing.T) {
	db := setupTenantTestDB(t)

	// Create a tenant
	if err := db.CreateTenant(Tenant{ID: "t1", Name: "Test", Slug: "test"}); err != nil {
		t.Fatal(err)
	}

	// Create app
	if err := db.CreateApp(Application{
		ID: "app1", Name: "App", Description: "Test", URL: "http://test",
		Icon: "icon", Category: "test", TenantID: "t1",
	}); err != nil {
		t.Fatal(err)
	}

	// Create sessions for tenant t1
	for i := 0; i < 3; i++ {
		s := Session{
			ID: fmt.Sprintf("s%d", i), UserID: "u1", AppID: "app1",
			PodName: fmt.Sprintf("pod%d", i), Status: SessionStatusRunning, TenantID: "t1",
		}
		s.CreatedAt = time.Now()
		s.UpdatedAt = time.Now()
		if err := db.CreateSession(s); err != nil {
			t.Fatalf("failed to create session %d: %v", i, err)
		}
	}

	// Count should be 3 for tenant t1
	count, err := db.CountActiveSessionsByTenant("t1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 active sessions for t1, got %d", count)
	}

	// Count should be 0 for default tenant
	count, err = db.CountActiveSessionsByTenant(DefaultTenantID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 active sessions for default tenant, got %d", count)
	}
}

func TestUserTenantFields(t *testing.T) {
	db := setupTenantTestDB(t)

	user := User{
		ID:           "u1",
		Username:     "testuser",
		PasswordHash: "$2a$10$fake",
		Roles:        []string{"user"},
		TenantID:     "default",
		TenantRoles:  []string{"tenant-admin"},
	}

	if err := db.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	got, err := db.GetUserByID("u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID != "default" {
		t.Errorf("expected tenant_id 'default', got %q", got.TenantID)
	}
	if len(got.TenantRoles) != 1 || got.TenantRoles[0] != "tenant-admin" {
		t.Errorf("expected tenant_roles [tenant-admin], got %v", got.TenantRoles)
	}
}
