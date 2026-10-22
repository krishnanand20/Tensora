# Playbook: add an API resource

Used by humans, Codex, and the Claude Code `/add-api-resource` skill. Example: adding `models`.

## 0. Decide

- What are the fields, and which are user-supplied? How is each validated?
- Is it tenant-scoped? (It almost always is.)
- Does it have a lifecycle? If so, use explicit states, transitions, and events, like workloads.
- Does it create cluster objects, or run ML layer Jobs? If so, the controller does that, never the API.
- Does it affect quota or billing? If so, update [metering-and-billing.md](../architecture/metering-and-billing.md) and get a `tenancy-reviewer` pass.
- Is it **regional** (it lives in one region, like workloads and models) or **global** (like orgs and API keys)? Regional resources carry an immutable `region`, are handled by that region's controller, and keep their data in-region (rule 9, ADR-0009).
- Could a **substitute** ever serve it (another class, CPU, model, or region)? If so, the substitute must be opt-in and visible in responses and billing (rule 10, ADR-0010).
- Prices and money amounts are in **USD**.

## 1. Schema

Add `controlplane/migrations/NNNNN_<name>.sql` with `-- +goose Up` and `-- +goose Down` sections.
Include a `tenant_id uuid NOT NULL REFERENCES tenants(id)` column, an index on `(tenant_id, created_at)`,
and CHECK constraints for enums.

## 2. Domain

Add types to `internal/domain/`. If there's a lifecycle, add a state table and a test mirroring `state_test.go`.

## 3. Store

1. Add the methods to `store.Queries`. Tenant-scoped methods take `tenantID` and filter by it.
2. Implement them in `memstore` **and** `pgstore`.
3. Add a case to `storetest` (including tenant scoping), so both implementations are tested the same way.

## 4. Service

Add `internal/service/<resource>.go`:

- Input struct with JSON tags and a `validate()` method: lengths, character sets, URI schemes, no control characters
- Methods that take `domain.Principal` and use `withTenant` or `withWorkload`-style helpers
- Expected failures returned as `domain.Errorf(code, …)`

## 5. API

1. Add view structs to `internal/api/json.go`. Never return domain structs directly, and never expose internals.
2. Add handlers to `server.go` and register routes under `/v1`. Async actions return 202.
3. Add tests to `server_test.go`: happy path, validation errors, **cross-tenant 404**, unknown fields rejected, and (for regional resources) region validation.
4. `POST` handlers honour `Idempotency-Key` once that exists (ADR-0010). Never add a non-idempotent create without it.

## 6. Cluster (if needed)

1. Add a pure builder in `internal/cluster/manifests/` and unit-test it (`CheckJSONTypes`, labels, namespace, hardening).
2. Add any new kinds to `kube.allowed` **and** the controller ClusterRole (`deploy/charts/tensora/templates/controller.yaml`).
3. Handle the new kind in a controller step, with tests using `fake.Backend`.

## 7. Dashboard and docs

- Update `web/src/api/types.ts` and `web/src/api/client.ts`, plus a page or component if the resource is user-facing.
- Update [control-plane.md](../architecture/control-plane.md) (lifecycle, API table) and `controlplane/README.md` (endpoint list).

## 8. Verify

Run [verify-changes.md](verify-changes.md) and the `tenancy-reviewer`.
