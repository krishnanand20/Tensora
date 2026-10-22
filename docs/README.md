# Tensora docs

| Area | Start here |
|---|---|
| **What the product is, who it's for, how it makes money** | [product/product-spec.md](product/product-spec.md) |
| **What we're building, and in what order** | [roadmap.md](roadmap.md) |
| **How it fits together** | [architecture/overview.md](architecture/overview.md) |
| Control plane: lifecycle, admission, reconciliation | [architecture/control-plane.md](architecture/control-plane.md) |
| Threat model and isolation controls | [architecture/tenancy-and-isolation.md](architecture/tenancy-and-isolation.md) |
| GPU passthrough, MIG, vGPU, time-slicing | [architecture/gpu-sharing.md](architecture/gpu-sharing.md) |
| Regions (global plane and regional cells) and fallbacks (CPU placement, inference failover) | [ADR-0009](adr/0009-regions-global-control-plane-regional-cells.md), [ADR-0010](adr/0010-fallbacks-cpu-placement-and-inference-failover.md) |
| Networking, storage, metering, observability | [architecture/](architecture/) |
| **Why it is this way** | [adr/](adr/README.md) |
| **Hardware, power, cooling** | [hardware/](hardware/bill-of-materials.md) |
| **Operating it** | [runbooks/](runbooks/README.md) |
| **Changing it** (step-by-step workflows for humans and agents) | [playbooks/](playbooks/) |
| Terms | [glossary.md](glossary.md) |

Update docs in the same change as the behavior they describe. Stale docs are worse than none.
