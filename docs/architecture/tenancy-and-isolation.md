# Tenancy and isolation

**Premise:** tenants are untrusted and may be hostile. They run arbitrary code on our GPUs,
upload arbitrary files, and probe everything they can reach.

## Threat model

| Threat | Example | Primary controls |
|---|---|---|
| Escaping to the host | Container breakout through a kernel bug | VMs or Kata for all untrusted code (ADR-0004); PSA baseline; no privileged pods |
| Cross-tenant data access | Reading another tenant's volume, logs, or GPU memory | Namespace per tenant; tenant-scoped queries returning 404; per-workload volumes deleted with the workload; see the GPU memory risk below |
| Lateral network movement | Scanning node IPs, the Kubernetes API, Postgres, or other tenants | Default-deny NetworkPolicy; egress to public internet only; no ServiceAccount token |
| Code execution via model files | A pickle payload in `pytorch_model.bin`; `auto_map` custom code | `tensora-ml validate` allowlist; `--load-format=safetensors`; no `trust_remote_code` |
| Resource exhaustion | Fork bombs, disk filling, hogging all GPUs | Guaranteed QoS limits; ResourceQuota; per-class quotas; volume sizes |
| Billing evasion | Keeping GPUs while showing as stopped | GPUs released before `stopped` or `failed` is recorded; allocation-based billing; append-only events |
| Abuse of third parties | Outbound attacks, spam, crypto mining on stolen cards | Acceptable use policy, egress monitoring, port 25 block (Phase 3), KYC for paid accounts (Phase 4) |
| Control plane compromise | Stolen controller credentials | The API has no cluster access; the controller's RBAC is limited to an allowlist of kinds; admin access only over WireGuard |

## Isolation tiers

| Tier | Who | Runtimes allowed | GPU sharing |
|---|---|---|---|
| `untrusted` | Every external customer (default) | KubeVirt VM, or Kata container (`TENSORA_UNTRUSTED_RUNTIME_CLASS`) | Whole GPUs only. Later, MIG behind a VM or Kata. |
| `trusted` | Our own team | runc containers too | May use internal-only GPU classes |

The tier is stored per tenant (`tenants.tier`). The manifest builders set `runtimeClassName` for untrusted tenants.

## Controls by layer

**Compute**
- Tenant pods get `automountServiceAccountToken: false`, no host namespaces or hostPath, dropped capabilities, the RuntimeDefault seccomp profile, and requests equal to limits.
- Namespaces enforce PSA `baseline` and warn on `restricted`. *Phase 1 task:* confirm virt-launcher runs under `baseline`.
- No GPU time-slicing, ever. Multi-tenant MIG needs a VM or Kata boundary.

**Network** (details in [networking.md](networking.md))
- `tenant-baseline` policy allows ingress from the same namespace only. Egress is allowed to the same namespace, DNS, the model store, and `0.0.0.0/0` **except** 10/8, 172.16/12, 192.168/16, 100.64/10, and 169.254/16.
- `allow-from-gateway` (Cilium) admits only the Gateway proxy.
- Tenants never receive Kubernetes credentials.

**Identity and API**
- The tenant comes from a verified token claim (`tensora_tenant`). There is no API parameter that names a tenant.
- Every store query is scoped by tenant ID, and cross-tenant reads return 404. Tests enforce both, and the `tenancy-reviewer` subagent checks them.
- Dev header auth is refused outside `TENSORA_ENV=dev`.

**Supply chain**
- Notebooks run catalog images only. Endpoints use the pinned `tensora-vllm` runtime.
- Bring-your-own images are not offered. When they are, Kata or a VM will be mandatory, plus scanning.

**Models**
- `ml/src/tensora_ml/validate.py` allows only safetensors, JSON, tokenizer, and text files. It checks safetensors headers and rejects `auto_map` and `trust_remote_code`. It never loads weights.

## Open risks

| Risk | Status | Plan |
|---|---|---|
| **GPU memory remanence** between tenants on passthrough. VFIO's function-level reset doesn't guarantee that VRAM is zeroed. | Open | Phase 1: test it by writing a pattern in VM A, then reading in VM B on the same GPU. If data survives, add a scrub step (a short wipe job) before reassigning a GPU, or use H100 confidential-computing mode. |
| Kata + GPU passthrough not yet validated | Open | Phase 3 gate. Until it's validated, untrusted notebooks must not be offered. Use VMs, or treat notebooks as internal-only. |
| Block storage isn't zeroed on reuse | Open | Encrypt volumes at rest with per-volume keys (Longhorn encrypted volumes or Ceph RBD encryption). Phase 4. |
| Admin network not firewalled | Open | Host firewall so SSH and the Kubernetes API are reachable only via WireGuard. Phase 2. |
| The controller's ServiceAccount can create pods in any namespace | Accepted | Kind allowlist, RBAC limited to listed resources, audit logging (Phase 2). |
| Tenant container logs reach operators via Loki | Accepted | Treated as customer data, with access restricted and retention limited (see observability.md). |

## Compliance notes (India)

- **DPDP Act 2023:** customer account data is personal data. Collect the minimum, define retention, and support deletion requests.
- Workload data (volumes, models) belongs to the customer. Our ToS must state that we don't back it up unless they opt in.
