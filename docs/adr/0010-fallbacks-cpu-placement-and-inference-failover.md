# 0010. Fallbacks: CPU placement when GPUs are unavailable, and inference failover

- **Status:** Accepted (design; not yet implemented in code; see the roadmap)
- **Date:** 2026-10-23

> **Updated 2026-10-23 ([ADR-0011](0011-free-for-everyone.md)):** Tensora is free. Wherever this record says *billed*, *priced*, or *paid*, read **accounted**, and read *customer* as **user**. There are no prices or invoices. Fallbacks apply in every profile, including on Macs (Metal GPU, then CPU).

## Context

GPUs will regularly be sold out in a region, and individual replicas will fail (OOM, XID errors,
node loss, overload). Today a workload simply waits in `queued`, and a failed inference request
returns a 5xx. Customers would rather get *something useful* than wait or see errors, but only
on terms they agreed to. Silently giving someone a CPU when they paid for an L40S is worse than an error.

**Principle: fallback is opt-in, explicit, and visible.** A customer chooses which substitutes they accept.
Every response and invoice line shows what actually served them, and they're billed for what actually ran.

## Decision

### A. Placement fallback, when the requested GPU class has no capacity

Workloads get a placement policy:

```json
{
  "gpu_class": "l40s-48gb",
  "fallback": ["a100-80gb", "cpu"],
  "fallback_after": "10m"
}
```

- `fallback` is an ordered list of acceptable substitutes: other GPU classes, and/or `cpu`. It's empty by default, which means queue as today.
- `fallback_after` is how long to wait for the requested class before trying the list. `0` means immediately.
- The controller's admission step tries the requested class, then each fallback in order. The workload records **`requested_class`** and **`placed_class`**, and the transition event says why, for example `placed on cpu-32c: l40s-48gb unavailable for 10m`.
- **CPU classes** join the catalog (for example `cpu-16c-64g`, `cpu-32c-128g`). They run on CPU-only worker nodes, which are cheap and can be added without GPUs. Isolation is unchanged: untrusted tenants still run in a VM or Kata.

| Kind | CPU fallback means | Moving back to GPU |
|---|---|---|
| Notebook | JupyterLab without a GPU. Good for editing and data prep while waiting. | Never automatic (the kernel state would be lost). The dashboard offers "restart on GPU" when capacity frees. |
| VM | A CPU-only VM. Mostly useful for setup; GPU drivers won't find a device. | Never automatic. The customer stops and starts it with GPU placement. |
| Endpoint | Serve the same safetensors model with the **`tensora-vllm-cpu`** runtime. Only allowed for models under a size limit (about 8B parameters, decided by the validator's byte count). | **Automatic.** When GPU capacity frees, the controller adds a GPU replica, shifts traffic, then removes the CPU replica. |

CPU inference is roughly 10–50× slower than GPU. The dashboard and API state this when the customer enables it.

### B. Inference request failover (API responses)

A regional **inference gateway** sits in front of all endpoints. It's needed anyway for API keys,
token metering, and rate limits (Phase 3). For each request it walks the endpoint's fallback chain:

1. **Another healthy replica** of the same endpoint (automatic)
2. **A CPU replica** of the same model, if the endpoint enabled CPU fallback
3. **The serverless shared pool** for the same model, if it's in the curated catalog (v1.5)
4. **A customer-chosen fallback model**, for example a smaller model (`fallback_models`)
5. **Another region**, only if the customer explicitly allows cross-region fallback (`allow_cross_region`), because it breaks data residency (ADR-0009)
6. Otherwise a clean OpenAI-style error: `503` with `Retry-After`. It never hangs.

Rules:

- **Triggers:** connection failure, 5xx, a time-to-first-token deadline, 429 or queue full, and a circuit breaker that opens after repeated failures so later requests skip the bad backend at once.
- **Fail over only before the first streamed token.** Once a response has started streaming, switching backends would splice two models' output, so the stream ends with an error event instead.
- **Transparency:** the response carries `X-Tensora-Served-By` (replica / cpu / serverless / fallback-model), `X-Tensora-Region`, and `X-Tensora-Fallback: true|false`. The body's `model` field names the model that actually answered.
- **Billing:** dedicated endpoint hours are billed as usual. Requests served by the serverless pool or a fallback model are billed per token at that product's price, so the invoice shows them separately.

### C. Control API resilience

- `POST` endpoints accept an **`Idempotency-Key`** header, so clients and the SDK can retry safely.
- A `503` always carries `Retry-After`. The SDK and CLI retry with exponential backoff and jitter.
- Running workloads never depend on the control API being up (ADR-0008, ADR-0009).

## Consequences

- **Data model:** workloads gain `fallback`, `fallback_after`, `requested_class`, and `placed_class`, and metering prices by `placed_class`. Add these in the same change as `region` (ADR-0009), before real data exists.
- **Catalog:** classes gain a `kind` (`gpu` or `cpu`). CPU capacity comes from CPU-node allocatable.
- **New components:** CPU worker nodes in each region; the `tensora-vllm-cpu` ClusterServingRuntime; the regional inference gateway (Envoy-based or a small Go proxy, decided in Phase 3).
- **Admission gets more complex:** fallback placement must not starve customers waiting for the requested class. A GPU freed up goes to queued GPU-requesters first, before promoting CPU-fallback endpoints.
- **Product:** CPU instances also become a cheap product of their own (data prep, CPU inference for small models), which helps fill idle CPU in GPU servers.

## Alternatives considered

- **Automatic fallback with no customer opt-in:** surprises customers and breaks their expectations about performance and cost.
- **Fallback only in client SDKs:** every customer reimplements it badly, and we can't fail over to resources we haven't exposed.
- **Over-provisioning instead of fallback:** idle GPUs are the most expensive thing we own (see utilization in the product spec).
