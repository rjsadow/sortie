package db

import (
	"testing"
)

// --- StringSlice ---

func TestStringSlice_Value_Nil(t *testing.T) {
	var s StringSlice
	v, err := s.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "[]" {
		t.Errorf("expected '[]', got %q", v)
	}
}

func TestStringSlice_Value_Empty(t *testing.T) {
	s := StringSlice{}
	v, err := s.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "[]" {
		t.Errorf("expected '[]', got %q", v)
	}
}

func TestStringSlice_Value_WithItems(t *testing.T) {
	s := StringSlice{"a", "b", "c"}
	v, err := s.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != `["a","b","c"]` {
		t.Errorf("expected '[\"a\",\"b\",\"c\"]', got %q", v)
	}
}

func TestStringSlice_Scan_Nil(t *testing.T) {
	var s StringSlice
	if err := s.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Errorf("expected nil, got %v", s)
	}
}

func TestStringSlice_Scan_EmptyString(t *testing.T) {
	var s StringSlice
	if err := s.Scan(""); err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Errorf("expected nil, got %v", s)
	}
}

func TestStringSlice_Scan_EmptyArray(t *testing.T) {
	var s StringSlice
	if err := s.Scan("[]"); err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Errorf("expected nil, got %v", s)
	}
}

func TestStringSlice_Scan_String(t *testing.T) {
	var s StringSlice
	if err := s.Scan(`["x","y"]`); err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 || s[0] != "x" || s[1] != "y" {
		t.Errorf("expected [x y], got %v", s)
	}
}

func TestStringSlice_Scan_Bytes(t *testing.T) {
	var s StringSlice
	if err := s.Scan([]byte(`["z"]`)); err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0] != "z" {
		t.Errorf("expected [z], got %v", s)
	}
}

func TestStringSlice_Scan_InvalidType(t *testing.T) {
	var s StringSlice
	if err := s.Scan(42); err == nil {
		t.Error("expected error for int type, got nil")
	}
}

func TestStringSlice_Scan_InvalidJSON(t *testing.T) {
	var s StringSlice
	if err := s.Scan("not json"); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestStringSlice_Roundtrip(t *testing.T) {
	original := StringSlice{"hello", "world"}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored StringSlice
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}

	if len(restored) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(restored), len(original))
	}
	for i := range original {
		if restored[i] != original[i] {
			t.Errorf("index %d: %q != %q", i, restored[i], original[i])
		}
	}
}

// --- EgressPolicyValue ---

func TestEgressPolicyValue_Value_Nil(t *testing.T) {
	e := EgressPolicyValue{}
	v, err := e.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Errorf("expected empty string, got %q", v)
	}
}

func TestEgressPolicyValue_Value_EmptyMode(t *testing.T) {
	e := EgressPolicyValue{EgressPolicy: &EgressPolicy{}}
	v, err := e.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Errorf("expected empty string for empty mode, got %q", v)
	}
}

func TestEgressPolicyValue_Value_WithData(t *testing.T) {
	e := EgressPolicyValue{EgressPolicy: &EgressPolicy{
		Mode: "allowlist",
		Rules: []EgressRule{
			{CIDR: "10.0.0.0/8", Port: 443, Protocol: "TCP"},
		},
	}}
	v, err := e.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T", v)
	}
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func TestEgressPolicyValue_Scan_Nil(t *testing.T) {
	e := EgressPolicyValue{EgressPolicy: &EgressPolicy{Mode: "old"}}
	if err := e.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if e.EgressPolicy != nil {
		t.Error("expected nil after scanning nil")
	}
}

func TestEgressPolicyValue_Scan_EmptyString(t *testing.T) {
	e := EgressPolicyValue{}
	if err := e.Scan(""); err != nil {
		t.Fatal(err)
	}
	if e.EgressPolicy != nil {
		t.Error("expected nil for empty string")
	}
}

func TestEgressPolicyValue_Scan_EmptyBytes(t *testing.T) {
	e := EgressPolicyValue{}
	if err := e.Scan([]byte{}); err != nil {
		t.Fatal(err)
	}
	if e.EgressPolicy != nil {
		t.Error("expected nil for empty bytes")
	}
}

func TestEgressPolicyValue_Scan_NoMode(t *testing.T) {
	e := EgressPolicyValue{}
	if err := e.Scan(`{"rules":[]}`); err != nil {
		t.Fatal(err)
	}
	if e.EgressPolicy != nil {
		t.Error("expected nil when mode is empty")
	}
}

func TestEgressPolicyValue_Scan_Valid(t *testing.T) {
	e := EgressPolicyValue{}
	if err := e.Scan(`{"mode":"denylist","rules":[{"cidr":"0.0.0.0/0"}]}`); err != nil {
		t.Fatal(err)
	}
	if e.EgressPolicy == nil {
		t.Fatal("expected non-nil EgressPolicy")
	}
	if e.Mode != "denylist" {
		t.Errorf("expected mode 'denylist', got %q", e.Mode)
	}
	if len(e.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(e.Rules))
	}
}

func TestEgressPolicyValue_Scan_InvalidType(t *testing.T) {
	e := EgressPolicyValue{}
	if err := e.Scan(123); err == nil {
		t.Error("expected error for int type")
	}
}

func TestEgressPolicyValue_Roundtrip(t *testing.T) {
	original := EgressPolicyValue{EgressPolicy: &EgressPolicy{
		Mode:  "allowlist",
		Rules: []EgressRule{{CIDR: "10.0.0.0/8", Port: 80}},
	}}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored EgressPolicyValue
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if restored.EgressPolicy == nil || restored.Mode != "allowlist" || len(restored.Rules) != 1 {
		t.Errorf("roundtrip mismatch: %+v", restored)
	}
}

// --- TenantSettingsValue ---

func TestTenantSettingsValue_Value(t *testing.T) {
	ts := TenantSettingsValue{TenantSettings{
		PrimaryColor: "#FF0000",
		DisplayName:  "Test Tenant",
	}}
	v, err := ts.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		t.Error("expected non-empty string")
	}
}

func TestTenantSettingsValue_Value_Empty(t *testing.T) {
	ts := TenantSettingsValue{}
	v, err := ts.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Error("expected non-nil value")
	}
}

func TestTenantSettingsValue_Scan_Nil(t *testing.T) {
	ts := TenantSettingsValue{TenantSettings{PrimaryColor: "old"}}
	if err := ts.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if ts.PrimaryColor != "" {
		t.Errorf("expected empty, got %q", ts.PrimaryColor)
	}
}

func TestTenantSettingsValue_Scan_EmptyObject(t *testing.T) {
	ts := TenantSettingsValue{}
	if err := ts.Scan("{}"); err != nil {
		t.Fatal(err)
	}
	if ts.PrimaryColor != "" {
		t.Errorf("expected empty after scanning '{}'")
	}
}

func TestTenantSettingsValue_Scan_EmptyString(t *testing.T) {
	ts := TenantSettingsValue{}
	if err := ts.Scan(""); err != nil {
		t.Fatal(err)
	}
	if ts.PrimaryColor != "" {
		t.Error("expected empty settings for empty string")
	}
}

func TestTenantSettingsValue_Scan_EmptyBytes(t *testing.T) {
	ts := TenantSettingsValue{}
	if err := ts.Scan([]byte{}); err != nil {
		t.Fatal(err)
	}
	if ts.PrimaryColor != "" {
		t.Error("expected empty settings for empty bytes")
	}
}

func TestTenantSettingsValue_Scan_Valid(t *testing.T) {
	ts := TenantSettingsValue{}
	if err := ts.Scan(`{"primary_color":"#00FF00","logo_url":"https://x.com/logo.png"}`); err != nil {
		t.Fatal(err)
	}
	if ts.PrimaryColor != "#00FF00" {
		t.Errorf("expected '#00FF00', got %q", ts.PrimaryColor)
	}
	if ts.LogoURL != "https://x.com/logo.png" {
		t.Errorf("expected logo URL, got %q", ts.LogoURL)
	}
}

func TestTenantSettingsValue_Scan_InvalidType(t *testing.T) {
	ts := TenantSettingsValue{}
	if err := ts.Scan(3.14); err == nil {
		t.Error("expected error for float type")
	}
}

func TestTenantSettingsValue_Roundtrip(t *testing.T) {
	original := TenantSettingsValue{TenantSettings{
		PrimaryColor:   "#123456",
		SecondaryColor: "#654321",
		LogoURL:        "https://example.com/logo.png",
		DisplayName:    "My Tenant",
	}}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored TenantSettingsValue
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if restored.PrimaryColor != original.PrimaryColor ||
		restored.SecondaryColor != original.SecondaryColor ||
		restored.LogoURL != original.LogoURL ||
		restored.DisplayName != original.DisplayName {
		t.Errorf("roundtrip mismatch: %+v vs %+v", restored, original)
	}
}

// --- TenantQuotasValue ---

func TestTenantQuotasValue_Value(t *testing.T) {
	tq := TenantQuotasValue{TenantQuotas{MaxUsers: 50, DefaultCPULimit: "2"}}
	v, err := tq.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v == nil || v == "" {
		t.Error("expected non-empty value")
	}
}

func TestTenantQuotasValue_Scan_Nil(t *testing.T) {
	tq := TenantQuotasValue{TenantQuotas{MaxUsers: 10}}
	if err := tq.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if tq.MaxUsers != 0 {
		t.Errorf("expected 0, got %d", tq.MaxUsers)
	}
}

func TestTenantQuotasValue_Scan_EmptyObject(t *testing.T) {
	tq := TenantQuotasValue{}
	if err := tq.Scan("{}"); err != nil {
		t.Fatal(err)
	}
	if tq.MaxUsers != 0 {
		t.Error("expected zero values for '{}'")
	}
}

func TestTenantQuotasValue_Scan_EmptyString(t *testing.T) {
	tq := TenantQuotasValue{}
	if err := tq.Scan(""); err != nil {
		t.Fatal(err)
	}
	if tq.MaxUsers != 0 {
		t.Error("expected zero values")
	}
}

func TestTenantQuotasValue_Scan_EmptyBytes(t *testing.T) {
	tq := TenantQuotasValue{}
	if err := tq.Scan([]byte{}); err != nil {
		t.Fatal(err)
	}
	if tq.MaxUsers != 0 {
		t.Error("expected zero values")
	}
}

func TestTenantQuotasValue_Scan_Valid(t *testing.T) {
	tq := TenantQuotasValue{}
	if err := tq.Scan(`{"max_users":100,"default_cpu_request":"500m"}`); err != nil {
		t.Fatal(err)
	}
	if tq.MaxUsers != 100 {
		t.Errorf("expected 100, got %d", tq.MaxUsers)
	}
	if tq.DefaultCPURequest != "500m" {
		t.Errorf("expected '500m', got %q", tq.DefaultCPURequest)
	}
}

func TestTenantQuotasValue_Scan_Bytes(t *testing.T) {
	tq := TenantQuotasValue{}
	if err := tq.Scan([]byte(`{"max_apps":5}`)); err != nil {
		t.Fatal(err)
	}
	if tq.MaxApps != 5 {
		t.Errorf("expected 5, got %d", tq.MaxApps)
	}
}

func TestTenantQuotasValue_Scan_InvalidType(t *testing.T) {
	tq := TenantQuotasValue{}
	if err := tq.Scan(true); err == nil {
		t.Error("expected error for bool type")
	}
}

func TestTenantQuotasValue_Roundtrip(t *testing.T) {
	original := TenantQuotasValue{TenantQuotas{
		MaxSessionsPerUser: 10,
		MaxTotalSessions:   500,
		MaxUsers:           200,
		DefaultCPURequest:  "250m",
		DefaultMemLimit:    "4Gi",
	}}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored TenantQuotasValue
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if restored.MaxSessionsPerUser != 10 || restored.MaxTotalSessions != 500 ||
		restored.MaxUsers != 200 || restored.DefaultCPURequest != "250m" ||
		restored.DefaultMemLimit != "4Gi" {
		t.Errorf("roundtrip mismatch: %+v", restored)
	}
}

// --- EnvVarSlice ---

func TestEnvVarSlice_Value_Nil(t *testing.T) {
	var e EnvVarSlice
	v, err := e.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "[]" {
		t.Errorf("expected '[]', got %q", v)
	}
}

func TestEnvVarSlice_Value_WithItems(t *testing.T) {
	e := EnvVarSlice{{Name: "FOO", Value: "bar"}}
	v, err := e.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok || s == "[]" {
		t.Error("expected non-empty JSON array")
	}
}

func TestEnvVarSlice_Scan_Nil(t *testing.T) {
	var e EnvVarSlice
	if err := e.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if e != nil {
		t.Error("expected nil")
	}
}

func TestEnvVarSlice_Scan_EmptyArray(t *testing.T) {
	var e EnvVarSlice
	if err := e.Scan("[]"); err != nil {
		t.Fatal(err)
	}
	if e != nil {
		t.Error("expected nil for '[]'")
	}
}

func TestEnvVarSlice_Scan_Valid(t *testing.T) {
	var e EnvVarSlice
	if err := e.Scan(`[{"name":"PATH","value":"/usr/bin"}]`); err != nil {
		t.Fatal(err)
	}
	if len(e) != 1 || e[0].Name != "PATH" || e[0].Value != "/usr/bin" {
		t.Errorf("unexpected result: %+v", e)
	}
}

func TestEnvVarSlice_Scan_InvalidType(t *testing.T) {
	var e EnvVarSlice
	if err := e.Scan(42); err == nil {
		t.Error("expected error for int type")
	}
}

func TestEnvVarSlice_Roundtrip(t *testing.T) {
	original := EnvVarSlice{
		{Name: "A", Value: "1"},
		{Name: "B", Value: "2"},
	}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored EnvVarSlice
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 || restored[0].Name != "A" || restored[1].Value != "2" {
		t.Errorf("roundtrip mismatch: %+v", restored)
	}
}

// --- VolumeMountSlice ---

func TestVolumeMountSlice_Value_Nil(t *testing.T) {
	var v VolumeMountSlice
	val, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}
	if val != "[]" {
		t.Errorf("expected '[]', got %q", val)
	}
}

func TestVolumeMountSlice_Value_WithItems(t *testing.T) {
	v := VolumeMountSlice{{Name: "data", MountPath: "/data", Size: "10Gi"}}
	val, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok || s == "[]" {
		t.Error("expected non-empty JSON")
	}
}

func TestVolumeMountSlice_Scan_Nil(t *testing.T) {
	var v VolumeMountSlice
	if err := v.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("expected nil")
	}
}

func TestVolumeMountSlice_Scan_EmptyArray(t *testing.T) {
	var v VolumeMountSlice
	if err := v.Scan("[]"); err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Error("expected nil")
	}
}

func TestVolumeMountSlice_Scan_Valid(t *testing.T) {
	var v VolumeMountSlice
	if err := v.Scan(`[{"name":"vol1","mount_path":"/mnt","size":"5Gi","read_only":true}]`); err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 || v[0].Name != "vol1" || v[0].MountPath != "/mnt" || !v[0].ReadOnly {
		t.Errorf("unexpected: %+v", v)
	}
}

func TestVolumeMountSlice_Scan_Bytes(t *testing.T) {
	var v VolumeMountSlice
	if err := v.Scan([]byte(`[{"name":"x","mount_path":"/x"}]`)); err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Errorf("expected 1 item, got %d", len(v))
	}
}

func TestVolumeMountSlice_Scan_InvalidType(t *testing.T) {
	var v VolumeMountSlice
	if err := v.Scan(false); err == nil {
		t.Error("expected error for bool type")
	}
}

func TestVolumeMountSlice_Roundtrip(t *testing.T) {
	original := VolumeMountSlice{
		{Name: "data", MountPath: "/data", Size: "1Gi", ReadOnly: false},
		{Name: "config", MountPath: "/etc/app", ReadOnly: true},
	}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored VolumeMountSlice
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 || restored[0].Name != "data" || !restored[1].ReadOnly {
		t.Errorf("roundtrip mismatch: %+v", restored)
	}
}

// --- NetworkRuleSlice ---

func TestNetworkRuleSlice_Value_Nil(t *testing.T) {
	var n NetworkRuleSlice
	v, err := n.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "[]" {
		t.Errorf("expected '[]', got %q", v)
	}
}

func TestNetworkRuleSlice_Value_WithItems(t *testing.T) {
	n := NetworkRuleSlice{{Port: 8080, Protocol: "TCP", AllowFrom: "10.0.0.0/8"}}
	v, err := n.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok || s == "[]" {
		t.Error("expected non-empty JSON")
	}
}

func TestNetworkRuleSlice_Scan_Nil(t *testing.T) {
	var n NetworkRuleSlice
	if err := n.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if n != nil {
		t.Error("expected nil")
	}
}

func TestNetworkRuleSlice_Scan_EmptyArray(t *testing.T) {
	var n NetworkRuleSlice
	if err := n.Scan("[]"); err != nil {
		t.Fatal(err)
	}
	if n != nil {
		t.Error("expected nil")
	}
}

func TestNetworkRuleSlice_Scan_Valid(t *testing.T) {
	var n NetworkRuleSlice
	if err := n.Scan(`[{"port":443,"protocol":"TCP","allow_from":"0.0.0.0/0"}]`); err != nil {
		t.Fatal(err)
	}
	if len(n) != 1 || n[0].Port != 443 || n[0].Protocol != "TCP" {
		t.Errorf("unexpected: %+v", n)
	}
}

func TestNetworkRuleSlice_Scan_InvalidType(t *testing.T) {
	var n NetworkRuleSlice
	if err := n.Scan(99); err == nil {
		t.Error("expected error for int type")
	}
}

func TestNetworkRuleSlice_Roundtrip(t *testing.T) {
	original := NetworkRuleSlice{
		{Port: 80, Protocol: "TCP"},
		{Port: 53, Protocol: "UDP", AllowFrom: "10.0.0.0/8"},
	}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored NetworkRuleSlice
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 || restored[0].Port != 80 || restored[1].Protocol != "UDP" {
		t.Errorf("roundtrip mismatch: %+v", restored)
	}
}

// --- ResourceLimitsValue ---

func TestResourceLimitsValue_Value_Nil(t *testing.T) {
	r := ResourceLimitsValue{}
	v, err := r.Value()
	if err != nil {
		t.Fatal(err)
	}
	if v != "{}" {
		t.Errorf("expected '{}', got %q", v)
	}
}

func TestResourceLimitsValue_Value_WithData(t *testing.T) {
	r := ResourceLimitsValue{ResourceLimits: &ResourceLimits{
		CPURequest:    "500m",
		CPULimit:      "2",
		MemoryRequest: "512Mi",
		MemoryLimit:   "2Gi",
	}}
	v, err := r.Value()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok || s == "{}" {
		t.Error("expected non-empty JSON")
	}
}

func TestResourceLimitsValue_Scan_Nil(t *testing.T) {
	r := ResourceLimitsValue{ResourceLimits: &ResourceLimits{CPURequest: "old"}}
	if err := r.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits != nil {
		t.Error("expected nil after scanning nil")
	}
}

func TestResourceLimitsValue_Scan_EmptyObject(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan("{}"); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits != nil {
		t.Error("expected nil for '{}'")
	}
}

func TestResourceLimitsValue_Scan_EmptyString(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan(""); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits != nil {
		t.Error("expected nil for empty string")
	}
}

func TestResourceLimitsValue_Scan_EmptyBytes(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan([]byte{}); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits != nil {
		t.Error("expected nil for empty bytes")
	}
}

func TestResourceLimitsValue_Scan_AllZero(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan(`{"cpu_request":"","cpu_limit":"","memory_request":"","memory_limit":""}`); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits != nil {
		t.Error("expected nil when all fields are empty strings")
	}
}

func TestResourceLimitsValue_Scan_Valid(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan(`{"cpu_request":"250m","memory_limit":"1Gi"}`); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits == nil {
		t.Fatal("expected non-nil ResourceLimits")
	}
	if r.CPURequest != "250m" {
		t.Errorf("expected '250m', got %q", r.CPURequest)
	}
	if r.MemoryLimit != "1Gi" {
		t.Errorf("expected '1Gi', got %q", r.MemoryLimit)
	}
}

func TestResourceLimitsValue_Scan_Bytes(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan([]byte(`{"cpu_limit":"4"}`)); err != nil {
		t.Fatal(err)
	}
	if r.ResourceLimits == nil || r.CPULimit != "4" {
		t.Errorf("expected cpu_limit='4', got %+v", r.ResourceLimits)
	}
}

func TestResourceLimitsValue_Scan_InvalidType(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan(42); err == nil {
		t.Error("expected error for int type")
	}
}

func TestResourceLimitsValue_Scan_InvalidJSON(t *testing.T) {
	r := ResourceLimitsValue{}
	if err := r.Scan("not json"); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestResourceLimitsValue_Roundtrip(t *testing.T) {
	original := ResourceLimitsValue{ResourceLimits: &ResourceLimits{
		CPURequest:    "100m",
		CPULimit:      "1",
		MemoryRequest: "256Mi",
		MemoryLimit:   "512Mi",
	}}
	v, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}

	var restored ResourceLimitsValue
	if err := restored.Scan(v); err != nil {
		t.Fatal(err)
	}
	if restored.ResourceLimits == nil {
		t.Fatal("expected non-nil")
	}
	if restored.CPURequest != "100m" || restored.CPULimit != "1" ||
		restored.MemoryRequest != "256Mi" || restored.MemoryLimit != "512Mi" {
		t.Errorf("roundtrip mismatch: %+v", restored.ResourceLimits)
	}
}
