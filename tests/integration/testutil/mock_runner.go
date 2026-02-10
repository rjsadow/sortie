package testutil

import "github.com/rjsadow/sortie/internal/runner"

// MockRunner is a re-export of runner.MockRunner for backward compatibility.
type MockRunner = runner.MockRunner

// MockWorkload is a re-export of runner.MockWorkload for backward compatibility.
type MockWorkload = runner.MockWorkload

// NewMockRunner creates a new MockRunner.
func NewMockRunner() *MockRunner {
	return runner.NewMockRunner()
}
