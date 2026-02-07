package db

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	database, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

// --- Database lifecycle tests ---

func TestOpen(t *testing.T) {
	t.Run("opens new database", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-open-*.db")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		database, err := Open(tmpFile.Name())
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer database.Close()

		if database == nil {
			t.Fatal("Open() returned nil")
		}
	})

	t.Run("fails with invalid path", func(t *testing.T) {
		_, err := Open("/nonexistent/dir/test.db")
		if err == nil {
			t.Fatal("expected error for invalid path, got nil")
		}
	})

	t.Run("reopens existing database", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-reopen-*.db")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		// Open and create data
		db1, err := Open(tmpFile.Name())
		if err != nil {
			t.Fatalf("first Open() error = %v", err)
		}
		app := Application{
			ID: "reopen-app", Name: "Test", Description: "d",
			URL: "http://x", Icon: "i", Category: "c",
		}
		if err := db1.CreateApp(app); err != nil {
			t.Fatalf("CreateApp() error = %v", err)
		}
		db1.Close()

		// Reopen and verify data persists
		db2, err := Open(tmpFile.Name())
		if err != nil {
			t.Fatalf("second Open() error = %v", err)
		}
		defer db2.Close()

		got, err := db2.GetApp("reopen-app")
		if err != nil {
			t.Fatalf("GetApp() error = %v", err)
		}
		if got == nil {
			t.Fatal("data did not persist after reopen")
		}
		if got.Name != "Test" {
			t.Errorf("got Name = %s, want Test", got.Name)
		}
	})
}

func TestPing(t *testing.T) {
	db := setupTestDB(t)

	if err := db.Ping(); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

// --- Application CRUD tests ---

func TestApplicationCRUD(t *testing.T) {
	db := setupTestDB(t)

	t.Run("create and get app", func(t *testing.T) {
		app := Application{
			ID:          "app-1",
			Name:        "My App",
			Description: "A great app",
			URL:         "https://example.com",
			Icon:        "icon.png",
			Category:    "Development",
			LaunchType:  LaunchTypeURL,
		}
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp() error = %v", err)
		}

		got, err := db.GetApp("app-1")
		if err != nil {
			t.Fatalf("GetApp() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetApp() returned nil")
		}
		if got.ID != "app-1" {
			t.Errorf("got ID = %s, want app-1", got.ID)
		}
		if got.Name != "My App" {
			t.Errorf("got Name = %s, want My App", got.Name)
		}
		if got.LaunchType != LaunchTypeURL {
			t.Errorf("got LaunchType = %s, want url", got.LaunchType)
		}
		if got.OsType != "linux" {
			t.Errorf("got OsType = %s, want linux", got.OsType)
		}
	})

	t.Run("create container app with all fields", func(t *testing.T) {
		app := Application{
			ID:             "app-container",
			Name:           "Container App",
			Description:    "A container app",
			URL:            "https://container.example.com",
			Icon:           "container.png",
			Category:       "DevOps",
			LaunchType:     LaunchTypeContainer,
			OsType:         "windows",
			ContainerImage: "myimage:v1",
			ContainerPort:  9090,
			ContainerArgs:  []string{"--flag", "value"},
			ResourceLimits: &ResourceLimits{
				CPURequest:    "100m",
				CPULimit:      "500m",
				MemoryRequest: "256Mi",
				MemoryLimit:   "1Gi",
			},
		}
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp() error = %v", err)
		}

		got, err := db.GetApp("app-container")
		if err != nil {
			t.Fatalf("GetApp() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetApp() returned nil")
		}
		if got.LaunchType != LaunchTypeContainer {
			t.Errorf("got LaunchType = %s, want container", got.LaunchType)
		}
		if got.OsType != "windows" {
			t.Errorf("got OsType = %s, want windows", got.OsType)
		}
		if got.ContainerImage != "myimage:v1" {
			t.Errorf("got ContainerImage = %s, want myimage:v1", got.ContainerImage)
		}
		if got.ContainerPort != 9090 {
			t.Errorf("got ContainerPort = %d, want 9090", got.ContainerPort)
		}
		if len(got.ContainerArgs) != 2 || got.ContainerArgs[0] != "--flag" {
			t.Errorf("got ContainerArgs = %v, want [--flag value]", got.ContainerArgs)
		}
		if got.ResourceLimits == nil {
			t.Fatal("got ResourceLimits = nil")
		}
		if got.ResourceLimits.CPURequest != "100m" {
			t.Errorf("got CPURequest = %s, want 100m", got.ResourceLimits.CPURequest)
		}
		if got.ResourceLimits.MemoryLimit != "1Gi" {
			t.Errorf("got MemoryLimit = %s, want 1Gi", got.ResourceLimits.MemoryLimit)
		}
	})

	t.Run("create app with empty launch type defaults to url", func(t *testing.T) {
		app := Application{
			ID: "app-default-launch", Name: "Default", Description: "d",
			URL: "http://x", Icon: "i", Category: "c",
		}
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp() error = %v", err)
		}

		got, err := db.GetApp("app-default-launch")
		if err != nil {
			t.Fatalf("GetApp() error = %v", err)
		}
		if got.LaunchType != LaunchTypeURL {
			t.Errorf("got LaunchType = %s, want url", got.LaunchType)
		}
	})

	t.Run("get nonexistent app", func(t *testing.T) {
		got, err := db.GetApp("nonexistent")
		if err != nil {
			t.Fatalf("GetApp() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for nonexistent app, got %+v", got)
		}
	})

	t.Run("create duplicate app", func(t *testing.T) {
		app := Application{
			ID: "app-1", Name: "Dup", Description: "d",
			URL: "http://x", Icon: "i", Category: "c",
		}
		err := db.CreateApp(app)
		if err == nil {
			t.Fatal("expected error for duplicate ID, got nil")
		}
	})

	t.Run("list apps sorted by category and name", func(t *testing.T) {
		apps, err := db.ListApps()
		if err != nil {
			t.Fatalf("ListApps() error = %v", err)
		}
		if len(apps) < 2 {
			t.Fatalf("ListApps() returned %d apps, want >= 2", len(apps))
		}
		// Verify sorting: category ascending, then name ascending
		for i := 1; i < len(apps); i++ {
			if apps[i].Category < apps[i-1].Category {
				t.Errorf("apps not sorted by category: %s < %s", apps[i].Category, apps[i-1].Category)
			}
			if apps[i].Category == apps[i-1].Category && apps[i].Name < apps[i-1].Name {
				t.Errorf("apps not sorted by name within category: %s < %s", apps[i].Name, apps[i-1].Name)
			}
		}
	})

	t.Run("update app", func(t *testing.T) {
		app := Application{
			ID:          "app-1",
			Name:        "Updated App",
			Description: "Updated desc",
			URL:         "https://updated.example.com",
			Icon:        "new-icon.png",
			Category:    "Updated",
			LaunchType:  LaunchTypeWebProxy,
			OsType:      "windows",
		}
		if err := db.UpdateApp(app); err != nil {
			t.Fatalf("UpdateApp() error = %v", err)
		}

		got, _ := db.GetApp("app-1")
		if got.Name != "Updated App" {
			t.Errorf("got Name = %s, want Updated App", got.Name)
		}
		if got.LaunchType != LaunchTypeWebProxy {
			t.Errorf("got LaunchType = %s, want web_proxy", got.LaunchType)
		}
		if got.OsType != "windows" {
			t.Errorf("got OsType = %s, want windows", got.OsType)
		}
	})

	t.Run("update nonexistent app", func(t *testing.T) {
		app := Application{
			ID: "nonexistent", Name: "N", Description: "d",
			URL: "http://x", Icon: "i", Category: "c",
		}
		err := db.UpdateApp(app)
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete app", func(t *testing.T) {
		if err := db.DeleteApp("app-1"); err != nil {
			t.Fatalf("DeleteApp() error = %v", err)
		}
		got, _ := db.GetApp("app-1")
		if got != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete nonexistent app", func(t *testing.T) {
		err := db.DeleteApp("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

// --- Audit log tests ---

func TestAuditLog(t *testing.T) {
	db := setupTestDB(t)

	t.Run("log and retrieve audit entries", func(t *testing.T) {
		if err := db.LogAudit("admin", "create_app", "Created app-1"); err != nil {
			t.Fatalf("LogAudit() error = %v", err)
		}
		if err := db.LogAudit("admin", "update_app", "Updated app-1"); err != nil {
			t.Fatalf("LogAudit() error = %v", err)
		}
		if err := db.LogAudit("user1", "delete_app", "Deleted app-2"); err != nil {
			t.Fatalf("LogAudit() error = %v", err)
		}

		logs, err := db.GetAuditLogs(10)
		if err != nil {
			t.Fatalf("GetAuditLogs() error = %v", err)
		}
		if len(logs) != 3 {
			t.Fatalf("GetAuditLogs() returned %d entries, want 3", len(logs))
		}
		// Results should be ordered by timestamp DESC (most recent first)
		if logs[0].Action != "delete_app" {
			t.Errorf("got first log action = %s, want delete_app", logs[0].Action)
		}
		if logs[0].User != "user1" {
			t.Errorf("got first log user = %s, want user1", logs[0].User)
		}
		if logs[0].Details != "Deleted app-2" {
			t.Errorf("got first log details = %s, want Deleted app-2", logs[0].Details)
		}
		if logs[0].ID == 0 {
			t.Error("expected non-zero ID")
		}
		if logs[0].Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	})

	t.Run("limit audit logs", func(t *testing.T) {
		logs, err := db.GetAuditLogs(2)
		if err != nil {
			t.Fatalf("GetAuditLogs() error = %v", err)
		}
		if len(logs) != 2 {
			t.Fatalf("GetAuditLogs(2) returned %d entries, want 2", len(logs))
		}
	})

	t.Run("empty audit logs", func(t *testing.T) {
		freshDB := setupTestDB(t)
		logs, err := freshDB.GetAuditLogs(10)
		if err != nil {
			t.Fatalf("GetAuditLogs() error = %v", err)
		}
		if logs != nil {
			t.Errorf("expected nil for empty audit logs, got %d entries", len(logs))
		}
	})
}

// --- Analytics tests ---

func TestAnalytics(t *testing.T) {
	db := setupTestDB(t)

	// Create apps for analytics
	for _, id := range []string{"analytics-app-1", "analytics-app-2"} {
		app := Application{
			ID: id, Name: id, Description: "d",
			URL: "http://x", Icon: "i", Category: "c",
		}
		if err := db.CreateApp(app); err != nil {
			t.Fatalf("CreateApp() error = %v", err)
		}
	}

	t.Run("record and get analytics", func(t *testing.T) {
		// Record launches
		for i := 0; i < 3; i++ {
			if err := db.RecordLaunch("analytics-app-1"); err != nil {
				t.Fatalf("RecordLaunch() error = %v", err)
			}
		}
		if err := db.RecordLaunch("analytics-app-2"); err != nil {
			t.Fatalf("RecordLaunch() error = %v", err)
		}

		stats, err := db.GetAnalyticsStats()
		if err != nil {
			t.Fatalf("GetAnalyticsStats() error = %v", err)
		}
		if stats == nil {
			t.Fatal("GetAnalyticsStats() returned nil")
		}
		if stats.TotalLaunches != 4 {
			t.Errorf("got TotalLaunches = %d, want 4", stats.TotalLaunches)
		}
		if len(stats.AppStats) != 2 {
			t.Fatalf("got %d app stats, want 2", len(stats.AppStats))
		}
		// Sorted by launch_count DESC
		if stats.AppStats[0].AppID != "analytics-app-1" {
			t.Errorf("got top app = %s, want analytics-app-1", stats.AppStats[0].AppID)
		}
		if stats.AppStats[0].LaunchCount != 3 {
			t.Errorf("got top app count = %d, want 3", stats.AppStats[0].LaunchCount)
		}
		if stats.AppStats[0].AppName != "analytics-app-1" {
			t.Errorf("got top app name = %s, want analytics-app-1", stats.AppStats[0].AppName)
		}
	})

	t.Run("empty analytics", func(t *testing.T) {
		freshDB := setupTestDB(t)
		stats, err := freshDB.GetAnalyticsStats()
		if err != nil {
			t.Fatalf("GetAnalyticsStats() error = %v", err)
		}
		if stats.TotalLaunches != 0 {
			t.Errorf("got TotalLaunches = %d, want 0", stats.TotalLaunches)
		}
	})

	t.Run("analytics for deleted app uses app_id as name", func(t *testing.T) {
		freshDB := setupTestDB(t)
		// Record launch for app that doesn't exist in applications table
		if err := freshDB.RecordLaunch("deleted-app"); err != nil {
			t.Fatalf("RecordLaunch() error = %v", err)
		}
		stats, err := freshDB.GetAnalyticsStats()
		if err != nil {
			t.Fatalf("GetAnalyticsStats() error = %v", err)
		}
		if len(stats.AppStats) != 1 {
			t.Fatalf("got %d stats, want 1", len(stats.AppStats))
		}
		// COALESCE should fall back to app_id
		if stats.AppStats[0].AppName != "deleted-app" {
			t.Errorf("got AppName = %s, want deleted-app", stats.AppStats[0].AppName)
		}
	})
}

// --- User CRUD tests ---

func TestUserCRUD(t *testing.T) {
	db := setupTestDB(t)

	t.Run("create and get user by ID", func(t *testing.T) {
		user := User{
			ID:           "user-1",
			Username:     "testuser",
			Email:        "test@example.com",
			DisplayName:  "Test User",
			PasswordHash: "hashed-password",
			Roles:        []string{"user", "editor"},
		}
		if err := db.CreateUser(user); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		got, err := db.GetUserByID("user-1")
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetUserByID() returned nil")
		}
		if got.ID != "user-1" {
			t.Errorf("got ID = %s, want user-1", got.ID)
		}
		if got.Username != "testuser" {
			t.Errorf("got Username = %s, want testuser", got.Username)
		}
		if got.Email != "test@example.com" {
			t.Errorf("got Email = %s, want test@example.com", got.Email)
		}
		if got.DisplayName != "Test User" {
			t.Errorf("got DisplayName = %s, want Test User", got.DisplayName)
		}
		if got.PasswordHash != "hashed-password" {
			t.Errorf("got PasswordHash = %s, want hashed-password", got.PasswordHash)
		}
		if len(got.Roles) != 2 || got.Roles[0] != "user" || got.Roles[1] != "editor" {
			t.Errorf("got Roles = %v, want [user editor]", got.Roles)
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
	})

	t.Run("get user by username", func(t *testing.T) {
		got, err := db.GetUserByUsername("testuser")
		if err != nil {
			t.Fatalf("GetUserByUsername() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetUserByUsername() returned nil")
		}
		if got.ID != "user-1" {
			t.Errorf("got ID = %s, want user-1", got.ID)
		}
	})

	t.Run("get nonexistent user by ID", func(t *testing.T) {
		got, err := db.GetUserByID("nonexistent")
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for nonexistent user, got %+v", got)
		}
	})

	t.Run("get nonexistent user by username", func(t *testing.T) {
		got, err := db.GetUserByUsername("nonexistent")
		if err != nil {
			t.Fatalf("GetUserByUsername() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for nonexistent user, got %+v", got)
		}
	})

	t.Run("create duplicate username", func(t *testing.T) {
		user := User{
			ID: "user-dup", Username: "testuser",
			PasswordHash: "hash", Roles: []string{"user"},
		}
		err := db.CreateUser(user)
		if err == nil {
			t.Fatal("expected error for duplicate username, got nil")
		}
	})

	t.Run("update user", func(t *testing.T) {
		user := User{
			ID:           "user-1",
			Email:        "updated@example.com",
			DisplayName:  "Updated User",
			PasswordHash: "new-hash",
			Roles:        []string{"admin", "user"},
		}
		if err := db.UpdateUser(user); err != nil {
			t.Fatalf("UpdateUser() error = %v", err)
		}

		got, _ := db.GetUserByID("user-1")
		if got.Email != "updated@example.com" {
			t.Errorf("got Email = %s, want updated@example.com", got.Email)
		}
		if got.DisplayName != "Updated User" {
			t.Errorf("got DisplayName = %s, want Updated User", got.DisplayName)
		}
		if got.PasswordHash != "new-hash" {
			t.Errorf("got PasswordHash = %s, want new-hash", got.PasswordHash)
		}
		if len(got.Roles) != 2 || got.Roles[0] != "admin" {
			t.Errorf("got Roles = %v, want [admin user]", got.Roles)
		}
	})

	t.Run("update nonexistent user", func(t *testing.T) {
		user := User{
			ID: "nonexistent", PasswordHash: "h", Roles: []string{"user"},
		}
		err := db.UpdateUser(user)
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("list users", func(t *testing.T) {
		// Create another user
		user2 := User{
			ID: "user-2", Username: "second",
			PasswordHash: "hash", Roles: []string{"user"},
		}
		if err := db.CreateUser(user2); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		users, err := db.ListUsers()
		if err != nil {
			t.Fatalf("ListUsers() error = %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("ListUsers() returned %d users, want 2", len(users))
		}
	})

	t.Run("delete user", func(t *testing.T) {
		if err := db.DeleteUser("user-2"); err != nil {
			t.Fatalf("DeleteUser() error = %v", err)
		}
		got, _ := db.GetUserByID("user-2")
		if got != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete nonexistent user", func(t *testing.T) {
		err := db.DeleteUser("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestSeedAdminUser(t *testing.T) {
	db := setupTestDB(t)

	t.Run("seed creates admin", func(t *testing.T) {
		if err := db.SeedAdminUser("admin", "admin-hash"); err != nil {
			t.Fatalf("SeedAdminUser() error = %v", err)
		}

		got, err := db.GetUserByUsername("admin")
		if err != nil {
			t.Fatalf("GetUserByUsername() error = %v", err)
		}
		if got == nil {
			t.Fatal("admin user not created")
		}
		if got.ID != "admin-admin" {
			t.Errorf("got ID = %s, want admin-admin", got.ID)
		}
		if got.DisplayName != "Administrator" {
			t.Errorf("got DisplayName = %s, want Administrator", got.DisplayName)
		}
		if len(got.Roles) != 2 {
			t.Fatalf("got %d roles, want 2", len(got.Roles))
		}
	})

	t.Run("seed is idempotent", func(t *testing.T) {
		// Call again - should not error
		if err := db.SeedAdminUser("admin", "different-hash"); err != nil {
			t.Fatalf("SeedAdminUser() error = %v", err)
		}

		// Original hash should remain
		got, _ := db.GetUserByUsername("admin")
		if got.PasswordHash != "admin-hash" {
			t.Errorf("got PasswordHash = %s, want admin-hash (unchanged)", got.PasswordHash)
		}
	})
}

// --- Settings tests ---

func TestSettings(t *testing.T) {
	db := setupTestDB(t)

	t.Run("get nonexistent setting returns empty", func(t *testing.T) {
		val, err := db.GetSetting("nonexistent")
		if err != nil {
			t.Fatalf("GetSetting() error = %v", err)
		}
		if val != "" {
			t.Errorf("got value = %q, want empty", val)
		}
	})

	t.Run("set and get setting", func(t *testing.T) {
		if err := db.SetSetting("theme", "dark"); err != nil {
			t.Fatalf("SetSetting() error = %v", err)
		}

		val, err := db.GetSetting("theme")
		if err != nil {
			t.Fatalf("GetSetting() error = %v", err)
		}
		if val != "dark" {
			t.Errorf("got value = %s, want dark", val)
		}
	})

	t.Run("update existing setting", func(t *testing.T) {
		if err := db.SetSetting("theme", "light"); err != nil {
			t.Fatalf("SetSetting() error = %v", err)
		}

		val, _ := db.GetSetting("theme")
		if val != "light" {
			t.Errorf("got value = %s, want light", val)
		}
	})

	t.Run("get all settings", func(t *testing.T) {
		if err := db.SetSetting("language", "en"); err != nil {
			t.Fatalf("SetSetting() error = %v", err)
		}

		settings, err := db.GetAllSettings()
		if err != nil {
			t.Fatalf("GetAllSettings() error = %v", err)
		}
		if len(settings) != 2 {
			t.Fatalf("GetAllSettings() returned %d settings, want 2", len(settings))
		}
		if settings["theme"] != "light" {
			t.Errorf("got theme = %s, want light", settings["theme"])
		}
		if settings["language"] != "en" {
			t.Errorf("got language = %s, want en", settings["language"])
		}
	})

	t.Run("empty settings", func(t *testing.T) {
		freshDB := setupTestDB(t)
		settings, err := freshDB.GetAllSettings()
		if err != nil {
			t.Fatalf("GetAllSettings() error = %v", err)
		}
		if len(settings) != 0 {
			t.Errorf("expected 0 settings, got %d", len(settings))
		}
	})
}

// --- Session CRUD tests ---

func TestSessionCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create an app for the session
	app := Application{
		ID:          "test-app",
		Name:        "Test App",
		Description: "A test application",
		URL:         "https://example.com",
		Icon:        "icon.png",
		Category:    "test",
		LaunchType:  LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	now := time.Now().Truncate(time.Second)

	t.Run("create and get session", func(t *testing.T) {
		session := Session{
			ID:          "sess-1",
			UserID:      "user-1",
			AppID:       "test-app",
			PodName:     "pod-sess-1",
			Status:      SessionStatusCreating,
			IdleTimeout: 3600,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := db.CreateSession(session); err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		got, err := db.GetSession("sess-1")
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetSession() returned nil")
		}
		if got.ID != "sess-1" {
			t.Errorf("got ID = %s, want sess-1", got.ID)
		}
		if got.IdleTimeout != 3600 {
			t.Errorf("got IdleTimeout = %d, want 3600", got.IdleTimeout)
		}
		if got.Status != SessionStatusCreating {
			t.Errorf("got Status = %s, want creating", got.Status)
		}
	})

	t.Run("get nonexistent session", func(t *testing.T) {
		got, err := db.GetSession("nonexistent")
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for nonexistent session, got %+v", got)
		}
	})

	t.Run("update session status", func(t *testing.T) {
		if err := db.UpdateSessionStatus("sess-1", SessionStatusRunning); err != nil {
			t.Fatalf("UpdateSessionStatus() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.Status != SessionStatusRunning {
			t.Errorf("got Status = %s, want running", got.Status)
		}
	})

	t.Run("update session status nonexistent", func(t *testing.T) {
		err := db.UpdateSessionStatus("nonexistent", SessionStatusRunning)
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("update session pod IP", func(t *testing.T) {
		if err := db.UpdateSessionPodIP("sess-1", "10.0.0.5"); err != nil {
			t.Fatalf("UpdateSessionPodIP() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.PodIP != "10.0.0.5" {
			t.Errorf("got PodIP = %s, want 10.0.0.5", got.PodIP)
		}
	})

	t.Run("update session pod IP nonexistent", func(t *testing.T) {
		err := db.UpdateSessionPodIP("nonexistent", "10.0.0.1")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("update session pod IP and status", func(t *testing.T) {
		if err := db.UpdateSessionPodIPAndStatus("sess-1", "10.0.0.10", SessionStatusRunning); err != nil {
			t.Fatalf("UpdateSessionPodIPAndStatus() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.PodIP != "10.0.0.10" {
			t.Errorf("got PodIP = %s, want 10.0.0.10", got.PodIP)
		}
		if got.Status != SessionStatusRunning {
			t.Errorf("got Status = %s, want running", got.Status)
		}
	})

	t.Run("update session pod IP and status nonexistent", func(t *testing.T) {
		err := db.UpdateSessionPodIPAndStatus("nonexistent", "10.0.0.1", SessionStatusRunning)
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("update session restart", func(t *testing.T) {
		// First stop the session
		if err := db.UpdateSessionStatus("sess-1", SessionStatusStopped); err != nil {
			t.Fatalf("UpdateSessionStatus(stopped) error = %v", err)
		}

		// Restart with new pod name
		if err := db.UpdateSessionRestart("sess-1", "pod-sess-1-v2"); err != nil {
			t.Fatalf("UpdateSessionRestart() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.PodName != "pod-sess-1-v2" {
			t.Errorf("got PodName = %s, want pod-sess-1-v2", got.PodName)
		}
		if got.PodIP != "" {
			t.Errorf("got PodIP = %s, want empty", got.PodIP)
		}
		if got.Status != SessionStatusCreating {
			t.Errorf("got Status = %s, want creating", got.Status)
		}
	})

	t.Run("update session restart nonexistent", func(t *testing.T) {
		err := db.UpdateSessionRestart("nonexistent", "pod-new")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		sessions, err := db.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions() error = %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("ListSessions() returned %d sessions, want 1", len(sessions))
		}
		if sessions[0].IdleTimeout != 3600 {
			t.Errorf("listed session IdleTimeout = %d, want 3600", sessions[0].IdleTimeout)
		}
	})

	t.Run("list sessions by user", func(t *testing.T) {
		sessions, err := db.ListSessionsByUser("user-1")
		if err != nil {
			t.Fatalf("ListSessionsByUser() error = %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("ListSessionsByUser() returned %d sessions, want 1", len(sessions))
		}

		// No sessions for different user
		sessions, err = db.ListSessionsByUser("user-999")
		if err != nil {
			t.Fatalf("ListSessionsByUser() error = %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("ListSessionsByUser(unknown) returned %d sessions, want 0", len(sessions))
		}
	})

	t.Run("list sessions excludes terminated and failed", func(t *testing.T) {
		// Create sessions with excluded statuses
		for _, s := range []struct {
			id     string
			status SessionStatus
		}{
			{"sess-terminated", SessionStatus("terminated")},
			{"sess-failed", SessionStatusFailed},
		} {
			sess := Session{
				ID: s.id, UserID: "user-1", AppID: "test-app",
				PodName: "pod-" + s.id, Status: s.status,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := db.CreateSession(sess); err != nil {
				t.Fatalf("CreateSession(%s) error = %v", s.id, err)
			}
		}

		sessions, err := db.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions() error = %v", err)
		}
		for _, s := range sessions {
			if s.Status == SessionStatus("terminated") || s.Status == SessionStatusFailed {
				t.Errorf("ListSessions() should not include status %s", s.Status)
			}
		}
	})

	t.Run("session with default idle timeout", func(t *testing.T) {
		session := Session{
			ID:        "sess-2",
			UserID:    "user-1",
			AppID:     "test-app",
			PodName:   "pod-sess-2",
			Status:    SessionStatusRunning,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := db.CreateSession(session); err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		got, _ := db.GetSession("sess-2")
		if got.IdleTimeout != 0 {
			t.Errorf("got IdleTimeout = %d, want 0 (default)", got.IdleTimeout)
		}
	})

	t.Run("delete session", func(t *testing.T) {
		if err := db.DeleteSession("sess-2"); err != nil {
			t.Fatalf("DeleteSession() error = %v", err)
		}

		got, err := db.GetSession("sess-2")
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if got != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete nonexistent session", func(t *testing.T) {
		err := db.DeleteSession("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

// --- AppSpec CRUD tests ---

func TestAppSpecCRUD(t *testing.T) {
	db := setupTestDB(t)

	t.Run("create and get app spec", func(t *testing.T) {
		spec := AppSpec{
			ID:            "spec-1",
			Name:          "Test App Spec",
			Description:   "A test app specification",
			Image:         "nginx:latest",
			LaunchCommand: "/bin/sh -c 'nginx -g daemon off;'",
			Resources: &ResourceLimits{
				CPURequest:    "100m",
				CPULimit:      "500m",
				MemoryRequest: "128Mi",
				MemoryLimit:   "512Mi",
			},
			EnvVars: []EnvVar{
				{Name: "PORT", Value: "8080"},
				{Name: "ENV", Value: "production"},
			},
			Volumes: []VolumeMount{
				{Name: "data", MountPath: "/data", Size: "1Gi"},
			},
			NetworkRules: []NetworkRule{
				{Port: 8080, Protocol: "TCP"},
				{Port: 443, Protocol: "TCP", AllowFrom: "10.0.0.0/8"},
			},
		}

		if err := db.CreateAppSpec(spec); err != nil {
			t.Fatalf("CreateAppSpec() error = %v", err)
		}

		got, err := db.GetAppSpec("spec-1")
		if err != nil {
			t.Fatalf("GetAppSpec() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetAppSpec() returned nil")
		}
		if got.ID != "spec-1" {
			t.Errorf("got ID = %s, want spec-1", got.ID)
		}
		if got.Name != "Test App Spec" {
			t.Errorf("got Name = %s, want Test App Spec", got.Name)
		}
		if got.Image != "nginx:latest" {
			t.Errorf("got Image = %s, want nginx:latest", got.Image)
		}
		if got.LaunchCommand != "/bin/sh -c 'nginx -g daemon off;'" {
			t.Errorf("got LaunchCommand = %s, want /bin/sh -c 'nginx -g daemon off;'", got.LaunchCommand)
		}
		if got.Resources == nil {
			t.Fatal("got Resources = nil, want non-nil")
		}
		if got.Resources.CPURequest != "100m" {
			t.Errorf("got CPURequest = %s, want 100m", got.Resources.CPURequest)
		}
		if got.Resources.MemoryLimit != "512Mi" {
			t.Errorf("got MemoryLimit = %s, want 512Mi", got.Resources.MemoryLimit)
		}
		if len(got.EnvVars) != 2 {
			t.Fatalf("got %d env vars, want 2", len(got.EnvVars))
		}
		if got.EnvVars[0].Name != "PORT" || got.EnvVars[0].Value != "8080" {
			t.Errorf("got EnvVars[0] = %+v, want PORT=8080", got.EnvVars[0])
		}
		if len(got.Volumes) != 1 {
			t.Fatalf("got %d volumes, want 1", len(got.Volumes))
		}
		if got.Volumes[0].MountPath != "/data" {
			t.Errorf("got Volumes[0].MountPath = %s, want /data", got.Volumes[0].MountPath)
		}
		if len(got.NetworkRules) != 2 {
			t.Fatalf("got %d network rules, want 2", len(got.NetworkRules))
		}
		if got.NetworkRules[1].AllowFrom != "10.0.0.0/8" {
			t.Errorf("got NetworkRules[1].AllowFrom = %s, want 10.0.0.0/8", got.NetworkRules[1].AllowFrom)
		}
	})

	t.Run("get nonexistent app spec", func(t *testing.T) {
		got, err := db.GetAppSpec("nonexistent")
		if err != nil {
			t.Fatalf("GetAppSpec() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for nonexistent app spec, got %+v", got)
		}
	})

	t.Run("create duplicate app spec", func(t *testing.T) {
		spec := AppSpec{
			ID:    "spec-1",
			Name:  "Duplicate",
			Image: "busybox",
		}
		err := db.CreateAppSpec(spec)
		if err == nil {
			t.Fatal("expected error for duplicate ID, got nil")
		}
	})

	t.Run("list app specs", func(t *testing.T) {
		specs, err := db.ListAppSpecs()
		if err != nil {
			t.Fatalf("ListAppSpecs() error = %v", err)
		}
		if len(specs) != 1 {
			t.Fatalf("ListAppSpecs() returned %d specs, want 1", len(specs))
		}
		if specs[0].ID != "spec-1" {
			t.Errorf("got ID = %s, want spec-1", specs[0].ID)
		}
	})

	t.Run("update app spec", func(t *testing.T) {
		spec := AppSpec{
			ID:            "spec-1",
			Name:          "Updated App Spec",
			Description:   "Updated description",
			Image:         "nginx:1.25",
			LaunchCommand: "nginx",
			Resources: &ResourceLimits{
				CPULimit:    "1",
				MemoryLimit: "1Gi",
			},
			EnvVars: []EnvVar{
				{Name: "PORT", Value: "9090"},
			},
		}

		if err := db.UpdateAppSpec(spec); err != nil {
			t.Fatalf("UpdateAppSpec() error = %v", err)
		}

		got, _ := db.GetAppSpec("spec-1")
		if got.Name != "Updated App Spec" {
			t.Errorf("got Name = %s, want Updated App Spec", got.Name)
		}
		if got.Image != "nginx:1.25" {
			t.Errorf("got Image = %s, want nginx:1.25", got.Image)
		}
		if len(got.EnvVars) != 1 {
			t.Fatalf("got %d env vars, want 1", len(got.EnvVars))
		}
		if got.EnvVars[0].Value != "9090" {
			t.Errorf("got EnvVars[0].Value = %s, want 9090", got.EnvVars[0].Value)
		}
		// Volumes and network rules should be empty after update
		if len(got.Volumes) != 0 {
			t.Errorf("got %d volumes, want 0", len(got.Volumes))
		}
		if len(got.NetworkRules) != 0 {
			t.Errorf("got %d network rules, want 0", len(got.NetworkRules))
		}
	})

	t.Run("update nonexistent app spec", func(t *testing.T) {
		spec := AppSpec{
			ID:    "nonexistent",
			Name:  "Doesn't Exist",
			Image: "busybox",
		}
		err := db.UpdateAppSpec(spec)
		if err == nil {
			t.Fatal("expected error for nonexistent app spec, got nil")
		}
	})

	t.Run("create minimal app spec", func(t *testing.T) {
		spec := AppSpec{
			ID:    "spec-minimal",
			Name:  "Minimal Spec",
			Image: "busybox:latest",
		}

		if err := db.CreateAppSpec(spec); err != nil {
			t.Fatalf("CreateAppSpec() error = %v", err)
		}

		got, _ := db.GetAppSpec("spec-minimal")
		if got == nil {
			t.Fatal("GetAppSpec() returned nil")
		}
		if got.Resources != nil {
			t.Errorf("expected nil Resources for minimal spec, got %+v", got.Resources)
		}
		if len(got.EnvVars) != 0 {
			t.Errorf("expected 0 env vars, got %d", len(got.EnvVars))
		}
		if len(got.Volumes) != 0 {
			t.Errorf("expected 0 volumes, got %d", len(got.Volumes))
		}
		if len(got.NetworkRules) != 0 {
			t.Errorf("expected 0 network rules, got %d", len(got.NetworkRules))
		}
	})

	t.Run("delete app spec", func(t *testing.T) {
		if err := db.DeleteAppSpec("spec-1"); err != nil {
			t.Fatalf("DeleteAppSpec() error = %v", err)
		}

		got, err := db.GetAppSpec("spec-1")
		if err != nil {
			t.Fatalf("GetAppSpec() error = %v", err)
		}
		if got != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete nonexistent app spec", func(t *testing.T) {
		err := db.DeleteAppSpec("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent app spec, got nil")
		}
	})
}

// --- Stale sessions tests ---

func TestGetStaleSessions(t *testing.T) {
	db := setupTestDB(t)

	// Create an app
	app := Application{
		ID:          "test-app",
		Name:        "Test App",
		Description: "A test application",
		URL:         "https://example.com",
		Icon:        "icon.png",
		Category:    "test",
		LaunchType:  LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Create a session with old updated_at using default timeout
	oldTime := time.Now().Add(-3 * time.Hour)
	session1 := Session{
		ID:        "stale-1",
		UserID:    "user-1",
		AppID:     "test-app",
		PodName:   "pod-stale-1",
		Status:    SessionStatusRunning,
		CreatedAt: oldTime,
		UpdatedAt: oldTime,
	}
	if err := db.CreateSession(session1); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	// Force the updated_at to be old
	db.conn.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", oldTime, "stale-1")

	// Create a fresh session
	freshTime := time.Now()
	session2 := Session{
		ID:        "fresh-1",
		UserID:    "user-1",
		AppID:     "test-app",
		PodName:   "pod-fresh-1",
		Status:    SessionStatusRunning,
		CreatedAt: freshTime,
		UpdatedAt: freshTime,
	}
	if err := db.CreateSession(session2); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Query with 2-hour default timeout
	stale, err := db.GetStaleSessions(2 * time.Hour)
	if err != nil {
		t.Fatalf("GetStaleSessions() error = %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("GetStaleSessions() returned %d, want 1", len(stale))
	}
	if stale[0].ID != "stale-1" {
		t.Errorf("got stale session ID = %s, want stale-1", stale[0].ID)
	}
}

func TestGetStaleSessionsExcludesInactiveStatuses(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "test-app", Name: "Test", Description: "d",
		URL: "http://x", Icon: "i", Category: "c", LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	oldTime := time.Now().Add(-3 * time.Hour)

	// Create sessions with various inactive statuses - these should NOT appear as stale
	for _, status := range []SessionStatus{
		SessionStatus("terminated"),
		SessionStatusFailed,
		SessionStatusStopped,
		SessionStatusExpired,
	} {
		sess := Session{
			ID: "stale-" + string(status), UserID: "user-1", AppID: "test-app",
			PodName: "pod-" + string(status), Status: status,
			CreatedAt: oldTime, UpdatedAt: oldTime,
		}
		if err := db.CreateSession(sess); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", status, err)
		}
		db.conn.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", oldTime, sess.ID)
	}

	stale, err := db.GetStaleSessions(1 * time.Hour)
	if err != nil {
		t.Fatalf("GetStaleSessions() error = %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("GetStaleSessions() returned %d sessions, want 0 (inactive statuses excluded)", len(stale))
	}
}

func TestGetStaleSessionsPerSessionTimeout(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "test-app", Name: "Test", Description: "d",
		URL: "http://x", Icon: "i", Category: "c", LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Session with short per-session timeout (60s), updated 2 minutes ago
	twoMinAgo := time.Now().Add(-2 * time.Minute)
	sess := Session{
		ID: "short-timeout", UserID: "user-1", AppID: "test-app",
		PodName: "pod-short", Status: SessionStatusRunning,
		IdleTimeout: 60,
		CreatedAt:   twoMinAgo, UpdatedAt: twoMinAgo,
	}
	if err := db.CreateSession(sess); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	db.conn.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", twoMinAgo, "short-timeout")

	// With a 1-hour global default, only the per-session timeout should catch it
	stale, err := db.GetStaleSessions(1 * time.Hour)
	if err != nil {
		t.Fatalf("GetStaleSessions() error = %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("GetStaleSessions() returned %d, want 1", len(stale))
	}
	if stale[0].ID != "short-timeout" {
		t.Errorf("got ID = %s, want short-timeout", stale[0].ID)
	}
}

// --- Template CRUD tests ---

func TestTemplateCRUD(t *testing.T) {
	db := setupTestDB(t)

	t.Run("create and get template", func(t *testing.T) {
		tmpl := Template{
			TemplateID:       "tmpl-vscode",
			TemplateVersion:  "1.0.0",
			TemplateCategory: "ide",
			Name:             "VS Code",
			Description:      "Visual Studio Code in browser",
			URL:              "",
			Icon:             "vscode.png",
			Category:         "Development",
			LaunchType:       "container",
			OsType:           "linux",
			ContainerImage:   "codercom/code-server:latest",
			ContainerPort:    8080,
			ContainerArgs:    []string{"--auth", "none"},
			Tags:             []string{"ide", "development"},
			Maintainer:       "coder",
			DocumentationURL: "https://docs.example.com",
			RecommendedLimits: &ResourceLimits{
				CPURequest:    "500m",
				CPULimit:      "2",
				MemoryRequest: "512Mi",
				MemoryLimit:   "4Gi",
			},
		}
		if err := db.CreateTemplate(tmpl); err != nil {
			t.Fatalf("CreateTemplate() error = %v", err)
		}

		got, err := db.GetTemplate("tmpl-vscode")
		if err != nil {
			t.Fatalf("GetTemplate() error = %v", err)
		}
		if got == nil {
			t.Fatal("GetTemplate() returned nil")
		}
		if got.TemplateID != "tmpl-vscode" {
			t.Errorf("got TemplateID = %s, want tmpl-vscode", got.TemplateID)
		}
		if got.Name != "VS Code" {
			t.Errorf("got Name = %s, want VS Code", got.Name)
		}
		if got.TemplateVersion != "1.0.0" {
			t.Errorf("got TemplateVersion = %s, want 1.0.0", got.TemplateVersion)
		}
		if got.TemplateCategory != "ide" {
			t.Errorf("got TemplateCategory = %s, want ide", got.TemplateCategory)
		}
		if got.OsType != "linux" {
			t.Errorf("got OsType = %s, want linux", got.OsType)
		}
		if got.ContainerImage != "codercom/code-server:latest" {
			t.Errorf("got ContainerImage = %s, want codercom/code-server:latest", got.ContainerImage)
		}
		if got.ContainerPort != 8080 {
			t.Errorf("got ContainerPort = %d, want 8080", got.ContainerPort)
		}
		if len(got.ContainerArgs) != 2 || got.ContainerArgs[0] != "--auth" {
			t.Errorf("got ContainerArgs = %v, want [--auth none]", got.ContainerArgs)
		}
		if len(got.Tags) != 2 || got.Tags[0] != "ide" {
			t.Errorf("got Tags = %v, want [ide development]", got.Tags)
		}
		if got.Maintainer != "coder" {
			t.Errorf("got Maintainer = %s, want coder", got.Maintainer)
		}
		if got.DocumentationURL != "https://docs.example.com" {
			t.Errorf("got DocumentationURL = %s, want https://docs.example.com", got.DocumentationURL)
		}
		if got.RecommendedLimits == nil {
			t.Fatal("got RecommendedLimits = nil")
		}
		if got.RecommendedLimits.CPULimit != "2" {
			t.Errorf("got CPULimit = %s, want 2", got.RecommendedLimits.CPULimit)
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
	})

	t.Run("get nonexistent template", func(t *testing.T) {
		got, err := db.GetTemplate("nonexistent")
		if err != nil {
			t.Fatalf("GetTemplate() error = %v", err)
		}
		if got != nil {
			t.Errorf("expected nil for nonexistent template, got %+v", got)
		}
	})

	t.Run("create minimal template", func(t *testing.T) {
		tmpl := Template{
			TemplateID:       "tmpl-minimal",
			TemplateVersion:  "1.0.0",
			TemplateCategory: "tools",
			Name:             "Minimal",
			Description:      "Minimal template",
			Category:         "Tools",
			LaunchType:       "container",
		}
		if err := db.CreateTemplate(tmpl); err != nil {
			t.Fatalf("CreateTemplate() error = %v", err)
		}

		got, _ := db.GetTemplate("tmpl-minimal")
		if got == nil {
			t.Fatal("GetTemplate() returned nil")
		}
		if got.OsType != "linux" {
			t.Errorf("got OsType = %s, want linux (default)", got.OsType)
		}
	})

	t.Run("create duplicate template", func(t *testing.T) {
		tmpl := Template{
			TemplateID: "tmpl-vscode", TemplateVersion: "2.0.0",
			TemplateCategory: "ide", Name: "Dup", Description: "d",
			Category: "c", LaunchType: "container",
		}
		err := db.CreateTemplate(tmpl)
		if err == nil {
			t.Fatal("expected error for duplicate template_id, got nil")
		}
	})

	t.Run("list templates", func(t *testing.T) {
		templates, err := db.ListTemplates()
		if err != nil {
			t.Fatalf("ListTemplates() error = %v", err)
		}
		if len(templates) != 2 {
			t.Fatalf("ListTemplates() returned %d templates, want 2", len(templates))
		}
	})

	t.Run("update template", func(t *testing.T) {
		tmpl := Template{
			TemplateID:       "tmpl-vscode",
			TemplateVersion:  "2.0.0",
			TemplateCategory: "ide",
			Name:             "VS Code Updated",
			Description:      "Updated description",
			Category:         "Development",
			LaunchType:       "container",
			OsType:           "windows",
			ContainerImage:   "codercom/code-server:v2",
			ContainerPort:    9090,
			Tags:             []string{"ide"},
		}
		if err := db.UpdateTemplate(tmpl); err != nil {
			t.Fatalf("UpdateTemplate() error = %v", err)
		}

		got, _ := db.GetTemplate("tmpl-vscode")
		if got.Name != "VS Code Updated" {
			t.Errorf("got Name = %s, want VS Code Updated", got.Name)
		}
		if got.TemplateVersion != "2.0.0" {
			t.Errorf("got TemplateVersion = %s, want 2.0.0", got.TemplateVersion)
		}
		if got.OsType != "windows" {
			t.Errorf("got OsType = %s, want windows", got.OsType)
		}
		if got.ContainerPort != 9090 {
			t.Errorf("got ContainerPort = %d, want 9090", got.ContainerPort)
		}
		if len(got.Tags) != 1 {
			t.Errorf("got %d tags, want 1", len(got.Tags))
		}
	})

	t.Run("update nonexistent template", func(t *testing.T) {
		tmpl := Template{
			TemplateID: "nonexistent", TemplateVersion: "1.0.0",
			TemplateCategory: "x", Name: "N", Description: "d",
			Category: "c", LaunchType: "container",
		}
		err := db.UpdateTemplate(tmpl)
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete template", func(t *testing.T) {
		if err := db.DeleteTemplate("tmpl-minimal"); err != nil {
			t.Fatalf("DeleteTemplate() error = %v", err)
		}
		got, _ := db.GetTemplate("tmpl-minimal")
		if got != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete nonexistent template", func(t *testing.T) {
		err := db.DeleteTemplate("nonexistent")
		if err != sql.ErrNoRows {
			t.Errorf("got error = %v, want sql.ErrNoRows", err)
		}
	})
}

// --- SeedFromJSON tests ---

func TestSeedFromJSON(t *testing.T) {
	t.Run("seeds apps from valid JSON", func(t *testing.T) {
		db := setupTestDB(t)

		// Create temp JSON file
		config := AppConfig{
			Applications: []Application{
				{
					ID: "seed-1", Name: "Seed App 1", Description: "d",
					URL: "http://a", Icon: "i", Category: "c",
				},
				{
					ID: "seed-2", Name: "Seed App 2", Description: "d",
					URL: "http://b", Icon: "i", Category: "c",
				},
			},
		}
		data, _ := json.Marshal(config)
		tmpFile, err := os.CreateTemp("", "apps-*.json")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		tmpFile.Write(data)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		if err := db.SeedFromJSON(tmpFile.Name()); err != nil {
			t.Fatalf("SeedFromJSON() error = %v", err)
		}

		apps, _ := db.ListApps()
		if len(apps) != 2 {
			t.Fatalf("got %d apps, want 2", len(apps))
		}
	})

	t.Run("seed is idempotent", func(t *testing.T) {
		db := setupTestDB(t)

		config := AppConfig{
			Applications: []Application{
				{ID: "seed-1", Name: "App 1", Description: "d", URL: "http://a", Icon: "i", Category: "c"},
			},
		}
		data, _ := json.Marshal(config)
		tmpFile, _ := os.CreateTemp("", "apps-*.json")
		tmpFile.Write(data)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		// Seed once
		db.SeedFromJSON(tmpFile.Name())

		// Create different JSON for second seed
		config2 := AppConfig{
			Applications: []Application{
				{ID: "seed-2", Name: "App 2", Description: "d", URL: "http://b", Icon: "i", Category: "c"},
			},
		}
		data2, _ := json.Marshal(config2)
		tmpFile2, _ := os.CreateTemp("", "apps2-*.json")
		tmpFile2.Write(data2)
		tmpFile2.Close()
		defer os.Remove(tmpFile2.Name())

		// Seed again - should be a no-op because table is not empty
		if err := db.SeedFromJSON(tmpFile2.Name()); err != nil {
			t.Fatalf("second SeedFromJSON() error = %v", err)
		}

		apps, _ := db.ListApps()
		if len(apps) != 1 {
			t.Fatalf("got %d apps, want 1 (idempotent)", len(apps))
		}
		if apps[0].ID != "seed-1" {
			t.Errorf("got ID = %s, want seed-1 (original)", apps[0].ID)
		}
	})

	t.Run("seed with nonexistent file", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.SeedFromJSON("/nonexistent/apps.json")
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
	})

	t.Run("seed with invalid JSON", func(t *testing.T) {
		db := setupTestDB(t)
		tmpFile, _ := os.CreateTemp("", "bad-*.json")
		tmpFile.Write([]byte("{invalid json"))
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		err := db.SeedFromJSON(tmpFile.Name())
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})
}

// --- SeedTemplatesFromJSON / SeedTemplatesFromData tests ---

func TestSeedTemplatesFromJSON(t *testing.T) {
	t.Run("seeds templates from valid JSON file", func(t *testing.T) {
		db := setupTestDB(t)

		catalog := TemplateCatalog{
			Version: "1.0",
			Templates: []Template{
				{
					TemplateID: "tmpl-1", TemplateVersion: "1.0.0",
					TemplateCategory: "ide", Name: "T1", Description: "d",
					Category: "Dev", LaunchType: "container",
					Tags: []string{"test"},
				},
			},
		}
		data, _ := json.Marshal(catalog)
		tmpFile, _ := os.CreateTemp("", "templates-*.json")
		tmpFile.Write(data)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		if err := db.SeedTemplatesFromJSON(tmpFile.Name()); err != nil {
			t.Fatalf("SeedTemplatesFromJSON() error = %v", err)
		}

		templates, _ := db.ListTemplates()
		if len(templates) != 1 {
			t.Fatalf("got %d templates, want 1", len(templates))
		}
		if templates[0].TemplateID != "tmpl-1" {
			t.Errorf("got TemplateID = %s, want tmpl-1", templates[0].TemplateID)
		}
	})

	t.Run("seed with nonexistent file", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.SeedTemplatesFromJSON("/nonexistent/templates.json")
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
	})
}

func TestSeedTemplatesFromData(t *testing.T) {
	t.Run("seeds from data", func(t *testing.T) {
		db := setupTestDB(t)

		catalog := TemplateCatalog{
			Version: "1.0",
			Templates: []Template{
				{
					TemplateID: "data-1", TemplateVersion: "1.0.0",
					TemplateCategory: "tools", Name: "Data1", Description: "d",
					Category: "Tools", LaunchType: "container",
				},
				{
					TemplateID: "data-2", TemplateVersion: "1.0.0",
					TemplateCategory: "tools", Name: "Data2", Description: "d",
					Category: "Tools", LaunchType: "container",
				},
			},
		}
		data, _ := json.Marshal(catalog)

		if err := db.SeedTemplatesFromData(data); err != nil {
			t.Fatalf("SeedTemplatesFromData() error = %v", err)
		}

		templates, _ := db.ListTemplates()
		if len(templates) != 2 {
			t.Fatalf("got %d templates, want 2", len(templates))
		}
	})

	t.Run("seed is idempotent", func(t *testing.T) {
		db := setupTestDB(t)

		catalog := TemplateCatalog{
			Version: "1.0",
			Templates: []Template{
				{
					TemplateID: "data-1", TemplateVersion: "1.0.0",
					TemplateCategory: "tools", Name: "Original", Description: "d",
					Category: "Tools", LaunchType: "container",
				},
			},
		}
		data, _ := json.Marshal(catalog)
		db.SeedTemplatesFromData(data)

		// Seed again with different data
		catalog2 := TemplateCatalog{
			Version: "2.0",
			Templates: []Template{
				{
					TemplateID: "data-2", TemplateVersion: "2.0.0",
					TemplateCategory: "ide", Name: "New", Description: "d",
					Category: "Dev", LaunchType: "container",
				},
			},
		}
		data2, _ := json.Marshal(catalog2)
		db.SeedTemplatesFromData(data2)

		templates, _ := db.ListTemplates()
		if len(templates) != 1 {
			t.Fatalf("got %d templates, want 1 (idempotent)", len(templates))
		}
		if templates[0].Name != "Original" {
			t.Errorf("got Name = %s, want Original", templates[0].Name)
		}
	})

	t.Run("seed with invalid JSON", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.SeedTemplatesFromData([]byte("{invalid"))
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})
}

// --- Migration idempotency test ---

func TestMigrateIdempotent(t *testing.T) {
	// Verify that opening the same DB twice runs migrations without error
	tmpFile, err := os.CreateTemp("", "test-migrate-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db1, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	db1.Close()

	db2, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("second Open() error = %v (migration not idempotent)", err)
	}
	db2.Close()
}

// --- Edge case: app with no resource limits ---

func TestAppWithoutResourceLimits(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "no-limits", Name: "No Limits", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeURL,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	got, _ := db.GetApp("no-limits")
	if got.ResourceLimits != nil {
		t.Errorf("expected nil ResourceLimits, got %+v", got.ResourceLimits)
	}
}

// --- Edge case: app with empty container args ---

func TestAppWithEmptyContainerArgs(t *testing.T) {
	db := setupTestDB(t)

	app := Application{
		ID: "empty-args", Name: "Empty Args", Description: "d",
		URL: "http://x", Icon: "i", Category: "c",
		LaunchType: LaunchTypeContainer,
	}
	if err := db.CreateApp(app); err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}

	got, _ := db.GetApp("empty-args")
	if len(got.ContainerArgs) != 0 {
		t.Errorf("expected empty ContainerArgs, got %v", got.ContainerArgs)
	}
}

// --- Edge case: concurrent access via SeedFromJSON with existing data in real file path ---

func TestSeedFromJSONWithContainerApp(t *testing.T) {
	db := setupTestDB(t)

	config := AppConfig{
		Applications: []Application{
			{
				ID: "container-seed", Name: "Container Seed", Description: "d",
				URL: "http://x", Icon: "i", Category: "c",
				LaunchType:     LaunchTypeContainer,
				ContainerImage: "myimage:v1",
				ContainerPort:  8080,
				ContainerArgs:  []string{"--port", "8080"},
				ResourceLimits: &ResourceLimits{
					CPURequest: "100m",
					CPULimit:   "1",
				},
			},
		},
	}
	data, _ := json.Marshal(config)
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "apps.json")
	os.WriteFile(jsonPath, data, 0644)

	if err := db.SeedFromJSON(jsonPath); err != nil {
		t.Fatalf("SeedFromJSON() error = %v", err)
	}

	got, _ := db.GetApp("container-seed")
	if got == nil {
		t.Fatal("seeded container app not found")
	}
	if got.ContainerImage != "myimage:v1" {
		t.Errorf("got ContainerImage = %s, want myimage:v1", got.ContainerImage)
	}
	if got.ContainerPort != 8080 {
		t.Errorf("got ContainerPort = %d, want 8080", got.ContainerPort)
	}
	if len(got.ContainerArgs) != 2 {
		t.Errorf("got %d container args, want 2", len(got.ContainerArgs))
	}
	if got.ResourceLimits == nil || got.ResourceLimits.CPURequest != "100m" {
		t.Errorf("got ResourceLimits = %+v, want CPURequest=100m", got.ResourceLimits)
	}
}
