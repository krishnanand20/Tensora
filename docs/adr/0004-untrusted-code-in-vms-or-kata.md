# 0004. Run untrusted code only in VMs or Kata containers

- **Status:** Accepted
- **Date:** 2026-10-22

> **Updated 2026-10-23 ([ADR-0011](0011-free-for-everyone.md), [ADR-0014](0014-sharing-idle-compute.md)):** Tensora is free and self-hosted, so nothing is "sold". *Untrusted* now means **invited and community users**, and "customers" below means users. The rule below still holds, and the per-driver trust matrix in [tenancy-and-isolation.md](../architecture/tenancy-and-isolation.md) applies it to Macs and node agents.

## Context

We sell compute to strangers. Container escapes through kernel bugs are a recurring class of
vulnerability, and GPU drivers add a large attack surface. A shared kernel is not a security boundary we can
defend for hostile tenants.

## Decision

- Tenants in the `untrusted` tier (all external customers) run code only in a **KubeVirt VM** or a **Kata container** (`runtimeClassName` set by the manifest builders).
- **No GPU time-slicing across tenants.** MIG is allowed only behind a VM or Kata boundary.
- Plain runc containers are allowed only for the `trusted` tier (our own team).
- Endpoints run our pinned vLLM runtime on validated safetensors weights (rule 6). Even so, they get the sandbox runtime for untrusted tenants as defense in depth.

## Consequences

- **Kata + GPU passthrough must be validated before untrusted notebooks or endpoints are offered** (Phase 3 gate). Until then, external customers get VMs. Notebooks and endpoints are internal-only or offered with that risk accepted explicitly.
- VMs and Kata add overhead (boot time, memory). This is acceptable for GPU workloads that run for minutes to days.
- The GPU Operator's sandbox features (vfio-manager, sandbox device plugin, and later the kata manager) are on the critical path.

## Alternatives considered

- **gVisor:** strong syscall isolation, but no practical GPU passthrough.
- **runc with hardening** (seccomp, AppArmor, user namespaces): still a shared kernel and shared GPU driver.
- **One node per tenant:** strong, but wasteful at our scale.

## Update 2026-10-23

CPU workloads, including CPU fallback (ADR-0010), follow the same rule: a VM or Kata for untrusted
tenants. Capacity pressure never relaxes isolation. A fallback is never a shared GPU or a plain container.
