# 0003. Go for the core platform, Python for the ML layer, TypeScript for the dashboard

- **Status:** Accepted
- **Date:** 2026-10-22

## Context

The product layer has three distinct jobs:

1. **The core platform:** API, reconciler, metering. It's long-running, concurrent, deeply integrated with Kubernetes, and correctness-critical, since it handles tenancy and billing.
2. **The ML layer:** model ingestion and validation, benchmarking, GPU burn-in. These need the Hugging Face, PyTorch, and safetensors ecosystem.
3. **The dashboard:** a web UI for customers.

This is the same split most GPU clouds use.

## Decision

- **Go** for `controlplane/`: one `tensora` binary with subcommands (api, controller, metering, migrate, dev). It uses stdlib `net/http`, pgx, client-go, and goose.
- **Python 3.12** for `ml/`: the `tensora-ml` CLI and image, managed with uv, with heavy dependencies like torch kept optional.
- **TypeScript** for `web/`: React, Vite, and TanStack Query.

Boundaries: the Go platform orchestrates and never imports ML libraries. Python tools are invoked as
CLIs or Kubernetes Jobs, with JSON on stdout. The dashboard talks only to the public API.

## Consequences

- client-go is the native Kubernetes client, and Go builds a single static binary for distroless images.
- Python code stays where its ecosystem is irreplaceable, and isn't on the request path.
- There are three toolchains in CI. The Makefile and the per-component AGENTS.md files keep this manageable.
- API types are duplicated between Go (`internal/api/json.go`) and TS (`web/src/api/types.ts`). If that drifts, generate an OpenAPI spec (Phase 3).

## Alternatives considered

- **Python everywhere** (FastAPI control plane): one language, but the Kubernetes client and concurrency story is weaker, and runtime type safety for billing and tenancy code is weaker too.
- **Go everywhere:** no viable story for model validation or burn-in without reimplementing parts of the Python ML ecosystem.
