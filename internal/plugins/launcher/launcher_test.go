package launcher

import (
	"testing"

	"github.com/rjsadow/launchpad/internal/plugins"
)

func TestReExportedConstants(t *testing.T) {
	// Verify re-exported launch type constants match the originals
	if LaunchTypeURL != plugins.LaunchTypeURL {
		t.Errorf("LaunchTypeURL = %q, want %q", LaunchTypeURL, plugins.LaunchTypeURL)
	}
	if LaunchTypeContainer != plugins.LaunchTypeContainer {
		t.Errorf("LaunchTypeContainer = %q, want %q", LaunchTypeContainer, plugins.LaunchTypeContainer)
	}

	// Verify re-exported status constants match the originals
	if LaunchStatusPending != plugins.LaunchStatusPending {
		t.Errorf("LaunchStatusPending = %q, want %q", LaunchStatusPending, plugins.LaunchStatusPending)
	}
	if LaunchStatusCreating != plugins.LaunchStatusCreating {
		t.Errorf("LaunchStatusCreating = %q, want %q", LaunchStatusCreating, plugins.LaunchStatusCreating)
	}
	if LaunchStatusRunning != plugins.LaunchStatusRunning {
		t.Errorf("LaunchStatusRunning = %q, want %q", LaunchStatusRunning, plugins.LaunchStatusRunning)
	}
	if LaunchStatusFailed != plugins.LaunchStatusFailed {
		t.Errorf("LaunchStatusFailed = %q, want %q", LaunchStatusFailed, plugins.LaunchStatusFailed)
	}
	if LaunchStatusStopped != plugins.LaunchStatusStopped {
		t.Errorf("LaunchStatusStopped = %q, want %q", LaunchStatusStopped, plugins.LaunchStatusStopped)
	}
	if LaunchStatusExpired != plugins.LaunchStatusExpired {
		t.Errorf("LaunchStatusExpired = %q, want %q", LaunchStatusExpired, plugins.LaunchStatusExpired)
	}
	if LaunchStatusRedirect != plugins.LaunchStatusRedirect {
		t.Errorf("LaunchStatusRedirect = %q, want %q", LaunchStatusRedirect, plugins.LaunchStatusRedirect)
	}
}

func TestReExportedTypes(t *testing.T) {
	// Verify type aliases work correctly
	var lt LaunchType = "url"
	plt := lt
	if plt != plugins.LaunchTypeURL {
		t.Errorf("LaunchType alias mismatch: %q != %q", plt, plugins.LaunchTypeURL)
	}

	var ls LaunchStatus = "running"
	pls := ls
	if pls != plugins.LaunchStatusRunning {
		t.Errorf("LaunchStatus alias mismatch: %q != %q", pls, plugins.LaunchStatusRunning)
	}

	// Verify struct type aliases
	var _ = plugins.LaunchRequest{}
	var _ = plugins.LaunchResult{}
}
