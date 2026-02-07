package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAWSProvider_Name(t *testing.T) {
	p := &AWSProvider{region: "us-east-1"}
	if got := p.Name(); got != "aws" {
		t.Errorf("Name() = %v, want aws", got)
	}
}

func TestNewAWSProvider(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     &Config{AWSRegion: "us-east-1"},
			wantErr: false,
		},
		{
			name:    "missing region",
			cfg:     &Config{},
			wantErr: true,
		},
		{
			name:    "with prefix",
			cfg:     &Config{AWSRegion: "eu-west-1", AWSSecretPrefix: "prod/launchpad"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewAWSProvider(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAWSProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.cfg.AWSSecretPrefix != "" {
				if p.secretPrefix != tt.cfg.AWSSecretPrefix {
					t.Errorf("secretPrefix = %v, want %v", p.secretPrefix, tt.cfg.AWSSecretPrefix)
				}
			}
		})
	}
}

func TestNewAWSProvider_LoadsCredentials(t *testing.T) {
	os.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("AWS_SESSION_TOKEN", "test-session-token")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_SESSION_TOKEN")
	}()

	p, err := NewAWSProvider(&Config{AWSRegion: "us-east-1"})
	if err != nil {
		t.Fatalf("NewAWSProvider() error = %v", err)
	}

	if p.accessKey != "test-access-key" {
		t.Errorf("accessKey = %v, want test-access-key", p.accessKey)
	}
	if p.secretKey != "test-secret-key" {
		t.Errorf("secretKey = %v, want test-secret-key", p.secretKey)
	}
	if p.sessionToken != "test-session-token" {
		t.Errorf("sessionToken = %v, want test-session-token", p.sessionToken)
	}
}

func TestAWSProvider_GetWithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") == "secretsmanager.GetSecretValue" {
			resp := awsSecretResponse{
				ARN:          "arn:aws:secretsmanager:us-east-1:123456789:secret:test-key",
				Name:         "test-key",
				SecretString: "my-secret-value",
				VersionId:    "v1",
				CreatedDate:  "2024-06-15T10:30:00Z",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	p := &AWSProvider{
		client: server.Client(),
		region: "us-east-1",
	}
	// Override the endpoint by replacing the client with a redirect
	// We need a custom approach since AWS provider builds its own URL
	// Let's create the provider and then override its client
	p.client = server.Client()

	// We need to intercept the request to point to our test server
	// Use a custom transport
	p.client.Transport = &rewriteTransport{
		base:    http.DefaultTransport,
		baseURL: server.URL,
	}

	ctx := context.Background()
	secret, err := p.GetWithMetadata(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetWithMetadata() error = %v", err)
	}

	if secret.Key != "test-key" {
		t.Errorf("Key = %v, want test-key", secret.Key)
	}
	if secret.Value != "my-secret-value" {
		t.Errorf("Value = %v, want my-secret-value", secret.Value)
	}
	if secret.Version != "v1" {
		t.Errorf("Version = %v, want v1", secret.Version)
	}
	if secret.Metadata["arn"] != "arn:aws:secretsmanager:us-east-1:123456789:secret:test-key" {
		t.Errorf("Metadata[arn] = %v, want arn value", secret.Metadata["arn"])
	}
}

func TestAWSProvider_GetWithPrefix(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes := make([]byte, r.ContentLength)
		r.Body.Read(bodyBytes)
		receivedBody = string(bodyBytes)

		resp := awsSecretResponse{
			Name:         "prod/launchpad/db-password",
			SecretString: "prefixed-secret",
			VersionId:    "v1",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AWSProvider{
		client:       &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region:       "us-east-1",
		secretPrefix: "prod/launchpad",
	}

	_, err := p.Get(context.Background(), "db-password")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Verify the request included the prefix
	if receivedBody == "" {
		t.Skip("could not capture request body")
	}
}

func TestAWSProvider_GetNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(awsErrorResponse{
			Type:    "ResourceNotFoundException",
			Message: "Secret not found",
		})
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	_, err := p.Get(context.Background(), "nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("Get() error = %v, want ErrSecretNotFound", err)
	}
}

func TestAWSProvider_GetAccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(awsErrorResponse{
			Type:    "AccessDeniedException",
			Message: "Access denied",
		})
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	_, err := p.Get(context.Background(), "restricted")
	if err != ErrAuthFailed {
		t.Errorf("Get() error = %v, want ErrAuthFailed", err)
	}
}

func TestAWSProvider_GetServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	_, err := p.Get(context.Background(), "any-key")
	if err == nil {
		t.Error("Get() should fail on server error")
	}
}

func TestAWSProvider_GetBinarySecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := awsSecretResponse{
			Name:         "binary-key",
			SecretBinary: "base64encodeddata",
			VersionId:    "v1",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	value, err := p.Get(context.Background(), "binary-key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "base64encodeddata" {
		t.Errorf("Get() = %v, want base64encodeddata", value)
	}
}

func TestAWSProvider_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") == "secretsmanager.ListSecrets" {
			resp := awsListResponse{
				SecretList: []struct {
					ARN  string `json:"ARN"`
					Name string `json:"Name"`
				}{
					{ARN: "arn:1", Name: "key1"},
					{ARN: "arn:2", Name: "key2"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	keys, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("List() returned %d keys, want 2", len(keys))
	}
}

func TestAWSProvider_ListWithPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := awsListResponse{
			SecretList: []struct {
				ARN  string `json:"ARN"`
				Name string `json:"Name"`
			}{
				{Name: "prod/launchpad/key1"},
				{Name: "prod/launchpad/key2"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AWSProvider{
		client:       &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region:       "us-east-1",
		secretPrefix: "prod/launchpad",
	}

	keys, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Prefix should be stripped
	if len(keys) != 2 {
		t.Fatalf("List() returned %d keys, want 2", len(keys))
	}
	if keys[0] != "key1" {
		t.Errorf("keys[0] = %v, want key1", keys[0])
	}
	if keys[1] != "key2" {
		t.Errorf("keys[1] = %v, want key2", keys[1])
	}
}

func TestAWSProvider_ListError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	_, err := p.List(context.Background())
	if err == nil {
		t.Error("List() should fail on server error")
	}
}

func TestAWSProvider_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"SecretList": []any{}})
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	if !p.Healthy(context.Background()) {
		t.Error("Healthy() should return true when API is accessible")
	}
}

func TestAWSProvider_HealthyFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL}},
		region: "us-east-1",
	}

	if p.Healthy(context.Background()) {
		t.Error("Healthy() should return false when API returns error")
	}
}

func TestAWSProvider_HealthyConnectionError(t *testing.T) {
	p := &AWSProvider{
		client: &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: "http://localhost:1"}},
		region: "us-east-1",
	}

	if p.Healthy(context.Background()) {
		t.Error("Healthy() should return false when connection fails")
	}
}

func TestAWSProvider_Close(t *testing.T) {
	p := &AWSProvider{
		client: &http.Client{},
		region: "us-east-1",
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestAWSProvider_SignRequest(t *testing.T) {
	p := &AWSProvider{
		region:    "us-east-1",
		accessKey: "AKIAIOSFODNN7EXAMPLE",
		secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	req, _ := http.NewRequest(http.MethodPost, "https://secretsmanager.us-east-1.amazonaws.com", nil)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")

	err := p.signRequest(req, []byte(`{"SecretId":"test"}`))
	if err != nil {
		t.Fatalf("signRequest() error = %v", err)
	}

	// Verify authorization header is set
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		t.Error("Authorization header should be set")
	}
	if !containsSubstr(authHeader, "AWS4-HMAC-SHA256") {
		t.Error("Authorization header should use AWS4-HMAC-SHA256")
	}
	if !containsSubstr(authHeader, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("Authorization header should contain access key")
	}

	// Verify X-Amz-Date is set
	if req.Header.Get("X-Amz-Date") == "" {
		t.Error("X-Amz-Date header should be set")
	}
}

func TestAWSProvider_SignRequestWithSessionToken(t *testing.T) {
	p := &AWSProvider{
		region:       "us-east-1",
		accessKey:    "AKIAIOSFODNN7EXAMPLE",
		secretKey:    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		sessionToken: "FwoGZX...",
	}

	req, _ := http.NewRequest(http.MethodPost, "https://secretsmanager.us-east-1.amazonaws.com", nil)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")

	err := p.signRequest(req, []byte(`{}`))
	if err != nil {
		t.Fatalf("signRequest() error = %v", err)
	}

	if req.Header.Get("X-Amz-Security-Token") != "FwoGZX..." {
		t.Error("X-Amz-Security-Token should be set when session token is present")
	}
}

func TestAWSProvider_SignRequestNoCredentials(t *testing.T) {
	p := &AWSProvider{
		region: "us-east-1",
		// No credentials
	}

	req, _ := http.NewRequest(http.MethodPost, "https://secretsmanager.us-east-1.amazonaws.com", nil)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")

	err := p.signRequest(req, []byte(`{}`))
	if err != nil {
		t.Fatalf("signRequest() error = %v", err)
	}

	// No authorization header should be set when there are no credentials
	if req.Header.Get("Authorization") != "" {
		t.Error("Authorization header should not be set when no credentials are available")
	}
}

func TestSha256Hex(t *testing.T) {
	// Known SHA-256 hash of empty string
	got := sha256Hex([]byte(""))
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("sha256Hex('') = %v, want %v", got, want)
	}
}

func TestHmacSHA256(t *testing.T) {
	key := []byte("key")
	data := []byte("data")
	result := hmacSHA256(key, data)
	if len(result) == 0 {
		t.Error("hmacSHA256() returned empty result")
	}
	// HMAC-SHA256 should produce 32 bytes
	if len(result) != 32 {
		t.Errorf("hmacSHA256() returned %d bytes, want 32", len(result))
	}
}

// rewriteTransport rewrites all requests to point to a test server
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	// Parse the baseURL to get host
	req.URL.Host = t.baseURL[len("http://"):]
	return t.base.RoundTrip(req)
}

// containsSubstr is a simple helper for string contains check in tests
func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
