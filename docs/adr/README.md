# Architecture Decision Records

Short records of decisions that are expensive to reverse. How to write one:
[docs/playbooks/write-an-adr.md](../playbooks/write-an-adr.md). Claude Code users can run `/new-adr`.

| # | Decision | Status |
|---|---|---|
| [0001](0001-kubernetes-kubevirt-gpu-operator.md) | Kubernetes + KubeVirt + GPU Operator, not Proxmox or OpenStack | Accepted |
| [0002](0002-k3s-single-node-first.md) | k3s with embedded etcd, starting on a single node | Accepted |
| [0003](0003-language-split.md) | Go for the core platform, Python for the ML layer, TypeScript for the dashboard | Accepted |
| [0004](0004-untrusted-code-in-vms-or-kata.md) | Untrusted code runs only in VMs or Kata containers | Accepted |
| [0005](0005-storage-longhorn-then-ceph.md) | Longhorn and a small S3 store first, Rook-Ceph from Phase 2 | Accepted (object store choice open) |
| [0006](0006-bill-on-allocation.md) | Bill GPU allocation time, not utilization | Accepted |
| [0007](0007-gateway-api-on-cilium.md) | Gateway API on Cilium for ingress | Accepted |
| [0008](0008-postgres-intent-kubernetes-runtime.md) | Postgres owns intent, Kubernetes owns runtime, and a controller reconciles them | Accepted |
| [0009](0009-regions-global-control-plane-regional-cells.md) | Regions: one global control plane, independent regional cells | Accepted (not yet in code) |
| [0010](0010-fallbacks-cpu-placement-and-inference-failover.md) | Fallbacks: CPU placement when GPUs are unavailable, and inference failover | Accepted (not yet in code) |

Statuses: Proposed → Accepted → (Superseded by NNNN | Deprecated). Never delete an ADR; supersede it.
