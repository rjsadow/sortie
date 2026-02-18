package db

import (
	"context"
	"encoding/json"

	"github.com/uptrace/bun"
)

// --- Application hooks ---

var _ bun.BeforeAppendModelHook = (*Application)(nil)
var _ bun.AfterScanRowHook = (*Application)(nil)

func (a *Application) BeforeAppendModel(_ context.Context, query bun.Query) error {
	// Set defaults
	if a.Visibility == "" {
		a.Visibility = CategoryVisibilityPublic
	}
	if a.LaunchType == "" {
		a.LaunchType = LaunchTypeURL
	}
	if a.OsType == "" {
		a.OsType = "linux"
	}
	if a.TenantID == "" {
		a.TenantID = DefaultTenantID
	}

	// Marshal ContainerArgs → ContainerArgsJSON
	a.ContainerArgsJSON = "[]"
	if len(a.ContainerArgs) > 0 {
		if b, err := json.Marshal(a.ContainerArgs); err == nil {
			a.ContainerArgsJSON = string(b)
		}
	}

	// Flatten ResourceLimits → individual columns
	if a.ResourceLimits != nil {
		a.CPURequest = a.ResourceLimits.CPURequest
		a.CPULimit = a.ResourceLimits.CPULimit
		a.MemoryRequest = a.ResourceLimits.MemoryRequest
		a.MemoryLimit = a.ResourceLimits.MemoryLimit
	} else {
		a.CPURequest = ""
		a.CPULimit = ""
		a.MemoryRequest = ""
		a.MemoryLimit = ""
	}

	// Marshal EgressPolicy → EgressPolicyJSON
	a.EgressPolicyJSON = ""
	if a.EgressPolicy != nil && a.EgressPolicy.Mode != "" {
		if b, err := json.Marshal(a.EgressPolicy); err == nil {
			a.EgressPolicyJSON = string(b)
		}
	}

	return nil
}

func (a *Application) AfterScanRow(_ context.Context) error {
	// Set defaults for empty values
	if a.Visibility == "" {
		a.Visibility = CategoryVisibilityPublic
	}
	if a.LaunchType == "" {
		a.LaunchType = LaunchTypeURL
	}
	if a.OsType == "" {
		a.OsType = "linux"
	}

	// Unmarshal ContainerArgsJSON → ContainerArgs
	a.ContainerArgs = nil
	if a.ContainerArgsJSON != "" && a.ContainerArgsJSON != "[]" {
		json.Unmarshal([]byte(a.ContainerArgsJSON), &a.ContainerArgs)
	}

	// Reconstruct ResourceLimits from individual columns
	if a.CPURequest != "" || a.CPULimit != "" || a.MemoryRequest != "" || a.MemoryLimit != "" {
		a.ResourceLimits = &ResourceLimits{
			CPURequest:    a.CPURequest,
			CPULimit:      a.CPULimit,
			MemoryRequest: a.MemoryRequest,
			MemoryLimit:   a.MemoryLimit,
		}
	} else {
		a.ResourceLimits = nil
	}

	// Unmarshal EgressPolicyJSON → EgressPolicy
	a.EgressPolicy = nil
	if a.EgressPolicyJSON != "" {
		var ep EgressPolicy
		if json.Unmarshal([]byte(a.EgressPolicyJSON), &ep) == nil && ep.Mode != "" {
			a.EgressPolicy = &ep
		}
	}

	return nil
}

// --- Template hooks ---

var _ bun.BeforeAppendModelHook = (*Template)(nil)
var _ bun.AfterScanRowHook = (*Template)(nil)

func (t *Template) BeforeAppendModel(_ context.Context, query bun.Query) error {
	// Set defaults
	if t.OsType == "" {
		t.OsType = "linux"
	}

	// Marshal ContainerArgs → ContainerArgsJSON
	t.ContainerArgsJSON = "[]"
	if len(t.ContainerArgs) > 0 {
		if b, err := json.Marshal(t.ContainerArgs); err == nil {
			t.ContainerArgsJSON = string(b)
		}
	}

	// Marshal Tags → TagsJSON
	t.TagsJSON = "[]"
	if len(t.Tags) > 0 {
		if b, err := json.Marshal(t.Tags); err == nil {
			t.TagsJSON = string(b)
		}
	}

	// Flatten RecommendedLimits → individual columns
	if t.RecommendedLimits != nil {
		t.CPURequest = t.RecommendedLimits.CPURequest
		t.CPULimit = t.RecommendedLimits.CPULimit
		t.MemoryRequest = t.RecommendedLimits.MemoryRequest
		t.MemoryLimit = t.RecommendedLimits.MemoryLimit
	} else {
		t.CPURequest = ""
		t.CPULimit = ""
		t.MemoryRequest = ""
		t.MemoryLimit = ""
	}

	return nil
}

func (t *Template) AfterScanRow(_ context.Context) error {
	// Set defaults
	if t.OsType == "" {
		t.OsType = "linux"
	}

	// Unmarshal ContainerArgsJSON → ContainerArgs
	t.ContainerArgs = nil
	if t.ContainerArgsJSON != "" && t.ContainerArgsJSON != "[]" {
		json.Unmarshal([]byte(t.ContainerArgsJSON), &t.ContainerArgs)
	}

	// Unmarshal TagsJSON → Tags
	t.Tags = nil
	if t.TagsJSON != "" && t.TagsJSON != "[]" {
		json.Unmarshal([]byte(t.TagsJSON), &t.Tags)
	}

	// Reconstruct RecommendedLimits from individual columns
	if t.CPURequest != "" || t.CPULimit != "" || t.MemoryRequest != "" || t.MemoryLimit != "" {
		t.RecommendedLimits = &ResourceLimits{
			CPURequest:    t.CPURequest,
			CPULimit:      t.CPULimit,
			MemoryRequest: t.MemoryRequest,
			MemoryLimit:   t.MemoryLimit,
		}
	} else {
		t.RecommendedLimits = nil
	}

	return nil
}

// --- User hooks ---

var _ bun.BeforeAppendModelHook = (*User)(nil)
var _ bun.AfterScanRowHook = (*User)(nil)

func (u *User) BeforeAppendModel(_ context.Context, query bun.Query) error {
	// Set defaults
	if u.AuthProvider == "" {
		u.AuthProvider = "local"
	}
	if u.TenantID == "" {
		u.TenantID = DefaultTenantID
	}

	// Marshal Roles → RolesJSON
	if len(u.Roles) > 0 {
		if b, err := json.Marshal(u.Roles); err == nil {
			u.RolesJSON = string(b)
		}
	} else {
		u.RolesJSON = `["user"]`
	}

	// Marshal TenantRoles → TenantRolesJSON
	u.TenantRolesJSON = "[]"
	if len(u.TenantRoles) > 0 {
		if b, err := json.Marshal(u.TenantRoles); err == nil {
			u.TenantRolesJSON = string(b)
		}
	}

	return nil
}

func (u *User) AfterScanRow(_ context.Context) error {
	// Unmarshal RolesJSON → Roles
	u.Roles = nil
	if u.RolesJSON != "" {
		if err := json.Unmarshal([]byte(u.RolesJSON), &u.Roles); err != nil {
			u.Roles = []string{"user"}
		}
	} else {
		u.Roles = []string{"user"}
	}

	// Unmarshal TenantRolesJSON → TenantRoles
	u.TenantRoles = nil
	if u.TenantRolesJSON != "" && u.TenantRolesJSON != "[]" {
		json.Unmarshal([]byte(u.TenantRolesJSON), &u.TenantRoles)
	}

	return nil
}

// --- Tenant hooks ---

var _ bun.BeforeAppendModelHook = (*Tenant)(nil)
var _ bun.AfterScanRowHook = (*Tenant)(nil)

func (t *Tenant) BeforeAppendModel(_ context.Context, query bun.Query) error {
	// Marshal Settings → SettingsJSON
	if b, err := json.Marshal(t.Settings); err == nil {
		t.SettingsJSON = string(b)
	}

	// Marshal Quotas → QuotasJSON
	if b, err := json.Marshal(t.Quotas); err == nil {
		t.QuotasJSON = string(b)
	}

	return nil
}

func (t *Tenant) AfterScanRow(_ context.Context) error {
	// Unmarshal SettingsJSON → Settings
	t.Settings = TenantSettings{}
	if t.SettingsJSON != "" && t.SettingsJSON != "{}" {
		json.Unmarshal([]byte(t.SettingsJSON), &t.Settings)
	}

	// Unmarshal QuotasJSON → Quotas
	t.Quotas = TenantQuotas{}
	if t.QuotasJSON != "" && t.QuotasJSON != "{}" {
		json.Unmarshal([]byte(t.QuotasJSON), &t.Quotas)
	}

	return nil
}

// --- AppSpec hooks ---

var _ bun.BeforeAppendModelHook = (*AppSpec)(nil)
var _ bun.AfterScanRowHook = (*AppSpec)(nil)

func (s *AppSpec) BeforeAppendModel(_ context.Context, query bun.Query) error {
	// Marshal EnvVars → EnvVarsJSON
	s.EnvVarsJSON = "[]"
	if len(s.EnvVars) > 0 {
		if b, err := json.Marshal(s.EnvVars); err == nil {
			s.EnvVarsJSON = string(b)
		}
	}

	// Marshal Volumes → VolumesJSON
	s.VolumesJSON = "[]"
	if len(s.Volumes) > 0 {
		if b, err := json.Marshal(s.Volumes); err == nil {
			s.VolumesJSON = string(b)
		}
	}

	// Marshal NetworkRules → NetworkRulesJSON
	s.NetworkRulesJSON = "[]"
	if len(s.NetworkRules) > 0 {
		if b, err := json.Marshal(s.NetworkRules); err == nil {
			s.NetworkRulesJSON = string(b)
		}
	}

	// Marshal EgressPolicy → EgressPolicyJSON
	s.EgressPolicyJSON = ""
	if s.EgressPolicy != nil && s.EgressPolicy.Mode != "" {
		if b, err := json.Marshal(s.EgressPolicy); err == nil {
			s.EgressPolicyJSON = string(b)
		}
	}

	// Flatten Resources → individual columns
	if s.Resources != nil {
		s.CPURequest = s.Resources.CPURequest
		s.CPULimit = s.Resources.CPULimit
		s.MemoryRequest = s.Resources.MemoryRequest
		s.MemoryLimit = s.Resources.MemoryLimit
	} else {
		s.CPURequest = ""
		s.CPULimit = ""
		s.MemoryRequest = ""
		s.MemoryLimit = ""
	}

	return nil
}

func (s *AppSpec) AfterScanRow(_ context.Context) error {
	// Unmarshal EnvVarsJSON → EnvVars
	s.EnvVars = nil
	if s.EnvVarsJSON != "" && s.EnvVarsJSON != "[]" {
		json.Unmarshal([]byte(s.EnvVarsJSON), &s.EnvVars)
	}

	// Unmarshal VolumesJSON → Volumes
	s.Volumes = nil
	if s.VolumesJSON != "" && s.VolumesJSON != "[]" {
		json.Unmarshal([]byte(s.VolumesJSON), &s.Volumes)
	}

	// Unmarshal NetworkRulesJSON → NetworkRules
	s.NetworkRules = nil
	if s.NetworkRulesJSON != "" && s.NetworkRulesJSON != "[]" {
		json.Unmarshal([]byte(s.NetworkRulesJSON), &s.NetworkRules)
	}

	// Unmarshal EgressPolicyJSON → EgressPolicy
	s.EgressPolicy = nil
	if s.EgressPolicyJSON != "" {
		var ep EgressPolicy
		if json.Unmarshal([]byte(s.EgressPolicyJSON), &ep) == nil && ep.Mode != "" {
			s.EgressPolicy = &ep
		}
	}

	// Reconstruct Resources from individual columns
	if s.CPURequest != "" || s.CPULimit != "" || s.MemoryRequest != "" || s.MemoryLimit != "" {
		s.Resources = &ResourceLimits{
			CPURequest:    s.CPURequest,
			CPULimit:      s.CPULimit,
			MemoryRequest: s.MemoryRequest,
			MemoryLimit:   s.MemoryLimit,
		}
	} else {
		s.Resources = nil
	}

	return nil
}
