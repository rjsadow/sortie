package k8s

import (
	"context"
	"fmt"

	"github.com/rjsadow/launchpad/internal/db"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NetworkPolicyLabelKey is the label key used to associate NetworkPolicies with sessions
const NetworkPolicyLabelKey = "launchpad.io/egress-policy"

// BuildSessionNetworkPolicy creates a Kubernetes NetworkPolicy for a session pod
// based on the application's egress policy. Returns nil if no custom policy is needed.
func BuildSessionNetworkPolicy(sessionID, appID string, policy *db.EgressPolicy) *networkingv1.NetworkPolicy {
	if policy == nil || policy.Mode == "" {
		return nil
	}

	name := fmt.Sprintf("launchpad-egress-%s", sessionID)

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: GetNamespace(),
			Labels: map[string]string{
				SessionLabelKey:       sessionID,
				AppLabelKey:           appID,
				NetworkPolicyLabelKey: "true",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					SessionLabelKey: sessionID,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	// Always allow DNS egress (required for all modes)
	dnsEgress := dnsEgressRule()

	switch policy.Mode {
	case "allowlist":
		np.Spec.Egress = buildAllowlistEgress(dnsEgress, policy.Rules)
	case "denylist":
		np.Spec.Egress = buildDenylistEgress(dnsEgress, policy.Rules)
	default:
		return nil
	}

	return np
}

// dnsEgressRule creates an egress rule that allows DNS traffic
func dnsEgressRule() networkingv1.NetworkPolicyEgressRule {
	udp := corev1.ProtocolUDP
	tcp := corev1.ProtocolTCP
	dnsPort := intstr.FromInt(53)

	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &udp, Port: &dnsPort},
			{Protocol: &tcp, Port: &dnsPort},
		},
	}
}

// buildAllowlistEgress creates egress rules for allowlist mode.
// Only DNS and explicitly listed destinations are permitted.
func buildAllowlistEgress(dnsRule networkingv1.NetworkPolicyEgressRule, rules []db.EgressRule) []networkingv1.NetworkPolicyEgressRule {
	egress := []networkingv1.NetworkPolicyEgressRule{dnsRule}

	for _, rule := range rules {
		egressRule := networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: rule.CIDR,
					},
				},
			},
		}

		// Add port restriction if specified
		if rule.Port > 0 {
			port := intstr.FromInt(rule.Port)
			npPort := networkingv1.NetworkPolicyPort{Port: &port}
			if rule.Protocol != "" {
				proto := parseProtocol(rule.Protocol)
				npPort.Protocol = &proto
			}
			egressRule.Ports = []networkingv1.NetworkPolicyPort{npPort}
		} else if rule.Protocol != "" {
			proto := parseProtocol(rule.Protocol)
			egressRule.Ports = []networkingv1.NetworkPolicyPort{
				{Protocol: &proto},
			}
		}

		egress = append(egress, egressRule)
	}

	return egress
}

// buildDenylistEgress creates egress rules for denylist mode.
// All traffic is permitted except the listed destinations.
// Uses ipBlock.except to block specific CIDRs.
func buildDenylistEgress(dnsRule networkingv1.NetworkPolicyEgressRule, rules []db.EgressRule) []networkingv1.NetworkPolicyEgressRule {
	var exceptCIDRs []string
	for _, rule := range rules {
		exceptCIDRs = append(exceptCIDRs, rule.CIDR)
	}

	egress := []networkingv1.NetworkPolicyEgressRule{dnsRule}

	if len(exceptCIDRs) > 0 {
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR:   "0.0.0.0/0",
						Except: exceptCIDRs,
					},
				},
			},
		})
	} else {
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "0.0.0.0/0",
					},
				},
			},
		})
	}

	return egress
}

// CreateNetworkPolicy creates a NetworkPolicy in the cluster
func CreateNetworkPolicy(ctx context.Context, np *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	return client.NetworkingV1().NetworkPolicies(GetNamespace()).Create(ctx, np, metav1.CreateOptions{})
}

// DeleteNetworkPolicy deletes a NetworkPolicy by name
func DeleteNetworkPolicy(ctx context.Context, name string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	return client.NetworkingV1().NetworkPolicies(GetNamespace()).Delete(ctx, name, metav1.DeleteOptions{})
}

// DeleteSessionNetworkPolicy deletes the NetworkPolicy for a session.
// Ignores not-found errors since the policy may not exist.
func DeleteSessionNetworkPolicy(ctx context.Context, sessionID string) error {
	name := fmt.Sprintf("launchpad-egress-%s", sessionID)
	_ = DeleteNetworkPolicy(ctx, name)
	return nil
}

// parseProtocol converts a string protocol to the K8s corev1.Protocol type
func parseProtocol(proto string) corev1.Protocol {
	switch proto {
	case "UDP":
		return corev1.ProtocolUDP
	default:
		return corev1.ProtocolTCP
	}
}
