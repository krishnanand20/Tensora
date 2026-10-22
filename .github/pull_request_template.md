## What and why

<!-- One or two sentences. Link the issue/ADR if there is one. -->

## Tenant isolation impact

<!-- Does this change what a tenant can see, reach, run, or escape to? Namespaces, NetworkPolicy,
RBAC, RuntimeClass, manifests, store queries, model validation. Write "None" only if you checked. -->

## Billing impact

<!-- Does this change workload states, events, metering, or what counts as billable? -->

## Checklist

- [ ] `make lint test` passes (or the subset for components touched; paste results if partial)
- [ ] `make deploy-check` passes if `deploy/` changed
- [ ] New tenant-scoped endpoints have a cross-tenant 404 test
- [ ] Docs updated (`docs/architecture/`, runbooks, `AGENTS.md`) if behavior changed
- [ ] ADR added if an architecture rule changed
- [ ] No secrets, kubeconfigs, or real IPs committed
