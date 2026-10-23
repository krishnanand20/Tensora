# controlplane

Tensora's core platform: one Go binary, `tensora`, that runs on macOS (Apple Silicon) and Linux.
The subcommand picks the role:

| Command | Role | Needs | Status |
|---|---|---|---|
| `tensora api` | REST API under `/v1` | A store (Postgres today) | Built |
| `tensora controller` | Reconciles workload intent with the runtime | A store, plus Kubernetes credentials or (planned) agents | Built |
| `tensora metering` | Usage accounting: lifecycle events → hourly usage records, for quotas and fairness (no money) | A store | Built |
| `tensora migrate` | Applies the SQL migrations embedded in the binary | Postgres | Built |
| `tensora dev` | API, controller, and usage accounting in one process with seeded tenants | Nothing | Built |
| `tensora up` | Solo: everything in one process, with SQLite and the embedded dashboard | Nothing | Planned |
| `tensora agent` | Node agent on each machine (Linux, macOS) | An enrolment token | Planned |

## Run locally

```bash
go run ./cmd/tensora dev
curl -s localhost:8080/v1/workloads -H 'X-Dev-Tenant: acme'
```

`dev` uses the in-memory store and the fake backend unless `TENSORA_DATABASE_URL` or
`TENSORA_CLUSTER_BACKEND=kubernetes` is set. To point the controller at a real Kubernetes cluster from
your laptop, use `TENSORA_CLUSTER_BACKEND=kubernetes`. It reads `KUBECONFIG`. Never point it at production.

## API

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz`, `/readyz` | Liveness, and readiness (checks the store) |
| GET | `/v1/catalog` | GPU classes and images visible to the caller's tenant |
| GET | `/v1/workloads` | `{"items": [...]}` |
| POST | `/v1/workloads` | `{name, kind, image_id, gpu_class, gpu_count, ssh_public_key?, model_uri?}` → 201. `kind` is `vm` (GPU VM, needs `ssh_public_key`), `notebook`, or `endpoint` (needs `model_uri`). |
| GET | `/v1/workloads/{id}` | |
| POST | `/v1/workloads/{id}/stop` | 202; VMs and notebooks keep their disks |
| POST | `/v1/workloads/{id}/start` | 202; re-checks quota |
| DELETE | `/v1/workloads/{id}` | 202; idempotent |

Errors look like `{"error": {"code": "quota_exceeded", "message": "..."}}`. The design behind the
lifecycle, admission, and usage accounting is in [docs/architecture/control-plane.md](../docs/architecture/control-plane.md).

**Planned** (ADR-0009, 0010, 0012, 0013, 0014; see the roadmap):

- create takes `region`, `fallback`, and `fallback_after`
- responses include `region`, `machine`, `requested_class`, and `placed_class`
- new resources: machines, users and teams, API tokens, and sharing settings
- `POST` accepts `Idempotency-Key`, and 503s carry `Retry-After`

## Build

```bash
go build -o bin/tensora ./cmd/tensora          # for this machine
make -C .. build-all                           # linux/amd64, linux/arm64, darwin/arm64 into bin/
docker build -t tensora-controlplane .         # Linux container image (Datacenter profile)
```
