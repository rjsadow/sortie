package runner

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/k8s"
	corev1 "k8s.io/api/core/v1"
)

// KubernetesRunner implements Runner for Kubernetes pod-based workloads.
// It delegates to the existing internal/k8s package for pod and network policy operations.
type KubernetesRunner struct{}

// NewKubernetesRunner creates a new Kubernetes runner.
// The k8s package must be configured via k8s.Configure() before use.
func NewKubernetesRunner() *KubernetesRunner {
	return &KubernetesRunner{}
}

// Type returns TypeKubernetes.
func (r *KubernetesRunner) Type() Type {
	return TypeKubernetes
}

// CreateWorkload creates a Kubernetes pod for the given workload configuration.
func (r *KubernetesRunner) CreateWorkload(ctx context.Context, config *WorkloadConfig) (*WorkloadResult, error) {
	podConfig := k8s.DefaultPodConfig(config.SessionID, config.AppID, config.AppName, config.ContainerImage)
	podConfig.ContainerPort = config.ContainerPort
	podConfig.Command = config.Command
	podConfig.Args = config.Args
	podConfig.EnvVars = config.EnvVars
	if config.CPULimit != "" {
		podConfig.CPULimit = config.CPULimit
	}
	if config.MemoryLimit != "" {
		podConfig.MemoryLimit = config.MemoryLimit
	}
	if config.CPURequest != "" {
		podConfig.CPURequest = config.CPURequest
	}
	if config.MemoryRequest != "" {
		podConfig.MemoryRequest = config.MemoryRequest
	}
	podConfig.ScreenResolution = config.ScreenResolution
	podConfig.ScreenWidth = config.ScreenWidth
	podConfig.ScreenHeight = config.ScreenHeight

	// Build the pod spec based on launch type and OS
	pod := buildPod(podConfig, config.LaunchType, config.OsType)

	createdPod, err := k8s.CreatePod(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	return &WorkloadResult{Name: createdPod.Name}, nil
}

// DeleteWorkload deletes a Kubernetes pod by name.
func (r *KubernetesRunner) DeleteWorkload(ctx context.Context, name string) error {
	return k8s.DeletePod(ctx, name)
}

// WaitForReady waits for a pod to become ready.
func (r *KubernetesRunner) WaitForReady(ctx context.Context, name string, timeout time.Duration) error {
	return k8s.WaitForPodReady(ctx, name, timeout)
}

// GetIP returns the pod IP address.
func (r *KubernetesRunner) GetIP(ctx context.Context, name string) (string, error) {
	return k8s.GetPodIP(ctx, name)
}

// ListWorkloads returns all launchpad session pods.
func (r *KubernetesRunner) ListWorkloads(ctx context.Context) ([]WorkloadInfo, error) {
	podList, err := k8s.ListSessionPods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list session pods: %w", err)
	}

	var workloads []WorkloadInfo
	for _, pod := range podList.Items {
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				ready = true
				break
			}
		}
		workloads = append(workloads, WorkloadInfo{
			Name:  pod.Name,
			IP:    pod.Status.PodIP,
			Ready: ready,
		})
	}

	return workloads, nil
}

// Healthy checks if the Kubernetes API is reachable.
func (r *KubernetesRunner) Healthy(_ context.Context) bool {
	_, err := k8s.GetClient()
	return err == nil
}

// Close is a no-op for the Kubernetes runner (the k8s client is a singleton).
func (r *KubernetesRunner) Close() error {
	return nil
}

// CreateNetworkPolicy creates a Kubernetes NetworkPolicy for a session.
func (r *KubernetesRunner) CreateNetworkPolicy(ctx context.Context, sessionID, appID string, policy *db.EgressPolicy) error {
	if policy == nil || policy.Mode == "" {
		return nil
	}

	np := k8s.BuildSessionNetworkPolicy(sessionID, appID, policy)
	if np == nil {
		return nil
	}

	if _, err := k8s.CreateNetworkPolicy(ctx, np); err != nil {
		return fmt.Errorf("failed to create network policy: %w", err)
	}

	log.Printf("Created egress NetworkPolicy for session %s (mode: %s, rules: %d)",
		sessionID, policy.Mode, len(policy.Rules))
	return nil
}

// DeleteNetworkPolicy removes the Kubernetes NetworkPolicy for a session.
func (r *KubernetesRunner) DeleteNetworkPolicy(ctx context.Context, sessionID string) error {
	return k8s.DeleteSessionNetworkPolicy(ctx, sessionID)
}

// buildPod selects the appropriate pod builder based on launch type and OS.
func buildPod(podConfig *k8s.PodConfig, launchType, osType string) *corev1.Pod {
	switch launchType {
	case "web_proxy":
		return k8s.BuildWebProxyPodSpec(podConfig)
	case "container":
		if osType == "windows" {
			return k8s.BuildWindowsPodSpec(podConfig)
		}
		return k8s.BuildPodSpec(podConfig)
	default:
		return k8s.BuildPodSpec(podConfig)
	}
}

// Compile-time interface checks.
var (
	_ Runner              = (*KubernetesRunner)(nil)
	_ NetworkPolicyRunner = (*KubernetesRunner)(nil)
)
