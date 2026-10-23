---
name: add-platform-component
description: Add a third-party add-on (Helm chart or operator) to the Datacenter profile in deploy/, with pinned versions, feature flags, values, docs, and an ADR when needed. Use when asked to install or add a component to a Datacenter cluster.
---

Follow [docs/playbooks/add-platform-component.md](../../../docs/playbooks/add-platform-component.md).

Component: $ARGUMENTS

`deploy/` is the Datacenter profile only. If the request is really about Solo or Lab machines (Macs,
personal Linux boxes), it belongs in the node agent's drivers instead. Say so, and don't add it here.

State whether it runs in **every regional cluster** or **once alongside the control plane** (ADR-0009).
Never apply to a cluster. Finish with `make deploy-check` and a `platform-reviewer` pass. If you can't
confirm the chart's current version or values keys, say so explicitly instead of guessing.
