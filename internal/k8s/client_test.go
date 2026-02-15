package k8s

import (
	"os"
	"testing"
)

func TestConfigure(t *testing.T) {
	defer ResetClient()

	Configure("test-ns", "/tmp/kubeconfig", "vnc:test")

	if configuredNamespace != "test-ns" {
		t.Errorf("configuredNamespace = %q, want %q", configuredNamespace, "test-ns")
	}
	if configuredKubeconfig != "/tmp/kubeconfig" {
		t.Errorf("configuredKubeconfig = %q, want %q", configuredKubeconfig, "/tmp/kubeconfig")
	}
	if configuredVNCSidecarImage != "vnc:test" {
		t.Errorf("configuredVNCSidecarImage = %q, want %q", configuredVNCSidecarImage, "vnc:test")
	}
}

func TestConfigureBrowserSidecar(t *testing.T) {
	defer ResetClient()

	ConfigureBrowserSidecar("browser:test")
	if configuredBrowserSidecarImage != "browser:test" {
		t.Errorf("configuredBrowserSidecarImage = %q, want %q", configuredBrowserSidecarImage, "browser:test")
	}
}

func TestConfigureGuacdSidecar(t *testing.T) {
	defer ResetClient()

	ConfigureGuacdSidecar("guacd:test")
	if configuredGuacdSidecarImage != "guacd:test" {
		t.Errorf("configuredGuacdSidecarImage = %q, want %q", configuredGuacdSidecarImage, "guacd:test")
	}
}

func TestGetVNCSidecarImage_Default(t *testing.T) {
	defer ResetClient()

	got := GetVNCSidecarImage()
	if got != VNCSidecarImage {
		t.Errorf("GetVNCSidecarImage() = %q, want default %q", got, VNCSidecarImage)
	}
}

func TestGetVNCSidecarImage_Configured(t *testing.T) {
	defer ResetClient()

	Configure("", "", "custom-vnc:v1")
	got := GetVNCSidecarImage()
	if got != "custom-vnc:v1" {
		t.Errorf("GetVNCSidecarImage() = %q, want %q", got, "custom-vnc:v1")
	}
}

func TestGetBrowserSidecarImage_Default(t *testing.T) {
	defer ResetClient()

	got := GetBrowserSidecarImage()
	if got != BrowserSidecarImage {
		t.Errorf("GetBrowserSidecarImage() = %q, want default %q", got, BrowserSidecarImage)
	}
}

func TestGetBrowserSidecarImage_Configured(t *testing.T) {
	defer ResetClient()

	ConfigureBrowserSidecar("custom-browser:v1")
	got := GetBrowserSidecarImage()
	if got != "custom-browser:v1" {
		t.Errorf("GetBrowserSidecarImage() = %q, want %q", got, "custom-browser:v1")
	}
}

func TestGetGuacdSidecarImage_Default(t *testing.T) {
	defer ResetClient()

	got := GetGuacdSidecarImage()
	if got != "guacamole/guacd:1.6.0" {
		t.Errorf("GetGuacdSidecarImage() = %q, want %q", got, "guacamole/guacd:1.6.0")
	}
}

func TestGetGuacdSidecarImage_Configured(t *testing.T) {
	defer ResetClient()

	ConfigureGuacdSidecar("custom-guacd:v2")
	got := GetGuacdSidecarImage()
	if got != "custom-guacd:v2" {
		t.Errorf("GetGuacdSidecarImage() = %q, want %q", got, "custom-guacd:v2")
	}
}

func TestGetNamespace_Configured(t *testing.T) {
	defer ResetClient()

	Configure("my-namespace", "", "")
	got := GetNamespace()
	if got != "my-namespace" {
		t.Errorf("GetNamespace() = %q, want %q", got, "my-namespace")
	}
}

func TestGetNamespace_EnvVar(t *testing.T) {
	defer ResetClient()

	os.Setenv("SORTIE_NAMESPACE", "env-namespace")
	defer os.Unsetenv("SORTIE_NAMESPACE")

	got := GetNamespace()
	if got != "env-namespace" {
		t.Errorf("GetNamespace() = %q, want %q", got, "env-namespace")
	}
}

func TestGetNamespace_ConfiguredOverridesEnv(t *testing.T) {
	defer ResetClient()

	os.Setenv("SORTIE_NAMESPACE", "env-namespace")
	defer os.Unsetenv("SORTIE_NAMESPACE")

	Configure("configured-namespace", "", "")
	got := GetNamespace()
	if got != "configured-namespace" {
		t.Errorf("GetNamespace() = %q, want %q (configured should override env)", got, "configured-namespace")
	}
}

func TestGetNamespace_DefaultFallback(t *testing.T) {
	defer ResetClient()

	os.Unsetenv("SORTIE_NAMESPACE")

	got := GetNamespace()
	if got != "default" {
		t.Errorf("GetNamespace() = %q, want %q", got, "default")
	}
}

func TestGetNamespace_CachesResult(t *testing.T) {
	defer ResetClient()

	Configure("first-ns", "", "")
	first := GetNamespace()
	if first != "first-ns" {
		t.Fatalf("GetNamespace() = %q, want %q", first, "first-ns")
	}

	// Change config after first call - should still return cached value
	configuredNamespace = "second-ns"
	second := GetNamespace()
	if second != "first-ns" {
		t.Errorf("GetNamespace() = %q, want cached %q", second, "first-ns")
	}
}

func TestResetClient(t *testing.T) {
	Configure("ns", "/kube", "vnc:img")
	ConfigureBrowserSidecar("browser:img")
	ConfigureGuacdSidecar("guacd:img")
	// Cache namespace
	GetNamespace()

	ResetClient()

	if namespace != "" {
		t.Errorf("namespace not reset, got %q", namespace)
	}
	if configuredNamespace != "" {
		t.Errorf("configuredNamespace not reset, got %q", configuredNamespace)
	}
	if configuredKubeconfig != "" {
		t.Errorf("configuredKubeconfig not reset, got %q", configuredKubeconfig)
	}
	if configuredVNCSidecarImage != "" {
		t.Errorf("configuredVNCSidecarImage not reset, got %q", configuredVNCSidecarImage)
	}
	if configuredGuacdSidecarImage != "" {
		t.Errorf("configuredGuacdSidecarImage not reset, got %q", configuredGuacdSidecarImage)
	}
	if client != nil {
		t.Error("client not reset")
	}
	if clientErr != nil {
		t.Error("clientErr not reset")
	}
}
