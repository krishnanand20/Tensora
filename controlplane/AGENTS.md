# AGENTS.md: controlplane/ (Go core platform)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

This module is the `tensora` binary. It must build and run on **linux/amd64, linux/arm64, and
darwin/arm64** (rule 11): run `make build-all` from the repo root.

## Layout and dependency direction

```
cmd/tensora/            main: wiring only, with subcommands api | controller | metering | migrate | seed | dev
                        (planned: up | agent | gateway | backup | export | import)
internal/domain/        types, the state machine, and error codes. Imports nothing internal.
internal/config/        TENSORA_* env parsing and production guards
internal/catalog/       loads images/catalog.yaml
internal/store/         Store/Queries interfaces
  memstore/             in-memory implementation (dev and tests)
  pgstore/              Postgres implementation (pgx), plus Migrate()
  storetest/            conformance suite every implementation must pass
                        (planned: sqlitestore/ for Solo and Lab, pure Go, no CGO)
internal/service/       business operations, tenant-scoped; Transition() is the single path for state changes
internal/api/           HTTP handlers (stdlib net/http); thin, and calls the service only
internal/auth/          Dev (header) and OIDC (JWT) authenticators (planned: local accounts and API tokens)
internal/cluster/       Backend interface, labels, CheckJSONTypes
  manifests/            pure builders that return Kubernetes objects as map[string]any
  fake/                 in-memory backend
  kube/                 client-go backend with server-side apply and a kind allowlist
                        (planned: agent/ backend that dispatches to node agents)
internal/controller/    reconcile loop: admit → observe → park → remove
internal/metering/      usage accounting: events → metered intervals → hourly usage records (no money)
internal/devseed/       dev tenants (acme and globex untrusted, internal trusted)
internal/testutil/      fixtures: real catalog, seeded store, manual clock
migrations/             goose SQL files embedded into the binary
(planned) internal/agent/            the node agent: enrolment, heartbeat, inventory, desired-state sync
(planned) internal/agent/drivers/    macosnative, macosvz, macoscontainer, docker, libvirt (OS code behind build tags)
```

Allowed imports flow downward: `api → service → store, domain`; `controller → service, cluster, store`.
`api` must never import `cluster`, `controller`, or `agent`, and `domain` imports nothing internal.

## Rules specific to this module

- **New store method:** add it to `store.Queries`, implement it in **every** store (memstore, pgstore, and sqlitestore once it exists), and add a case to `storetest`. Tenant-scoped methods take `tenantID` and filter by it.
- **Schema change:** add a new `migrations/NNNNN_*.sql` file. Never edit an applied migration. Keep SQL portable between Postgres and SQLite (ADR-0012). Keep `workload_events` append-only (a trigger enforces this).
- **New cluster kind:** add the builder in `manifests/`, add the GVK to `kube.allowed`, and add the RBAC rule in `deploy/charts/tensora/templates/controller.yaml`. `TestBuildersOnlyEmitAllowedKinds` fails if the allowlist is missing an entry.
- **Unstructured types:** builders may use only `map[string]any`, `[]any`, `string`, `bool`, `int64`, and `float64`. Quantities are strings (`"64Gi"`, `"2"`). `cluster.CheckJSONTypes` enforces this in tests and in the fake backend.
- **OS-specific code** (agents and drivers): only in driver packages, behind build tags (`//go:build darwin`, `//go:build linux`), with a stub for other platforms so `make build-all` stays green.
- **No CGO.** Pick pure-Go dependencies so cross-compiling stays trivial.
- **State changes:** call `service.Transition(ctx, q, &w, to, reason, now)` inside `store.InTx`. Never call `UpdateWorkloadState` directly.
- **Time:** inject `now func() time.Time`, never call `time.Now()` inside logic, and store UTC.
- **Errors:** expected failures are `domain.Errorf(code, ...)`, and their message is shown to users. Wrap everything else with `%w`; it becomes a logged 500.
- **Logging:** `log/slog` with key-value pairs. Never log tokens, SSH keys, agent certificates, or model URIs that carry credentials.
- **No money** (rule 12): `metering` is usage accounting for quotas and fairness only.

## Testing

- `go test ./...` runs everything except Postgres tests, on macOS and Linux. `make test-cp-pg` runs those too, using Docker.
- Use `testutil.SeededStore`, `testutil.Catalog`, and `testutil.NewClock` rather than new fixtures.
- Every new endpoint needs a cross-tenant test: tenant B gets 404 for tenant A's object.
- Prefer table-driven tests. Controller tests drive `ReconcileOnce` with `fake.Backend` and assert on states.

## Planned changes (design accepted, not in code yet)

The [roadmap's Phase 1 task breakdown](../docs/roadmap.md#task-breakdown-the-solo-foundation-next-code-change)
has the full list. Until it lands:

- **Profiles and agents (ADR-0012, ADR-0013):** don't assume Kubernetes, Postgres, or Linux outside their own packages. Everything above `cluster.Backend` and `store.Store` must work with the agent backend and SQLite.
- **Regions (ADR-0009):** don't write code that assumes one region. New regional queries take a `region` parameter.
- **Fallbacks (ADR-0010):** don't treat the requested class as the class that runs. Capacity, usage accounting, and display will use `placed_class`.
- **Sharing (ADR-0014):** admission will need the machine's sharing level and the user's trust level, so keep placement logic in one place (the controller's admit step).

## Commands

```bash
go run ./cmd/tensora dev        # local stack on :8080 (macOS or Linux)
go test ./...                   # tests
go vet ./... && golangci-lint run
gofmt -l .                      # must print nothing
make -C .. build-all            # cross-compile for linux/amd64, linux/arm64, darwin/arm64
```
