package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VaultProvider reads secrets from HashiCorp Vault.
type VaultProvider struct {
	client     *http.Client
	addr       string
	token      string
	mountPath  string
	namespace  string
}

// NewVaultProvider creates a new Vault provider.
func NewVaultProvider(cfg *Config) (*VaultProvider, error) {
	if cfg.VaultAddr == "" {
		return nil, fmt.Errorf("vault address is required")
	}

	// Normalize address
	addr := strings.TrimSuffix(cfg.VaultAddr, "/")

	p := &VaultProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		addr:      addr,
		token:     cfg.VaultToken,
		mountPath: cfg.VaultMountPath,
		namespace: cfg.VaultNamespace,
	}

	// Default mount path
	if p.mountPath == "" {
		p.mountPath = "secret"
	}

	return p, nil
}

// Name returns the provider name.
func (p *VaultProvider) Name() string {
	return "vault"
}

// Get retrieves a secret from Vault.
func (p *VaultProvider) Get(ctx context.Context, key string) (string, error) {
	secret, err := p.GetWithMetadata(ctx, key)
	if err != nil {
		return "", err
	}
	return secret.Value, nil
}

// GetWithMetadata retrieves a secret with full metadata from Vault.
func (p *VaultProvider) GetWithMetadata(ctx context.Context, key string) (*Secret, error) {
	// Build the URL for KV v2 secrets engine
	// Format: /v1/{mount}/data/{path}
	url := fmt.Sprintf("%s/v1/%s/data/%s", p.addr, p.mountPath, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Vault token
	if p.token != "" {
		req.Header.Set("X-Vault-Token", p.token)
	}

	// Set namespace if configured (Vault Enterprise)
	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSecretNotFound
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrAuthFailed
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse Vault response
	var vaultResp vaultSecretResponse
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("failed to parse vault response: %w", err)
	}

	// Extract value - KV v2 stores data under data.data
	data, ok := vaultResp.Data.Data["value"]
	if !ok {
		// Try to get the first value if "value" key doesn't exist
		for _, v := range vaultResp.Data.Data {
			if str, ok := v.(string); ok {
				data = str
				break
			}
		}
		if data == nil {
			return nil, fmt.Errorf("secret has no 'value' field")
		}
	}

	value, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("secret value is not a string")
	}

	// Build metadata
	metadata := make(map[string]string)
	for k, v := range vaultResp.Data.Metadata {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}

	secret := &Secret{
		Key:      key,
		Value:    value,
		Version:  fmt.Sprintf("%d", vaultResp.Data.Metadata["version"]),
		Metadata: metadata,
	}

	// Parse created time if available
	if created, ok := vaultResp.Data.Metadata["created_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			secret.CreatedAt = t
		}
	}

	return secret, nil
}

// List returns available secret keys from Vault.
func (p *VaultProvider) List(ctx context.Context) ([]string, error) {
	// List uses the metadata endpoint for KV v2
	url := fmt.Sprintf("%s/v1/%s/metadata/?list=true", p.addr, p.mountPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if p.token != "" {
		req.Header.Set("X-Vault-Token", p.token)
	}

	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []string{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault returned status %d", resp.StatusCode)
	}

	var listResp vaultListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to parse vault response: %w", err)
	}

	return listResp.Data.Keys, nil
}

// Close releases resources.
func (p *VaultProvider) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// Healthy checks if Vault is accessible.
func (p *VaultProvider) Healthy(ctx context.Context) bool {
	url := fmt.Sprintf("%s/v1/sys/health", p.addr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Vault returns 200 for initialized, unsealed, active
	// 429 for unsealed and standby
	// 472 for disaster recovery secondary
	// 473 for performance standby
	return resp.StatusCode == http.StatusOK ||
		resp.StatusCode == 429 ||
		resp.StatusCode == 472 ||
		resp.StatusCode == 473
}

// vaultSecretResponse represents a Vault KV v2 secret response.
type vaultSecretResponse struct {
	Data struct {
		Data     map[string]any `json:"data"`
		Metadata map[string]any `json:"metadata"`
	} `json:"data"`
}

// vaultListResponse represents a Vault list response.
type vaultListResponse struct {
	Data struct {
		Keys []string `json:"keys"`
	} `json:"data"`
}
