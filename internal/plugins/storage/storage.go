// Package storage provides StorageProvider plugin implementations for data persistence.
//
// Built-in providers:
//   - sqlite: SQLite database storage (default)
//   - memory: In-memory storage (for testing)
//
// To add a new storage provider:
//  1. Create a new file implementing StorageProvider
//  2. Register it in init() using plugins.RegisterGlobal()
//  3. Configure via LAUNCHPAD_PLUGIN_STORAGE environment variable
package storage

import (
	"github.com/rjsadow/launchpad/internal/plugins"
)

// Re-export types for convenience
type (
	Application = plugins.Application
	Session     = plugins.Session
	AuditEntry  = plugins.AuditEntry
)
