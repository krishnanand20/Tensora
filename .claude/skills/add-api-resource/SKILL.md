---
name: add-api-resource
description: Add a new tenant-scoped resource to the Tensora API end to end (migration, store, service, handlers, cluster manifests, tests, web types). Use when asked to add an endpoint or resource to the control plane.
---

Follow [docs/playbooks/add-api-resource.md](../../../docs/playbooks/add-api-resource.md) step by step.

Resource to add: $ARGUMENTS

Before writing code, state:

- the resource's fields and lifecycle
- whether it creates cluster objects
- whether it affects quota or billing (prices in USD)
- whether it's **regional** or **global**, and whether any **fallback** can serve it (AGENTS.md rules 9 and 10)

Ask if any of that is ambiguous. When you're done:

1. Run the `verify` skill.
2. Run the `tenancy-reviewer` subagent on the change.
