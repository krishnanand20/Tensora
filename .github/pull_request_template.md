## What and why

<!-- One or two sentences. Link the issue/ADR if there is one. -->

## Isolation and host safety

<!-- Does this change what a user can see, reach, run, or escape to, or what a host is exposed to?
Tenant scoping, the trust matrix, drivers, sandbox profiles, NetworkPolicy, RBAC, manifests, model
validation, sharing levels. Write "None" only if you checked. -->

## Region and fallback impact

<!-- Could this move workload data or traffic across regions (rule 9)? Could it give a user something
other than what they asked for (another class, CPU, model, machine, or region) without opt-in and
visibility (rule 10)? Write "None" only if you checked. -->

## Usage accounting impact

<!-- Does this change workload states, events, or what counts as metered time? (There is no billing,
but quotas and fair share depend on accounting.) -->

## Platforms

<!-- Which platforms did you test on: macOS (Apple Silicon), Linux amd64, Linux arm64? -->

## Checklist

- [ ] `make lint test` passes (or the subset for components touched; paste results if partial)
- [ ] `make build-all` passes if `controlplane/` changed
- [ ] `make deploy-check` passes if `deploy/` changed
- [ ] New tenant-scoped endpoints have a cross-tenant 404 test
- [ ] Regional logic has a cross-region test; fallbacks are recorded and accounted as what ran
- [ ] Nothing adds pricing, payments, or paid-only features
- [ ] Docs updated (`docs/architecture/`, runbooks, `AGENTS.md`) if behavior changed
- [ ] ADR added if an architecture rule changed
- [ ] No secrets, kubeconfigs, certificates, or real IPs committed
