# controlplane

Tensora's core platform: one Go binary, `tensora`, with these roles:

| Command | Role | Needs |
|---|---|---|
| `tensora api` | REST API under `/v1` | Postgres |
| `tensora controller` | Reconciles workload intent with Kubernetes | Postgres, cluster credentials |
| `tensora metering` | Turns lifecycle events into hourly usage records | Postgres |
| `tensora migrate` | Applies the SQL migrations embedded in the binary | Postgres |
| `tensora dev` | Runs api, controller, and metering in one process with seeded tenants | nothing |

## Run locally

```bash
go run ./cmd/tensora dev
curl -s localhost:8080/v1/workloads -H 'X-Dev-Tenant: acme'
```

`dev` uses the in-memory store and the fake cluster unless `TENSORA_DATABASE_URL` or
`TENSORA_CLUSTER_BACKEND=kubernetes` is set. To point the controller at a real cluster from
your laptop, use `TENSORA_CLUSTER_BACKEND=kubernetes`. It reads `KUBECONFIG`. Never point it at production.

## API

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz`, `/readyz` | liveness, and readiness (checks the DB) |
| GET | `/v1/catalog` | GPU classes and images visible to the caller's tenant |
| GET | `/v1/workloads` | `{"items": [...]}` |
| POST | `/v1/workloads` | `{name, kind, image_id, gpu_class, gpu_count, ssh_public_key?, model_uri?}` → 201. `kind` is `vm` (GPU VM, needs `ssh_public_key`), `notebook`, or `endpoint` (needs `model_uri`). |
| GET | `/v1/workloads/{id}` | |
| POST | `/v1/workloads/{id}/stop` | 202; VMs and notebooks keep their disks |
| POST | `/v1/workloads/{id}/start` | 202; re-checks quota |
| DELETE | `/v1/workloads/{id}` | 202; idempotent |

Errors look like `{"error": {"code": "quota_exceeded", "message": "..."}}`. The design behind the
lifecycle, admission, and metering is in [docs/architecture/control-plane.md](../docs/architecture/control-plane.md).

**Planned** (ADR-0009, ADR-0010; see the roadmap):

- create takes `region`, `fallback`, and `fallback_after`
- responses include `region`, `requested_class`, and `placed_class`
- `POST` accepts `Idempotency-Key`, and 503s carry `Retry-After`
- the controller runs once per region (`TENSORA_REGION`)
- a `tensora gateway` role fronts model endpoints

## Build

```bash
docker build -t tensora-controlplane .
docker run --rm tensora-controlplane help
```
