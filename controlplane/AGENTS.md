# AGENTS.md: controlplane/ (Go core platform)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

## Layout and dependency direction

```
cmd/tensora/            main: wiring only, with subcommands api | controller | metering | migrate | seed | dev
internal/domain/        types, the state machine, and error codes. Imports nothing internal.
internal/config/        TENSORA_* env parsing and production guards
internal/catalog/       loads images/catalog.yaml
internal/store/         Store/Queries interfaces
  memstore/             in-memory implementation (dev and tests)
  pgstore/              Postgres implementation (pgx), plus Migrate()
  storetest/            conformance suite that both implementations must pass
internal/service/       business operations, tenant-scoped; Transition() is the single path for state changes
internal/api/           HTTP handlers (stdlib net/http); thin, and calls the service only
internal/auth/          Dev (header) and OIDC (Keycloak JWT) authenticators
internal/cluster/       Backend interface, labels, CheckJSONTypes
  manifests/            pure builders that return Kubernetes objects as map[string]any
  fake/                 in-memory backend
  kube/                 client-go backend with server-side apply and a kind allowlist
internal/controller/    reconcile loop: admit → observe → park → remove
internal/metering/      events → billable intervals → hourly usage records
internal/devseed/       dev tenants (acme and globex untrusted, internal trusted)
internal/testutil/      fixtures: real catalog, seeded store, manual clock
migrations/             goose SQL files embedded into the binary
```

Allowed imports flow downward: `api → service → store, domain`; `controller → service, cluster, store`.
`api` must never import `cluster` or `controller`, and `domain` imports nothing internal.

## Rules specific to this module

- **New store method:** add it to `store.Queries`, implement it in both `memstore` and `pgstore`, and add a case to `storetest`. Tenant-scoped methods take `tenantID` and filter by it.
- **Schema change:** add a new `migrations/NNNNN_*.sql` file. Never edit an applied migration. Keep `workload_events` append-only (a trigger enforces this).
- **New cluster kind:** add the builder in `manifests/`, add the GVK to `kube.allowed`, and add the RBAC rule in `deploy/charts/tensora/templates/controller.yaml`. `TestBuildersOnlyEmitAllowedKinds` fails if the allowlist is missing an entry.
- **Unstructured types:** builders may use only `map[string]any`, `[]any`, `string`, `bool`, `int64`, and `float64`. Quantities are strings (`"64Gi"`, `"2"`). `cluster.CheckJSONTypes` enforces this in tests and in the fake backend.
- **State changes:** call `service.Transition(ctx, q, &w, to, reason, now)` inside `store.InTx`. Never call `UpdateWorkloadState` directly.
- **Time:** inject `now func() time.Time`, never call `time.Now()` inside logic, and store UTC.
- **Errors:** expected failures are `domain.Errorf(code, ...)`, and their message is shown to users. Wrap everything else with `%w`; it becomes a logged 500.
- **Logging:** `log/slog` with key-value pairs. Never log tokens, SSH keys, or model URIs that carry credentials.

## Testing

- `go test ./...` runs everything except Postgres tests. `make test-cp-pg` runs those too, using Docker.
- Use `testutil.SeededStore`, `testutil.Catalog`, and `testutil.NewClock` rather than new fixtures.
- Every new endpoint needs a cross-tenant test: tenant B gets 404 for tenant A's object.
- Prefer table-driven tests. Controller tests drive `ReconcileOnce` with `fake.Backend` and assert on states.

## Commands

```bash
go run ./cmd/tensora dev        # local stack on :8080
go test ./...                   # tests
go vet ./... && golangci-lint run
gofmt -l .                      # must print nothing
```
