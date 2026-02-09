// Package runner provides a pluggable interface for workload orchestration backends.
// It abstracts the details of running containerized workloads across different
// infrastructure providers (Kubernetes, Docker, Nomad, etc.).
//
// The session manager uses this interface to create, monitor, and tear down
// workloads without being coupled to a specific orchestrator.
package runner

import (
	"context"
	"time"

	"github.com/rjsadow/sortie/internal/db"
)

// Type identifies the workload orchestration backend.
type Type string

const (
	TypeKubernetes Type = "kubernetes"
	TypeDocker     Type = "docker"
	TypeNomad      Type = "nomad"
)

// WorkloadConfig contains the configuration for creating a workload.
// This generalizes k8s.PodConfig to work across backends.
type WorkloadConfig struct {
	SessionID        string
	AppID            string
	AppName          string
	ContainerImage   string
	ContainerPort    int
	Command          []string
	Args             []string
	EnvVars          map[string]string
	CPULimit         string
	MemoryLimit      string
	CPURequest       string
	MemoryRequest    string
	ScreenResolution string
	ScreenWidth      int
	ScreenHeight     int
	LaunchType       string // "container" or "web_proxy"
	OsType           string // "linux" or "windows"
}

// WorkloadResult contains the result of creating a workload.
type WorkloadResult struct {
	Name string // Unique workload identifier (pod name, container ID, etc.)
}

// WorkloadInfo contains runtime information about a workload.
type WorkloadInfo struct {
	Name  string
	IP    string
	Ready bool
}

// Runner is the pluggable interface for workload orchestration backends.
// Implementations manage the full lifecycle of containerized workloads.
type Runner interface {
	// Type returns the runner backend type.
	Type() Type

	// CreateWorkload provisions a new workload and returns its identifier.
	CreateWorkload(ctx context.Context, config *WorkloadConfig) (*WorkloadResult, error)

	// DeleteWorkload terminates and removes a workload by name.
	DeleteWorkload(ctx context.Context, name string) error

	// WaitForReady blocks until the workload is ready or the context/timeout expires.
	WaitForReady(ctx context.Context, name string, timeout time.Duration) error

	// GetIP returns the routable IP address of a running workload.
	GetIP(ctx context.Context, name string) (string, error)

	// ListWorkloads returns all session-managed workloads.
	ListWorkloads(ctx context.Context) ([]WorkloadInfo, error)

	// Healthy returns true if the runner backend is reachable.
	Healthy(ctx context.Context) bool

	// Close releases any resources held by the runner.
	Close() error
}

// NetworkPolicyRunner is an optional interface for runners that support
// network egress policies. Runners that don't support network policies
// can simply not implement this interface.
type NetworkPolicyRunner interface {
	// CreateNetworkPolicy creates a network egress policy for a session.
	CreateNetworkPolicy(ctx context.Context, sessionID, appID string, policy *db.EgressPolicy) error

	// DeleteNetworkPolicy removes the network policy for a session.
	DeleteNetworkPolicy(ctx context.Context, sessionID string) error
}
