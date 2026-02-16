package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	clearEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Server defaults
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %v, want %v", cfg.Port, DefaultPort)
	}
	if cfg.DB != DefaultDBPath {
		t.Errorf("DB = %v, want %v", cfg.DB, DefaultDBPath)
	}
	if cfg.Seed != "" {
		t.Errorf("Seed = %v, want empty", cfg.Seed)
	}

	// Branding defaults
	if cfg.BrandingConfigPath != DefaultBrandingConfigPath {
		t.Errorf("BrandingConfigPath = %v, want %v", cfg.BrandingConfigPath, DefaultBrandingConfigPath)
	}
	if cfg.LogoURL != "" {
		t.Errorf("LogoURL = %v, want empty", cfg.LogoURL)
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

	// Kubernetes defaults
	if cfg.Namespace != DefaultNamespace {
		t.Errorf("Namespace = %v, want %v", cfg.Namespace, DefaultNamespace)
	}
	if cfg.Kubeconfig != "" {
		t.Errorf("Kubeconfig = %v, want empty", cfg.Kubeconfig)
	}
	if cfg.VNCSidecarImage != DefaultVNCSidecarImage {
		t.Errorf("VNCSidecarImage = %v, want %v", cfg.VNCSidecarImage, DefaultVNCSidecarImage)
	}
	if cfg.GuacdSidecarImage != DefaultGuacdSidecarImage {
		t.Errorf("GuacdSidecarImage = %v, want %v", cfg.GuacdSidecarImage, DefaultGuacdSidecarImage)
	}

	// Session defaults
	if cfg.SessionTimeout != DefaultSessionTimeout {
		t.Errorf("SessionTimeout = %v, want %v", cfg.SessionTimeout, DefaultSessionTimeout)
	}
	if cfg.SessionCleanupInterval != DefaultSessionCleanupInterval {
		t.Errorf("SessionCleanupInterval = %v, want %v", cfg.SessionCleanupInterval, DefaultSessionCleanupInterval)
	}
	if cfg.PodReadyTimeout != DefaultPodReadyTimeout {
		t.Errorf("PodReadyTimeout = %v, want %v", cfg.PodReadyTimeout, DefaultPodReadyTimeout)
	}

	// JWT defaults
	if cfg.JWTSecret != "" {
		t.Errorf("JWTSecret = %v, want empty", cfg.JWTSecret)
	}
	if cfg.JWTAccessExpiry != DefaultJWTAccessExpiry {
		t.Errorf("JWTAccessExpiry = %v, want %v", cfg.JWTAccessExpiry, DefaultJWTAccessExpiry)
	}
	if cfg.JWTRefreshExpiry != DefaultJWTRefreshExpiry {
		t.Errorf("JWTRefreshExpiry = %v, want %v", cfg.JWTRefreshExpiry, DefaultJWTRefreshExpiry)
	}
	if cfg.AdminUsername != DefaultAdminUsername {
		t.Errorf("AdminUsername = %v, want %v", cfg.AdminUsername, DefaultAdminUsername)
	}
	if cfg.AdminPassword != "" {
		t.Errorf("AdminPassword = %v, want empty", cfg.AdminPassword)
	}
	if cfg.AllowRegistration != false {
		t.Errorf("AllowRegistration = %v, want false", cfg.AllowRegistration)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	clearEnvVars(t)

	// Set custom values
	t.Setenv("SORTIE_PORT", "9000")
	t.Setenv("SORTIE_DB", "/data/app.db")
	t.Setenv("SORTIE_NAMESPACE", "sortie-prod")
	t.Setenv("SORTIE_SESSION_TIMEOUT", "60")
	t.Setenv("SORTIE_POD_READY_TIMEOUT", "180")
	t.Setenv("SORTIE_PRIMARY_COLOR", "#FF0000")

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
	if cfg.Namespace != "sortie-prod" {
		t.Errorf("Namespace = %v, want sortie-prod", cfg.Namespace)
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

func TestLoad_AllEnvVars(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_PORT", "3000")
	t.Setenv("SORTIE_DB", "/tmp/test.db")
	t.Setenv("SORTIE_SEED", "seed.json")
	t.Setenv("SORTIE_CONFIG", "custom-branding.json")
	t.Setenv("SORTIE_LOGO_URL", "https://example.com/logo.png")
	t.Setenv("SORTIE_PRIMARY_COLOR", "#AABBCC")
	t.Setenv("SORTIE_SECONDARY_COLOR", "#112233")
	t.Setenv("SORTIE_TENANT_NAME", "TestCorp")
	t.Setenv("SORTIE_NAMESPACE", "test-ns")
	t.Setenv("KUBECONFIG", "/home/user/.kube/config")
	t.Setenv("SORTIE_VNC_SIDECAR_IMAGE", "custom/vnc:v2")
	t.Setenv("SORTIE_GUACD_SIDECAR_IMAGE", "custom/guacd:v2")
	t.Setenv("SORTIE_SESSION_TIMEOUT", "30")
	t.Setenv("SORTIE_SESSION_CLEANUP_INTERVAL", "10")
	t.Setenv("SORTIE_POD_READY_TIMEOUT", "60")
	t.Setenv("SORTIE_JWT_SECRET", "my-secret-key")
	t.Setenv("SORTIE_JWT_ACCESS_EXPIRY", "30")
	t.Setenv("SORTIE_JWT_REFRESH_EXPIRY", "48")
	t.Setenv("SORTIE_ADMIN_USERNAME", "superadmin")
	t.Setenv("SORTIE_ADMIN_PASSWORD", "s3cret")
	t.Setenv("SORTIE_ALLOW_REGISTRATION", "true")
	t.Setenv("SORTIE_VIDEO_RECORDING_ENABLED", "true")
	t.Setenv("SORTIE_RECORDING_STORAGE_BACKEND", "s3")
	t.Setenv("SORTIE_RECORDING_STORAGE_PATH", "/mnt/recordings")
	t.Setenv("SORTIE_RECORDING_MAX_SIZE_MB", "2048")
	t.Setenv("SORTIE_RECORDING_RETENTION_DAYS", "30")
	t.Setenv("SORTIE_RECORDING_S3_BUCKET", "my-recordings")
	t.Setenv("SORTIE_RECORDING_S3_REGION", "eu-west-1")
	t.Setenv("SORTIE_RECORDING_S3_ENDPOINT", "https://minio.local:9000")
	t.Setenv("SORTIE_RECORDING_S3_PREFIX", "tenant1/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != 3000 {
		t.Errorf("Port = %v, want 3000", cfg.Port)
	}
	if cfg.DB != "/tmp/test.db" {
		t.Errorf("DB = %v, want /tmp/test.db", cfg.DB)
	}
	if cfg.Seed != "seed.json" {
		t.Errorf("Seed = %v, want seed.json", cfg.Seed)
	}
	if cfg.BrandingConfigPath != "custom-branding.json" {
		t.Errorf("BrandingConfigPath = %v, want custom-branding.json", cfg.BrandingConfigPath)
	}
	if cfg.LogoURL != "https://example.com/logo.png" {
		t.Errorf("LogoURL = %v, want https://example.com/logo.png", cfg.LogoURL)
	}
	if cfg.PrimaryColor != "#AABBCC" {
		t.Errorf("PrimaryColor = %v, want #AABBCC", cfg.PrimaryColor)
	}
	if cfg.SecondaryColor != "#112233" {
		t.Errorf("SecondaryColor = %v, want #112233", cfg.SecondaryColor)
	}
	if cfg.TenantName != "TestCorp" {
		t.Errorf("TenantName = %v, want TestCorp", cfg.TenantName)
	}
	if cfg.Namespace != "test-ns" {
		t.Errorf("Namespace = %v, want test-ns", cfg.Namespace)
	}
	if cfg.Kubeconfig != "/home/user/.kube/config" {
		t.Errorf("Kubeconfig = %v, want /home/user/.kube/config", cfg.Kubeconfig)
	}
	if cfg.VNCSidecarImage != "custom/vnc:v2" {
		t.Errorf("VNCSidecarImage = %v, want custom/vnc:v2", cfg.VNCSidecarImage)
	}
	if cfg.GuacdSidecarImage != "custom/guacd:v2" {
		t.Errorf("GuacdSidecarImage = %v, want custom/guacd:v2", cfg.GuacdSidecarImage)
	}
	if cfg.SessionTimeout != 30*time.Minute {
		t.Errorf("SessionTimeout = %v, want 30m", cfg.SessionTimeout)
	}
	if cfg.SessionCleanupInterval != 10*time.Minute {
		t.Errorf("SessionCleanupInterval = %v, want 10m", cfg.SessionCleanupInterval)
	}
	if cfg.PodReadyTimeout != 60*time.Second {
		t.Errorf("PodReadyTimeout = %v, want 60s", cfg.PodReadyTimeout)
	}
	if cfg.JWTSecret != "my-secret-key" {
		t.Errorf("JWTSecret = %v, want my-secret-key", cfg.JWTSecret)
	}
	if cfg.JWTAccessExpiry != 30*time.Minute {
		t.Errorf("JWTAccessExpiry = %v, want 30m", cfg.JWTAccessExpiry)
	}
	if cfg.JWTRefreshExpiry != 48*time.Hour {
		t.Errorf("JWTRefreshExpiry = %v, want 48h", cfg.JWTRefreshExpiry)
	}
	if cfg.AdminUsername != "superadmin" {
		t.Errorf("AdminUsername = %v, want superadmin", cfg.AdminUsername)
	}
	if cfg.AdminPassword != "s3cret" {
		t.Errorf("AdminPassword = %v, want s3cret", cfg.AdminPassword)
	}
	if cfg.AllowRegistration != true {
		t.Errorf("AllowRegistration = %v, want true", cfg.AllowRegistration)
	}
	if cfg.VideoRecordingEnabled != true {
		t.Errorf("VideoRecordingEnabled = %v, want true", cfg.VideoRecordingEnabled)
	}
	if cfg.RecordingStorageBackend != "s3" {
		t.Errorf("RecordingStorageBackend = %v, want s3", cfg.RecordingStorageBackend)
	}
	if cfg.RecordingStoragePath != "/mnt/recordings" {
		t.Errorf("RecordingStoragePath = %v, want /mnt/recordings", cfg.RecordingStoragePath)
	}
	if cfg.RecordingMaxSizeMB != 2048 {
		t.Errorf("RecordingMaxSizeMB = %v, want 2048", cfg.RecordingMaxSizeMB)
	}
	if cfg.RecordingRetentionDays != 30 {
		t.Errorf("RecordingRetentionDays = %v, want 30", cfg.RecordingRetentionDays)
	}
	if cfg.RecordingS3Bucket != "my-recordings" {
		t.Errorf("RecordingS3Bucket = %v, want my-recordings", cfg.RecordingS3Bucket)
	}
	if cfg.RecordingS3Region != "eu-west-1" {
		t.Errorf("RecordingS3Region = %v, want eu-west-1", cfg.RecordingS3Region)
	}
	if cfg.RecordingS3Endpoint != "https://minio.local:9000" {
		t.Errorf("RecordingS3Endpoint = %v, want https://minio.local:9000", cfg.RecordingS3Endpoint)
	}
	if cfg.RecordingS3Prefix != "tenant1/" {
		t.Errorf("RecordingS3Prefix = %v, want tenant1/", cfg.RecordingS3Prefix)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_PORT", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid port")
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_SESSION_TIMEOUT", "-5")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for negative timeout")
	}
}

func TestLoad_InvalidColor(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_PRIMARY_COLOR", "red")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid color format")
	}
}

func TestLoad_InvalidSecondaryColor(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_SECONDARY_COLOR", "not-a-color")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid secondary color")
	}
}

func TestLoad_InvalidSessionCleanupInterval(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric", "abc"},
		{"negative", "-1"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv("SORTIE_SESSION_CLEANUP_INTERVAL", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() expected error for session cleanup interval %q", tt.value)
			}
		})
	}
}

func TestLoad_InvalidPodReadyTimeout(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric", "xyz"},
		{"negative", "-10"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv("SORTIE_POD_READY_TIMEOUT", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() expected error for pod ready timeout %q", tt.value)
			}
		})
	}
}

func TestLoad_InvalidSessionTimeout_NonNumeric(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("SORTIE_SESSION_TIMEOUT", "abc")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for non-numeric session timeout")
	}
}

func TestLoad_InvalidSessionTimeout_Zero(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("SORTIE_SESSION_TIMEOUT", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for zero session timeout")
	}
}

func TestLoad_InvalidJWTAccessExpiry(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric", "abc"},
		{"negative", "-5"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv("SORTIE_JWT_ACCESS_EXPIRY", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() expected error for JWT access expiry %q", tt.value)
			}
		})
	}
}

func TestLoad_InvalidJWTRefreshExpiry(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric", "abc"},
		{"negative", "-1"},
		{"zero", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv("SORTIE_JWT_REFRESH_EXPIRY", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() expected error for JWT refresh expiry %q", tt.value)
			}
		})
	}
}

func TestLoad_AllowRegistrationParsing(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"true lowercase", "true", true},
		{"TRUE uppercase", "TRUE", true},
		{"True mixed", "True", true},
		{"1", "1", true},
		{"false", "false", false},
		{"0", "0", false},
		{"empty-like", "no", false},
		{"random", "yes", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv("SORTIE_ALLOW_REGISTRATION", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.AllowRegistration != tt.want {
				t.Errorf("AllowRegistration = %v, want %v for input %q", cfg.AllowRegistration, tt.want, tt.value)
			}
		})
	}
}

func TestLoad_MultipleParseErrors(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_PORT", "invalid")
	t.Setenv("SORTIE_SESSION_TIMEOUT", "bad")
	t.Setenv("SORTIE_JWT_ACCESS_EXPIRY", "nope")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for multiple invalid values")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "SORTIE_PORT") {
		t.Errorf("error should mention SORTIE_PORT: %s", errStr)
	}
	if !strings.Contains(errStr, "SORTIE_SESSION_TIMEOUT") {
		t.Errorf("error should mention SORTIE_SESSION_TIMEOUT: %s", errStr)
	}
	if !strings.Contains(errStr, "SORTIE_JWT_ACCESS_EXPIRY") {
		t.Errorf("error should mention SORTIE_JWT_ACCESS_EXPIRY: %s", errStr)
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

func TestValidate_EmptyDB(t *testing.T) {
	cfg := &Config{
		Port:            8080,
		DB:              "",
		VNCSidecarImage: "test:latest",
		PrimaryColor:    "#FFFFFF",
		SecondaryColor:  "#000000",
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Error("Validate() expected error for empty DB")
	}

	found := false
	for _, e := range errs {
		if e.Field == "SORTIE_DB" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Validate() expected SORTIE_DB in validation errors")
	}
}

func TestValidate_EmptyVNCSidecarImage(t *testing.T) {
	cfg := &Config{
		Port:            8080,
		DB:              "test.db",
		VNCSidecarImage: "",
		PrimaryColor:    "#FFFFFF",
		SecondaryColor:  "#000000",
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Error("Validate() expected error for empty VNCSidecarImage")
	}

	found := false
	for _, e := range errs {
		if e.Field == "SORTIE_VNC_SIDECAR_IMAGE" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Validate() expected SORTIE_VNC_SIDECAR_IMAGE in validation errors")
	}
}

func TestValidate_InvalidColors(t *testing.T) {
	tests := []struct {
		name      string
		primary   string
		secondary string
		wantErr   bool
	}{
		{"both valid", "#FFFFFF", "#000000", false},
		{"invalid primary", "red", "#000000", true},
		{"invalid secondary", "#FFFFFF", "blue", true},
		{"both invalid", "red", "blue", true},
		{"empty primary skips validation", "", "#000000", false},
		{"empty secondary skips validation", "#FFFFFF", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Port:            8080,
				DB:              "test.db",
				VNCSidecarImage: "test:latest",
				PrimaryColor:    tt.primary,
				SecondaryColor:  tt.secondary,
			}

			errs := cfg.Validate()
			gotErr := len(errs) > 0
			if gotErr != tt.wantErr {
				t.Errorf("Validate() primary=%q secondary=%q gotErr=%v, wantErr=%v, errs=%v", tt.primary, tt.secondary, gotErr, tt.wantErr, errs)
			}
		})
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Port:            0,
		DB:              "",
		VNCSidecarImage: "",
		PrimaryColor:    "bad",
		SecondaryColor:  "bad",
	}

	errs := cfg.Validate()
	if len(errs) < 4 {
		t.Errorf("Validate() expected at least 4 errors, got %d: %v", len(errs), errs)
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
		{"#", false},         // just hash
		{"#12345", false},    // 5 hex chars
		{"#ZZZZZZ", false},   // invalid hex chars
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
	t.Setenv("SORTIE_PORT", "8000")

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

func TestLoadWithFlags_DefaultsDoNotOverride(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_PORT", "9000")
	t.Setenv("SORTIE_DB", "/data/custom.db")

	// Passing default values (0 for port, DefaultDBPath for db) should not override env
	cfg, err := LoadWithFlags(0, "", "")
	if err != nil {
		t.Fatalf("LoadWithFlags() error = %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Port = %v, want 9000 (zero flag should not override env)", cfg.Port)
	}
	if cfg.DB != "/data/custom.db" {
		t.Errorf("DB = %v, want /data/custom.db (empty flag should not override env)", cfg.DB)
	}
}

func TestLoadWithFlags_DefaultPortDoesNotOverride(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_PORT", "9000")

	// Passing DefaultPort should not override env
	cfg, err := LoadWithFlags(DefaultPort, DefaultDBPath, "")
	if err != nil {
		t.Fatalf("LoadWithFlags() error = %v", err)
	}

	// DefaultPort (8080) == the flag value, so it won't override the env value of 9000
	if cfg.Port != 9000 {
		t.Errorf("Port = %v, want 9000 (DefaultPort flag should not override env)", cfg.Port)
	}
}

func TestLoadWithFlags_InvalidOverrideCausesValidationError(t *testing.T) {
	clearEnvVars(t)

	// Port 99999 is out of valid range
	_, err := LoadWithFlags(99999, "", "")
	if err == nil {
		t.Fatal("LoadWithFlags() expected error for out-of-range port override")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{Field: "TEST_FIELD", Message: "something went wrong"}
	got := err.Error()
	want := "TEST_FIELD: something went wrong"
	if got != want {
		t.Errorf("ValidationError.Error() = %q, want %q", got, want)
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
	if !strings.Contains(s, "FIELD1") || !strings.Contains(s, "error 1") {
		t.Errorf("ValidationErrors.Error() missing first error: %s", s)
	}
	if !strings.Contains(s, "FIELD2") || !strings.Contains(s, "error 2") {
		t.Errorf("ValidationErrors.Error() missing second error: %s", s)
	}
	if !strings.Contains(s, "configuration errors:") {
		t.Errorf("ValidationErrors.Error() missing prefix: %s", s)
	}
}

func TestValidationErrors_Empty(t *testing.T) {
	errs := ValidationErrors{}
	s := errs.Error()
	if s != "" {
		t.Errorf("ValidationErrors.Error() for empty = %q, want empty string", s)
	}
}

func TestValidationErrors_Single(t *testing.T) {
	errs := ValidationErrors{
		{Field: "FIELD1", Message: "only error"},
	}
	s := errs.Error()
	if !strings.Contains(s, "FIELD1") || !strings.Contains(s, "only error") {
		t.Errorf("ValidationErrors.Error() single error not formatted correctly: %s", s)
	}
}

func TestLoad_QuotaDefaults(t *testing.T) {
	clearEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MaxSessionsPerUser != DefaultMaxSessionsPerUser {
		t.Errorf("MaxSessionsPerUser = %v, want %v", cfg.MaxSessionsPerUser, DefaultMaxSessionsPerUser)
	}
	if cfg.MaxGlobalSessions != DefaultMaxGlobalSessions {
		t.Errorf("MaxGlobalSessions = %v, want %v", cfg.MaxGlobalSessions, DefaultMaxGlobalSessions)
	}
	if cfg.DefaultCPURequest != DefaultDefaultCPURequest {
		t.Errorf("DefaultCPURequest = %v, want %v", cfg.DefaultCPURequest, DefaultDefaultCPURequest)
	}
	if cfg.DefaultCPULimit != DefaultDefaultCPULimit {
		t.Errorf("DefaultCPULimit = %v, want %v", cfg.DefaultCPULimit, DefaultDefaultCPULimit)
	}
	if cfg.DefaultMemRequest != DefaultDefaultMemRequest {
		t.Errorf("DefaultMemRequest = %v, want %v", cfg.DefaultMemRequest, DefaultDefaultMemRequest)
	}
	if cfg.DefaultMemLimit != DefaultDefaultMemLimit {
		t.Errorf("DefaultMemLimit = %v, want %v", cfg.DefaultMemLimit, DefaultDefaultMemLimit)
	}
}

func TestLoad_QuotaFromEnv(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_MAX_SESSIONS_PER_USER", "10")
	t.Setenv("SORTIE_MAX_GLOBAL_SESSIONS", "200")
	t.Setenv("SORTIE_DEFAULT_CPU_REQUEST", "250m")
	t.Setenv("SORTIE_DEFAULT_CPU_LIMIT", "4")
	t.Setenv("SORTIE_DEFAULT_MEM_REQUEST", "1Gi")
	t.Setenv("SORTIE_DEFAULT_MEM_LIMIT", "4Gi")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MaxSessionsPerUser != 10 {
		t.Errorf("MaxSessionsPerUser = %v, want 10", cfg.MaxSessionsPerUser)
	}
	if cfg.MaxGlobalSessions != 200 {
		t.Errorf("MaxGlobalSessions = %v, want 200", cfg.MaxGlobalSessions)
	}
	if cfg.DefaultCPURequest != "250m" {
		t.Errorf("DefaultCPURequest = %v, want 250m", cfg.DefaultCPURequest)
	}
	if cfg.DefaultCPULimit != "4" {
		t.Errorf("DefaultCPULimit = %v, want 4", cfg.DefaultCPULimit)
	}
	if cfg.DefaultMemRequest != "1Gi" {
		t.Errorf("DefaultMemRequest = %v, want 1Gi", cfg.DefaultMemRequest)
	}
	if cfg.DefaultMemLimit != "4Gi" {
		t.Errorf("DefaultMemLimit = %v, want 4Gi", cfg.DefaultMemLimit)
	}
}

func TestLoad_QuotaUnlimited(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_MAX_SESSIONS_PER_USER", "0")
	t.Setenv("SORTIE_MAX_GLOBAL_SESSIONS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.MaxSessionsPerUser != 0 {
		t.Errorf("MaxSessionsPerUser = %v, want 0 (unlimited)", cfg.MaxSessionsPerUser)
	}
	if cfg.MaxGlobalSessions != 0 {
		t.Errorf("MaxGlobalSessions = %v, want 0 (unlimited)", cfg.MaxGlobalSessions)
	}
}

func TestLoad_QuotaInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"negative per-user", "SORTIE_MAX_SESSIONS_PER_USER", "-1"},
		{"non-numeric per-user", "SORTIE_MAX_SESSIONS_PER_USER", "abc"},
		{"negative global", "SORTIE_MAX_GLOBAL_SESSIONS", "-5"},
		{"non-numeric global", "SORTIE_MAX_GLOBAL_SESSIONS", "xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() expected error for %s=%s", tt.key, tt.value)
			}
		})
	}
}

func TestLoad_VideoRecordingDefaults(t *testing.T) {
	clearEnvVars(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.VideoRecordingEnabled != false {
		t.Errorf("VideoRecordingEnabled = %v, want false", cfg.VideoRecordingEnabled)
	}
	if cfg.RecordingStorageBackend != DefaultRecordingStorageBackend {
		t.Errorf("RecordingStorageBackend = %v, want %v", cfg.RecordingStorageBackend, DefaultRecordingStorageBackend)
	}
	if cfg.RecordingStoragePath != DefaultRecordingStoragePath {
		t.Errorf("RecordingStoragePath = %v, want %v", cfg.RecordingStoragePath, DefaultRecordingStoragePath)
	}
	if cfg.RecordingMaxSizeMB != DefaultRecordingMaxSizeMB {
		t.Errorf("RecordingMaxSizeMB = %v, want %v", cfg.RecordingMaxSizeMB, DefaultRecordingMaxSizeMB)
	}
	if cfg.RecordingRetentionDays != 0 {
		t.Errorf("RecordingRetentionDays = %v, want 0", cfg.RecordingRetentionDays)
	}
	if cfg.RecordingS3Region != DefaultRecordingS3Region {
		t.Errorf("RecordingS3Region = %v, want %v", cfg.RecordingS3Region, DefaultRecordingS3Region)
	}
	if cfg.RecordingS3Prefix != DefaultRecordingS3Prefix {
		t.Errorf("RecordingS3Prefix = %v, want %v", cfg.RecordingS3Prefix, DefaultRecordingS3Prefix)
	}
	if cfg.RecordingS3Bucket != "" {
		t.Errorf("RecordingS3Bucket = %v, want empty", cfg.RecordingS3Bucket)
	}
	if cfg.RecordingS3Endpoint != "" {
		t.Errorf("RecordingS3Endpoint = %v, want empty", cfg.RecordingS3Endpoint)
	}
}

func TestValidate_S3BucketRequiredWhenS3Backend(t *testing.T) {
	clearEnvVars(t)

	// backend=s3 without bucket should fail validation
	t.Setenv("SORTIE_RECORDING_STORAGE_BACKEND", "s3")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error when s3 backend has no bucket")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "SORTIE_RECORDING_S3_BUCKET") {
		t.Errorf("error should mention SORTIE_RECORDING_S3_BUCKET, got: %s", errStr)
	}
}

func TestValidate_LocalBackendDoesNotRequireS3Bucket(t *testing.T) {
	clearEnvVars(t)

	// backend=local without bucket should pass
	t.Setenv("SORTIE_RECORDING_STORAGE_BACKEND", "local")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error for local backend: %v", err)
	}
}

func TestLoad_VideoRecordingFromEnv(t *testing.T) {
	clearEnvVars(t)

	t.Setenv("SORTIE_VIDEO_RECORDING_ENABLED", "true")
	t.Setenv("SORTIE_RECORDING_STORAGE_BACKEND", "s3")
	t.Setenv("SORTIE_RECORDING_STORAGE_PATH", "/custom/recordings")
	t.Setenv("SORTIE_RECORDING_MAX_SIZE_MB", "1024")
	t.Setenv("SORTIE_RECORDING_RETENTION_DAYS", "90")
	t.Setenv("SORTIE_RECORDING_S3_BUCKET", "test-bucket")
	t.Setenv("SORTIE_RECORDING_S3_REGION", "ap-southeast-1")
	t.Setenv("SORTIE_RECORDING_S3_ENDPOINT", "https://s3.local")
	t.Setenv("SORTIE_RECORDING_S3_PREFIX", "vids/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.VideoRecordingEnabled != true {
		t.Errorf("VideoRecordingEnabled = %v, want true", cfg.VideoRecordingEnabled)
	}
	if cfg.RecordingStorageBackend != "s3" {
		t.Errorf("RecordingStorageBackend = %v, want s3", cfg.RecordingStorageBackend)
	}
	if cfg.RecordingStoragePath != "/custom/recordings" {
		t.Errorf("RecordingStoragePath = %v, want /custom/recordings", cfg.RecordingStoragePath)
	}
	if cfg.RecordingMaxSizeMB != 1024 {
		t.Errorf("RecordingMaxSizeMB = %v, want 1024", cfg.RecordingMaxSizeMB)
	}
	if cfg.RecordingRetentionDays != 90 {
		t.Errorf("RecordingRetentionDays = %v, want 90", cfg.RecordingRetentionDays)
	}
	if cfg.RecordingS3Bucket != "test-bucket" {
		t.Errorf("RecordingS3Bucket = %v, want test-bucket", cfg.RecordingS3Bucket)
	}
	if cfg.RecordingS3Region != "ap-southeast-1" {
		t.Errorf("RecordingS3Region = %v, want ap-southeast-1", cfg.RecordingS3Region)
	}
	if cfg.RecordingS3Endpoint != "https://s3.local" {
		t.Errorf("RecordingS3Endpoint = %v, want https://s3.local", cfg.RecordingS3Endpoint)
	}
	if cfg.RecordingS3Prefix != "vids/" {
		t.Errorf("RecordingS3Prefix = %v, want vids/", cfg.RecordingS3Prefix)
	}
}

func TestLoad_VideoRecordingInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"negative max size", "SORTIE_RECORDING_MAX_SIZE_MB", "-1"},
		{"non-numeric max size", "SORTIE_RECORDING_MAX_SIZE_MB", "abc"},
		{"negative retention", "SORTIE_RECORDING_RETENTION_DAYS", "-5"},
		{"non-numeric retention", "SORTIE_RECORDING_RETENTION_DAYS", "xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() expected error for %s=%s", tt.key, tt.value)
			}
		})
	}
}

func clearEnvVars(t *testing.T) {
	t.Helper()
	envVars := []string{
		"SORTIE_PORT",
		"SORTIE_DB",
		"SORTIE_SEED",
		"SORTIE_CONFIG",
		"SORTIE_LOGO_URL",
		"SORTIE_PRIMARY_COLOR",
		"SORTIE_SECONDARY_COLOR",
		"SORTIE_TENANT_NAME",
		"SORTIE_NAMESPACE",
		"KUBECONFIG",
		"SORTIE_VNC_SIDECAR_IMAGE",
		"SORTIE_GUACD_SIDECAR_IMAGE",
		"SORTIE_SESSION_TIMEOUT",
		"SORTIE_SESSION_CLEANUP_INTERVAL",
		"SORTIE_POD_READY_TIMEOUT",
		"SORTIE_JWT_SECRET",
		"SORTIE_JWT_ACCESS_EXPIRY",
		"SORTIE_JWT_REFRESH_EXPIRY",
		"SORTIE_ADMIN_USERNAME",
		"SORTIE_ADMIN_PASSWORD",
		"SORTIE_ALLOW_REGISTRATION",
		"SORTIE_MAX_SESSIONS_PER_USER",
		"SORTIE_MAX_GLOBAL_SESSIONS",
		"SORTIE_DEFAULT_CPU_REQUEST",
		"SORTIE_DEFAULT_CPU_LIMIT",
		"SORTIE_DEFAULT_MEM_REQUEST",
		"SORTIE_DEFAULT_MEM_LIMIT",
		"SORTIE_VIDEO_RECORDING_ENABLED",
		"SORTIE_RECORDING_STORAGE_BACKEND",
		"SORTIE_RECORDING_STORAGE_PATH",
		"SORTIE_RECORDING_MAX_SIZE_MB",
		"SORTIE_RECORDING_RETENTION_DAYS",
		"SORTIE_RECORDING_S3_BUCKET",
		"SORTIE_RECORDING_S3_REGION",
		"SORTIE_RECORDING_S3_ENDPOINT",
		"SORTIE_RECORDING_S3_PREFIX",
	}
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}
