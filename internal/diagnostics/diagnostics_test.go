package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/config"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/plugins"
)

func setupTestCollector(t *testing.T) *Collector {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		Port:                   8080,
		DB:                     ":memory:",
		Namespace:              "test-ns",
		SessionTimeout:         30 * time.Minute,
		SessionCleanupInterval: 5 * time.Minute,
		PodReadyTimeout:        2 * time.Minute,
		MaxSessionsPerUser:     5,
		MaxGlobalSessions:      50,
		DefaultCPURequest:      "500m",
		DefaultCPULimit:        "2",
		DefaultMemRequest:      "512Mi",
		DefaultMemLimit:        "2Gi",
	}

	registry := plugins.NewRegistry()
	started := time.Now().Add(-1 * time.Hour)

	return NewCollector(database, cfg, registry, started)
}

func TestCollect(t *testing.T) {
	collector := setupTestCollector(t)

	bundle, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	// Verify system info
	if bundle.System.GoVersion == "" {
		t.Error("expected non-empty GoVersion")
	}
	if bundle.System.GOOS == "" {
		t.Error("expected non-empty GOOS")
	}
	if bundle.System.GOARCH == "" {
		t.Error("expected non-empty GOARCH")
	}
	if bundle.System.NumCPU <= 0 {
		t.Error("expected positive NumCPU")
	}
	if bundle.System.UptimeSeconds <= 0 {
		t.Error("expected positive uptime")
	}

	// Verify redacted config
	if bundle.Config.Port != 8080 {
		t.Errorf("expected port 8080, got %d", bundle.Config.Port)
	}
	if bundle.Config.Namespace != "test-ns" {
		t.Errorf("expected namespace test-ns, got %s", bundle.Config.Namespace)
	}
	if bundle.Config.AuthEnabled {
		t.Error("expected auth disabled (no JWT secret)")
	}
	if bundle.Config.MaxSessionsPerUser != 5 {
		t.Errorf("expected MaxSessionsPerUser 5, got %d", bundle.Config.MaxSessionsPerUser)
	}

	// Verify health
	if bundle.Health.Overall != "healthy" {
		t.Errorf("expected overall healthy, got %s", bundle.Health.Overall)
	}
	if !bundle.Health.Database.Healthy {
		t.Error("expected database healthy")
	}

	// Verify runtime
	if bundle.Runtime.NumGoroutine <= 0 {
		t.Error("expected positive goroutine count")
	}
	if bundle.Runtime.Memory.SysMB <= 0 {
		t.Error("expected positive system memory")
	}

	// Verify generated_at is recent
	if time.Since(bundle.GeneratedAt) > 5*time.Second {
		t.Error("expected generated_at to be recent")
	}
}

func TestCollectJSON(t *testing.T) {
	collector := setupTestCollector(t)

	bundle, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	// Verify it can be marshaled to JSON
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal bundle: %v", err)
	}

	// Verify it can be unmarshaled back
	var decoded Bundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal bundle: %v", err)
	}

	if decoded.System.GoVersion != bundle.System.GoVersion {
		t.Error("decoded GoVersion mismatch")
	}
}

func TestWriteTarGz(t *testing.T) {
	collector := setupTestCollector(t)

	var buf bytes.Buffer
	if err := collector.WriteTarGz(context.Background(), &buf); err != nil {
		t.Fatalf("WriteTarGz returned error: %v", err)
	}

	// Verify it's a valid gzip archive
	gzr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	// Verify it's a valid tar archive with expected files
	tr := tar.NewReader(gzr)
	expectedFiles := map[string]bool{
		"diagnostics/bundle.json":   false,
		"diagnostics/system.json":   false,
		"diagnostics/config.json":   false,
		"diagnostics/health.json":   false,
		"diagnostics/database.json": false,
		"diagnostics/sessions.json": false,
		"diagnostics/runtime.json":  false,
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("error reading tar: %v", err)
		}

		if _, ok := expectedFiles[header.Name]; ok {
			expectedFiles[header.Name] = true
		} else {
			t.Errorf("unexpected file in archive: %s", header.Name)
		}

		// Verify each file contains valid JSON
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("error reading file %s: %v", header.Name, err)
		}

		var jsonCheck json.RawMessage
		if err := json.Unmarshal(data, &jsonCheck); err != nil {
			t.Errorf("file %s contains invalid JSON: %v", header.Name, err)
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected file %s not found in archive", name)
		}
	}
}

func TestRedactedConfigExcludesSecrets(t *testing.T) {
	collector := setupTestCollector(t)

	// Set sensitive values on the config
	collector.config.JWTSecret = "super-secret-key"
	collector.config.AdminPassword = "admin-pass"
	collector.config.OIDCClientSecret = "oidc-secret"

	bundle, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	// Marshal to JSON and check no secrets leak
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	secrets := []string{"super-secret-key", "admin-pass", "oidc-secret"}
	for _, secret := range secrets {
		if bytes.Contains([]byte(jsonStr), []byte(secret)) {
			t.Errorf("secret %q found in diagnostics output", secret)
		}
	}

	// But auth_enabled should reflect that JWT is set
	if !bundle.Config.AuthEnabled {
		t.Error("expected AuthEnabled=true when JWTSecret is set")
	}
}

func TestHealthDegraded(t *testing.T) {
	collector := setupTestCollector(t)

	// Close the database to simulate unhealthy state
	collector.db.Close()

	bundle, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if bundle.Health.Overall != "degraded" {
		t.Errorf("expected overall degraded, got %s", bundle.Health.Overall)
	}
	if bundle.Health.Database.Healthy {
		t.Error("expected database unhealthy after close")
	}
}
