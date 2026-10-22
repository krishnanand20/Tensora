# AGENTS.md: deploy/ (cluster add-ons and charts)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

## Layout

```
helmfile.yaml.gotmpl          every add-on in install order, grouped by layer, with feature flags
environments/<env>.yaml       one file per regional cell (single-node = the first region today):
                              flags, IPs, GPU devices, storage
platform/<component>/         values files (or kustomizations) for third-party components
charts/cluster-config/        OUR cluster objects: namespaces, MetalLB pool, issuer, Gateway,
                              KubeVirt/CDI CRs, tensora-vllm runtime, tensora-block StorageClass
charts/tensora/               the control plane (api, controller, metering, migrate Job) and web
examples/                     hand-applied smoke tests used by the runbooks
```

## Rules

- **Never apply.** Agents run `make deploy-check` (helm lint, helm template, kubeconform) and nothing that touches a cluster. `helmfile apply` and `sync` are for humans.
- **Pin everything.** Chart `version:` and image tags must be exact; no `latest` and no ranges. When you bump a version, read the upstream upgrade notes and record the date in `README.md#versions`.
- **No secrets in values.** Reference Secrets created out of band (`existingSecret`, `secretKeyRef`). List any new one under "Secrets created out of band" in `README.md`.
- **Keep the control plane in sync.** If you change a kind the controller creates, update `charts/tensora/templates/controller.yaml` RBAC and `controlplane/internal/cluster/kube/kube.go` (`allowed`) together. If you change `tensora.io/role` namespace labels, check `controlplane/internal/cluster/manifests/tenant.go`.
- **Isolation-sensitive files** need a `tenancy-reviewer` pass:
  - `gpu-operator/values.yaml`: never add time-slicing
  - `serving-runtime.yaml`: keep `--load-format=safetensors`; never add `--trust-remote-code`
  - Gateway `allowedRoutes`
  - anything that grants RBAC
- **Order matters.** Express dependencies with `needs:`. Custom resources go in `cluster-config`, which `needs` their operators.
- **Feature flags:** a component that isn't used in Phase 1 gets an `installed: {{ .Values.features.<x> }}` flag. Add the flag to every environment file.
- **Regions (ADR-0009):** most add-ons deploy into every regional cell. Mark global-only components (Keycloak, the global Postgres) as such. A regional cell must never depend on another region. Until the second region, the first cell also hosts the global plane.
- **Fallbacks (ADR-0010):** the planned `tensora-vllm-cpu` runtime follows the same rules as `tensora-vllm` (safetensors only, no remote code). CPU worker nodes get the same isolation runtime classes as GPU nodes.

## Commands

```bash
make deploy-check                                  # from repo root: lint, render, and validate
helmfile -e single-node template > /tmp/out.yaml   # full render (needs helmfile and kustomize)
```
