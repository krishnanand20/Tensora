# Roadmap (development plan)

This is the engineering plan. **What** each milestone must deliver is defined in the
[product spec](product/product-spec.md#milestones). The order follows the product: one machine first
(a Mac or a Linux box), then many machines, then datacenters, then sharing.

| Phase | Delivers | Needs hardware |
|---|---|---|
| 0. Foundations | M0 ✓ and the redesign | — |
| 1. Solo | M1: install on one Mac or Linux machine and use its GPU | Your own Mac or PC |
| 2. Lab | M2: many mixed machines, many users | 2+ of your own machines |
| 3. Datacenter | M3: the Kubernetes profile | One Linux NVIDIA server (borrowed is fine), more later |
| 4. Sharing | M4: invite-only sharing and community pools | Any |
| 5. Ecosystem | M5: Windows, AMD, serverless API across pools | Any |

## Phase 0: Foundations ← *we are here*

- [x] Repo structure, agent instructions (AGENTS.md and CLAUDE.md), CI, and docs
- [x] Core platform skeleton (Go): API, controller, usage accounting, lifecycle, quotas, fake and Kubernetes backends
- [x] It already builds and runs on macOS Apple Silicon: `make dev` runs there, and `make build-all` cross-compiles for linux/amd64, linux/arm64, and darwin/arm64
- [x] ML layer (Python): model validator, ingestion, benchmarking, burn-in (CUDA)
- [x] Dashboard skeleton (TypeScript)
- [x] Redesign: free for everyone (ADR-0011), profiles (ADR-0012), node agent and Apple Silicon (ADR-0013), sharing (ADR-0014), regions (ADR-0009), fallbacks (ADR-0010)
- [ ] **Choose the final product name** and clear it: trademark search in your main markets (classes 9 and 42), plus domain and GitHub org. Several companies already use "Tensora".
- [ ] **Rename in one change** once the name is final: Go module path, `tensora.io/` labels (use a domain the project owns), `TENSORA_` env vars, `X-Tensora-*` headers, binary, charts, docs
- [ ] **Choose the open-source licence** (Apache-2.0 recommended) and add `LICENSE`
- [ ] Set the image registry (`ghcr.io/your-org` is a placeholder)

## Phase 1: Solo (→ M1)

**Done when:** on a fresh Apple Silicon Mac and a fresh Linux box, one command installs Tensora,
`tensora up` opens the dashboard, and a notebook and a model endpoint run on the local GPU (Metal or
CUDA), or on CPU, from the dashboard, CLI, or API.

### Task breakdown: the Solo foundation (next code change)

Each step keeps `make lint test deploy-check build-all` green. Do the data-model steps before any real data exists.

| # | Area | Task |
|---|---|---|
| 1 | Store | `sqlitestore`, using a pure-Go SQLite driver so builds stay CGO-free. It must pass `storetest`. Run goose migrations for both SQLite and Postgres. |
| 2 | Data model | Amend `00001_init.sql` (never applied anywhere). Add a `machines` table: owner, region, OS, architecture, accelerators (JSON), sharing level, last seen. `workloads` gains `region`, `machine_id`, `requested_class`, `placed_class`, `fallback`, and `fallback_after_s`. `usage_records` gains `region`, `machine_id`, and `compute_class`. Add an `idempotency_keys` table. |
| 3 | Domain | Accelerator kinds (`nvidia`, `apple`, `amd`, `cpu`), `Region`, and placement fields. Tests: a metered workload must have a `placed_class`, and a machine must belong to one region. |
| 4 | Catalog | A `regions:` list (Solo has one, `local`). Classes gain `accelerator` and `regions`. Add Apple Silicon and CPU classes. Each image lists a runtime per platform: vLLM (NVIDIA), MLX (Apple), llama.cpp (any). |
| 5 | Auth | `local` mode: built-in accounts with password hashing, API tokens, and a first-run setup |
| 6 | Agent | The agent protocol: mTLS, enrolment tokens, heartbeat, inventory, desired-state sync. Add the `agent` implementation of `cluster.Backend`, with an in-process agent for Solo. |
| 7 | Drivers | `macos-native`: llama.cpp and MLX servers, and Jupyter in a uv-managed environment, all under a sandbox profile. `docker` (Linux): detect the NVIDIA toolkit and fall back to CPU. Driver code lives behind build tags. |
| 8 | `tensora up` | Data directory per OS, embedded dashboard (`go:embed` of `web/dist`), binds to localhost, first-run account |
| 9 | Web | Machines page; region and placed class on each workload; fallback opt-in on the launch form. No money anywhere. |
| 10 | ML | GGUF validation in `validate.py`; an MPS (Metal) path for `burnin`; bench docs for llama.cpp and MLX servers |
| 11 | Packaging | Homebrew tap, `install.sh`, `.deb` and `.rpm` packages, launchd and systemd units, signed and notarized macOS builds, a release workflow |
| 12 | Docs | Turn [runbook 00](runbooks/00-quickstart.md) from planned into verified; update the control-plane.md API tables; remove "planned" notes as each item lands |

## Phase 2: Lab (→ M2)

**Done when:** a class or small team runs one instance across mixed Mac and Linux machines, with
users, quotas, VMs, fallbacks, and endpoint API keys.

- [ ] Agents join over the network: outbound mTLS (works behind NAT); enrolment from the dashboard; machine health
- [ ] Users, invites, teams, and per-user quotas; usage reports (no money)
- [ ] VMs: `libvirt` driver (Linux KVM, with VFIO GPU passthrough) and `macos-vz` driver (macOS guests with Metal, max 2 per Mac; Linux guests CPU-only)
- [ ] `macos-container` driver (a Linux container in its own lightweight VM) for strong-isolation CPU workloads on Macs
- [ ] Fallback placement across machines and to CPU (ADR-0010); `Idempotency-Key` on POST; `Retry-After` on 503
- [ ] Built-in inference gateway: endpoint API keys, token counts, failover to another replica or CPU
- [ ] Access to workloads from outside the LAN through the control plane's relay (notebook UI, endpoint API, VM SSH)
- [ ] `tensora` CLI and Python SDK; Postgres option for larger Labs; `tensora export` and `import`

## Phase 3: Datacenter (→ M3)

**Done when:** the Kubernetes profile runs in production on real servers, with VMs, endpoints, SSO,
regions, and HA, and its install docs are verified.

- [ ] One Linux NVIDIA server: bootstrap ([runbook 01](runbooks/01-node-bootstrap.md)), passthrough and burn-in ([runbook 02](runbooks/02-gpu-passthrough.md)), first endpoint ([runbook 03](runbooks/03-first-model-endpoint.md))
- [ ] Verify and record every pinned chart version (deploy/README.md)
- [ ] Kubernetes backend end to end, plus an orphan sweep; regions in the controller (`TENSORA_REGION`)
- [ ] **Test GPU memory remanence** between VMs (tenancy-and-isolation.md, open risks)
- [ ] **Validate Kata + GPU passthrough** (needed before untrusted notebooks and endpoints run in containers)
- [ ] Models resource, with `tensora-ml ingest` running as a Job
- [ ] More nodes; 3 k3s servers; Rook-Ceph; object store; Postgres HA (CloudNativePG); backups and restore drills
- [ ] SSO through Keycloak or the company's IdP; audit logs; RBAC
- [ ] Regional inference gateway with autoscaling; Prometheus `/metrics` and alerts; host firewall (admin access only over WireGuard); upgrade runbook
- [ ] Controller leader election; usage-accounting correction for workloads that died while the controller was down

## Phase 4: Sharing (→ M4)

**Done when:** owners can safely lend idle machines to invited people, and anyone can run a
community pool with fair share and abuse tools.

- [ ] Sharing levels per machine (private, invite-only, community) and the trust matrix enforced at admission
- [ ] Idle detection (macOS IOKit, Linux logind), schedules, AC-power checks, and resource caps
- [ ] Preemption: a `preempted` state, instant reclaim, and re-queueing through fallbacks
- [ ] Managed inference for community users on hosts without strong isolation (Macs)
- [ ] Community workloads with no internet egress by default; models delivered through the pool's model store
- [ ] Verified accounts, fair-share queues, rate limits, reports and bans, and an AUP template
- [ ] Clear disclosure in the product that host owners can see guest data; contributed-hours reports for hosts
- [ ] Host reputation, and an optional directory of public pools

## Phase 5: Ecosystem (→ M5)

- [ ] Windows agent (through WSL2, with CUDA)
- [ ] AMD GPUs (ROCm)
- [ ] Serverless model API across a pool
- [ ] Terraform provider; templates gallery; LAN auto-discovery; upgrades without downtime
- [ ] Batch and fine-tuning jobs; MIG-backed classes; SCIM

## Open decisions

| Decision | Current assumption | Needed by |
|---|---|---|
| **Final product name** | "Tensora" is a working name; several companies already use it | Before the first release |
| **Open-source licence** | Apache-2.0 recommended | Before the first release |
| SQLite driver | A pure-Go driver, to keep cross-compiles CGO-free | Phase 1 |
| Agent protocol | gRPC streams, or HTTPS with WebSocket. Either way the agent connects outbound with mTLS. | Phase 1 |
| Sandbox for native macOS workloads | A macOS sandbox profile per workload, for trusted users only | Phase 1 |
| Linux containers on Mac | Apple's containerization framework, or Lima | Phase 2 |
| Remote access to workloads | A relay through the control plane (Solo and Lab); in a datacenter, a shared IP with a port per VM | Phase 2 |
| Inference gateway implementation | Built into the Go control plane (Lab); Envoy-based or Go in a datacenter | Phase 2 |
| Datacenter object store | SeaweedFS or Garage first, then Ceph RGW | Phase 3 |
| Untrusted notebooks in a datacenter | VMs until Kata + GPU is validated | Phase 3 |
| Does the project run a public community pool? | Not decided; anyone can run one | Phase 4 |
