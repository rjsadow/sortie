package k8s

import (
	"testing"

	"github.com/rjsadow/sortie/internal/db"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildSessionNetworkPolicy_NilPolicy(t *testing.T) {
	np := BuildSessionNetworkPolicy("sess-1", "app-1", nil)
	if np != nil {
		t.Error("expected nil NetworkPolicy for nil egress policy")
	}
}

func TestBuildSessionNetworkPolicy_EmptyMode(t *testing.T) {
	np := BuildSessionNetworkPolicy("sess-1", "app-1", &db.EgressPolicy{})
	if np != nil {
		t.Error("expected nil NetworkPolicy for empty mode")
	}
}

func TestBuildSessionNetworkPolicy_InvalidMode(t *testing.T) {
	np := BuildSessionNetworkPolicy("sess-1", "app-1", &db.EgressPolicy{Mode: "invalid"})
	if np != nil {
		t.Error("expected nil NetworkPolicy for invalid mode")
	}
}

func TestBuildSessionNetworkPolicy_Allowlist(t *testing.T) {
	defer ResetClient()
	policy := &db.EgressPolicy{
		Mode: "allowlist",
		Rules: []db.EgressRule{
			{CIDR: "10.0.0.0/8", Port: 443, Protocol: "TCP"},
			{CIDR: "172.16.0.0/12"},
		},
	}

	np := BuildSessionNetworkPolicy("sess-1", "app-1", policy)
	if np == nil {
		t.Fatal("expected non-nil NetworkPolicy")
	}

	if np.Name != "sortie-egress-sess-1" {
		t.Errorf("expected name sortie-egress-sess-1, got %s", np.Name)
	}

	if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != "Egress" {
		t.Error("expected Egress policy type")
	}

	// Should have 3 egress rules: DNS + 2 allowlist rules
	if len(np.Spec.Egress) != 3 {
		t.Fatalf("expected 3 egress rules, got %d", len(np.Spec.Egress))
	}

	// First rule should be DNS (2 ports: UDP 53, TCP 53)
	dnsRule := np.Spec.Egress[0]
	if len(dnsRule.Ports) != 2 {
		t.Errorf("expected 2 DNS ports, got %d", len(dnsRule.Ports))
	}

	// Second rule should have CIDR 10.0.0.0/8 with port 443/TCP
	rule1 := np.Spec.Egress[1]
	if rule1.To[0].IPBlock.CIDR != "10.0.0.0/8" {
		t.Errorf("expected CIDR 10.0.0.0/8, got %s", rule1.To[0].IPBlock.CIDR)
	}
	if len(rule1.Ports) != 1 || rule1.Ports[0].Port.IntValue() != 443 {
		t.Error("expected port 443")
	}
	if *rule1.Ports[0].Protocol != corev1.ProtocolTCP {
		t.Error("expected TCP protocol")
	}

	// Third rule should have CIDR 172.16.0.0/12 with no port restriction
	rule2 := np.Spec.Egress[2]
	if rule2.To[0].IPBlock.CIDR != "172.16.0.0/12" {
		t.Errorf("expected CIDR 172.16.0.0/12, got %s", rule2.To[0].IPBlock.CIDR)
	}
	if len(rule2.Ports) != 0 {
		t.Error("expected no port restriction for second rule")
	}
}

func TestBuildSessionNetworkPolicy_Denylist(t *testing.T) {
	defer ResetClient()
	policy := &db.EgressPolicy{
		Mode: "denylist",
		Rules: []db.EgressRule{
			{CIDR: "10.0.0.0/8"},
			{CIDR: "192.168.0.0/16"},
		},
	}

	np := BuildSessionNetworkPolicy("sess-2", "app-2", policy)
	if np == nil {
		t.Fatal("expected non-nil NetworkPolicy")
	}

	// Should have 2 egress rules: DNS + broad allow with exceptions
	if len(np.Spec.Egress) != 2 {
		t.Fatalf("expected 2 egress rules, got %d", len(np.Spec.Egress))
	}

	// Second rule should be 0.0.0.0/0 with except list
	broadRule := np.Spec.Egress[1]
	if broadRule.To[0].IPBlock.CIDR != "0.0.0.0/0" {
		t.Errorf("expected CIDR 0.0.0.0/0, got %s", broadRule.To[0].IPBlock.CIDR)
	}
	if len(broadRule.To[0].IPBlock.Except) != 2 {
		t.Fatalf("expected 2 except CIDRs, got %d", len(broadRule.To[0].IPBlock.Except))
	}
	if broadRule.To[0].IPBlock.Except[0] != "10.0.0.0/8" {
		t.Errorf("expected first except 10.0.0.0/8, got %s", broadRule.To[0].IPBlock.Except[0])
	}
}

func TestBuildSessionNetworkPolicy_DenylistEmpty(t *testing.T) {
	defer ResetClient()
	policy := &db.EgressPolicy{
		Mode:  "denylist",
		Rules: []db.EgressRule{},
	}

	np := BuildSessionNetworkPolicy("sess-3", "app-3", policy)
	if np == nil {
		t.Fatal("expected non-nil NetworkPolicy")
	}

	// Should have 2 rules: DNS + broad allow without exceptions
	if len(np.Spec.Egress) != 2 {
		t.Fatalf("expected 2 egress rules, got %d", len(np.Spec.Egress))
	}

	broadRule := np.Spec.Egress[1]
	if broadRule.To[0].IPBlock.CIDR != "0.0.0.0/0" {
		t.Errorf("expected CIDR 0.0.0.0/0, got %s", broadRule.To[0].IPBlock.CIDR)
	}
	if len(broadRule.To[0].IPBlock.Except) != 0 {
		t.Error("expected no except CIDRs")
	}
}

func TestBuildSessionNetworkPolicy_Labels(t *testing.T) {
	defer ResetClient()
	policy := &db.EgressPolicy{Mode: "allowlist"}
	np := BuildSessionNetworkPolicy("sess-4", "app-4", policy)
	if np == nil {
		t.Fatal("expected non-nil NetworkPolicy")
	}

	if np.Labels[SessionLabelKey] != "sess-4" {
		t.Errorf("expected session label sess-4, got %s", np.Labels[SessionLabelKey])
	}
	if np.Labels[AppLabelKey] != "app-4" {
		t.Errorf("expected app label app-4, got %s", np.Labels[AppLabelKey])
	}
	if np.Labels[NetworkPolicyLabelKey] != "true" {
		t.Error("expected egress-policy label")
	}

	// Pod selector should match session
	if np.Spec.PodSelector.MatchLabels[SessionLabelKey] != "sess-4" {
		t.Error("expected pod selector to match session ID")
	}
}

func TestParseProtocol(t *testing.T) {
	tests := []struct {
		input    string
		expected corev1.Protocol
	}{
		{"TCP", corev1.ProtocolTCP},
		{"UDP", corev1.ProtocolUDP},
		{"", corev1.ProtocolTCP},
		{"tcp", corev1.ProtocolTCP},
	}

	for _, tt := range tests {
		result := parseProtocol(tt.input)
		if result != tt.expected {
			t.Errorf("parseProtocol(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}
