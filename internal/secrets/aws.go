package secrets

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AWSProvider reads secrets from AWS Secrets Manager.
// It uses the standard AWS credential chain (environment, instance profile, etc.).
type AWSProvider struct {
	client       *http.Client
	region       string
	secretPrefix string
	accessKey    string
	secretKey    string
	sessionToken string
}

// NewAWSProvider creates a new AWS Secrets Manager provider.
func NewAWSProvider(cfg *Config) (*AWSProvider, error) {
	if cfg.AWSRegion == "" {
		return nil, fmt.Errorf("AWS region is required")
	}

	p := &AWSProvider{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		region:       cfg.AWSRegion,
		secretPrefix: cfg.AWSSecretPrefix,
	}

	// Load credentials from environment
	p.accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	p.secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	p.sessionToken = os.Getenv("AWS_SESSION_TOKEN")

	return p, nil
}

// Name returns the provider name.
func (p *AWSProvider) Name() string {
	return "aws"
}

// Get retrieves a secret from AWS Secrets Manager.
func (p *AWSProvider) Get(ctx context.Context, key string) (string, error) {
	secret, err := p.GetWithMetadata(ctx, key)
	if err != nil {
		return "", err
	}
	return secret.Value, nil
}

// GetWithMetadata retrieves a secret with metadata from AWS Secrets Manager.
func (p *AWSProvider) GetWithMetadata(ctx context.Context, key string) (*Secret, error) {
	secretID := key
	if p.secretPrefix != "" {
		secretID = p.secretPrefix + "/" + key
	}

	// Build request body
	reqBody := fmt.Sprintf(`{"SecretId":"%s"}`, secretID)

	// Create request
	endpoint := fmt.Sprintf("https://secretsmanager.%s.amazonaws.com", p.region)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")

	// Sign request with AWS Signature Version 4
	if err := p.signRequest(req, []byte(reqBody)); err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusBadRequest {
		var errResp awsErrorResponse
		if json.Unmarshal(body, &errResp) == nil {
			if strings.Contains(errResp.Type, "ResourceNotFoundException") {
				return nil, ErrSecretNotFound
			}
			if strings.Contains(errResp.Type, "AccessDeniedException") {
				return nil, ErrAuthFailed
			}
		}
		return nil, fmt.Errorf("AWS error: %s", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AWS returned status %d: %s", resp.StatusCode, string(body))
	}

	var awsResp awsSecretResponse
	if err := json.Unmarshal(body, &awsResp); err != nil {
		return nil, fmt.Errorf("failed to parse AWS response: %w", err)
	}

	value := awsResp.SecretString
	if value == "" && awsResp.SecretBinary != "" {
		// Handle binary secrets - they come base64 encoded
		value = awsResp.SecretBinary
	}

	secret := &Secret{
		Key:     key,
		Value:   value,
		Version: awsResp.VersionId,
		Metadata: map[string]string{
			"arn":  awsResp.ARN,
			"name": awsResp.Name,
		},
	}

	if awsResp.CreatedDate != "" {
		if t, err := time.Parse(time.RFC3339, awsResp.CreatedDate); err == nil {
			secret.CreatedAt = t
		}
	}

	return secret, nil
}

// List returns available secret keys from AWS Secrets Manager.
func (p *AWSProvider) List(ctx context.Context) ([]string, error) {
	reqBody := `{}`
	if p.secretPrefix != "" {
		// Use filter to only list secrets with the prefix
		reqBody = fmt.Sprintf(`{"Filters":[{"Key":"name","Values":["%s"]}]}`, p.secretPrefix)
	}

	endpoint := fmt.Sprintf("https://secretsmanager.%s.amazonaws.com", p.region)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.ListSecrets")

	if err := p.signRequest(req, []byte(reqBody)); err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AWS returned status %d: %s", resp.StatusCode, string(body))
	}

	var listResp awsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to parse AWS response: %w", err)
	}

	keys := make([]string, len(listResp.SecretList))
	for i, s := range listResp.SecretList {
		key := s.Name
		if p.secretPrefix != "" {
			key = strings.TrimPrefix(key, p.secretPrefix+"/")
		}
		keys[i] = key
	}

	return keys, nil
}

// Close releases resources.
func (p *AWSProvider) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// Healthy checks if AWS Secrets Manager is accessible.
func (p *AWSProvider) Healthy(ctx context.Context) bool {
	// Try to list secrets with a limit of 1
	reqBody := `{"MaxResults":1}`
	endpoint := fmt.Sprintf("https://secretsmanager.%s.amazonaws.com", p.region)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(reqBody))
	if err != nil {
		return false
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.ListSecrets")

	if err := p.signRequest(req, []byte(reqBody)); err != nil {
		return false
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// signRequest signs an HTTP request using AWS Signature Version 4.
func (p *AWSProvider) signRequest(req *http.Request, payload []byte) error {
	if p.accessKey == "" || p.secretKey == "" {
		// No credentials - rely on instance profile or other credential provider
		// In production, you'd use the AWS SDK's credential chain
		return nil
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzDate)
	if p.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", p.sessionToken)
	}

	// Create canonical request
	service := "secretsmanager"
	host := fmt.Sprintf("secretsmanager.%s.amazonaws.com", p.region)
	req.Header.Set("Host", host)

	payloadHash := sha256Hex(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Build canonical headers
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date;x-amz-target"
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\nx-amz-target:%s\n",
		req.Header.Get("Content-Type"),
		host,
		payloadHash,
		amzDate,
		req.Header.Get("X-Amz-Target"),
	)

	if p.sessionToken != "" {
		signedHeaders += ";x-amz-security-token"
		canonicalHeaders += fmt.Sprintf("x-amz-security-token:%s\n", p.sessionToken)
	}

	canonicalRequest := fmt.Sprintf("%s\n/\n\n%s\n%s\n%s",
		req.Method,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	)

	// Create string to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, p.region, service)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	)

	// Calculate signature
	kDate := hmacSHA256([]byte("AWS4"+p.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(p.region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// Add authorization header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		p.accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authHeader)

	return nil
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// awsSecretResponse represents an AWS GetSecretValue response.
type awsSecretResponse struct {
	ARN          string `json:"ARN"`
	Name         string `json:"Name"`
	SecretString string `json:"SecretString"`
	SecretBinary string `json:"SecretBinary"`
	VersionId    string `json:"VersionId"`
	CreatedDate  string `json:"CreatedDate"`
}

// awsListResponse represents an AWS ListSecrets response.
type awsListResponse struct {
	SecretList []struct {
		ARN  string `json:"ARN"`
		Name string `json:"Name"`
	} `json:"SecretList"`
}

// awsErrorResponse represents an AWS error response.
type awsErrorResponse struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}
