package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear all env vars that might affect the test
	clearEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check defaults
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %v, want %v", cfg.Port, DefaultPort)
	}
	if cfg.DB != DefaultDBPath {
		t.Errorf("DB = %v, want %v", cfg.DB, DefaultDBPath)
	}
	if cfg.PrimaryColor != DefaultPrimaryColor {
		t.Errorf("PrimaryColor = %v, want %v", cfg.PrimaryColor, DefaultPrimaryColor)
	}
	if cfg.SecondaryColor != DefaultSecondaryColor {
		t.Errorf("SecondaryColor = %v, want %v", cfg.SecondaryColor, DefaultSecondaryColor)
	}
	if cfg.TenantName != DefaultTenantName {
		t.Errorf("TenantName = %v, want %v", cfg.TenantName, DefaultTenantName)
	}
	if cfg.Namespace != DefaultNamespace {
		t.Errorf("Namespace = %v, want %v", cfg.Namespace, DefaultNamespace)
	}
	if cfg.VNCSidecarImage != DefaultVNCSidecarImage {
		t.Errorf("VNCSidecarImage = %v, want %v", cfg.VNCSidecarImage, DefaultVNCSidecarImage)
	}
	if cfg.SessionTimeout != DefaultSessionTimeout {
		t.Errorf("SessionTimeout = %v, want %v", cfg.SessionTimeout, DefaultSessionTimeout)
	}
	if cfg.SessionCleanupInterval != DefaultSessionCleanupInterval {
		t.Errorf("SessionCleanupInterval = %v, want %v", cfg.SessionCleanupInterval, DefaultSessionCleanupInterval)
	}
	if cfg.PodReadyTimeout != DefaultPodReadyTimeout {
		t.Errorf("PodReadyTimeout = %v, want %v", cfg.PodReadyTimeout, DefaultPodReadyTimeout)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	clearEnvVars(t)

	// Set custom values
	t.Setenv("LAUNCHPAD_PORT", "9000")
	t.Setenv("LAUNCHPAD_DB", "/data/app.db")
	t.Setenv("LAUNCHPAD_NAMESPACE", "launchpad-prod")
	t.Setenv("LAUNCHPAD_SESSION_TIMEOUT", "60")
	t.Setenv("LAUNCHPAD_POD_READY_TIMEOUT", "180")
	t.Setenv("LAUNCHPAD_PRIMARY_COLOR", "#FF0000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Port = %v, want 9000", cfg.Port)
	}
	if cfg.DB != "/data/app.db" {
		t.Errorf("DB = %v, want /data/app.db", cfg.DB)
	}
	if cfg.Namespace != "launchpad-prod" {
		t.Errorf("Namespace = %v, want launchpad-prod", cfg.Namespace)
	}
	if cfg.SessionTimeout != 60*time.Minute {
		t.Errorf("SessionTimeout = %v, want 60m", cfg.SessionTimeout)
	}
	if cfg.PodReadyTimeout != 180*time.Second {
		t.Errorf("PodReadyTimeout = %v, want 180s", cfg.PodReadyTimeout)
	}
	if cfg.PrimaryColor != "#FF0000" {
		t.Errorf("PrimaryColor = %v, want #FF0000", cfg.PrimaryColor)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("LAUNCHPAD_PORT", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid port")
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("LAUNCHPAD_SESSION_TIMEOUT", "-5")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for negative timeout")
	}
}

func TestLoad_InvalidColor(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("LAUNCHPAD_PRIMARY_COLOR", "red")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid color format")
	}
}

func TestValidate_PortRange(t *testing.T) {
	tests := []struct {
		port    int
		wantErr bool
	}{
		{0, true},
		{1, false},
		{8080, false},
		{65535, false},
		{65536, true},
		{-1, true},
	}

	for _, tt := range tests {
		cfg := &Config{
			Port:            tt.port,
			DB:              "test.db",
			VNCSidecarImage: "test:latest",
			PrimaryColor:    "#FFFFFF",
			SecondaryColor:  "#000000",
		}

		errs := cfg.Validate()
		gotErr := len(errs) > 0

		if gotErr != tt.wantErr {
			t.Errorf("Validate() port=%d, gotErr=%v, wantErr=%v", tt.port, gotErr, tt.wantErr)
		}
	}
}

func TestIsValidHexColor(t *testing.T) {
	tests := []struct {
		color string
		valid bool
	}{
		{"#FFFFFF", true},
		{"#000000", true},
		{"#398D9B", true},
		{"#ff00ff", true},
		{"#FfAaBb", true},
		{"FFFFFF", false},    // missing #
		{"#FFF", false},      // too short
		{"#FFFFFFAA", false}, // too long
		{"#GGGGGG", false},   // invalid chars
		{"red", false},       // named color
		{"", false},          // empty
	}

	for _, tt := range tests {
		got := isValidHexColor(tt.color)
		if got != tt.valid {
			t.Errorf("isValidHexColor(%q) = %v, want %v", tt.color, got, tt.valid)
		}
	}
}

func TestLoadWithFlags(t *testing.T) {
	clearEnvVars(t)

	// Set env var
	t.Setenv("LAUNCHPAD_PORT", "8000")

	// Flag overrides env
	cfg, err := LoadWithFlags(9000, "/custom/path.db", "seed.json")
	if err != nil {
		t.Fatalf("LoadWithFlags() error = %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Port = %v, want 9000 (flag should override env)", cfg.Port)
	}
	if cfg.DB != "/custom/path.db" {
		t.Errorf("DB = %v, want /custom/path.db", cfg.DB)
	}
	if cfg.Seed != "seed.json" {
		t.Errorf("Seed = %v, want seed.json", cfg.Seed)
	}
}

func TestValidationErrors_String(t *testing.T) {
	errs := ValidationErrors{
		{Field: "FIELD1", Message: "error 1"},
		{Field: "FIELD2", Message: "error 2"},
	}

	s := errs.Error()
	if s == "" {
		t.Error("ValidationErrors.Error() returned empty string")
	}
	if !contains(s, "FIELD1") || !contains(s, "error 1") {
		t.Errorf("ValidationErrors.Error() missing first error: %s", s)
	}
	if !contains(s, "FIELD2") || !contains(s, "error 2") {
		t.Errorf("ValidationErrors.Error() missing second error: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func clearEnvVars(t *testing.T) {
	t.Helper()
	envVars := []string{
		"LAUNCHPAD_PORT",
		"LAUNCHPAD_DB",
		"LAUNCHPAD_SEED",
		"LAUNCHPAD_CONFIG",
		"LAUNCHPAD_LOGO_URL",
		"LAUNCHPAD_PRIMARY_COLOR",
		"LAUNCHPAD_SECONDARY_COLOR",
		"LAUNCHPAD_TENANT_NAME",
		"LAUNCHPAD_NAMESPACE",
		"KUBECONFIG",
		"LAUNCHPAD_VNC_SIDECAR_IMAGE",
		"LAUNCHPAD_SESSION_TIMEOUT",
		"LAUNCHPAD_SESSION_CLEANUP_INTERVAL",
		"LAUNCHPAD_POD_READY_TIMEOUT",
		// Legacy env vars
		"SESSION_TIMEOUT",
		"SESSION_CLEANUP_INTERVAL",
		"POD_READY_TIMEOUT",
	}
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}
