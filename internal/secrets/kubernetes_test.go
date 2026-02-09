package secrets

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesProvider_Name(t *testing.T) {
	p := &KubernetesProvider{}
	if got := p.Name(); got != "kubernetes" {
		t.Errorf("Name() = %v, want kubernetes", got)
	}
}

func newFakeK8sProvider(namespace, secretName string, secretData map[string][]byte, labels map[string]string) *KubernetesProvider {
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: secretData,
	})

	return &KubernetesProvider{
		client:     client,
		namespace:  namespace,
		secretName: secretName,
	}
}

func newFakeK8sProviderEmpty(namespace, secretName string) *KubernetesProvider {
	client := fake.NewSimpleClientset()
	return &KubernetesProvider{
		client:     client,
		namespace:  namespace,
		secretName: secretName,
	}
}

func TestKubernetesProvider_Get(t *testing.T) {
	p := newFakeK8sProvider("default", "my-secrets", map[string][]byte{
		"db-password": []byte("super-secret"),
		"api-key":     []byte("my-api-key"),
	}, nil)

	ctx := context.Background()

	t.Run("get existing key", func(t *testing.T) {
		value, err := p.Get(ctx, "db-password")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "super-secret" {
			t.Errorf("Get() = %v, want super-secret", value)
		}
	})

	t.Run("get another existing key", func(t *testing.T) {
		value, err := p.Get(ctx, "api-key")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if value != "my-api-key" {
			t.Errorf("Get() = %v, want my-api-key", value)
		}
	})

	t.Run("get nonexistent key", func(t *testing.T) {
		_, err := p.Get(ctx, "nonexistent")
		if err != ErrSecretNotFound {
			t.Errorf("Get() error = %v, want ErrSecretNotFound", err)
		}
	})
}

func TestKubernetesProvider_GetSecretNotExists(t *testing.T) {
	p := newFakeK8sProviderEmpty("default", "nonexistent-secret")
	ctx := context.Background()

	_, err := p.Get(ctx, "any-key")
	if err == nil {
		t.Error("Get() should fail when secret doesn't exist")
	}
}

func TestKubernetesProvider_GetWithMetadata(t *testing.T) {
	p := newFakeK8sProvider("my-namespace", "app-secrets",
		map[string][]byte{
			"db-password": []byte("the-password"),
		},
		map[string]string{
			"app":     "sortie",
			"env":     "production",
		},
	)

	ctx := context.Background()
	secret, err := p.GetWithMetadata(ctx, "db-password")
	if err != nil {
		t.Fatalf("GetWithMetadata() error = %v", err)
	}

	if secret.Key != "db-password" {
		t.Errorf("Key = %v, want db-password", secret.Key)
	}
	if secret.Value != "the-password" {
		t.Errorf("Value = %v, want the-password", secret.Value)
	}
	if secret.Metadata["namespace"] != "my-namespace" {
		t.Errorf("Metadata[namespace] = %v, want my-namespace", secret.Metadata["namespace"])
	}
	if secret.Metadata["secret_name"] != "app-secrets" {
		t.Errorf("Metadata[secret_name] = %v, want app-secrets", secret.Metadata["secret_name"])
	}
	if secret.Metadata["label.app"] != "sortie" {
		t.Errorf("Metadata[label.app] = %v, want sortie", secret.Metadata["label.app"])
	}
	if secret.Metadata["label.env"] != "production" {
		t.Errorf("Metadata[label.env] = %v, want production", secret.Metadata["label.env"])
	}
}

func TestKubernetesProvider_GetWithMetadata_NotFound(t *testing.T) {
	p := newFakeK8sProvider("default", "my-secrets",
		map[string][]byte{"key1": []byte("value1")}, nil)

	ctx := context.Background()
	_, err := p.GetWithMetadata(ctx, "nonexistent")
	if err != ErrSecretNotFound {
		t.Errorf("GetWithMetadata() error = %v, want ErrSecretNotFound", err)
	}
}

func TestKubernetesProvider_List(t *testing.T) {
	p := newFakeK8sProvider("default", "my-secrets", map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}, nil)

	ctx := context.Background()
	keys, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(keys) != 3 {
		t.Fatalf("List() returned %d keys, want 3", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, expected := range []string{"key1", "key2", "key3"} {
		if !keySet[expected] {
			t.Errorf("List() missing key %v", expected)
		}
	}
}

func TestKubernetesProvider_ListSecretNotExists(t *testing.T) {
	p := newFakeK8sProviderEmpty("default", "nonexistent")

	ctx := context.Background()
	keys, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List() returned %d keys, want 0", len(keys))
	}
}

func TestKubernetesProvider_Healthy(t *testing.T) {
	p := newFakeK8sProvider("default", "my-secrets",
		map[string][]byte{"key": []byte("value")}, nil)

	ctx := context.Background()
	if !p.Healthy(ctx) {
		t.Error("Healthy() should return true when secret exists")
	}
}

func TestKubernetesProvider_HealthySecretNotFound(t *testing.T) {
	p := newFakeK8sProviderEmpty("default", "nonexistent")

	ctx := context.Background()
	// Secret not found is still considered healthy (API is reachable)
	if !p.Healthy(ctx) {
		t.Error("Healthy() should return true even when secret not found (API is reachable)")
	}
}

func TestKubernetesProvider_HealthyTimeout(t *testing.T) {
	p := newFakeK8sProvider("default", "my-secrets",
		map[string][]byte{"key": []byte("value")}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !p.Healthy(ctx) {
		t.Error("Healthy() should return true with timeout context")
	}
}

func TestKubernetesProvider_Close(t *testing.T) {
	p := newFakeK8sProvider("default", "my-secrets", nil, nil)
	if err := p.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"not found error", errors.New("resource not found"), true},
		{"NotFound error", errors.New("NotFound"), true},
		{"unrelated error", errors.New("connection refused"), false},
		{"contains not found", errors.New("the secret was not found in store"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFound(tt.err); got != tt.want {
				t.Errorf("isNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello", "hello", true},
		{"hello", "world", false},
		{"", "", true},
		{"hello", "", true},
		{"", "hello", false},
		{"abc", "abcd", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestContainsAt(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		start  int
		want   bool
	}{
		{"hello world", "world", 0, true},
		{"hello world", "world", 6, true},
		{"hello world", "world", 7, false},
		{"hello world", "hello", 0, true},
		{"hello world", "hello", 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			if got := containsAt(tt.s, tt.substr, tt.start); got != tt.want {
				t.Errorf("containsAt(%q, %q, %d) = %v, want %v", tt.s, tt.substr, tt.start, got, tt.want)
			}
		})
	}
}
