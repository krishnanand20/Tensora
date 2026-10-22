# Roadmap

The phases follow the build order: one node, then many nodes, then the product, then production.
The control plane can be built in parallel with Phase 1 and 2, because `make dev` needs no hardware.

## Phase 0: Foundations ← *we are here*

- [x] Repo structure, agent instructions (AGENTS.md and CLAUDE.md), CI, and docs
- [x] Core platform skeleton (Go): API, controller, metering, lifecycle, quotas, fake cluster, real cluster backend
- [x] ML layer (Python): model validator, ingestion, benchmarking, burn-in
- [x] Dashboard skeleton (TypeScript)
- [ ] **Confirm the design assumptions** (see the open decisions below)
- [ ] Choose hardware and get quotes ([bill-of-materials.md](hardware/bill-of-materials.md))
- [ ] Set the Go module path and image registry (`github.com/tensora/tensora`, `ghcr.io/your-org` are placeholders), and choose a licence

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

## Phase 4: Production

**Done when:** we can take money, keep promises, and survive an audit.

- [ ] HA: controller leader election, Postgres HA (CloudNativePG), several API replicas
- [ ] Billing: price book, prepaid credits, invoices with GST, Razorpay or Stripe, spend limits, failure credits
- [ ] ToS, acceptable use policy, privacy policy (DPDP Act), abuse handling process, KYC for paid accounts
- [ ] Encryption at rest, audit logging, external penetration test
- [ ] Status page, SLA definitions, incident process exercised
- [ ] MIG-backed small GPU classes (behind Kata)

## Open decisions

| Decision | Current assumption | Needed by | Owner |
|---|---|---|---|
| **Who are the customers?** Own team, or selling to outsiders? | Selling to external, untrusted customers | Phase 0 | — |
| GPU model and count to start | 1 node with 4× L40S | Phase 0 | — |
| On-premises versus colocation | Colocation recommended for selling (power, cooling, connectivity) | Phase 0 | — |
| Phase 1 object store | SeaweedFS or Garage | Phase 1 | — |
| Notebooks for untrusted users | VMs until Kata + GPU is validated | Phase 3 | — |
| VM SSH exposure | Shared IP with a port per VM | Phase 3 | — |
| Payment provider | Razorpay | Phase 4 | — |
| Licence for this repository | — | Phase 0 | — |
