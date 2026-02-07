package db

import (
	"os"
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

	t.Run("update session status", func(t *testing.T) {
		if err := db.UpdateSessionStatus("sess-1", SessionStatusRunning); err != nil {
			t.Fatalf("UpdateSessionStatus() error = %v", err)
		}

		got, _ := db.GetSession("sess-1")
		if got.Status != SessionStatusRunning {
			t.Errorf("got Status = %s, want running", got.Status)
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
}

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
