# AGENTS.md

Instructions for AI coding agents (Codex, Claude Code, others) working in this repository.
Humans should start with [README.md](README.md).

This file is the **single source of truth** for agent instructions. `CLAUDE.md` imports it.
Each top-level component has its own `AGENTS.md` with component rules — read the one
closest to the files you are changing. Rules here apply everywhere; nested files add to them.

## What Tensora is

Tensora (a working name) is a global, multi-region GPU cloud: customers launch **GPU VMs**,
**Jupyter notebooks**, and **model endpoints** on bare-metal GPU servers we operate, in the
region they choose. It is assembled from open-source
infrastructure (k3s/Kubernetes, KubeVirt, NVIDIA GPU Operator, KServe + vLLM, Cilium,
MetalLB, Longhorn → Rook-Ceph, Keycloak, Prometheus/DCGM/Loki). The code we write is the
product layer on top.

**Design assumption:** we sell compute to untrusted outside customers. Every change is
judged against that bar. See [docs/architecture/tenancy-and-isolation.md](docs/architecture/tenancy-and-isolation.md).

## Repo map

| Path | Owns | Language / tooling |
|---|---|---|
| `controlplane/` | Core platform: REST API, controller (reconciles Postgres ↔ Kubernetes), metering, and (planned) the inference gateway. One `tensora` binary with subcommands; one controller per region. | Go 1.27, stdlib `net/http`, pgx, client-go, goose, golangci-lint |
| `ml/` | ML layer: model ingestion + validation, endpoint benchmarking, GPU burn-in. Runs as CLI and as in-cluster Jobs. | Python 3.12, uv, ruff, mypy, pytest |
| `web/` | Customer dashboard | TypeScript, React, Vite, TanStack Query |
| `deploy/` | Cluster add-ons (helmfile), `cluster-config` chart (our CRs), `tensora` chart | Helm, helmfile, Kustomize |
| `infra/` | Bare-metal node bootstrap (OS, IOMMU/VFIO, k3s, WireGuard) | Ansible, bash |
| `images/` | Launchable catalog (`catalog.yaml`), notebook/VM image definitions | Dockerfile, cloud-init |
| `docs/` | Architecture, ADRs, runbooks, playbooks, roadmap, hardware | Markdown |

## Commands

Run from the repo root (`make help` lists everything):

- `make setup` — install dev dependencies for all components
- `make dev` — **all-in-one local stack**: API + controller + metering in one process, in-memory store, fake cluster, seeded tenants. No GPU, Kubernetes, or Docker needed. API on `:8080`.
- `make web-dev` — dashboard on `:5173`, proxied to the local API
- `make test` — all tests (`make test-cp`, `make test-ml`, `make test-web` for one component)
- `make lint` — golangci-lint, ruff, mypy, tsc, yamllint
- `make fmt` — auto-format everything
- `make deploy-check` — render helm charts + kubeconform (no cluster needed)

**Before saying a task is done:** run the lint and test targets for every component you
touched and report the actual output. If a tool is missing, say so — do not claim success.

## Architecture rules (changing one requires an ADR)

1. **Postgres owns business state** (tenants, quotas, catalog choices, workload intent, events, usage). **Kubernetes owns runtime state.** The controller reconciles between them. The API never talks to Kubernetes — it records intent and returns.
2. **All cluster access goes through `cluster.Backend`** (`controlplane/internal/cluster`). Manifests are built by pure functions in `internal/cluster/manifests` and unit-tested as data. No ad-hoc client calls elsewhere.
3. **Tenant resources live in the tenant's namespace** (`t-<slug>`) and carry the labels defined in `internal/cluster/labels.go`. Never place tenant workloads in shared namespaces.
4. **Untrusted code runs in a KubeVirt VM or a Kata container**, never a plain runc container. GPU time-slicing is never shared across tenants.
5. **Billing comes from allocation intervals** (workload lifecycle events), not GPU utilization. DCGM is for observability and idle detection only — passthrough GPUs are invisible to host DCGM anyway.
6. **Tenant model weights must be safetensors; `trust_remote_code` is never enabled** for tenant models. The ML layer's validator (`ml/src/tensora_ml/validate.py`) is the enforcement point.
7. **Every workload state change goes through `service.Transition`**, which validates against `domain` state rules and appends a `workload_events` row. Metering depends on those rows. Never write `state` directly.
8. **Every store query that touches tenant data is scoped by tenant ID.** A request from tenant B for tenant A's object returns 404, never 403 (don't leak existence).
9. **Every workload belongs to exactly one region, and its data never leaves that region** unless the customer explicitly opts in ([ADR-0009](docs/adr/0009-regions-global-control-plane-regional-cells.md)). *This isn't in code yet (roadmap Phase 0). New code must not assume a single region.*
10. **Fallbacks are opt-in and visible** ([ADR-0010](docs/adr/0010-fallbacks-cpu-placement-and-inference-failover.md)). Never silently give a customer a different GPU class, CPU, model, or region. Record `placed_class`, say what served each response, and bill for what actually ran.

## Conventions

- **Go:** `gofmt`/`goimports`; `context.Context` first parameter; wrap errors with `fmt.Errorf("...: %w", err)`; domain errors from `internal/domain/errors.go`; table-driven tests; no package-level mutable state; everything under `internal/`.
- **Python (ml/):** type hints everywhere (mypy strict), ruff, pytest. Heavy deps (`torch`) are optional extras and imported lazily.
- **TypeScript (web/):** strict mode; API types in `web/src/api/types.ts` mirror the Go JSON shapes — update both together.
- **API:** JSON over HTTP, versioned under `/v1`. Errors are `{"error": {"code", "message"}}`. Responses never expose cluster internals (node names, object names, IPs).
- **Config:** environment variables prefixed `TENSORA_` (see `.env.example`). No secrets in code, values files, tests, or docs.
- **Kubernetes/Helm:** pin every chart and image version; never `latest`.
- **Commits:** Conventional Commits — `feat(api): ...`, `fix(deploy): ...`, `docs(adr): ...`.

## Workflows

Step-by-step playbooks live in `docs/playbooks/` (Claude Code exposes them as skills):

- Add an API resource → [docs/playbooks/add-api-resource.md](docs/playbooks/add-api-resource.md)
- Add a cluster add-on → [docs/playbooks/add-platform-component.md](docs/playbooks/add-platform-component.md)
- Record a decision → [docs/playbooks/write-an-adr.md](docs/playbooks/write-an-adr.md)
- Verify a change → [docs/playbooks/verify-changes.md](docs/playbooks/verify-changes.md)

## Safety rules for agents

- **Never mutate real infrastructure** unless the user explicitly asks in the current session: no `kubectl apply|delete|patch|edit|drain`, `helm install|upgrade|uninstall`, `helmfile apply|sync|destroy`, or `ansible-playbook` without `--check`. Rendering, linting, and `--dry-run=client` are always fine.
- **Never read or print secrets:** `.env`, kubeconfigs, `*.key`, `*.pem`, `secrets/`, Ansible vault files.
- Don't add a dependency without stating why; prefer the standard library or what is already in `go.mod` / `pyproject.toml` / `package.json`.
- If a change contradicts an architecture rule, stop and propose an ADR instead.
- Keep docs in sync: if you change behavior described in `docs/architecture/` or a runbook, update it in the same change.

## Where to find things

- What we sell, to whom, and the milestone scope: [docs/product/product-spec.md](docs/product/product-spec.md). Check new features against its milestone tables before building.
- Current phase, scope, and open decisions: [docs/roadmap.md](docs/roadmap.md)
- Why things are the way they are: [docs/adr/](docs/adr/)
- Terms (MIG, VFIO, RawDeployment, cohort, …): [docs/glossary.md](docs/glossary.md)
