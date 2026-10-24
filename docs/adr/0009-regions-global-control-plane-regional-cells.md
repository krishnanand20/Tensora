# 0009. Regions: one global control plane, independent regional cells

- **Status:** Accepted (data model in code: every workload, machine, and usage record has a region. Region in the create API, the catalog's region list, and per-region controllers are planned; see the roadmap)
- **Date:** 2026-10-23

> **Reframed 2026-10-23 ([ADR-0011](0011-free-for-everyone.md), [ADR-0012](0012-deployment-profiles.md)):** Tensora is self-hosted software, not a cloud we operate or sell. Read "global control plane" as *the control plane of one deployment*, and "regional cell" as *one location within it* (a home, an office, a datacenter). Solo installs have one region, `local`. Statements below about selling worldwide or India as a first region **no longer apply**.

## Context

Tensora sells worldwide and will run GPUs in several locations, starting with India
([product spec](../product/product-spec.md#how-you-deploy-it)). Customers need to choose where workloads
and data live, for latency, for law (GDPR, DPDP, and others), and for sovereign-AI contracts.
A failure in one location must not take down the others. Today the data model has no notion of location.

## Decision

Split the system into one **global control plane** and one **regional cell** per region.

| Global (one instance) | Regional cell (one per region) |
|---|---|
| Accounts, orgs, auth (Keycloak), API keys | Kubernetes cluster(s): KubeVirt, GPU Operator, KServe |
| Public API and dashboard | **A regional controller** that reconciles only its own region's workloads |
| Catalog (GPU classes, images, *which regions offer them*) | Object store (models, region backups) |
| Quotas, billing, payments, usage aggregation | Gateway for workload traffic (endpoints, notebooks, SSH) |
| Global Postgres (intent, events, usage) | Observability stack (metrics and logs stay in the region) |

The rules:

- **Every workload has one immutable `region`.** Its disks, models, logs, and traffic stay in that region. Moving a workload means creating a new one.
- Regional controllers run in their region (`TENSORA_REGION`). They select only workloads for that region, and report capacity per region.
- Workload URLs include the region, for example `<id>.<region>.ep.<domain>`, so traffic goes straight to the cell and never through the global plane.
- Region IDs are stable, lowercase, and location-based, for example `in-mum-1`, `eu-fra-1`, `us-east-1`.

## Consequences

- **Implement it now, before real data exists.** Add a `region` column to `workloads`, a region list to the catalog with each GPU class's available regions, a required `region` field in the create API, region filters in the controller's store queries, and region in the API views and dashboard. Retrofitting this after launch would mean migrating live data and breaking the API.
- A region outage affects only that region. A global outage stops new launches and the dashboard, but running workloads keep running, and their billing events are written once the controller reconnects. That needs a small local buffer in each regional controller (Phase 4).
- **Account data is global, workload data is regional.** Strict-residency customers (EU, some governments) may later require account and personal data to be stored in-region too. That's a v2 option: regional Postgres for personally identifiable data, or a fully independent sovereign stack.
- Where the global plane runs has legal weight (cross-border data transfers). Put it in a jurisdiction with clear transfer mechanisms, and document it in the privacy policy.
- Quotas stay global per GPU class for now. Per-region quotas are added if abuse or capacity planning needs them.
- Until the second region opens, the global plane and the first region's cell share one cluster. Splitting them is a deployment change, not a code change, as long as code never assumes they're co-located.

## Alternatives considered

- **Fully separate stack per region** (separate accounts per region): simple isolation, but poor UX (several logins and invoices) and duplicated billing.
- **One cluster stretched across regions:** etcd and storage across a WAN is fragile, and a single failure domain.
- **Federated Kubernetes (multi-cluster schedulers):** more machinery than we need. Tensora's own controller already decides placement.
