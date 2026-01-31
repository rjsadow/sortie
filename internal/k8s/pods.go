package k8s

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// VNCSidecarImage is the default VNC sidecar container image
	VNCSidecarImage = "ghcr.io/rjsadow/launchpad-vnc-sidecar:latest"

	// X11SocketVolumeName is the name of the shared X11 socket volume
	X11SocketVolumeName = "x11-socket"

	// SessionLabelKey is the label key for session identification
	SessionLabelKey = "launchpad.io/session-id"

	// AppLabelKey is the label key for app identification
	AppLabelKey = "launchpad.io/app-id"

	// ComponentLabelKey is the label key for component identification
	ComponentLabelKey = "app.kubernetes.io/component"
)

// PodConfig contains configuration for creating a session pod
type PodConfig struct {
	SessionID      string
	AppID          string
	AppName        string
	ContainerImage string
	Command        []string
	Args           []string
	EnvVars        map[string]string
	CPULimit       string
	MemoryLimit    string
	CPURequest     string
	MemoryRequest  string
}

// DefaultPodConfig returns a PodConfig with sensible defaults
func DefaultPodConfig(sessionID, appID, appName, containerImage string) *PodConfig {
	return &PodConfig{
		SessionID:      sessionID,
		AppID:          appID,
		AppName:        appName,
		ContainerImage: containerImage,
		CPULimit:       "2",
		MemoryLimit:    "2Gi",
		CPURequest:     "500m",
		MemoryRequest:  "512Mi",
	}
}

// BuildPodSpec creates a Kubernetes Pod specification for a session
func BuildPodSpec(config *PodConfig) *corev1.Pod {
	// Get VNC sidecar image from env or use default
	vncImage := os.Getenv("LAUNCHPAD_VNC_SIDECAR_IMAGE")
	if vncImage == "" {
		vncImage = VNCSidecarImage
	}

	// Sanitize pod name (must be DNS-1123 compliant)
	podName := fmt.Sprintf("launchpad-session-%s", config.SessionID)

	// Build environment variables for app container
	appEnv := []corev1.EnvVar{
		{Name: "DISPLAY", Value: ":99"},
	}
	for key, value := range config.EnvVars {
		appEnv = append(appEnv, corev1.EnvVar{Name: key, Value: value})
	}

	// Parse resource limits
	cpuLimit := resource.MustParse(config.CPULimit)
	memoryLimit := resource.MustParse(config.MemoryLimit)
	cpuRequest := resource.MustParse(config.CPURequest)
	memoryRequest := resource.MustParse(config.MemoryRequest)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: GetNamespace(),
			Labels: map[string]string{
				SessionLabelKey:   config.SessionID,
				AppLabelKey:       config.AppID,
				ComponentLabelKey: "session",
			},
			Annotations: map[string]string{
				"launchpad.io/app-name":   config.AppName,
				"launchpad.io/created-at": time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			// Security context for the pod
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: boolPtr(true),
				RunAsUser:    int64Ptr(1000),
				RunAsGroup:   int64Ptr(1000),
				FSGroup:      int64Ptr(1000),
			},
			// Shared volumes
			Volumes: []corev1.Volume{
				{
					Name: X11SocketVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			Containers: []corev1.Container{
				// VNC Sidecar container (runs first to set up X11)
				{
					Name:  "vnc-sidecar",
					Image: vncImage,
					Ports: []corev1.ContainerPort{
						{Name: "vnc", ContainerPort: 5900, Protocol: corev1.ProtocolTCP},
						{Name: "websocket", ContainerPort: 6080, Protocol: corev1.ProtocolTCP},
					},
					Env: []corev1.EnvVar{
						{Name: "DISPLAY", Value: ":99"},
						{Name: "VNC_PASSWORD", Value: ""}, // No password for internal use
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: X11SocketVolumeName, MountPath: "/tmp/.X11-unix"},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: boolPtr(false),
						ReadOnlyRootFilesystem:   boolPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.FromInt(6080),
							},
						},
						InitialDelaySeconds: 2,
						PeriodSeconds:       5,
						TimeoutSeconds:      2,
						SuccessThreshold:    1,
						FailureThreshold:    6,
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.FromInt(6080),
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       30,
						TimeoutSeconds:      5,
						SuccessThreshold:    1,
						FailureThreshold:    3,
					},
				},
				// Application container
				{
					Name:    "app",
					Image:   config.ContainerImage,
					Command: config.Command,
					Args:    config.Args,
					Env:     appEnv,
					VolumeMounts: []corev1.VolumeMount{
						{Name: X11SocketVolumeName, MountPath: "/tmp/.X11-unix"},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    cpuLimit,
							corev1.ResourceMemory: memoryLimit,
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    cpuRequest,
							corev1.ResourceMemory: memoryRequest,
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: boolPtr(false),
						ReadOnlyRootFilesystem:   boolPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			},
		},
	}

	return pod
}

// CreatePod creates a new pod in the cluster
func CreatePod(ctx context.Context, pod *corev1.Pod) (*corev1.Pod, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	return client.CoreV1().Pods(GetNamespace()).Create(ctx, pod, metav1.CreateOptions{})
}

// DeletePod deletes a pod by name
func DeletePod(ctx context.Context, podName string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	return client.CoreV1().Pods(GetNamespace()).Delete(ctx, podName, metav1.DeleteOptions{})
}

// GetPod retrieves a pod by name
func GetPod(ctx context.Context, podName string) (*corev1.Pod, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	return client.CoreV1().Pods(GetNamespace()).Get(ctx, podName, metav1.GetOptions{})
}

// WaitForPodReady waits for a pod to be ready with a timeout
func WaitForPodReady(ctx context.Context, podName string, timeout time.Duration) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := client.CoreV1().Pods(GetNamespace()).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// Check if pod is ready
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		// Check if pod has failed
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return false, fmt.Errorf("pod %s is in terminal state: %s", podName, pod.Status.Phase)
		}

		return false, nil
	})
}

// GetPodIP returns the IP address of a pod
func GetPodIP(ctx context.Context, podName string) (string, error) {
	pod, err := GetPod(ctx, podName)
	if err != nil {
		return "", err
	}

	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("pod %s has no IP address yet", podName)
	}

	return pod.Status.PodIP, nil
}

// ListSessionPods lists all pods belonging to launchpad sessions
func ListSessionPods(ctx context.Context) (*corev1.PodList, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	return client.CoreV1().Pods(GetNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s,%s=session", SessionLabelKey, ComponentLabelKey),
	})
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}
