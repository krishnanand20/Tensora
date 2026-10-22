# 0002. Use k3s with embedded etcd, starting on a single node

- **Status:** Accepted
- **Date:** 2026-10-22

## Context

Phase 1 is one GPU server. We want a path to an HA control plane without reinstalling, and
minimal moving parts while we learn.

## Decision

- Install **k3s** with `cluster-init` (embedded etcd) from the first node, so more servers can join later for HA (3 servers in Phase 4).
- Disable flannel, network policy, kube-proxy, traefik, and servicelb. Cilium and MetalLB replace them (`infra/ansible/roles/k3s`).
- Encrypt Secrets at rest (`secrets-encryption: true`).
- Bootstrap nodes with Ansible, and install the platform with helmfile.

## Consequences

- A single binary is easy to install and upgrade, with a small footprint left for workloads.
- k3s's containerd paths differ from upstream, so the GPU Operator toolkit needs explicit paths (gpu-operator values).
- Embedded etcd on a single node is a single point of failure until Phase 2. Take etcd snapshots off-node.

## Alternatives considered

- **kubeadm:** upstream and flexible, but more to manage by hand.
- **RKE2:** hardened k3s sibling with CIS defaults; worth reconsidering for production (Phase 4) if compliance demands it.
- **Managed Kubernetes:** not applicable on our own hardware.
