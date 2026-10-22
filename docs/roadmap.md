# Roadmap (development plan)

This is the engineering plan. **What** each milestone must deliver to customers is defined in the
[product spec](product/product-spec.md#milestones). The phases below are the work that gets us there.
The control plane can be built in parallel with the hardware phases, because `make dev` needs no hardware.

| Phase | Delivers | Runs alongside |
|---|---|---|
| 0. Foundations | M0 ✓, plus the regions and fallback code | — |
| 1. First region, single node | M1 lab | Phase 3 |
| 2. Multi-node in the first region | The reliability part of M3 | Phase 3 |
| 3. Platform | M2 private beta | Phases 1 and 2 |
| 4. Production | M3 public launch | — |
| 5. Growth | M4: second region, serverless API | — |
| 6. Enterprise | M5 | — |

## Phase 0: Foundations ← *we are here*

- [x] Repo structure, agent instructions (AGENTS.md and CLAUDE.md), CI, and docs
- [x] Core platform skeleton (Go): API, controller, metering, lifecycle, quotas, fake cluster, real cluster backend
- [x] ML layer (Python): model validator, ingestion, benchmarking, burn-in
- [x] Dashboard skeleton (TypeScript)
- [x] Product definition: worldwide market, USD pricing, staged customers, launch product mix ([product spec](product/product-spec.md))
- [x] Design: regions ([ADR-0009](adr/0009-regions-global-control-plane-regional-cells.md)) and fallbacks ([ADR-0010](adr/0010-fallbacks-cpu-placement-and-inference-failover.md))
- [ ] **Choose the final product name** and clear it: trademark search in your launch markets (classes 9 and 42), plus domain and GitHub org. Several companies already use "Tensora".
- [ ] **Rename in one change** once the name is final: Go module path, `tensora.io/` labels (use a domain we own), `TENSORA_` env vars, `X-Tensora-*` headers, binary, charts, docs. These are hard to change after the first deployment.
- [ ] Set the image registry (`ghcr.io/your-org` is a placeholder) and choose a licence
- [ ] Choose first-region hardware and get quotes ([bill-of-materials.md](hardware/bill-of-materials.md))
- [ ] **Regions and fallback placement in code**: see the task breakdown below

### Task breakdown: regions and fallback placement

One change, or a short series, **before any real data exists**. Each step keeps `make lint test deploy-check` green.

| # | Area | Task |
|---|---|---|
| 1 | Migration | Amend `00001_init.sql`, which has never been applied anywhere. `workloads` gets `region`, `requested_class`, `placed_class` (nullable until admitted), `fallback` (jsonb list), and `fallback_after_s`. `usage_records` gets `region`, and `gpu_class` becomes `compute_class` (the placed class). Add an `idempotency_keys` table. |
| 2 | Domain | Add `Region`, `RequestedClass`, `PlacedClass`, `Fallback`, and `FallbackAfter` to `Workload`, plus `ValidRegionID`. Tests: a billable workload must have a `placed_class`. |
| 3 | Catalog | Add a `regions:` list (id, display name, country). Classes gain `kind: gpu|cpu` and `regions: [...]`. Add CPU classes (for example `cpu-16c-64g`). Validation, plus a test that loads the real catalog. |
| 4 | Config | Add `TENSORA_REGION` (required for `controller` with the kubernetes backend; `dev` defaults to the first catalog region). Update `.env.example`. |
| 5 | Store | `WorkloadsInStates(region, …)` and `GPUsAllocated(region)`, in both stores. New `storetest` case: one region never sees another's workloads. |
| 6 | Service | Create input gains `region`, `fallback`, and `fallback_after`. Validate that the class is offered in the region, that fallbacks exist there, and that CPU fallback is only for allowed kinds. Workloads are created with `requested_class`. |
| 7 | Controller | Region filter. Admission tries the requested class, then each fallback once `fallback_after` has passed, records `placed_class` with a reason in the event, and gives freed GPUs to requested-class waiters first. Tests cover sold out → CPU placement, and that requested-class waiters win. |
| 8 | Manifests | CPU variants: no GPU resources, a CPU node selector, VMs without `gpus`, endpoints on the `tensora-vllm-cpu` runtime. Add `kube.allowed` and RBAC entries if any new kinds appear. |
| 9 | Metering | Usage keyed by `placed_class` and `region`. Test that a CPU fallback is billed as CPU. |
| 10 | API | Views gain `region`, `requested_class`, and `placed_class`. The catalog view lists regions and class kinds. `Idempotency-Key` on POST, `Retry-After` on 503. Cross-tenant and cross-region tests. |
| 11 | Web | Region selector and a fallback option on the launch form; show the placed class and a "fallback" badge; update `types.ts`. |
| 12 | Deploy | `tensora-vllm-cpu` ClusterServingRuntime in cluster-config; `region:` in environment files; `TENSORA_REGION` in the tensora chart. |
| 13 | Docs | control-plane.md (API and lifecycle), the `controlplane/README.md` API table, and remove the "not yet in code" notes from AGENTS.md rules 9 and 10 and from ADR-0009 and ADR-0010. |

## Phase 1: First region, single node (→ M1)

**Done when:** in the first region (India), a passthrough-GPU VM is reachable over SSH, a vLLM endpoint
answers, a CPU-placed workload runs, and the node can be rebuilt from scratch with Ansible and helmfile.

- [ ] Pick the region ID (for example `in-mum-1`), the site, and colocation; plan its network ranges ([networking.md](architecture/networking.md))
- [ ] Rack the node, set the BIOS, install Ubuntu 24.04 ([runbook 01](runbooks/01-node-bootstrap.md))
- [ ] Ansible bootstrap, then `helmfile -e single-node apply`; verify and record the pinned versions
- [ ] Container mode: GPU smoke test, and `tensora-ml burnin` passes
- [ ] Passthrough mode: first VM ([runbook 02](runbooks/02-gpu-passthrough.md)); confirm the PSA baseline works for virt-launcher
- [ ] **Test GPU memory remanence** between VMs (see tenancy-and-isolation.md, open risks)
- [ ] First endpoint ([runbook 03](runbooks/03-first-model-endpoint.md)), benchmarked with `tensora-ml bench`
- [ ] CPU placement works on the node's spare cores (CPU class with no GPU)
- [ ] Choose and deploy the object store (ADR-0005)
- [ ] Postgres backups off-node

## Phase 2: Multi-node in the first region (→ reliability for M3)

**Done when:** losing any one node loses no data and no control plane, and alerts fire for the important failures.

- [ ] Nodes 2 and 3 ([add-node](runbooks/add-node.md)); 3 k3s servers with an API VIP
- [ ] Small **CPU servers for the control plane**, so a GPU node failure is never a platform outage
- [ ] **CPU worker nodes** for CPU instances and fallback (ADR-0010)
- [ ] Rook-Ceph; `tensora-block` moves to RBD; object store moves to RGW
- [ ] Observability alerts wired to on-call ([observability.md](architecture/observability.md)); BMC/IPMI exporter
- [ ] etcd snapshots and Postgres WAL archiving; a restore drill
- [ ] Host firewall: SSH and the Kubernetes API only via WireGuard
- [ ] Upgrade runbook (k3s, GPU driver and Operator, KubeVirt) with drain procedures

## Phase 3: Platform (→ M2 private beta)

**Done when:** an invited user anywhere in the world can sign up, launch each workload kind from the
dashboard in the first region, use it, and see their usage.

- [ ] Run the controller against the real cluster end to end (kube backend), plus an orphan sweep
- [ ] Keycloak + OIDC in the API (already supported) and in the dashboard (PKCE)
- [ ] **Organizations, members, roles, and API keys** (global)
- [ ] **Validate Kata + GPU passthrough** (the gate for untrusted notebooks and endpoints)
- [ ] Models resource, with `tensora-ml ingest` running as a Job; it records model size for CPU-fallback eligibility
- [ ] Access: endpoint routes, a notebook auth proxy, VM SSH exposure ([networking.md](architecture/networking.md))
- [ ] Regional **inference gateway**: API keys, token metering, rate limits, and failover steps 1–2 (another replica, then CPU) (ADR-0010)
- [ ] `tensora-vllm-cpu` in production; automatic promotion of CPU-fallback endpoints back to GPU
- [ ] `GET /v1/usage`, and a usage page in the dashboard
- [ ] `tensora` CLI and Python SDK (minimal: auth, launch, list, stop, logs), with retries and idempotency keys
- [ ] Control plane `/metrics` (including fallback rate); admission aging; prebaked VM images
- [ ] Status page per region, public docs site, and beta ToS, AUP, and privacy policy

## Phase 4: Production (→ M3 public launch)

**Done when:** anyone eligible worldwide can sign up, pay in USD, and rely on an SLA.

- [ ] HA: controller leader election, Postgres HA (CloudNativePG), several API replicas
- [ ] Billing: per-region price book in USD, prepaid credits, Stripe worldwide and Razorpay for UPI (converted at checkout), tax-compliant invoices (GST, VAT, sales tax), spend limits, failure credits
- [ ] Legal entity and invoicing jurisdiction; ToS, AUP, and privacy policy (GDPR, DPDP, CCPA)
- [ ] KYC, plus **sanctions and export-control screening** before GPU access
- [ ] Encryption at rest, audit logging, external penetration test
- [ ] 99.5% SLA defined and monitored; incident process exercised

## Phase 5: Growth (→ M4)

**Done when:** a second region is live and the serverless model API is selling.

- [ ] **Second region:** chosen from signup and latency data; colocation, hardware, network ranges, local legal and tax registration
- [ ] Split the global control plane from the first region's cell (ADR-0009); add a regional controller, object store, and observability per region; regional DNS
- [ ] **Serverless model API**: shared pools, per-token billing, gateway failover steps 3–4 (serverless pool, fallback models)
- [ ] Custom container images (Kata), snapshots, reserved instances, team budgets
- [ ] More GPU types, including MIG-backed small classes behind Kata
- [ ] 99.9% SLA on endpoints; Terraform provider; webhooks

## Phase 6: Enterprise (→ M5)

- [ ] SSO (SAML/OIDC), SCIM, audit log export
- [ ] Dedicated nodes and clusters; sovereign and partner regions; private networking
- [ ] Opt-in **cross-region failover** (gateway step 5)
- [ ] In-region storage of personal data for strict-residency customers (ADR-0009)
- [ ] Postpaid contracts and PO invoicing
- [ ] SOC 2 Type II and ISO 27001
- [ ] Tensora Private Cloud edition (licensable)

## Open decisions

| Decision | Current assumption | Needed by | Owner |
|---|---|---|---|
| **Final product name** | "Tensora" is a working name; several companies already use it | Before M1 (before anything is deployed) | founder + trademark counsel |
| ~~Who are the customers?~~ | **Decided 2026-10-23:** all segments, staged: self-serve, then growing companies, then enterprise | — | founder |
| ~~Flagship product~~ | **Decided 2026-10-23:** GPU VMs + notebooks + inference at launch | — | founder |
| ~~Market~~ | **Decided 2026-10-23:** worldwide; India is the first region (ADR-0009) | — | founder |
| ~~Pricing currency~~ | **Decided 2026-10-23:** USD everywhere | — | founder |
| ~~Fallbacks~~ | **Decided 2026-10-23:** opt-in CPU or other-class placement, and per-request failover (ADR-0010) | — | founder |
| GPU model and count to start | Undecided. Plan for 1–2 servers (4× L40S each) and design to grow. | Before M1 | founder |
| How to fund the hardware | Revenue-funded single server, partner regions, or private-cloud licensing ([product spec](product/product-spec.md#running-this-solo-with-ai-agents)) | Before M1 | founder |
| On-premises versus colocation | Colocation recommended for selling (power, cooling, connectivity) | Before M1 | founder |
| Phase 1 object store | SeaweedFS or Garage | Phase 1 | — |
| Notebooks for untrusted users | VMs until Kata + GPU is validated | Phase 3 | — |
| VM SSH exposure | Shared IP with a port per VM | Phase 3 | — |
| Inference gateway implementation | Envoy-based or a small Go proxy (ADR-0010) | Phase 3 | — |
| Legal entity and where to invoice from | — | Before M3 | founder + counsel |
| Payment providers | Stripe worldwide, plus Razorpay for UPI in India | Before M3 | founder |
| Second region | Decide from signup and latency data (EU, US, Middle East, or Southeast Asia) | M4 | founder |
| Licence for this repository | — | Phase 0 | founder |
