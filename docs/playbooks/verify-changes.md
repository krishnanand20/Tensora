# Playbook: verify a change

Run the checks for every component you touched, then report the **actual** results. A check
you couldn't run is reported as "not run (reason)", never as passing.

| Paths changed | Run (from repo root) |
|---|---|
| `controlplane/**` | `make lint-cp test-cp` (`make test-cp-pg` too if `store/` or `migrations/` changed) |
| `images/catalog.yaml` | `make test-cp` (the tests load the real catalog) |
| `ml/**` | `make lint-ml test-ml` |
| `web/**` | `make lint-web test-web` and `cd web && npm run build` |
| `deploy/**` | `make deploy-check` |
| `infra/**` | `cd infra/ansible && uvx ansible-lint --offline`, plus `shellcheck infra/scripts/*.sh` |
| Any `*.yaml` | `make lint-yaml` |
| Docs only | Check that links resolve and that code references (paths, function names) still exist |

Also check these when they apply:

- **API shape changed:** `web/src/api/types.ts` matches `controlplane/internal/api/json.go`.
- **New cluster kind:** `kube.allowed` and the controller ClusterRole were both updated (`TestBuildersOnlyEmitAllowedKinds` passes).
- **Lifecycle changed:** the diagram in `docs/architecture/control-plane.md` matches `domain/state.go`.
- **Isolation-sensitive paths changed** (see `CONTRIBUTING.md`): run the `tenancy-reviewer`.

## Missing tools

| Tool | Install |
|---|---|
| Go 1.27+ | https://go.dev/dl |
| uv | `curl -LsSf https://astral.sh/uv/install.sh \| sh` or `brew install uv` |
| Node 22+ | https://nodejs.org or `brew install node` |
| golangci-lint | `brew install golangci-lint` |
| helm, kubeconform | `brew install helm kubeconform` |
