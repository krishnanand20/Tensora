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
- [x] Logs and transparency (ADR-0015): a timeline for every workload with a specific reason for each change, complete JSON export (deleted workloads too), secret redaction that is always reported, request IDs on every response and log line, `TENSORA_LOG_FILE`, CLI client commands (`workloads`, `events`, `export`), and a dashboard timeline with **Download full log**
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
| 1 | Store | ✅ **Done.** `sqlitestore` with `modernc.org/sqlite` (pure Go, CGO-free). It passes `storetest`, and migrations live in `migrations/postgres` and `migrations/sqlite`. Select it with `TENSORA_DATABASE_URL=sqlite:<path>`. |
| 2 | Data model | ✅ **Done**, as migration `00002` rather than an edit to `00001`, which had been applied by then; `00003` repairs its backfill (admitted means it ever moved to provisioning) and rebuilds `idempotency_keys` per user. New tables: `machines` (owner, region, OS, architecture, accelerators, sharing level, last seen) and `idempotency_keys` (used from Phase 2). `workloads` gains `region`, `machine_id`, `placed_class`, `fallback`, and `fallback_after_s`; `gpu_class` stays the requested class (it's the API's name for it). `usage_records` gains `region` and `machine_id`, and its `gpu_class` is now the class that actually ran, which covers what `compute_class` was meant to do. |
| 3 | Domain | ✅ **Done.** `Region`, accelerator kinds (`nvidia`, `apple`, `amd`, `cpu`), sharing levels, `Machine` with validation, and placement fields on `Workload`. Tested: a workload can't enter a metered state before it's placed (`Transition` refuses), and a machine needs exactly one valid region. The controller records placement in the same transaction as admission, capacity is counted by placed class, and usage is accounted on the placed class and region. |
| 4 | Catalog | ✅ **Done.** A `regions:` list (the first is the default; Solo has `local`). Classes have an `accelerator` and optional `regions`. Apple Silicon classes by unified-memory tier (`apple-silicon-32gb`; owners only, rule 4) and CPU classes (`cpu-4c-16g`, `cpu-16c-64g`). Images list runtimes in order: the endpoint image (`openai-endpoint`) uses vLLM on NVIDIA, MLX on Apple, and llama.cpp on anything. Create takes `region`; the controller reconciles one region (`TENSORA_REGION`). |
| 5 | Auth | ✅ **Done.** `TENSORA_AUTH_MODE=local`: accounts with argon2id passwords; dashboard sessions (HttpOnly, SameSite=Strict cookie, CSRF header, 7 days); API tokens (`tsk_…`, shown once, stored as hashes, expiring or not, revocable); guessing limited per email-and-address pair and per address; passwords and cookies only over HTTPS or this machine (`TENSORA_TRUSTED_PROXIES`, `TENSORA_INSECURE_HTTP`); tokens can't make tokens or change the password; timelines name the credential; and `tensora setup` (with `-reset-password`) for the owner account. The dashboard's own first-run page arrives with `tensora up` (step 8). |
| 6 | Agent | The agent protocol: mTLS, enrolment tokens, heartbeat, inventory, desired-state sync. Add the `agent` implementation of `cluster.Backend`, with an in-process agent for Solo. A machine's region comes from its enrolment token, never from what the agent reports. Choose the machine (with `Machine.CanHost`, extended to weigh the runtime's isolation) before applying anything. |
| 7 | Drivers | `macos-native`: llama.cpp and MLX servers, and Jupyter in a uv-managed environment, all under a sandbox profile. `docker` (Linux): detect the NVIDIA toolkit and fall back to CPU. Driver code lives behind build tags. Workload web UIs (Jupyter, endpoint UIs) are served from their own hostname, never the dashboard's origin, so code in a workload can't act with the user's session ([tenancy](architecture/tenancy-and-isolation.md)). |
| 8 | `tensora up` | Data directory per OS, embedded dashboard (`go:embed` of `web/dist`), binds to localhost, first-run account |
| 9 | Web | Machines page; region and placed class on each workload; fallback opt-in on the launch form. No money anywhere. |
| 10 | ML | GGUF validation in `validate.py`; an MPS (Metal) path for `burnin`; bench docs for llama.cpp and MLX servers |
| 11 | Packaging | Homebrew tap, `install.sh`, `.deb` and `.rpm` packages, launchd and systemd units, signed and notarized macOS builds, a release workflow |
| 12 | Docs | Turn [runbook 00](runbooks/00-quickstart.md) from planned into verified; update the control-plane.md API tables; remove "planned" notes as each item lands |
| 13 | CLI | Write commands: `tensora launch`, `stop`, `start`, `delete`, and `catalog`, with `-json` output. The read commands (`workloads`, `events`, `export`) are built. |
| 14 | Logs | Workload output (ADR-0015): drivers capture stdout and stderr, and VM boot logs. The agent ships them to the control plane, which keeps them per workload for 14 days. Add `GET /v1/workloads/{id}/logs` (with follow), `tensora logs <id> --follow`, a Logs tab in the dashboard, and `output_logs` in the export. |

## Phase 2: Lab (→ M2)

**Done when:** a class or small team runs one instance across mixed Mac and Linux machines, with
users, quotas, VMs, fallbacks, and endpoint API keys.

- [ ] Agents join over the network: outbound mTLS (works behind NAT); enrolment from the dashboard; machine health
- [ ] Users, invites, teams, and per-user quotas; usage reports (no money)
- [ ] Activity log per tenant (ADR-0015): every API action by every user, with its request ID; tenant admins can read and export it
- [ ] VMs: `libvirt` driver (Linux KVM, with VFIO GPU passthrough) and `macos-vz` driver (macOS guests with Metal, max 2 per Mac; Linux guests CPU-only)
- [ ] `macos-container` driver (a Linux container in its own lightweight VM) for strong-isolation CPU workloads on Macs
- [ ] Fallback placement across machines and to CPU (ADR-0010); `Idempotency-Key` on POST; `Retry-After` on 503
- [ ] Built-in inference gateway: endpoint API keys, token counts, failover to another replica or CPU
- [ ] Access to workloads from outside the LAN through the control plane's relay (notebook UI, endpoint API, VM SSH)
- [ ] Python SDK; Postgres option for larger Labs; `tensora backup` and `restore`, portable between SQLite and Postgres (moving up a profile)

## Phase 3: Datacenter (→ M3)

**Done when:** the Kubernetes profile runs in production on real servers, with VMs, endpoints, SSO,
regions, and HA, and its install docs are verified.

- [ ] One Linux NVIDIA server: bootstrap ([runbook 01](runbooks/01-node-bootstrap.md)), passthrough and burn-in ([runbook 02](runbooks/02-gpu-passthrough.md)), first endpoint ([runbook 03](runbooks/03-first-model-endpoint.md))
- [ ] Verify and record every pinned chart version (deploy/README.md)
- [ ] Kubernetes backend end to end, plus an orphan sweep (regional controllers, `TENSORA_REGION`, are built)
- [ ] **Test GPU memory remanence** between VMs (tenancy-and-isolation.md, open risks)
- [ ] **Validate Kata + GPU passthrough** (needed before untrusted notebooks and endpoints run in containers)
- [ ] Models resource, with `tensora-ml ingest` running as a Job
- [ ] More nodes; 3 k3s servers; Rook-Ceph; object store; Postgres HA (CloudNativePG); backups and restore drills
- [ ] SSO through Keycloak or the company's IdP; RBAC; the activity log kept in Postgres with configurable retention
- [ ] Workload output logs from Loki (Alloy ships pod logs and VM consoles), served by the API per tenant (ADR-0015)
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
| Agent protocol | gRPC streams, or HTTPS with WebSocket. Either way the agent connects outbound with mTLS. | Phase 1 |
| Sandbox for native macOS workloads | A macOS sandbox profile per workload, for trusted users only | Phase 1 |
| Linux containers on Mac | Apple's containerization framework, or Lima | Phase 2 |
| Remote access to workloads | A relay through the control plane (Solo and Lab); in a datacenter, a shared IP with a port per VM | Phase 2 |
| Inference gateway implementation | Built into the Go control plane (Lab); Envoy-based or Go in a datacenter | Phase 2 |
| Datacenter object store | SeaweedFS or Garage first, then Ceph RGW | Phase 3 |
| Untrusted notebooks in a datacenter | VMs until Kata + GPU is validated | Phase 3 |
| Does the project run a public community pool? | Not decided; anyone can run one | Phase 4 |
