// Package diagnostics provides enterprise support bundle generation
// for collecting system health, configuration, and runtime information.
package diagnostics

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/rjsadow/launchpad/internal/config"
	"github.com/rjsadow/launchpad/internal/db"
	"github.com/rjsadow/launchpad/internal/plugins"
)

// Collector gathers diagnostic information from the system.
type Collector struct {
	db       *db.DB
	config   *config.Config
	registry *plugins.Registry
	started  time.Time
}

// NewCollector creates a new diagnostics collector.
func NewCollector(database *db.DB, cfg *config.Config, registry *plugins.Registry, started time.Time) *Collector {
	return &Collector{
		db:       database,
		config:   cfg,
		registry: registry,
		started:  started,
	}
}

// Bundle represents a complete diagnostics bundle.
type Bundle struct {
	GeneratedAt time.Time       `json:"generated_at"`
	System      SystemInfo      `json:"system"`
	Config      RedactedConfig  `json:"config"`
	Health      HealthSummary   `json:"health"`
	Database    DatabaseStats   `json:"database"`
	Sessions    SessionStats    `json:"sessions"`
	Runtime     RuntimeInfo     `json:"runtime"`
}

// SystemInfo contains basic system information.
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	NumCPU       int    `json:"num_cpu"`
	Hostname     string `json:"hostname"`
	Uptime       string `json:"uptime"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// RedactedConfig contains configuration with secrets removed.
type RedactedConfig struct {
	Port               int    `json:"port"`
	DB                 string `json:"db"`
	Namespace          string `json:"namespace"`
	SessionTimeout     string `json:"session_timeout"`
	CleanupInterval    string `json:"cleanup_interval"`
	PodReadyTimeout    string `json:"pod_ready_timeout"`
	AuthEnabled        bool   `json:"auth_enabled"`
	OIDCEnabled        bool   `json:"oidc_enabled"`
	RegistrationAllowed bool  `json:"registration_allowed"`
	MaxUploadSize      int64  `json:"max_upload_size"`
	GatewayRateLimit   float64 `json:"gateway_rate_limit"`
	GatewayBurst       int    `json:"gateway_burst"`
	MaxSessionsPerUser int    `json:"max_sessions_per_user"`
	MaxGlobalSessions  int    `json:"max_global_sessions"`
	DefaultCPURequest  string `json:"default_cpu_request"`
	DefaultCPULimit    string `json:"default_cpu_limit"`
	DefaultMemRequest  string `json:"default_mem_request"`
	DefaultMemLimit    string `json:"default_mem_limit"`
	RecordingEnabled   bool   `json:"recording_enabled"`
}

// HealthSummary contains the overall health status.
type HealthSummary struct {
	Overall       string                `json:"overall"`
	Database      ComponentHealth       `json:"database"`
	Plugins       []plugins.HealthStatus `json:"plugins"`
}

// ComponentHealth represents health of a single component.
type ComponentHealth struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

// DatabaseStats contains database statistics.
type DatabaseStats struct {
	AppCount       int `json:"app_count"`
	UserCount      int `json:"user_count"`
	ActiveSessions int `json:"active_sessions"`
	TemplateCount  int `json:"template_count"`
}

// SessionStats contains session statistics.
type SessionStats struct {
	ActiveSessions int `json:"active_sessions"`
}

// RuntimeInfo contains Go runtime information.
type RuntimeInfo struct {
	NumGoroutine int          `json:"num_goroutine"`
	Memory       MemoryStats  `json:"memory"`
}

// MemoryStats contains memory statistics.
type MemoryStats struct {
	AllocMB      float64 `json:"alloc_mb"`
	TotalAllocMB float64 `json:"total_alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	NumGC        uint32  `json:"num_gc"`
}

// Collect gathers all diagnostic information into a Bundle.
func (c *Collector) Collect(ctx context.Context) (*Bundle, error) {
	bundle := &Bundle{
		GeneratedAt: time.Now().UTC(),
	}

	bundle.System = c.collectSystemInfo()
	bundle.Config = c.collectRedactedConfig()
	bundle.Health = c.collectHealth(ctx)
	bundle.Database = c.collectDatabaseStats()
	bundle.Sessions = c.collectSessionStats()
	bundle.Runtime = c.collectRuntimeInfo()

	return bundle, nil
}

// WriteTarGz writes the diagnostics bundle as a tar.gz archive to the given writer.
func (c *Collector) WriteTarGz(ctx context.Context, w io.Writer) error {
	bundle, err := c.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collecting diagnostics: %w", err)
	}

	gzw := gzip.NewWriter(w)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Write bundle.json
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling bundle: %w", err)
	}

	if err := addFileToTar(tw, "diagnostics/bundle.json", bundleJSON); err != nil {
		return fmt.Errorf("adding bundle.json to archive: %w", err)
	}

	// Write individual sections for easier parsing
	sections := map[string]any{
		"diagnostics/system.json":   bundle.System,
		"diagnostics/config.json":   bundle.Config,
		"diagnostics/health.json":   bundle.Health,
		"diagnostics/database.json": bundle.Database,
		"diagnostics/sessions.json": bundle.Sessions,
		"diagnostics/runtime.json":  bundle.Runtime,
	}

	for name, data := range sections {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling %s: %w", name, err)
		}
		if err := addFileToTar(tw, name, jsonData); err != nil {
			return fmt.Errorf("adding %s to archive: %w", name, err)
		}
	}

	return nil
}

func addFileToTar(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err := tw.Write(data)
	return err
}

func (c *Collector) collectSystemInfo() SystemInfo {
	hostname, _ := os.Hostname()
	uptime := time.Since(c.started)

	return SystemInfo{
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		Hostname:      hostname,
		Uptime:        uptime.Round(time.Second).String(),
		UptimeSeconds: uptime.Seconds(),
	}
}

func (c *Collector) collectRedactedConfig() RedactedConfig {
	return RedactedConfig{
		Port:                c.config.Port,
		DB:                  c.config.DB,
		Namespace:           c.config.Namespace,
		SessionTimeout:      c.config.SessionTimeout.String(),
		CleanupInterval:     c.config.SessionCleanupInterval.String(),
		PodReadyTimeout:     c.config.PodReadyTimeout.String(),
		AuthEnabled:         c.config.JWTSecret != "",
		OIDCEnabled:         c.config.OIDCEnabled(),
		RegistrationAllowed: c.config.AllowRegistration,
		MaxUploadSize:       c.config.MaxUploadSize,
		GatewayRateLimit:    c.config.GatewayRateLimit,
		GatewayBurst:        c.config.GatewayBurst,
		MaxSessionsPerUser:  c.config.MaxSessionsPerUser,
		MaxGlobalSessions:   c.config.MaxGlobalSessions,
		DefaultCPURequest:   c.config.DefaultCPURequest,
		DefaultCPULimit:     c.config.DefaultCPULimit,
		DefaultMemRequest:   c.config.DefaultMemRequest,
		DefaultMemLimit:     c.config.DefaultMemLimit,
		RecordingEnabled:    c.config.RecordingEnabled,
	}
}

func (c *Collector) collectHealth(ctx context.Context) HealthSummary {
	summary := HealthSummary{
		Overall: "healthy",
	}

	// Check database
	if err := c.db.Ping(); err != nil {
		summary.Database = ComponentHealth{Healthy: false, Message: err.Error()}
		summary.Overall = "degraded"
	} else {
		summary.Database = ComponentHealth{Healthy: true, Message: "OK"}
	}

	// Check plugins
	summary.Plugins = c.registry.HealthCheck(ctx)
	for _, ps := range summary.Plugins {
		if !ps.Healthy {
			summary.Overall = "degraded"
		}
	}

	return summary
}

func (c *Collector) collectDatabaseStats() DatabaseStats {
	stats := DatabaseStats{}

	apps, err := c.db.ListApps()
	if err == nil {
		stats.AppCount = len(apps)
	}

	users, err := c.db.ListUsers()
	if err == nil {
		stats.UserCount = len(users)
	}

	count, err := c.db.CountActiveSessions()
	if err == nil {
		stats.ActiveSessions = count
	}

	templates, err := c.db.ListTemplates()
	if err == nil {
		stats.TemplateCount = len(templates)
	}

	return stats
}

func (c *Collector) collectSessionStats() SessionStats {
	count, err := c.db.CountActiveSessions()
	if err != nil {
		return SessionStats{}
	}
	return SessionStats{
		ActiveSessions: count,
	}
}

func (c *Collector) collectRuntimeInfo() RuntimeInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return RuntimeInfo{
		NumGoroutine: runtime.NumGoroutine(),
		Memory: MemoryStats{
			AllocMB:      float64(memStats.Alloc) / 1024 / 1024,
			TotalAllocMB: float64(memStats.TotalAlloc) / 1024 / 1024,
			SysMB:        float64(memStats.Sys) / 1024 / 1024,
			NumGC:        memStats.NumGC,
		},
	}
}
