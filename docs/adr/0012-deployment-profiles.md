# 0012. Deployment profiles: Solo, Lab, and Datacenter

- **Status:** Accepted (design; not yet implemented, see the roadmap)
- **Date:** 2026-10-23
- **Amends:** ADR-0001, ADR-0002, ADR-0008

## Context

Users range from a student with one MacBook to a company with racks of NVIDIA servers. The existing
design is a Kubernetes datacenter stack (k3s, KubeVirt, GPU Operator, Ceph, Postgres, Keycloak).
That's far too heavy for one machine, and it can't run natively on macOS at all.

## Decision

Ship **one product in three profiles**. The API, CLI, and dashboard are the same everywhere, and only
what's underneath changes.

| Profile | For | Machines | Store | Runs workloads with | Sign-in |
|---|---|---|---|---|---|
| **Solo** | One person on one machine: a laptop, a Mac, a gaming PC | 1 | SQLite (a file) | The node agent, in the same process | Local account |
| **Lab** | Students, classes, developers, small teams, home labs, small companies | 2 to ~20 mixed Linux and Mac machines | SQLite, or Postgres | Node agents on each machine (ADR-0013) | Local accounts and invite links; OIDC optional |
| **Datacenter** | Startups at scale, enterprises, big tech | 3 to thousands of servers | Postgres (HA) | Kubernetes: KubeVirt, KServe, GPU Operator (ADR-0001) | OIDC or SSO (Keycloak or the company's IdP) |

- **`tensora up`** is the Solo entry point. It runs the control plane, the agent, and the dashboard in one process, keeps its data in the user's data directory, and listens on localhost by default.
- **Store implementations:** `memstore` (dev and tests), **`sqlitestore`** (Solo and Lab), and `pgstore` (Datacenter, or any Lab that wants it). All of them must pass `storetest`. SQLite uses a pure-Go driver, so cross-compiling stays CGO-free.
- **Auth modes:** `local` (built-in accounts and API tokens), `oidc`, and `dev`.
- **The dashboard is embedded in the binary** (Go `embed`), so a Solo install is a single file.
- **Moving up a profile** is an export and import (`tensora export`, `tensora import`), not a rebuild. Planned.
- A Datacenter deployment can also enroll agent machines, for example a fleet of Macs, as their own region.

## Consequences

- AGENTS.md rule 1 is generalized: *the store* (SQLite or Postgres) owns business state, and *the runtime* (Kubernetes or node agents) owns runtime state.
- `deploy/` and `infra/` (helmfile, k3s, Ansible) are the **Datacenter profile**. Solo and Lab need none of it.
- New install channels: Homebrew (macOS), an install script, `.deb` and `.rpm` packages, and a container image, with launchd and systemd service units.
- SQLite allows a single writer. That's fine for one control plane. Larger Labs, or many concurrent users, should use Postgres.

## Alternatives considered

- **k3s everywhere:** on macOS, k3s only runs inside a Linux VM, which has no access to the Mac's GPU.
- **Docker Compose only:** gives no VMs, no Mac GPU access, and no path to a datacenter.
