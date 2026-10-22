---
name: add-platform-component
description: Add a third-party cluster add-on (Helm chart or operator) to deploy/ with pinned versions, feature flags, values, docs, and an ADR when needed. Use when asked to install or add a component to the cluster.
---

Follow [docs/playbooks/add-platform-component.md](../../../docs/playbooks/add-platform-component.md).

Component: $ARGUMENTS

State whether it runs in **every regional cell** or **once in the global plane** (ADR-0009).
Never apply to a cluster. Finish with `make deploy-check` and a `platform-reviewer` pass.
If you can't confirm the chart's current version or values keys, say so explicitly
instead of guessing.
