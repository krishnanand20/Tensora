# 0008. Postgres owns intent, Kubernetes owns runtime, and a controller reconciles them

- **Status:** Accepted
- **Date:** 2026-10-22

> **Generalized 2026-10-23 ([ADR-0012](0012-deployment-profiles.md), [ADR-0013](0013-node-agent-and-apple-silicon.md)):** the store may be **SQLite or Postgres**, and the runtime may be **node agents or Kubernetes**. Mentions of customers and billing below are historical (ADR-0011: Tensora is free; usage is accounted, not billed). The principle (store owns intent, runtime owns runtime state, the controller reconciles) is unchanged.

## Context

Customer requests must be fast and reliable even when the cluster is slow or partially down.
Billing and tenancy need transactional, queryable history. Kubernetes is the authority on what
is actually running.

## Decision

- The **API** validates, checks quota in a transaction, records intent (`workloads` and `workload_events`), and returns. It has **no Kubernetes credentials**.
- The **controller** is the only cluster client. It admits, applies, observes, and transitions.
- Kubernetes objects carry `tensora.io/workload-id` and `tensora.io/tenant` labels. Names are derived from IDs, never from customer input.
- Every state change is a compare-and-swap plus an appended event (`service.Transition`).

## Consequences

- An API compromise can't touch the cluster directly. The blast radius is the database.
- The design is eventually consistent: users see `pending` and `provisioning`, so the UI polls.
- Two sources of truth need a reconciler. Drift (objects without rows, or rows without objects) must be detected; an orphan sweep is planned for Phase 3.
- `tensora dev` can run the whole loop in memory with a fake cluster, which makes development and tests fast.

## Alternatives considered

- **CRDs as the source of truth** (Tensora resources stored in Kubernetes): native and watch-driven, but it puts tenant and billing data in etcd, makes queries hard, and couples the API's availability to the cluster.
- **The API calls Kubernetes synchronously:** simpler at first, but slow, fragile, and the API would need cluster credentials.

## Update 2026-10-23

With regions (ADR-0009) there is **one controller per region**, each the only cluster client for its
region. The store and the API serve the whole deployment. Fallback placement (ADR-0010) is part of the controller's
admission step, and it records both `requested_class` and `placed_class`.
