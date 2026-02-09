package k8s

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDefaultPodConfig(t *testing.T) {
	cfg := DefaultPodConfig("sess-1", "app-1", "My App", "myimage:latest")

	if cfg.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", cfg.SessionID, "sess-1")
	}
	if cfg.AppID != "app-1" {
		t.Errorf("AppID = %q, want %q", cfg.AppID, "app-1")
	}
	if cfg.AppName != "My App" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "My App")
	}
	if cfg.ContainerImage != "myimage:latest" {
		t.Errorf("ContainerImage = %q, want %q", cfg.ContainerImage, "myimage:latest")
	}
	if cfg.CPULimit != "2" {
		t.Errorf("CPULimit = %q, want %q", cfg.CPULimit, "2")
	}
	if cfg.MemoryLimit != "2Gi" {
		t.Errorf("MemoryLimit = %q, want %q", cfg.MemoryLimit, "2Gi")
	}
	if cfg.CPURequest != "500m" {
		t.Errorf("CPURequest = %q, want %q", cfg.CPURequest, "500m")
	}
	if cfg.MemoryRequest != "512Mi" {
		t.Errorf("MemoryRequest = %q, want %q", cfg.MemoryRequest, "512Mi")
	}
}

func TestIsJlesageImage(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"jlesage/firefox", true},
		{"jlesage/filezilla:latest", true},
		{"jlesage/", true},
		{"ubuntu:latest", false},
		{"my-registry/jlesage/app", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isJlesageImage(tt.image)
		if got != tt.want {
			t.Errorf("isJlesageImage(%q) = %v, want %v", tt.image, got, tt.want)
		}
	}
}

func TestBuildPodSpec_StandardImage(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-123", "app-1", "Test App", "myapp:v1")
	pod := BuildPodSpec(config)

	// Check pod metadata
	if pod.Name != "sortie-session-sess-123" {
		t.Errorf("pod.Name = %q, want %q", pod.Name, "sortie-session-sess-123")
	}
	if pod.Namespace != "test-ns" {
		t.Errorf("pod.Namespace = %q, want %q", pod.Namespace, "test-ns")
	}

	// Check labels
	if pod.Labels[SessionLabelKey] != "sess-123" {
		t.Errorf("session label = %q, want %q", pod.Labels[SessionLabelKey], "sess-123")
	}
	if pod.Labels[AppLabelKey] != "app-1" {
		t.Errorf("app label = %q, want %q", pod.Labels[AppLabelKey], "app-1")
	}
	if pod.Labels[ComponentLabelKey] != "session" {
		t.Errorf("component label = %q, want %q", pod.Labels[ComponentLabelKey], "session")
	}

	// Check annotations
	if pod.Annotations["sortie.io/app-name"] != "Test App" {
		t.Errorf("app-name annotation = %q, want %q", pod.Annotations["sortie.io/app-name"], "Test App")
	}
	if pod.Annotations["sortie.io/websocket-port"] != "6080" {
		t.Errorf("websocket-port annotation = %q, want %q", pod.Annotations["sortie.io/websocket-port"], "6080")
	}

	// Check restart policy
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy = %v, want Never", pod.Spec.RestartPolicy)
	}

	// Should have 2 containers (vnc-sidecar + app)
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("len(Containers) = %d, want 2", len(pod.Spec.Containers))
	}

	// Check VNC sidecar
	vnc := pod.Spec.Containers[0]
	if vnc.Name != "vnc-sidecar" {
		t.Errorf("vnc container name = %q, want %q", vnc.Name, "vnc-sidecar")
	}
	if vnc.Image != VNCSidecarImage {
		t.Errorf("vnc image = %q, want %q", vnc.Image, VNCSidecarImage)
	}
	if vnc.SecurityContext == nil {
		t.Error("vnc SecurityContext is nil")
	} else {
		if *vnc.SecurityContext.RunAsUser != 1000 {
			t.Errorf("vnc RunAsUser = %d, want 1000", *vnc.SecurityContext.RunAsUser)
		}
		if *vnc.SecurityContext.AllowPrivilegeEscalation != false {
			t.Error("vnc AllowPrivilegeEscalation should be false")
		}
	}

	// Check app container
	app := pod.Spec.Containers[1]
	if app.Name != "app" {
		t.Errorf("app container name = %q, want %q", app.Name, "app")
	}
	if app.Image != "myapp:v1" {
		t.Errorf("app image = %q, want %q", app.Image, "myapp:v1")
	}

	// App should have DISPLAY env var
	hasDisplay := false
	for _, env := range app.Env {
		if env.Name == "DISPLAY" && env.Value == ":99" {
			hasDisplay = true
		}
	}
	if !hasDisplay {
		t.Error("app container missing DISPLAY=:99 env var")
	}

	// Should have X11 volume + workspace volume
	if len(pod.Spec.Volumes) != 2 {
		t.Fatalf("len(Volumes) = %d, want 2", len(pod.Spec.Volumes))
	}
	hasX11 := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == X11SocketVolumeName {
			hasX11 = true
		}
	}
	if !hasX11 {
		t.Error("missing X11 volume")
	}

	// Both containers should mount the X11 volume
	hasX11Mount := false
	for _, vm := range vnc.VolumeMounts {
		if vm.Name == X11SocketVolumeName {
			hasX11Mount = true
		}
	}
	if !hasX11Mount {
		t.Error("vnc container missing X11 volume mount")
	}
	hasX11Mount = false
	for _, vm := range app.VolumeMounts {
		if vm.Name == X11SocketVolumeName {
			hasX11Mount = true
		}
	}
	if !hasX11Mount {
		t.Error("app container missing X11 volume mount")
	}
}

func TestBuildPodSpec_JlesageImage(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-456", "app-2", "Firefox", "jlesage/firefox:latest")
	pod := BuildPodSpec(config)

	// jlesage images: single container, no sidecar
	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("len(Containers) = %d, want 1 for jlesage image", len(pod.Spec.Containers))
	}

	app := pod.Spec.Containers[0]
	if app.Name != "app" {
		t.Errorf("container name = %q, want %q", app.Name, "app")
	}
	if app.Image != "jlesage/firefox:latest" {
		t.Errorf("image = %q, want %q", app.Image, "jlesage/firefox:latest")
	}

	// Should expose ports 5800 and 5900
	if len(app.Ports) != 2 {
		t.Fatalf("len(Ports) = %d, want 2", len(app.Ports))
	}
	portMap := make(map[string]int32)
	for _, p := range app.Ports {
		portMap[p.Name] = p.ContainerPort
	}
	if portMap["http"] != 5800 {
		t.Errorf("http port = %d, want 5800", portMap["http"])
	}
	if portMap["vnc"] != 5900 {
		t.Errorf("vnc port = %d, want 5900", portMap["vnc"])
	}

	// Websocket port annotation should be 5800 for jlesage
	if pod.Annotations["sortie.io/websocket-port"] != "5800" {
		t.Errorf("websocket-port annotation = %q, want %q", pod.Annotations["sortie.io/websocket-port"], "5800")
	}

	// Readiness probe on port 5800
	if app.ReadinessProbe == nil {
		t.Fatal("ReadinessProbe is nil")
	}
	if app.ReadinessProbe.TCPSocket.Port.IntValue() != 5800 {
		t.Errorf("readiness probe port = %d, want 5800", app.ReadinessProbe.TCPSocket.Port.IntValue())
	}

	// Only workspace volume for jlesage images
	if len(pod.Spec.Volumes) != 1 {
		t.Errorf("len(Volumes) = %d, want 1 (workspace only) for jlesage image", len(pod.Spec.Volumes))
	}

	// jlesage images should NOT have DISPLAY env var
	for _, env := range app.Env {
		if env.Name == "DISPLAY" {
			t.Error("jlesage image should not have DISPLAY env var")
		}
	}
}

func TestBuildPodSpec_CustomEnvVars(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "myapp:v1")
	config.EnvVars = map[string]string{
		"MY_VAR": "my_value",
		"FOO":    "bar",
	}
	pod := BuildPodSpec(config)

	app := pod.Spec.Containers[1] // app container
	envMap := make(map[string]string)
	for _, env := range app.Env {
		envMap[env.Name] = env.Value
	}

	if envMap["MY_VAR"] != "my_value" {
		t.Errorf("MY_VAR = %q, want %q", envMap["MY_VAR"], "my_value")
	}
	if envMap["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", envMap["FOO"], "bar")
	}
}

func TestBuildPodSpec_ScreenResolution(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "myapp:v1")
	config.ScreenResolution = "1920x1080x24"
	pod := BuildPodSpec(config)

	vnc := pod.Spec.Containers[0]
	hasResolution := false
	for _, env := range vnc.Env {
		if env.Name == "SCREEN_RESOLUTION" && env.Value == "1920x1080x24" {
			hasResolution = true
		}
	}
	if !hasResolution {
		t.Error("vnc sidecar missing SCREEN_RESOLUTION env var")
	}
}

func TestBuildPodSpec_ScreenDimensions(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "myapp:v1")
	config.ScreenWidth = 1920
	config.ScreenHeight = 1080
	pod := BuildPodSpec(config)

	app := pod.Spec.Containers[1]
	envMap := make(map[string]string)
	for _, env := range app.Env {
		envMap[env.Name] = env.Value
	}

	if envMap["SELKIES_MANUAL_WIDTH"] != "1920" {
		t.Errorf("SELKIES_MANUAL_WIDTH = %q, want %q", envMap["SELKIES_MANUAL_WIDTH"], "1920")
	}
	if envMap["SELKIES_MANUAL_HEIGHT"] != "1080" {
		t.Errorf("SELKIES_MANUAL_HEIGHT = %q, want %q", envMap["SELKIES_MANUAL_HEIGHT"], "1080")
	}
}

func TestBuildPodSpec_CustomCommand(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "myapp:v1")
	config.Command = []string{"/bin/sh", "-c"}
	config.Args = []string{"echo hello"}
	pod := BuildPodSpec(config)

	app := pod.Spec.Containers[1]
	if len(app.Command) != 2 || app.Command[0] != "/bin/sh" {
		t.Errorf("Command = %v, want [/bin/sh -c]", app.Command)
	}
	if len(app.Args) != 1 || app.Args[0] != "echo hello" {
		t.Errorf("Args = %v, want [echo hello]", app.Args)
	}
}

func TestBuildPodSpec_CustomVNCSidecarImage(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "custom-vnc:v2")

	config := DefaultPodConfig("sess-1", "app-1", "App", "myapp:v1")
	pod := BuildPodSpec(config)

	vnc := pod.Spec.Containers[0]
	if vnc.Image != "custom-vnc:v2" {
		t.Errorf("vnc image = %q, want %q", vnc.Image, "custom-vnc:v2")
	}
}

func TestBuildWebProxyPodSpec(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-789", "app-3", "Web App", "webapp:v1")
	config.ContainerPort = 3000
	pod := BuildWebProxyPodSpec(config)

	// Check pod metadata
	if pod.Name != "sortie-session-sess-789" {
		t.Errorf("pod.Name = %q, want %q", pod.Name, "sortie-session-sess-789")
	}
	if pod.Namespace != "test-ns" {
		t.Errorf("pod.Namespace = %q, want %q", pod.Namespace, "test-ns")
	}

	// Check annotations
	if pod.Annotations["sortie.io/container-port"] != "3000" {
		t.Errorf("container-port annotation = %q, want %q", pod.Annotations["sortie.io/container-port"], "3000")
	}
	if pod.Annotations["sortie.io/websocket-port"] != "6080" {
		t.Errorf("websocket-port annotation = %q, want %q", pod.Annotations["sortie.io/websocket-port"], "6080")
	}

	// Should have 2 containers (browser-sidecar + app)
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("len(Containers) = %d, want 2", len(pod.Spec.Containers))
	}

	browser := pod.Spec.Containers[0]
	if browser.Name != "browser-sidecar" {
		t.Errorf("browser container name = %q, want %q", browser.Name, "browser-sidecar")
	}
	if browser.Image != BrowserSidecarImage {
		t.Errorf("browser image = %q, want %q", browser.Image, BrowserSidecarImage)
	}

	// Check BROWSER_URL env
	hasBrowserURL := false
	for _, env := range browser.Env {
		if env.Name == "BROWSER_URL" && env.Value == "http://localhost:3000" {
			hasBrowserURL = true
		}
	}
	if !hasBrowserURL {
		t.Error("browser sidecar missing BROWSER_URL=http://localhost:3000")
	}

	// Check app container
	app := pod.Spec.Containers[1]
	if app.Name != "app" {
		t.Errorf("app container name = %q, want %q", app.Name, "app")
	}
	if len(app.Ports) != 1 || app.Ports[0].ContainerPort != 3000 {
		t.Errorf("app port = %v, want containerPort=3000", app.Ports)
	}

	// App should have readiness probe on port 3000
	if app.ReadinessProbe == nil {
		t.Fatal("app ReadinessProbe is nil")
	}
	if app.ReadinessProbe.TCPSocket.Port.IntValue() != 3000 {
		t.Errorf("app readiness probe port = %d, want 3000", app.ReadinessProbe.TCPSocket.Port.IntValue())
	}

	// Browser sidecar should have security context
	if browser.SecurityContext == nil {
		t.Error("browser SecurityContext is nil")
	} else {
		if *browser.SecurityContext.RunAsUser != 1000 {
			t.Errorf("browser RunAsUser = %d, want 1000", *browser.SecurityContext.RunAsUser)
		}
	}
}

func TestBuildWebProxyPodSpec_DefaultPort(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "webapp:v1")
	// ContainerPort left at 0
	pod := BuildWebProxyPodSpec(config)

	if pod.Annotations["sortie.io/container-port"] != "8080" {
		t.Errorf("container-port annotation = %q, want %q (default)", pod.Annotations["sortie.io/container-port"], "8080")
	}

	// Check BROWSER_URL uses default port 8080
	browser := pod.Spec.Containers[0]
	for _, env := range browser.Env {
		if env.Name == "BROWSER_URL" && env.Value != "http://localhost:8080" {
			t.Errorf("BROWSER_URL = %q, want %q", env.Value, "http://localhost:8080")
		}
	}
}

func TestBuildWebProxyPodSpec_ScreenResolution(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "webapp:v1")
	config.ScreenResolution = "1280x720x24"
	pod := BuildWebProxyPodSpec(config)

	browser := pod.Spec.Containers[0]
	hasResolution := false
	for _, env := range browser.Env {
		if env.Name == "SCREEN_RESOLUTION" && env.Value == "1280x720x24" {
			hasResolution = true
		}
	}
	if !hasResolution {
		t.Error("browser sidecar missing SCREEN_RESOLUTION env var")
	}
}

func TestBuildWindowsPodSpec(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-win", "app-win", "Windows App", "windows:v1")
	pod := BuildWindowsPodSpec(config)

	// Check pod metadata
	if pod.Name != "sortie-session-sess-win" {
		t.Errorf("pod.Name = %q, want %q", pod.Name, "sortie-session-sess-win")
	}

	// Check annotations
	if pod.Annotations["sortie.io/protocol"] != "rdp" {
		t.Errorf("protocol annotation = %q, want %q", pod.Annotations["sortie.io/protocol"], "rdp")
	}
	if pod.Annotations["sortie.io/guacd-port"] != "4822" {
		t.Errorf("guacd-port annotation = %q, want %q", pod.Annotations["sortie.io/guacd-port"], "4822")
	}

	// Should have 2 containers (guacd-sidecar + app)
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("len(Containers) = %d, want 2", len(pod.Spec.Containers))
	}

	guacd := pod.Spec.Containers[0]
	if guacd.Name != "guacd-sidecar" {
		t.Errorf("guacd container name = %q, want %q", guacd.Name, "guacd-sidecar")
	}
	if guacd.Image != "guacamole/guacd:1.5.5" {
		t.Errorf("guacd image = %q, want %q", guacd.Image, "guacamole/guacd:1.5.5")
	}
	if len(guacd.Ports) != 1 || guacd.Ports[0].ContainerPort != 4822 {
		t.Errorf("guacd port = %v, want containerPort=4822", guacd.Ports)
	}

	// Guacd should have readiness and liveness probes
	if guacd.ReadinessProbe == nil {
		t.Error("guacd ReadinessProbe is nil")
	}
	if guacd.LivenessProbe == nil {
		t.Error("guacd LivenessProbe is nil")
	}

	// Check app container
	app := pod.Spec.Containers[1]
	if app.Name != "app" {
		t.Errorf("app container name = %q, want %q", app.Name, "app")
	}
	if app.Image != "windows:v1" {
		t.Errorf("app image = %q, want %q", app.Image, "windows:v1")
	}
	if len(app.Ports) != 1 || app.Ports[0].ContainerPort != 3389 {
		t.Errorf("app port = %v, want containerPort=3389 (RDP)", app.Ports)
	}

	// App should have readiness probe on RDP port
	if app.ReadinessProbe == nil {
		t.Fatal("app ReadinessProbe is nil")
	}
	if app.ReadinessProbe.TCPSocket.Port.IntValue() != 3389 {
		t.Errorf("app readiness probe port = %d, want 3389", app.ReadinessProbe.TCPSocket.Port.IntValue())
	}
}

func TestBuildWindowsPodSpec_CustomGuacdImage(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")
	ConfigureGuacdSidecar("custom-guacd:v3")

	config := DefaultPodConfig("sess-1", "app-1", "App", "windows:v1")
	pod := BuildWindowsPodSpec(config)

	guacd := pod.Spec.Containers[0]
	if guacd.Image != "custom-guacd:v3" {
		t.Errorf("guacd image = %q, want %q", guacd.Image, "custom-guacd:v3")
	}
}

func TestBuildWindowsPodSpec_EnvVars(t *testing.T) {
	defer ResetClient()
	Configure("test-ns", "", "")

	config := DefaultPodConfig("sess-1", "app-1", "App", "windows:v1")
	config.EnvVars = map[string]string{"RDP_USER": "admin"}
	pod := BuildWindowsPodSpec(config)

	app := pod.Spec.Containers[1]
	hasEnv := false
	for _, env := range app.Env {
		if env.Name == "RDP_USER" && env.Value == "admin" {
			hasEnv = true
		}
	}
	if !hasEnv {
		t.Error("app container missing RDP_USER env var")
	}
}

// Tests using the fake k8s client for CRUD operations

func setFakeClient(t *testing.T) *fake.Clientset {
	t.Helper()
	ResetClient()
	Configure("test-ns", "", "")

	fakeClient := fake.NewSimpleClientset()
	client = fakeClient
	clientErr = nil
	clientOnce.Do(func() {}) // prevent re-initialization
	return fakeClient
}

func TestCreatePod_WithFakeClient(t *testing.T) {
	defer ResetClient()
	fakeClient := setFakeClient(t)
	_ = fakeClient

	config := DefaultPodConfig("sess-create", "app-1", "App", "myapp:v1")
	pod := BuildPodSpec(config)

	created, err := CreatePod(context.Background(), pod)
	if err != nil {
		t.Fatalf("CreatePod() error = %v", err)
	}
	if created.Name != "sortie-session-sess-create" {
		t.Errorf("created pod name = %q, want %q", created.Name, "sortie-session-sess-create")
	}
}

func TestGetPod_WithFakeClient(t *testing.T) {
	defer ResetClient()
	setFakeClient(t)

	// Create a pod first
	config := DefaultPodConfig("sess-get", "app-1", "App", "myapp:v1")
	pod := BuildPodSpec(config)
	_, err := CreatePod(context.Background(), pod)
	if err != nil {
		t.Fatalf("CreatePod() error = %v", err)
	}

	// Get it back
	got, err := GetPod(context.Background(), "sortie-session-sess-get")
	if err != nil {
		t.Fatalf("GetPod() error = %v", err)
	}
	if got.Name != "sortie-session-sess-get" {
		t.Errorf("GetPod().Name = %q, want %q", got.Name, "sortie-session-sess-get")
	}
}

func TestDeletePod_WithFakeClient(t *testing.T) {
	defer ResetClient()
	setFakeClient(t)

	// Create then delete
	config := DefaultPodConfig("sess-del", "app-1", "App", "myapp:v1")
	pod := BuildPodSpec(config)
	_, err := CreatePod(context.Background(), pod)
	if err != nil {
		t.Fatalf("CreatePod() error = %v", err)
	}

	err = DeletePod(context.Background(), "sortie-session-sess-del")
	if err != nil {
		t.Fatalf("DeletePod() error = %v", err)
	}

	// Verify deleted
	_, err = GetPod(context.Background(), "sortie-session-sess-del")
	if err == nil {
		t.Error("GetPod() after delete should return error")
	}
}

func TestGetPodIP_NoIP(t *testing.T) {
	defer ResetClient()
	fakeClient := setFakeClient(t)

	// Create pod without IP
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-ip-pod",
			Namespace: "test-ns",
		},
	}
	_, err := fakeClient.CoreV1().Pods("test-ns").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = GetPodIP(context.Background(), "no-ip-pod")
	if err == nil {
		t.Error("GetPodIP() should return error for pod with no IP")
	}
}

func TestListSessionPods_WithFakeClient(t *testing.T) {
	defer ResetClient()
	fakeClient := setFakeClient(t)

	// Create a session pod
	config := DefaultPodConfig("sess-list", "app-1", "App", "myapp:v1")
	pod := BuildPodSpec(config)
	_, err := CreatePod(context.Background(), pod)
	if err != nil {
		t.Fatalf("CreatePod() error = %v", err)
	}

	// Create a non-session pod (no sortie labels)
	otherPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-pod",
			Namespace: "test-ns",
		},
	}
	_, err = fakeClient.CoreV1().Pods("test-ns").Create(context.Background(), otherPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// List should only return session pods
	list, err := ListSessionPods(context.Background())
	if err != nil {
		t.Fatalf("ListSessionPods() error = %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("len(ListSessionPods) = %d, want 1", len(list.Items))
	}
}

func TestWaitForPodReady_AlreadyReady(t *testing.T) {
	defer ResetClient()
	fakeClient := setFakeClient(t)

	// Create a pod then set it to ready via UpdateStatus
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ready-pod",
			Namespace: "test-ns",
		},
	}

	createdPod, err := fakeClient.CoreV1().Pods("test-ns").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	createdPod.Status = corev1.PodStatus{
		Phase: corev1.PodRunning,
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	_, err = fakeClient.CoreV1().Pods("test-ns").UpdateStatus(context.Background(), createdPod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	err = WaitForPodReady(context.Background(), "ready-pod", 5*time.Second)
	if err != nil {
		t.Errorf("WaitForPodReady() error = %v, want nil for ready pod", err)
	}
}

func TestWaitForPodReady_Failed(t *testing.T) {
	defer ResetClient()
	fakeClient := setFakeClient(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failed-pod",
			Namespace: "test-ns",
		},
	}

	createdPod, err := fakeClient.CoreV1().Pods("test-ns").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	createdPod.Status = corev1.PodStatus{Phase: corev1.PodFailed}
	_, err = fakeClient.CoreV1().Pods("test-ns").UpdateStatus(context.Background(), createdPod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	err = WaitForPodReady(context.Background(), "failed-pod", 5*time.Second)
	if err == nil {
		t.Error("WaitForPodReady() should return error for failed pod")
	}
}

func TestWaitForPodReady_Timeout(t *testing.T) {
	defer ResetClient()
	fakeClient := setFakeClient(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod",
			Namespace: "test-ns",
		},
	}
	createdPod, err := fakeClient.CoreV1().Pods("test-ns").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	createdPod.Status = corev1.PodStatus{Phase: corev1.PodPending}
	_, err = fakeClient.CoreV1().Pods("test-ns").UpdateStatus(context.Background(), createdPod, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	err = WaitForPodReady(context.Background(), "pending-pod", 3*time.Second)
	if err == nil {
		t.Error("WaitForPodReady() should return error on timeout")
	}
}

func TestGetPod_NotFound(t *testing.T) {
	defer ResetClient()
	setFakeClient(t)

	_, err := GetPod(context.Background(), "nonexistent-pod")
	if err == nil {
		t.Error("GetPod() should return error for nonexistent pod")
	}
}

func TestDeletePod_NotFound(t *testing.T) {
	defer ResetClient()
	setFakeClient(t)

	err := DeletePod(context.Background(), "nonexistent-pod")
	if err == nil {
		t.Error("DeletePod() should return error for nonexistent pod")
	}
}

func TestClientError_PropagatesOnOperations(t *testing.T) {
	defer ResetClient()

	// Set client error
	clientErr = fmt.Errorf("connection refused")
	clientOnce.Do(func() {})

	ctx := context.Background()

	_, err := CreatePod(ctx, &corev1.Pod{})
	if err == nil {
		t.Error("CreatePod() should return error when client has error")
	}

	err = DeletePod(ctx, "pod")
	if err == nil {
		t.Error("DeletePod() should return error when client has error")
	}

	_, err = GetPod(ctx, "pod")
	if err == nil {
		t.Error("GetPod() should return error when client has error")
	}

	_, err = GetPodIP(ctx, "pod")
	if err == nil {
		t.Error("GetPodIP() should return error when client has error")
	}

	_, err = ListSessionPods(ctx)
	if err == nil {
		t.Error("ListSessionPods() should return error when client has error")
	}

	err = WaitForPodReady(ctx, "pod", time.Second)
	if err == nil {
		t.Error("WaitForPodReady() should return error when client has error")
	}
}

func TestHelperFunctions(t *testing.T) {
	b := boolPtr(true)
	if *b != true {
		t.Errorf("boolPtr(true) = %v, want true", *b)
	}
	b = boolPtr(false)
	if *b != false {
		t.Errorf("boolPtr(false) = %v, want false", *b)
	}

	i := int64Ptr(42)
	if *i != 42 {
		t.Errorf("int64Ptr(42) = %v, want 42", *i)
	}
	i = int64Ptr(0)
	if *i != 0 {
		t.Errorf("int64Ptr(0) = %v, want 0", *i)
	}
}
