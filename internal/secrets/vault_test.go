package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVaultProvider_Name(t *testing.T) {
	p := &VaultProvider{}
	if got := p.Name(); got != "vault" {
		t.Errorf("Name() = %v, want vault", got)
	}
}

func TestNewVaultProvider(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				VaultAddr:      "http://vault:8200",
				VaultToken:     "test-token",
				VaultMountPath: "secret",
			},
			wantErr: false,
		},
		{
			name: "missing address",
			cfg: &Config{
				VaultToken: "test-token",
			},
			wantErr: true,
		},
		{
			name: "empty mount path defaults to secret",
			cfg: &Config{
				VaultAddr: "http://vault:8200",
			},
			wantErr: false,
		},
		{
			name: "trailing slash is trimmed from address",
			cfg: &Config{
				VaultAddr: "http://vault:8200/",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewVaultProvider(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewVaultProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if p.mountPath == "" {
					t.Error("mountPath should default to 'secret' if empty")
				}
				// Verify trailing slash is trimmed
				if tt.name == "trailing slash is trimmed from address" && p.addr != "http://vault:8200" {
					t.Errorf("addr = %v, want http://vault:8200", p.addr)
				}
			}
		})
	}
}

func TestVaultProvider_Get(t *testing.T) {
	// Mock Vault server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("X-Vault-Token") != "test-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		switch r.URL.Path {
		case "/v1/secret/data/db-password":
			resp := vaultSecretResponse{}
			resp.Data.Data = map[string]any{"value": "super-secret"}
			resp.Data.Metadata = map[string]any{
				"version":      float64(1),
				"created_time": "2024-01-01T00:00:00Z",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/v1/secret/data/multi-field":
			resp := vaultSecretResponse{}
			resp.Data.Data = map[string]any{"username": "admin", "password": "secret"}
			resp.Data.Metadata = map[string]any{"version": float64(2)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/v1/secret/data/nonexistent":
			w.WriteHeader(http.StatusNotFound)

		case "/v1/secret/data/non-string":
			resp := vaultSecretResponse{}
			resp.Data.Data = map[string]any{"value": 12345}
			resp.Data.Metadata = map[string]any{"version": float64(1)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/v1/secret/data/empty-data":
			resp := vaultSecretResponse{}
			resp.Data.Data = map[string]any{}
			resp.Data.Metadata = map[string]any{"version": float64(1)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p, err := NewVaultProvider(&Config{
		VaultAddr:      server.URL,
		VaultToken:     "test-token",
		VaultMountPath: "secret",
	})
	if err != nil {
		t.Fatalf("NewVaultProvider() error = %v", err)
	}
	defer p.Close()

	ctx := context.Background()

	t.Run("get existing secret", func(t *testing.T) {
		value, err := p.Get(ctx, "db-password")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "super-secret" {
			t.Errorf("Get() = %v, want super-secret", value)
		}
	})

	t.Run("get nonexistent secret", func(t *testing.T) {
		_, err := p.Get(ctx, "nonexistent")
		if err != ErrSecretNotFound {
			t.Errorf("Get() error = %v, want ErrSecretNotFound", err)
		}
	})

	t.Run("get multi-field falls back to first string value", func(t *testing.T) {
		value, err := p.Get(ctx, "multi-field")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		// Should get one of the string values since there's no "value" key
		if value != "admin" && value != "secret" {
			t.Errorf("Get() = %v, want admin or secret", value)
		}
	})

	t.Run("get non-string value returns error", func(t *testing.T) {
		_, err := p.Get(ctx, "non-string")
		if err == nil {
			t.Error("Get() should fail for non-string value")
		}
	})

	t.Run("get empty data returns error", func(t *testing.T) {
		_, err := p.Get(ctx, "empty-data")
		if err == nil {
			t.Error("Get() should fail for empty data")
		}
	})
}

func TestVaultProvider_GetWithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vaultSecretResponse{}
		resp.Data.Data = map[string]any{"value": "my-secret"}
		resp.Data.Metadata = map[string]any{
			"version":      float64(3),
			"created_time": "2024-06-15T10:30:00Z",
			"custom_field": "custom_value",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr:      server.URL,
		VaultToken:     "test-token",
		VaultMountPath: "secret",
	})
	defer p.Close()

	ctx := context.Background()
	secret, err := p.GetWithMetadata(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetWithMetadata() error = %v", err)
	}

	if secret.Key != "test-key" {
		t.Errorf("Key = %v, want test-key", secret.Key)
	}
	if secret.Value != "my-secret" {
		t.Errorf("Value = %v, want my-secret", secret.Value)
	}
	if secret.Metadata["custom_field"] != "custom_value" {
		t.Errorf("Metadata[custom_field] = %v, want custom_value", secret.Metadata["custom_field"])
	}
	// created_time should be parsed
	expectedTime, _ := time.Parse(time.RFC3339, "2024-06-15T10:30:00Z")
	if !secret.CreatedAt.Equal(expectedTime) {
		t.Errorf("CreatedAt = %v, want %v", secret.CreatedAt, expectedTime)
	}
}

func TestVaultProvider_GetAuthFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr:  server.URL,
		VaultToken: "bad-token",
	})
	defer p.Close()

	_, err := p.Get(context.Background(), "any-key")
	if err != ErrAuthFailed {
		t.Errorf("Get() error = %v, want ErrAuthFailed", err)
	}
}

func TestVaultProvider_GetUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr: server.URL,
	})
	defer p.Close()

	_, err := p.Get(context.Background(), "any-key")
	if err != ErrAuthFailed {
		t.Errorf("Get() error = %v, want ErrAuthFailed", err)
	}
}

func TestVaultProvider_GetServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr: server.URL,
	})
	defer p.Close()

	_, err := p.Get(context.Background(), "any-key")
	if err == nil {
		t.Error("Get() should fail on server error")
	}
}

func TestVaultProvider_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/secret/metadata/" && r.URL.Query().Get("list") == "true" {
			resp := vaultListResponse{}
			resp.Data.Keys = []string{"key1", "key2", "key3"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr:      server.URL,
		VaultToken:     "test-token",
		VaultMountPath: "secret",
	})
	defer p.Close()

	keys, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("List() returned %d keys, want 3", len(keys))
	}
}

func TestVaultProvider_ListNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr: server.URL,
	})
	defer p.Close()

	keys, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List() returned %d keys, want 0", len(keys))
	}
}

func TestVaultProvider_ListError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr: server.URL,
	})
	defer p.Close()

	_, err := p.List(context.Background())
	if err == nil {
		t.Error("List() should fail on server error")
	}
}

func TestVaultProvider_Healthy(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"healthy - 200", http.StatusOK, true},
		{"standby - 429", 429, true},
		{"DR secondary - 472", 472, true},
		{"performance standby - 473", 473, true},
		{"unhealthy - 500", http.StatusInternalServerError, false},
		{"sealed - 503", http.StatusServiceUnavailable, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/sys/health" {
					w.WriteHeader(tt.statusCode)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			p, _ := NewVaultProvider(&Config{VaultAddr: server.URL})
			defer p.Close()

			if got := p.Healthy(context.Background()); got != tt.want {
				t.Errorf("Healthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVaultProvider_HealthyConnectionError(t *testing.T) {
	p, _ := NewVaultProvider(&Config{VaultAddr: "http://localhost:1"})
	defer p.Close()

	if p.Healthy(context.Background()) {
		t.Error("Healthy() should return false when connection fails")
	}
}

func TestVaultProvider_NamespaceHeader(t *testing.T) {
	var receivedNamespace string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedNamespace = r.Header.Get("X-Vault-Namespace")
		resp := vaultSecretResponse{}
		resp.Data.Data = map[string]any{"value": "ns-secret"}
		resp.Data.Metadata = map[string]any{"version": float64(1)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := NewVaultProvider(&Config{
		VaultAddr:      server.URL,
		VaultToken:     "test-token",
		VaultNamespace: "my-team",
	})
	defer p.Close()

	_, err := p.Get(context.Background(), "test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if receivedNamespace != "my-team" {
		t.Errorf("X-Vault-Namespace = %v, want my-team", receivedNamespace)
	}
}

func TestVaultProvider_Close(t *testing.T) {
	p, _ := NewVaultProvider(&Config{VaultAddr: "http://vault:8200"})
	if err := p.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
