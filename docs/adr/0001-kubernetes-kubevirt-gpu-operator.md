# 0001. Use Kubernetes with KubeVirt and the NVIDIA GPU Operator

- **Status:** Accepted
- **Date:** 2026-10-22

> **Scope since 2026-10-23 ([ADR-0012](0012-deployment-profiles.md), [ADR-0013](0013-node-agent-and-apple-silicon.md)):** this decision covers the **Datacenter profile** only. Solo and Lab installs (including Macs) use the node agent instead of Kubernetes. Mentions of "customers" below mean users.

## Context

We need GPU VMs (customers who want a whole machine), containers (notebooks), and model
endpoints (autoscaled serving) on the same hardware, with multi-tenancy and an API on top.
The team already knows Docker. Model serving tooling (KServe, vLLM, Triton, Ray) is container-native.

## Decision

Run **Kubernetes** (k3s, see ADR-0002) as the single orchestration layer:

- **KubeVirt** with **CDI** for VMs, with GPUs attached through VFIO passthrough
- The **NVIDIA GPU Operator** for drivers, the container toolkit, device plugins, vfio-manager, MIG, and DCGM
- **KServe** for model endpoints

## Consequences

- VMs and containers share one scheduler, one network (Cilium), one storage layer, and one API (the controller applies manifests for both).
- We inherit Kubernetes complexity. That's mitigated by k3s, a small add-on set, and helmfile.
- GPU Operator modes are per node (see gpu-sharing.md), which shapes capacity planning.
- KubeVirt is less mature than Proxmox or OpenStack for VM-only features like snapshots, live migration with GPUs, and a VM console UI. Acceptable, because GPU VMs can't live-migrate anyway.

## Alternatives considered

- **Proxmox:** the easiest VM platform for 1–5 nodes, but it's weak for container serving and multi-tenancy, and it has no declarative API we could build on easily.
- **OpenStack** (Nova, Neutron, Cinder): a real cloud, but very heavy to operate for a small team, and container serving would still need a second system.
- **Plain libvirt and our own scheduler:** maximum control, but we'd reinvent scheduling, networking, and storage.

## Update 2026-10-23

This stack is deployed once per region, as a regional cell (ADR-0009). CPU worker nodes join the same
clusters to run CPU instances and fallbacks (ADR-0010).
