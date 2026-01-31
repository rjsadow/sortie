package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	clientOnce sync.Once
	client     *kubernetes.Clientset
	clientErr  error
	namespace  string
)

// GetNamespace returns the Kubernetes namespace to use for sessions.
// Priority: LAUNCHPAD_NAMESPACE env var > in-cluster namespace > "default"
func GetNamespace() string {
	if namespace != "" {
		return namespace
	}

	// Check environment variable first
	if ns := os.Getenv("LAUNCHPAD_NAMESPACE"); ns != "" {
		namespace = ns
		return namespace
	}

	// Try to read in-cluster namespace
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		namespace = string(data)
		return namespace
	}

	namespace = "default"
	return namespace
}

// GetClient returns a Kubernetes clientset, initializing it if necessary.
// It supports both in-cluster config and kubeconfig file.
func GetClient() (*kubernetes.Clientset, error) {
	clientOnce.Do(func() {
		var config *rest.Config

		// Try in-cluster config first
		config, clientErr = rest.InClusterConfig()
		if clientErr != nil {
			// Fall back to kubeconfig
			config, clientErr = buildConfigFromKubeconfig()
			if clientErr != nil {
				clientErr = fmt.Errorf("failed to create kubernetes config: %w", clientErr)
				return
			}
		}

		client, clientErr = kubernetes.NewForConfig(config)
		if clientErr != nil {
			clientErr = fmt.Errorf("failed to create kubernetes client: %w", clientErr)
			return
		}
	})

	return client, clientErr
}

// buildConfigFromKubeconfig builds a REST config from kubeconfig file.
// Priority: KUBECONFIG env var > ~/.kube/config
func buildConfigFromKubeconfig() (*rest.Config, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig at %s: %w", kubeconfigPath, err)
	}

	return config, nil
}

// ResetClient resets the client singleton (useful for testing)
func ResetClient() {
	clientOnce = sync.Once{}
	client = nil
	clientErr = nil
	namespace = ""
}
