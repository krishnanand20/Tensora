# 0006. Bill GPU allocation time, not utilization

- **Status:** Superseded in part by [ADR-0011](0011-free-for-everyone.md) (no billing; usage accounting kept)
- **Date:** 2026-10-22

> **Superseded in part by [ADR-0011](0011-free-for-everyone.md) (2026-10-23): Tensora is free, and there is no billing.** The mechanism below, allocation-based records from lifecycle events, is kept as **usage accounting** for quotas, fair share, and reports ([usage-accounting.md](../architecture/usage-accounting.md)). Read every "bill" or "price" below as historical.

## Context

The original idea was to track GPU-hours from DCGM exporter metrics. But:

- GPUs passed through to VMs are bound to `vfio-pci` and are **invisible to host DCGM**.
- A reserved GPU costs us the same whether it's busy or idle, so utilization-based billing invites "reserve and idle".
- Metrics pipelines are lossy (scrape gaps, restarts), which isn't acceptable for invoices.

## Decision

- Bill **wall-clock time in a billable state** (`running`), per second, aggregated per UTC hour per workload. The source is the append-only `workload_events` table written by `service.Transition`.
- Provisioning time is **not** billed. That's a customer-friendly cost we absorb, and can revisit.
- DCGM is used for observability and idle notifications only.

## Consequences

- Billing correctness depends on lifecycle correctness. GPUs must be released before `stopped` or `failed` is recorded. A domain test checks that billable states hold capacity.
- Controller downtime can overbill a workload that died. A crediting process is planned (see metering-and-billing.md).
- Usage records are immutable and idempotent, so they're auditable.

## Alternatives considered

- **DCGM utilization:** doesn't work for VMs, and is lossy.
- **Kubernetes pod lifetimes from kube-state-metrics:** closer to right, but not durable and not tied to our tenant model.

## Update 2026-10-23 (superseded by ADR-0011)

There are no prices and no billing. Usage is **accounted** by the class a workload actually ran on
(`placed_class`, ADR-0010) and by inference gateway token counts, for quotas, fair share, and reports.
