# Tenancy, trust, and isolation

Tensora involves three parties. In a Solo install they're all the same person. Everywhere else they can differ.

| Party | Who | Can be hostile to |
|---|---|---|
| **Operator** | Whoever runs the control plane: you, a company's platform team, a university, a pool operator | — (holds the keys) |
| **Host** | The owner of a machine that runs workloads | Users: a host has root on their own machine and can see guest data |
| **User** | The person launching a workload (a tenant) | Hosts, the operator's infrastructure, and other users |

**The design premise:** treat every non-owner as potentially hostile, and be honest about what hosts can see.

## Trust levels

| Level | Who | Typical setting |
|---|---|---|
| **Owner** | The machine's owner, or the operator's own team (`trusted` tier in code) | Solo; a company's internal deployment |
| **Invited** | Named people, groups, or organizations | A Lab: a class, a team, friends |
| **Community** | Anyone with an account on a pool | A community pool ([ADR-0014](../adr/0014-sharing-idle-compute.md)) |

The code today has two tenant tiers: `trusted` (Owner) and `untrusted` (Invited and Community).
Per-machine trust from sharing levels is planned (Phase 4).

## Trust matrix: what may run where

| Runtime or driver | Isolation | Owner | Invited | Community |
|---|---|---|---|---|
| `kubernetes`: KubeVirt VM or Kata container | Strong | ✓ | ✓ | ✓ |
| `kubernetes`: plain runc container | Weak | ✓ | ✗ | ✗ |
| `libvirt` VM (Linux KVM) | Strong | ✓ | ✓ | ✓ if the host opts in |
| `docker` with Kata or gVisor | Strong | ✓ | ✓ | ✓ if the host opts in |
| `docker` (runc) | Weak (shared kernel) | ✓ | ✗ | ✗ |
| `macos-vz` macOS guest | Strong | ✓ | ✓ | ✓ if the host opts in (max 2 per Mac) |
| `macos-container` (a Linux container in its own VM, CPU only) | Strong | ✓ | ✓ | ✓ if the host opts in |
| `macos-native` | Weak (macOS sandbox) | ✓ | Managed inference only | Managed inference only |

**Managed inference:** the host runs a model server from the catalog, with validated weights, and the
user only sends prompts. No user code runs on the host.

## Threat model

| Threat | Example | Controls |
|---|---|---|
| Guest escapes to the host | A container breakout, or a macOS sandbox escape | The trust matrix: non-owners only get strong isolation. Plus PSA baseline, no privileged pods, and seccomp. |
| Guest attacks the host's network | Scanning a home LAN or the router admin page | No access to private, link-local, or LAN ranges. Community workloads have **no internet by default**. |
| Guest abuses the host's IP address | Spam, attacks, or illegal downloads from someone's home connection | No community egress by default; AUP, reports, and bans |
| Guest reads or fills the host's disk | | No host mounts; per-workload disk caps |
| Guest exhausts the host | Hogging RAM, overheating a laptop | Caps; Guaranteed QoS in a datacenter; share only when idle and on AC power; instant reclaim |
| **Host reads guest data** | An owner inspects a VM disk or process memory | Unavoidable without confidential computing. **Disclosed in the product.** Users shouldn't put secrets on community hosts. |
| Guest-to-guest access | Reading another user's data or traffic | Tenant-scoped queries (404); a namespace or VM per workload; volumes deleted with the workload; **one untrusted user per Mac GPU** (Metal has no partitioning) |
| Code execution through model files | A pickle payload, or `auto_map` custom code | `tensora-ml validate` allows safetensors (and GGUF once validated) only; never `trust_remote_code` |
| Control plane or agent compromise | Stolen controller or agent credentials | The API has no runtime access; controller RBAC is an allowlist; each agent has its own revocable mTLS certificate; datacenter admin access only over WireGuard |
| Usage-accounting tampering | Gaming quotas or fair share | Append-only events; metered states must hold capacity |
| Data leaving its region | A fallback or bug moves data abroad | Rule 9; cross-region fallback only with explicit opt-in |
| Gateway misrouting | User A's request served by user B's endpoint | The gateway routes by API key → tenant → endpoint, never by hostname alone |
| Leaks through logs | Another user's timeline or export, a credential inside an export, internals or other tenants' activity in a timeline reason, internals in an error message | Logs and exports are tenant-scoped (404), including the store queries behind them (`EventsForWorkload`). Reasons come from a fixed vocabulary, never raw runtime text. Secrets are redacted (failing closed), and each redaction is listed in the export. Model URIs with credentials are refused. Internal errors return only a request ID, and the CLI writes exports owner-only (`0600`). See [logs and transparency](logs-and-transparency.md). |
| Capacity side channel | Repeatedly queueing a workload to learn when another user is using a shared machine | Untrusted tenants see `not enough free right now`, never a free-GPU count (see Open risks) |
| Hidden decisions | A user can't tell why their work waited, moved, or failed | Rule 14: every decision is in the timeline with its reason, and the full log is exportable |

## Controls by layer

**Compute**
- Datacenter tenant pods get `automountServiceAccountToken: false`, no host namespaces, dropped capabilities, RuntimeDefault seccomp, requests equal to limits, and the Kata runtime class for untrusted tenants.
- Agents run non-owner workloads only through strong-isolation drivers, per the matrix. They cap memory, CPU, GPU memory, and disk on every workload.
- No GPU time-slicing between different users, ever. On a Mac, an untrusted user gets the whole GPU or none of it.

**Network** (details in [networking.md](networking.md))
- Datacenter: default-deny NetworkPolicy per tenant, egress to the public internet only, and no Kubernetes credentials for users.
- Agents: guest workloads can't reach the LAN, private ranges, or the host. Community workloads have no internet unless the pool operator allows it.

**Identity**
- The tenant comes from a verified identity: a local account, OIDC, or an API token. There's no API parameter that names a tenant.
- Every store query is scoped by tenant. Dev header auth is refused outside `TENSORA_ENV=dev`.

**Logs** (details in [logs-and-transparency.md](logs-and-transparency.md))

| Who | Sees |
|---|---|
| A user | Their own workloads' timeline, usage, and export (output logs planned). Never another tenant's. |
| A tenant admin | The tenant's activity log (planned, Phase 2) |
| A host | Their machine's status and which workloads ran on it. Never a guest's output through the product, though as root a host could technically read guest data (disclosed). |
| The operator | System logs. In a datacenter, Loki access is limited to on-call staff, and audited. |

**Supply chain**
- Notebooks run catalog images or environments. Endpoints use pinned runtimes (vLLM, MLX, llama.cpp).
- Release binaries are signed, and macOS builds are notarized (Phase 1).

## Open risks

| Risk | Status | Plan |
|---|---|---|
| **GPU memory remanence** between users: VFIO passthrough, and Metal on shared Macs | Open | Test both (Phase 3 for passthrough; Phase 4 before Macs serve untrusted users). Add a scrub step if data survives. |
| Strength of the macOS sandbox for native workloads | Accepted for owners only | The trust matrix limits non-owners to managed inference or VMs |
| Kata + GPU passthrough not yet validated | Open | Phase 3 gate |
| Block storage isn't zeroed on reuse (datacenter) | Open | Encrypted volumes with per-volume keys |
| Datacenter admin network not firewalled | Open | Phase 3 |
| The controller's ServiceAccount can create pods in any namespace | Accepted | Kind allowlist, RBAC limited to listed resources, audit logging |
| Hosts can see guest data | **Accepted and disclosed** | Product warnings; confidential computing out of scope |
| Admission outcomes reveal whether a GPU class is full | Accepted (mitigated) | Untrusted tenants get no free-GPU counts, but queued versus admitted still shows whether a class has room. When sharing lands (Phase 4), compute capacity only over machines that user may use, so the signal covers only what they could run on anyway. |

## Compliance notes

Tensora is software. **Whoever operates an instance is responsible** for the law where they operate.

- **Solo** keeps everything on the user's own machine.
- **Labs and pools** hold account data, so their operator needs a privacy notice (GDPR, DPDP, CCPA and others apply to operators), an acceptable use policy, and a way to handle abuse reports and lawful requests.
- **Models** come with their own licences, and users must follow them.
