package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesProvider reads secrets from Kubernetes Secrets.
type KubernetesProvider struct {
	client     kubernetes.Interface
	namespace  string
	secretName string
}

// NewKubernetesProvider creates a new Kubernetes secrets provider.
func NewKubernetesProvider(cfg *Config) (*KubernetesProvider, error) {
	if cfg.K8sSecretName == "" {
		return nil, fmt.Errorf("Kubernetes secret name is required")
	}

	var config *rest.Config
	var err error

	if cfg.K8sInCluster {
		// Try in-cluster config first
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fall back to kubeconfig
			kubeconfigPath := cfg.K8sKubeconfig
			if kubeconfigPath == "" {
				home, _ := os.UserHomeDir()
				kubeconfigPath = filepath.Join(home, ".kube", "config")
			}
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to build Kubernetes config: %w", err)
			}
		}
	} else {
		kubeconfigPath := cfg.K8sKubeconfig
		if kubeconfigPath == "" {
			home, _ := os.UserHomeDir()
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build Kubernetes config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	namespace := cfg.K8sNamespace
	if namespace == "" {
		// Try to read namespace from mounted service account
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			namespace = string(data)
		} else {
			namespace = "default"
		}
	}

	return &KubernetesProvider{
		client:     client,
		namespace:  namespace,
		secretName: cfg.K8sSecretName,
	}, nil
}

// Name returns the provider name.
func (p *KubernetesProvider) Name() string {
	return "kubernetes"
}

// Get retrieves a secret key from the Kubernetes Secret.
func (p *KubernetesProvider) Get(ctx context.Context, key string) (string, error) {
	secret, err := p.GetWithMetadata(ctx, key)
	if err != nil {
		return "", err
	}
	return secret.Value, nil
}

// GetWithMetadata retrieves a secret key with metadata from the Kubernetes Secret.
func (p *KubernetesProvider) GetWithMetadata(ctx context.Context, key string) (*Secret, error) {
	k8sSecret, err := p.client.CoreV1().Secrets(p.namespace).Get(ctx, p.secretName, metav1.GetOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to get Kubernetes secret: %w", err)
	}

	data, ok := k8sSecret.Data[key]
	if !ok {
		return nil, ErrSecretNotFound
	}

	secret := &Secret{
		Key:       key,
		Value:     string(data),
		Version:   string(k8sSecret.ResourceVersion),
		CreatedAt: k8sSecret.CreationTimestamp.Time,
		Metadata: map[string]string{
			"namespace":   p.namespace,
			"secret_name": p.secretName,
		},
	}

	// Add labels as metadata
	for k, v := range k8sSecret.Labels {
		secret.Metadata["label."+k] = v
	}

	return secret, nil
}

// List returns all keys in the Kubernetes Secret.
func (p *KubernetesProvider) List(ctx context.Context) ([]string, error) {
	k8sSecret, err := p.client.CoreV1().Secrets(p.namespace).Get(ctx, p.secretName, metav1.GetOptions{})
	if err != nil {
		if isNotFound(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to get Kubernetes secret: %w", err)
	}

	keys := make([]string, 0, len(k8sSecret.Data))
	for key := range k8sSecret.Data {
		keys = append(keys, key)
	}

	return keys, nil
}

// Close is a no-op for Kubernetes provider.
func (p *KubernetesProvider) Close() error {
	return nil
}

// Healthy checks if the Kubernetes API is accessible.
func (p *KubernetesProvider) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := p.client.CoreV1().Secrets(p.namespace).Get(ctx, p.secretName, metav1.GetOptions{})
	if err != nil {
		// Secret not found is OK - API is still healthy
		if isNotFound(err) {
			return true
		}
		return false
	}
	return true
}

// isNotFound checks if an error is a Kubernetes "not found" error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Check for "not found" in the error message
	// This is a simple check - in production you'd use apierrors.IsNotFound
	return contains(err.Error(), "not found") || contains(err.Error(), "NotFound")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
