# 0006. Bill GPU allocation time, not utilization

- **Status:** Accepted
- **Date:** 2026-10-22

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

## Update 2026-10-23

- Usage is priced by the class a workload actually ran on (`placed_class`), so an opt-in CPU or other-class fallback is billed at its own price (ADR-0010).
- Requests served by the serverless pool or a fallback model are billed per token, from inference gateway counts.
- All prices are in USD.
