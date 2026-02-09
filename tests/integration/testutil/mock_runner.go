package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rjsadow/sortie/internal/db"
	"github.com/rjsadow/sortie/internal/runner"
)

// MockWorkload tracks a workload created by MockRunner.
type MockWorkload struct {
	Name   string
	Config *runner.WorkloadConfig
	IP     string
	Ready  bool
}

// MockRunner implements runner.Runner and runner.NetworkPolicyRunner for tests.
// It stores workloads in-memory and supports failure injection.
type MockRunner struct {
	mu        sync.Mutex
	workloads map[string]*MockWorkload
	ipCounter int

	// Error injection: set these to non-nil to simulate failures.
	CreateError error
	ReadyError  error
	DeleteError error

	// ReadyDelay adds a delay before WaitForReady returns. Default 0 for fast tests.
	ReadyDelay time.Duration
}

// NewMockRunner creates a new MockRunner.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		workloads: make(map[string]*MockWorkload),
	}
}

func (m *MockRunner) Type() runner.Type {
	return "mock"
}

func (m *MockRunner) CreateWorkload(_ context.Context, config *runner.WorkloadConfig) (*runner.WorkloadResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.CreateError != nil {
		return nil, m.CreateError
	}

	name := fmt.Sprintf("session-%s", config.SessionID)
	m.ipCounter++
	ip := fmt.Sprintf("10.0.0.%d", m.ipCounter)

	m.workloads[name] = &MockWorkload{
		Name:   name,
		Config: config,
		IP:     ip,
		Ready:  true,
	}

	return &runner.WorkloadResult{Name: name}, nil
}

func (m *MockRunner) DeleteWorkload(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.DeleteError != nil {
		return m.DeleteError
	}

	delete(m.workloads, name)
	return nil
}

func (m *MockRunner) WaitForReady(ctx context.Context, name string, _ time.Duration) error {
	if m.ReadyDelay > 0 {
		select {
		case <-time.After(m.ReadyDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ReadyError != nil {
		return m.ReadyError
	}

	if w, ok := m.workloads[name]; ok {
		w.Ready = true
	}
	return nil
}

func (m *MockRunner) GetIP(_ context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.workloads[name]
	if !ok {
		return "", fmt.Errorf("workload %s not found", name)
	}
	return w.IP, nil
}

func (m *MockRunner) ListWorkloads(_ context.Context) ([]runner.WorkloadInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]runner.WorkloadInfo, 0, len(m.workloads))
	for _, w := range m.workloads {
		result = append(result, runner.WorkloadInfo{
			Name:  w.Name,
			IP:    w.IP,
			Ready: w.Ready,
		})
	}
	return result, nil
}

func (m *MockRunner) Healthy(_ context.Context) bool {
	return true
}

func (m *MockRunner) Close() error {
	return nil
}

// NetworkPolicyRunner implementation

func (m *MockRunner) CreateNetworkPolicy(_ context.Context, _, _ string, _ *db.EgressPolicy) error {
	return nil
}

func (m *MockRunner) DeleteNetworkPolicy(_ context.Context, _ string) error {
	return nil
}

// WorkloadCount returns the number of active workloads.
func (m *MockRunner) WorkloadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.workloads)
}

// Compile-time interface checks.
var _ runner.Runner = (*MockRunner)(nil)
var _ runner.NetworkPolicyRunner = (*MockRunner)(nil)
