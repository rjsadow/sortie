package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringSlice is a []string that serializes to/from JSON in the database.
// Used for columns like container_args, roles, tags, and tenant_roles.
type StringSlice []string

// Value implements driver.Valuer for database storage.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal StringSlice: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("cannot scan %T into StringSlice", src)
	}

	if len(data) == 0 || string(data) == "[]" {
		*s = nil
		return nil
	}

	return json.Unmarshal(data, s)
}

// EgressPolicyValue is an *EgressPolicy that serializes to/from JSON in the database.
type EgressPolicyValue struct {
	*EgressPolicy
}

// Value implements driver.Valuer for database storage.
func (e EgressPolicyValue) Value() (driver.Value, error) {
	if e.EgressPolicy == nil || e.Mode == "" {
		return "", nil
	}
	data, err := json.Marshal(e.EgressPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EgressPolicy: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (e *EgressPolicyValue) Scan(src any) error {
	if src == nil {
		e.EgressPolicy = nil
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		if v == "" {
			e.EgressPolicy = nil
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			e.EgressPolicy = nil
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into EgressPolicyValue", src)
	}

	var ep EgressPolicy
	if err := json.Unmarshal(data, &ep); err != nil {
		return err
	}
	if ep.Mode == "" {
		e.EgressPolicy = nil
		return nil
	}
	e.EgressPolicy = &ep
	return nil
}

// TenantSettingsValue is a TenantSettings that serializes to/from JSON in the database.
type TenantSettingsValue struct {
	TenantSettings
}

// Value implements driver.Valuer for database storage.
func (t TenantSettingsValue) Value() (driver.Value, error) {
	data, err := json.Marshal(t.TenantSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TenantSettings: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (t *TenantSettingsValue) Scan(src any) error {
	if src == nil {
		t.TenantSettings = TenantSettings{}
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		if v == "" || v == "{}" {
			t.TenantSettings = TenantSettings{}
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			t.TenantSettings = TenantSettings{}
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into TenantSettingsValue", src)
	}

	return json.Unmarshal(data, &t.TenantSettings)
}

// TenantQuotasValue is a TenantQuotas that serializes to/from JSON in the database.
type TenantQuotasValue struct {
	TenantQuotas
}

// Value implements driver.Valuer for database storage.
func (t TenantQuotasValue) Value() (driver.Value, error) {
	data, err := json.Marshal(t.TenantQuotas)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TenantQuotas: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (t *TenantQuotasValue) Scan(src any) error {
	if src == nil {
		t.TenantQuotas = TenantQuotas{}
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		if v == "" || v == "{}" {
			t.TenantQuotas = TenantQuotas{}
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			t.TenantQuotas = TenantQuotas{}
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into TenantQuotasValue", src)
	}

	return json.Unmarshal(data, &t.TenantQuotas)
}

// EnvVarSlice is a []EnvVar that serializes to/from JSON in the database.
type EnvVarSlice []EnvVar

// Value implements driver.Valuer for database storage.
func (e EnvVarSlice) Value() (driver.Value, error) {
	if e == nil {
		return "[]", nil
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EnvVarSlice: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (e *EnvVarSlice) Scan(src any) error {
	if src == nil {
		*e = nil
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("cannot scan %T into EnvVarSlice", src)
	}

	if len(data) == 0 || string(data) == "[]" {
		*e = nil
		return nil
	}

	return json.Unmarshal(data, e)
}

// VolumeMountSlice is a []VolumeMount that serializes to/from JSON in the database.
type VolumeMountSlice []VolumeMount

// Value implements driver.Valuer for database storage.
func (v VolumeMountSlice) Value() (driver.Value, error) {
	if v == nil {
		return "[]", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal VolumeMountSlice: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (v *VolumeMountSlice) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}

	var data []byte
	switch val := src.(type) {
	case string:
		data = []byte(val)
	case []byte:
		data = val
	default:
		return fmt.Errorf("cannot scan %T into VolumeMountSlice", src)
	}

	if len(data) == 0 || string(data) == "[]" {
		*v = nil
		return nil
	}

	return json.Unmarshal(data, v)
}

// NetworkRuleSlice is a []NetworkRule that serializes to/from JSON in the database.
type NetworkRuleSlice []NetworkRule

// Value implements driver.Valuer for database storage.
func (n NetworkRuleSlice) Value() (driver.Value, error) {
	if n == nil {
		return "[]", nil
	}
	data, err := json.Marshal(n)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal NetworkRuleSlice: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (n *NetworkRuleSlice) Scan(src any) error {
	if src == nil {
		*n = nil
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("cannot scan %T into NetworkRuleSlice", src)
	}

	if len(data) == 0 || string(data) == "[]" {
		*n = nil
		return nil
	}

	return json.Unmarshal(data, n)
}

// ResourceLimitsValue is a *ResourceLimits that serializes to/from JSON in the database.
type ResourceLimitsValue struct {
	*ResourceLimits
}

// Value implements driver.Valuer for database storage.
func (r ResourceLimitsValue) Value() (driver.Value, error) {
	if r.ResourceLimits == nil {
		return "{}", nil
	}
	data, err := json.Marshal(r.ResourceLimits)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ResourceLimits: %w", err)
	}
	return string(data), nil
}

// Scan implements sql.Scanner for database retrieval.
func (r *ResourceLimitsValue) Scan(src any) error {
	if src == nil {
		r.ResourceLimits = nil
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case string:
		if v == "" || v == "{}" {
			r.ResourceLimits = nil
			return nil
		}
		data = []byte(v)
	case []byte:
		if len(v) == 0 {
			r.ResourceLimits = nil
			return nil
		}
		data = v
	default:
		return fmt.Errorf("cannot scan %T into ResourceLimitsValue", src)
	}

	var rl ResourceLimits
	if err := json.Unmarshal(data, &rl); err != nil {
		return err
	}
	if rl.CPURequest == "" && rl.CPULimit == "" && rl.MemoryRequest == "" && rl.MemoryLimit == "" {
		r.ResourceLimits = nil
		return nil
	}
	r.ResourceLimits = &rl
	return nil
}
