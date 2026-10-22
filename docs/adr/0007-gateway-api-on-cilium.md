# 0007. Use Gateway API on Cilium for ingress

- **Status:** Accepted
- **Date:** 2026-10-22

## Context

We need TLS ingress for the API, dashboard, and Keycloak, and later per-workload routes for endpoints
and notebooks. ingress-nginx was the usual default, but upstream announced its retirement
(best-effort maintenance ended in early 2026). We already run Cilium as the CNI.

## Decision

- Use **Gateway API**, implemented by **Cilium** (`gatewayAPI.enabled`): one `Gateway` named `tensora` in `tensora-gateway`, and HTTPRoutes owned by each component.
- **cert-manager** issues certificates per listener (ACME HTTP-01 through the Gateway). Wildcards for per-workload hosts will need DNS-01 later.
- **MetalLB** in L2 mode provides the Gateway's external IP.

## Consequences

- There's one fewer component (no separate ingress controller), and policy and ingress share Cilium.
- Traffic from Cilium's Gateway reaches pods with the reserved `ingress` identity, so tenant namespaces need a `CiliumNetworkPolicy` (`fromEntities: [ingress]`). Plain NetworkPolicy can't express it.
- We're tied to Cilium's Gateway API conformance level, and must check it on each upgrade.

## Alternatives considered

- **ingress-nginx:** retired upstream.
- **Envoy Gateway or Traefik:** good options, but they add another data plane. Revisit if Cilium's Gateway lacks a feature we need (for example ext-auth for API keys).
