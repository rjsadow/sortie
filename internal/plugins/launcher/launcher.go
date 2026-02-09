// Package launcher provides built-in LauncherPlugin implementations
// for launching applications in different ways.
//
// Built-in launchers:
//   - url: Opens applications via URL redirect (default)
//   - container: Launches applications in Kubernetes pods with VNC
//
// To add a new launcher:
//  1. Create a new file implementing LauncherPlugin
//  2. Register it in init() using plugins.RegisterGlobal()
//  3. Configure via SORTIE_PLUGIN_LAUNCHER environment variable
package launcher

import (
	"github.com/rjsadow/sortie/internal/plugins"
)

// Re-export types for convenience
type (
	LaunchType    = plugins.LaunchType
	LaunchRequest = plugins.LaunchRequest
	LaunchResult  = plugins.LaunchResult
	LaunchStatus  = plugins.LaunchStatus
)

// Re-export constants
const (
	LaunchTypeURL       = plugins.LaunchTypeURL
	LaunchTypeContainer = plugins.LaunchTypeContainer

	LaunchStatusPending  = plugins.LaunchStatusPending
	LaunchStatusCreating = plugins.LaunchStatusCreating
	LaunchStatusRunning  = plugins.LaunchStatusRunning
	LaunchStatusFailed   = plugins.LaunchStatusFailed
	LaunchStatusStopped  = plugins.LaunchStatusStopped
	LaunchStatusExpired  = plugins.LaunchStatusExpired
	LaunchStatusRedirect = plugins.LaunchStatusRedirect
)
