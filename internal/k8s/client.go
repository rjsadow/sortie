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
	client     kubernetes.Interface
	restConfig *rest.Config
	clientErr  error
	namespace  string

	// Configuration set via Configure()
	configuredNamespace           string
	configuredKubeconfig          string
	configuredVNCSidecarImage     string
	configuredBrowserSidecarImage string
	configuredGuacdSidecarImage   string
)

// Configure sets the Kubernetes configuration from the application config.
// This should be called once at application startup before any other k8s operations.
func Configure(ns, kubeconfig, vncSidecarImage string) {
	configuredNamespace = ns
	configuredKubeconfig = kubeconfig
	configuredVNCSidecarImage = vncSidecarImage
}

// ConfigureBrowserSidecar sets the browser sidecar image.
func ConfigureBrowserSidecar(browserSidecarImage string) {
	configuredBrowserSidecarImage = browserSidecarImage
}

// GetVNCSidecarImage returns the configured VNC sidecar image.
func GetVNCSidecarImage() string {
	if configuredVNCSidecarImage != "" {
		return configuredVNCSidecarImage
	}
	return VNCSidecarImage
}

// GetBrowserSidecarImage returns the configured browser sidecar image.
func GetBrowserSidecarImage() string {
	if configuredBrowserSidecarImage != "" {
		return configuredBrowserSidecarImage
	}
	return BrowserSidecarImage
}

// ConfigureGuacdSidecar sets the guacd sidecar image for Windows RDP support.
func ConfigureGuacdSidecar(guacdSidecarImage string) {
	configuredGuacdSidecarImage = guacdSidecarImage
}

// GetGuacdSidecarImage returns the configured guacd sidecar image.
func GetGuacdSidecarImage() string {
	if configuredGuacdSidecarImage != "" {
		return configuredGuacdSidecarImage
	}
	return "guacamole/guacd:1.5.5"
}

// GetNamespace returns the Kubernetes namespace to use for sessions.
// Priority: configured value > SORTIE_NAMESPACE env var > in-cluster namespace > "default"
func GetNamespace() string {
	if namespace != "" {
		return namespace
	}

	// Check configured value first
	if configuredNamespace != "" {
		namespace = configuredNamespace
		return namespace
	}

	// Check environment variable (legacy support)
	if ns := os.Getenv("SORTIE_NAMESPACE"); ns != "" {
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
func GetClient() (kubernetes.Interface, error) {
	clientOnce.Do(func() {
		var cfg *rest.Config

		// Try in-cluster config first
		cfg, clientErr = rest.InClusterConfig()
		if clientErr != nil {
			// Fall back to kubeconfig
			cfg, clientErr = buildConfigFromKubeconfig()
			if clientErr != nil {
				clientErr = fmt.Errorf("failed to create kubernetes config: %w", clientErr)
				return
			}
		}

		restConfig = cfg

		client, clientErr = kubernetes.NewForConfig(cfg)
		if clientErr != nil {
			clientErr = fmt.Errorf("failed to create kubernetes client: %w", clientErr)
			return
		}
	})

	return client, clientErr
}

// GetRESTConfig returns the Kubernetes REST config, initializing the client if necessary.
func GetRESTConfig() (*rest.Config, error) {
	if _, err := GetClient(); err != nil {
		return nil, err
	}
	return restConfig, nil
}

// buildConfigFromKubeconfig builds a REST config from kubeconfig file.
// Priority: configured value > KUBECONFIG env var > ~/.kube/config
func buildConfigFromKubeconfig() (*rest.Config, error) {
	kubeconfigPath := configuredKubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
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
	restConfig = nil
	clientErr = nil
	namespace = ""
	configuredNamespace = ""
	configuredKubeconfig = ""
	configuredVNCSidecarImage = ""
	configuredBrowserSidecarImage = ""
	configuredGuacdSidecarImage = ""
}
