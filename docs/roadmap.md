# Roadmap

The phases follow the build order: one node, then many nodes, then the product, then production.
The control plane can be built in parallel with Phase 1 and 2, because `make dev` needs no hardware.

The **product milestones** (M1 lab, M2 private beta, M3 public launch, M4 growth, M5 enterprise) and
what each must include are defined in [product/product-spec.md](product/product-spec.md#milestones).
These phases are the engineering work that gets us there.

## Phase 0: Foundations ← *we are here*

- [x] Repo structure, agent instructions (AGENTS.md and CLAUDE.md), CI, and docs
- [x] Core platform skeleton (Go): API, controller, metering, lifecycle, quotas, fake cluster, real cluster backend
- [x] ML layer (Python): model validator, ingestion, benchmarking, burn-in
- [x] Dashboard skeleton (TypeScript)
- [ ] **Confirm the design assumptions** (see the open decisions below)
- [ ] Choose hardware and get quotes ([bill-of-materials.md](hardware/bill-of-materials.md))
- [ ] **Choose the final product name** and clear it: a trademark search in your launch markets (classes 9 and 42), plus domain and GitHub org availability. Several companies already use "Tensora".
- [ ] **Rename in one change** once the name is final: Go module path, `tensora.io/` labels (use a domain we own), `TENSORA_` env vars, `X-Tensora-*` headers, binary, charts, docs. Labels, env vars, and headers are hard to change after the first deployment.
- [ ] Set the image registry (`ghcr.io/your-org` is a placeholder) and choose a licence
- [ ] **Add regions and fallback placement** to the data model, API, catalog, controller, and dashboard (ADR-0009, ADR-0010), before any real data exists

## Phase 1: Single node

**Done when:** a passthrough-GPU VM is reachable over SSH, a vLLM endpoint answers chat completions, and the node can be rebuilt from scratch with Ansible and helmfile.

- [ ] Rack the node, set the BIOS, install Ubuntu 24.04 ([runbook 01](runbooks/01-node-bootstrap.md))
- [ ] Ansible bootstrap, then `helmfile -e single-node apply`; verify and record the pinned versions
- [ ] Container mode: GPU smoke test and `tensora-ml burnin` passes
- [ ] Passthrough mode: first VM ([runbook 02](runbooks/02-gpu-passthrough.md)); confirm the PSA baseline works for virt-launcher
- [ ] **Test GPU memory remanence** between VMs (tenancy-and-isolation.md, open risks)
- [ ] First endpoint ([runbook 03](runbooks/03-first-model-endpoint.md)), benchmarked with `tensora-ml bench`
- [ ] Choose and deploy the object store (ADR-0005)
- [ ] Postgres backups off-node

## Phase 2: Multi-node

**Done when:** losing any one node loses no data and no control plane, and alerts fire for the important failures.

- [ ] Nodes 2 and 3 ([add-node](runbooks/add-node.md)); 3 k3s servers with an API VIP
- [ ] Rook-Ceph; `tensora-block` moves to RBD; object store moves to RGW
- [ ] Observability alerts wired to on-call ([observability.md](architecture/observability.md)); BMC/IPMI exporter
- [ ] etcd snapshots and Postgres WAL archiving to S3; restore drill
- [ ] Host firewall: SSH and the Kubernetes API only via WireGuard
- [ ] Upgrade runbook (k3s, GPU driver and Operator, KubeVirt) with drain procedures

## Phase 3: Platform

**Done when:** an invited external user can sign up, launch each workload kind from the dashboard, use it, and see their usage.

- [ ] Run the controller against the real cluster end to end (kube backend), plus an orphan sweep
- [ ] Keycloak + OIDC in the API (already supported) and in the dashboard (PKCE)
- [ ] **Validate Kata + GPU passthrough** (the gate for untrusted notebooks and endpoints)
- [ ] Models resource, with `tensora-ml ingest` running as a Job
- [ ] Access: endpoint routes with API keys, a notebook auth proxy, VM SSH exposure (networking.md)
- [ ] `GET /v1/usage`, and a usage page in the dashboard
- [ ] Control plane `/metrics`; admission aging; prebaked VM images
- [ ] Regional **inference gateway**: API keys, token metering, rate limits, and per-request failover (ADR-0010)
- [ ] CPU worker nodes, CPU classes in the catalog, and the `tensora-vllm-cpu` runtime (ADR-0010)
- [ ] `Idempotency-Key` support on POST, and `Retry-After` on every 503

## Phase 4: Production

**Done when:** we can take money, keep promises, and survive an audit.

- [ ] HA: controller leader election, Postgres HA (CloudNativePG), several API replicas
- [ ] Billing: per-region price book in USD, prepaid credits, tax-compliant invoices (GST, VAT, sales tax), Stripe worldwide and Razorpay in India, spend limits, failure credits
- [ ] ToS, acceptable use policy, privacy policy (GDPR, DPDP, CCPA), abuse handling, KYC, and sanctions and export-control screening
- [ ] Encryption at rest, audit logging, external penetration test
- [ ] Status page, SLA definitions, incident process exercised
- [ ] MIG-backed small GPU classes (behind Kata)

## Open decisions

| Decision | Current assumption | Needed by | Owner |
|---|---|---|---|
| ~~Who are the customers?~~ | **Decided 2026-10-23:** all segments, staged: self-serve, then growing companies, then enterprise ([product spec](product/product-spec.md#customers-in-stages)) | — | founder |
| ~~Flagship product~~ | **Decided 2026-10-23:** GPU VMs + notebooks + inference at launch | — | founder |
| GPU model and count to start | Undecided. Plan for 1–2 servers (4× L40S each) and design to grow. | Before M1 | founder |
| How to fund the hardware | Options in the [product spec](product/product-spec.md#running-this-solo-with-ai-agents): revenue-funded single server, datacenter partnership, private-cloud licensing | Before M1 | founder |
| On-premises versus colocation | Colocation recommended for selling (power, cooling, connectivity) | Phase 0 | — |
| Phase 1 object store | SeaweedFS or Garage | Phase 1 | — |
| Notebooks for untrusted users | VMs until Kata + GPU is validated | Phase 3 | — |
| VM SSH exposure | Shared IP with a port per VM | Phase 3 | — |
| **Final product name** | "Tensora" is a working name; several companies already use it | Before M1 (before anything is deployed) | founder + trademark counsel |
| ~~Market~~ | **Decided 2026-10-23:** worldwide; India is the first region (ADR-0009) | — | founder |
| ~~Pricing currency~~ | **Decided 2026-10-23:** USD everywhere | — | founder |
| Second region | Decide from signup and latency data (EU, US, Middle East, or Southeast Asia) | M4 | founder |
| Legal entity and where to invoice from | — | Before M3 | founder + counsel |
| Payment providers | Stripe worldwide, plus Razorpay for UPI in India | Before M3 | founder |
| Inference gateway implementation | Envoy-based or a small Go proxy (ADR-0010) | Phase 3 | — |
| Licence for this repository | — | Phase 0 | — |
