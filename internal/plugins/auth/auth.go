// Package auth provides AuthProvider plugin implementations for authentication.
//
// Built-in providers:
//   - noop: No authentication (default, for development)
//   - oidc: OpenID Connect authentication (placeholder for future)
//
// To add a new auth provider:
//  1. Create a new file implementing AuthProvider
//  2. Register it in init() using plugins.RegisterGlobal()
//  3. Configure via LAUNCHPAD_PLUGIN_AUTH environment variable
package auth

import (
	"github.com/rjsadow/launchpad/internal/plugins"
)

// Re-export types for convenience
type (
	User       = plugins.User
	AuthResult = plugins.AuthResult
)
