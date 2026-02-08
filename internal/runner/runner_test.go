package runner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rjsadow/launchpad/internal/db"
)

// --- Mock Runner for testing the interface contract ---

type mockRunner struct {
	mu        sync.Mutex
	workloads map[string]*mockWorkload
	healthy   bool
	closed    bool
}

type mockWorkload struct {
	name  string
	ip    string
	ready bool
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		workloads: make(map[string]*mockWorkload),
		healthy:   true,
	}
}

func (r *mockRunner) Type() Type { return "mock" }

func (r *mockRunner) CreateWorkload(_ context.Context, config *WorkloadConfig) (*WorkloadResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := fmt.Sprintf("mock-%s", config.SessionID)
	r.workloads[name] = &mockWorkload{name: name}
	return &WorkloadResult{Name: name}, nil
}

func (r *mockRunner) DeleteWorkload(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workloads[name]; !ok {
		return fmt.Errorf("workload not found: %s", name)
	}
	delete(r.workloads, name)
	return nil
}

func (r *mockRunner) WaitForReady(_ context.Context, name string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workloads[name]; ok {
		w.ready = true
		w.ip = "10.0.0.1"
		return nil
	}
	return fmt.Errorf("workload not found: %s", name)
}

func (r *mockRunner) GetIP(_ context.Context, name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workloads[name]; ok {
		if w.ip == "" {
			return "", fmt.Errorf("workload %s has no IP", name)
		}
		return w.ip, nil
	}
	return "", fmt.Errorf("workload not found: %s", name)
}

func (r *mockRunner) ListWorkloads(_ context.Context) ([]WorkloadInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []WorkloadInfo
	for _, w := range r.workloads {
		result = append(result, WorkloadInfo{Name: w.name, IP: w.ip, Ready: w.ready})
	}
	return result, nil
}

func (r *mockRunner) Healthy(_ context.Context) bool {
	return r.healthy
}

func (r *mockRunner) Close() error {
	r.closed = true
	return nil
}

// mockNetworkPolicyRunner extends mockRunner with network policy support.
type mockNetworkPolicyRunner struct {
	mockRunner
	policies map[string]string // sessionID -> appID
}

func newMockNetworkPolicyRunner() *mockNetworkPolicyRunner {
	return &mockNetworkPolicyRunner{
		mockRunner: *newMockRunner(),
		policies:   make(map[string]string),
	}
}

func (r *mockNetworkPolicyRunner) CreateNetworkPolicy(_ context.Context, sessionID, appID string, _ *db.EgressPolicy) error {
	r.policies[sessionID] = appID
	return nil
}

func (r *mockNetworkPolicyRunner) DeleteNetworkPolicy(_ context.Context, sessionID string) error {
	delete(r.policies, sessionID)
	return nil
}

// --- Interface compliance ---

func TestMockRunner_ImplementsRunner(t *testing.T) {
	var _ Runner = (*mockRunner)(nil)
}

func TestMockNetworkPolicyRunner_ImplementsNetworkPolicyRunner(t *testing.T) {
	var _ NetworkPolicyRunner = (*mockNetworkPolicyRunner)(nil)
}

func TestKubernetesRunner_ImplementsRunner(t *testing.T) {
	var _ Runner = (*KubernetesRunner)(nil)
}

func TestKubernetesRunner_ImplementsNetworkPolicyRunner(t *testing.T) {
	var _ NetworkPolicyRunner = (*KubernetesRunner)(nil)
}

// --- Type constants ---

func TestRunnerTypes(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{TypeKubernetes, "kubernetes"},
		{TypeDocker, "docker"},
		{TypeNomad, "nomad"},
	}
	for _, tt := range tests {
		if string(tt.typ) != tt.want {
			t.Errorf("Type %v = %q, want %q", tt.typ, string(tt.typ), tt.want)
		}
	}
}

// --- WorkloadConfig ---

func TestWorkloadConfig_Fields(t *testing.T) {
	wc := &WorkloadConfig{
		SessionID:        "sess-1",
		AppID:            "app-1",
		AppName:          "Test App",
		ContainerImage:   "nginx:latest",
		ContainerPort:    8080,
		Command:          []string{"/bin/sh"},
		Args:             []string{"-c", "echo hello"},
		EnvVars:          map[string]string{"FOO": "bar"},
		CPULimit:         "2",
		MemoryLimit:      "2Gi",
		CPURequest:       "500m",
		MemoryRequest:    "512Mi",
		ScreenResolution: "1920x1080x24",
		ScreenWidth:      1920,
		ScreenHeight:     1080,
		LaunchType:       "container",
		OsType:           "linux",
	}

	if wc.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", wc.SessionID)
	}
	if wc.LaunchType != "container" {
		t.Errorf("LaunchType = %q, want container", wc.LaunchType)
	}
	if wc.OsType != "linux" {
		t.Errorf("OsType = %q, want linux", wc.OsType)
	}
}

// --- Mock Runner lifecycle tests ---

func TestMockRunner_CreateAndDelete(t *testing.T) {
	r := newMockRunner()
	ctx := context.Background()

	result, err := r.CreateWorkload(ctx, &WorkloadConfig{SessionID: "s1", AppID: "a1"})
	if err != nil {
		t.Fatalf("CreateWorkload error: %v", err)
	}
	if result.Name != "mock-s1" {
		t.Errorf("Name = %q, want mock-s1", result.Name)
	}

	workloads, err := r.ListWorkloads(ctx)
	if err != nil {
		t.Fatalf("ListWorkloads error: %v", err)
	}
	if len(workloads) != 1 {
		t.Errorf("ListWorkloads returned %d, want 1", len(workloads))
	}

	if err := r.DeleteWorkload(ctx, result.Name); err != nil {
		t.Fatalf("DeleteWorkload error: %v", err)
	}

	workloads, err = r.ListWorkloads(ctx)
	if err != nil {
		t.Fatalf("ListWorkloads error: %v", err)
	}
	if len(workloads) != 0 {
		t.Errorf("ListWorkloads returned %d, want 0", len(workloads))
	}
}

func TestMockRunner_WaitForReadyAndGetIP(t *testing.T) {
	r := newMockRunner()
	ctx := context.Background()

	result, _ := r.CreateWorkload(ctx, &WorkloadConfig{SessionID: "s1"})

	// Before ready, no IP
	_, err := r.GetIP(ctx, result.Name)
	if err == nil {
		t.Error("GetIP before ready should fail")
	}

	// Wait for ready
	if err := r.WaitForReady(ctx, result.Name, time.Second); err != nil {
		t.Fatalf("WaitForReady error: %v", err)
	}

	ip, err := r.GetIP(ctx, result.Name)
	if err != nil {
		t.Fatalf("GetIP error: %v", err)
	}
	if ip != "10.0.0.1" {
		t.Errorf("IP = %q, want 10.0.0.1", ip)
	}
}

func TestMockRunner_DeleteNonexistent(t *testing.T) {
	r := newMockRunner()
	err := r.DeleteWorkload(context.Background(), "nonexistent")
	if err == nil {
		t.Error("DeleteWorkload on nonexistent should fail")
	}
}

func TestMockRunner_HealthyAndClose(t *testing.T) {
	r := newMockRunner()

	if !r.Healthy(context.Background()) {
		t.Error("Healthy should return true")
	}

	r.healthy = false
	if r.Healthy(context.Background()) {
		t.Error("Healthy should return false")
	}

	if r.closed {
		t.Error("Should not be closed yet")
	}
	r.Close()
	if !r.closed {
		t.Error("Should be closed after Close()")
	}
}

func TestMockRunner_Type(t *testing.T) {
	r := newMockRunner()
	if r.Type() != "mock" {
		t.Errorf("Type = %q, want mock", r.Type())
	}
}

// --- NetworkPolicyRunner tests ---

func TestNetworkPolicyRunner_CreateAndDelete(t *testing.T) {
	r := newMockNetworkPolicyRunner()
	ctx := context.Background()

	policy := &db.EgressPolicy{Mode: "allowlist"}
	if err := r.CreateNetworkPolicy(ctx, "sess-1", "app-1", policy); err != nil {
		t.Fatalf("CreateNetworkPolicy error: %v", err)
	}

	if r.policies["sess-1"] != "app-1" {
		t.Errorf("policy not stored: %v", r.policies)
	}

	if err := r.DeleteNetworkPolicy(ctx, "sess-1"); err != nil {
		t.Fatalf("DeleteNetworkPolicy error: %v", err)
	}

	if _, ok := r.policies["sess-1"]; ok {
		t.Error("policy should be deleted")
	}
}

func TestNetworkPolicyRunner_TypeAssertion(t *testing.T) {
	// Verify the type assertion pattern used in the session manager works
	var r Runner = &mockNetworkPolicyRunner{
		mockRunner: *newMockRunner(),
		policies:   make(map[string]string),
	}

	npr, ok := r.(NetworkPolicyRunner)
	if !ok {
		t.Fatal("mockNetworkPolicyRunner should implement NetworkPolicyRunner")
	}

	if err := npr.CreateNetworkPolicy(context.Background(), "s1", "a1", &db.EgressPolicy{Mode: "allowlist"}); err != nil {
		t.Fatalf("CreateNetworkPolicy error: %v", err)
	}

	// Plain mockRunner should NOT implement NetworkPolicyRunner
	var plainRunner Runner = newMockRunner()
	if _, ok := plainRunner.(NetworkPolicyRunner); ok {
		t.Error("plain mockRunner should NOT implement NetworkPolicyRunner")
	}
}

// --- KubernetesRunner unit tests (no cluster required) ---

func TestKubernetesRunner_Type(t *testing.T) {
	r := NewKubernetesRunner()
	if r.Type() != TypeKubernetes {
		t.Errorf("Type = %q, want %q", r.Type(), TypeKubernetes)
	}
}

func TestKubernetesRunner_Close(t *testing.T) {
	r := NewKubernetesRunner()
	if err := r.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
}
