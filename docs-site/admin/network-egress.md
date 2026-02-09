# Network Egress Rules

Sortie supports per-application network egress policies
to control what network destinations session pods can reach.
This provides defense-in-depth by restricting outbound
connections from containerized sessions.

## Overview

Each application can have an **egress policy** that defines
what outbound network traffic is allowed from its session
pods. Two modes are supported:

- **Allowlist** (recommended): Only explicitly listed
  destinations are permitted. All other egress traffic is
  blocked (except DNS).
- **Denylist**: All egress traffic is permitted except
  explicitly listed destinations.

When no egress policy is configured, sessions inherit the
cluster-level default NetworkPolicy (which typically allows
DNS + HTTP/HTTPS to any destination).

## Configuration

Egress policies are configured per-application via the API
when creating or updating an application.

### Data Model

```json
{
  "egress_policy": {
    "mode": "allowlist",
    "rules": [
      {
        "cidr": "10.0.0.0/8",
        "port": 443,
        "protocol": "TCP"
      },
      {
        "cidr": "0.0.0.0/0",
        "port": 80
      }
    ]
  }
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `mode` | string | `"allowlist"` or `"denylist"`. Empty = inherit cluster default. |
| `rules` | array | List of egress rules. |
| `rules[].cidr` | string | Destination CIDR (e.g., `"10.0.0.0/8"`, `"0.0.0.0/0"`). Required. |
| `rules[].port` | int | Destination port. 0 or omitted = all ports. |
| `rules[].protocol` | string | `"TCP"`, `"UDP"`, or empty for both. |

### API Examples

**Create an app with allowlist egress (only HTTPS to
specific subnets):**

```bash
curl -X POST /api/apps \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "id": "secure-browser",
    "name": "Secure Browser",
    "category": "Tools",
    "launch_type": "container",
    "container_image": "ghcr.io/example/browser:latest",
    "egress_policy": {
      "mode": "allowlist",
      "rules": [
        {"cidr": "10.0.0.0/8", "port": 443, "protocol": "TCP"},
        {"cidr": "10.0.0.0/8", "port": 80, "protocol": "TCP"}
      ]
    }
  }'
```

**Create an app with denylist egress (block internal
networks):**

```bash
curl -X POST /api/apps \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "id": "dev-workstation",
    "name": "Dev Workstation",
    "category": "Development",
    "launch_type": "container",
    "container_image": "ghcr.io/example/devbox:latest",
    "egress_policy": {
      "mode": "denylist",
      "rules": [
        {"cidr": "10.0.0.0/8"},
        {"cidr": "172.16.0.0/12"},
        {"cidr": "192.168.0.0/16"}
      ]
    }
  }'
```

**App with no egress policy (inherits cluster default):**

```bash
curl -X POST /api/apps \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "id": "basic-app",
    "name": "Basic App",
    "category": "General",
    "launch_type": "container",
    "container_image": "ghcr.io/example/app:latest"
  }'
```

## Enforcement

Egress rules are enforced using Kubernetes
[NetworkPolicies][np]. When a session is created for an
app with an egress policy:

[np]: https://kubernetes.io/docs/concepts/services-networking/network-policies/

1. A pod is created for the session (as usual).
2. A per-session `NetworkPolicy` named
   `sortie-egress-{session-id}` is created.
3. The policy targets only the session pod via label
   selectors.
4. When the session is terminated, the NetworkPolicy is
   automatically cleaned up.

### Allowlist Mode

Creates a NetworkPolicy that permits:

- DNS traffic (UDP/TCP port 53) — always allowed
- Each rule becomes an explicit egress allow entry

All other egress traffic is implicitly denied by the
NetworkPolicy.

### Denylist Mode

Creates a NetworkPolicy that permits:

- DNS traffic (UDP/TCP port 53) — always allowed
- All traffic to `0.0.0.0/0` **except** the listed
  CIDRs (using `ipBlock.except`)

**Note:** Kubernetes `ipBlock.except` works at the CIDR
level only. Port-level denylist filtering is not supported
by K8s NetworkPolicies. For port-level control, use
allowlist mode.

## Prerequisites

### CNI Plugin

Your Kubernetes cluster must have a CNI plugin that
supports NetworkPolicies. Common options:

- **Calico** (recommended)
- **Cilium**
- **Weave Net**

Clusters using basic `kubenet` or `flannel` (without
Calico) do **not** enforce NetworkPolicies — the policies
will be created but have no effect.

### RBAC

The Sortie service account needs permission to manage
NetworkPolicies. This is included in the Helm chart and
deploy manifests:

```yaml
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["create", "delete", "get", "list"]
```

## Cluster-Level Defaults

The Helm chart includes a default NetworkPolicy for
session pods that allows:

- **Ingress**: Only from the Sortie server on
  VNC/RDP/proxy ports
- **Egress**: DNS + HTTP/HTTPS to any destination

Per-app egress policies create **additional**
NetworkPolicies. Since Kubernetes NetworkPolicies are
additive (union), the per-app policy will be the effective
policy when it is more restrictive than the cluster
default. For maximum control, configure the cluster-level
session egress to be restrictive (e.g., DNS only) and use
per-app allowlists to grant access.

## Limitations

- **Denylist port filtering**: K8s NetworkPolicies cannot
  deny specific ports while allowing others. Use allowlist
  mode for port-level control.
- **Domain-based rules**: K8s NetworkPolicies work with
  IP CIDRs, not domain names. Use a network proxy or
  service mesh for domain-based filtering.
- **IPv6**: Currently only IPv4 CIDRs are supported in
  egress rules.
- **CNI dependency**: Enforcement requires a
  NetworkPolicy-capable CNI plugin.
