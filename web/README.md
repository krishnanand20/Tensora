# web: Tensora dashboard

The customer dashboard, built with React, Vite, and TanStack Query. It lists workloads with
live state, provides stop, start, and delete actions, and has a launch form driven by the catalog.

```bash
make dev        # from the repo root: API on :8080
npm install
npm run dev     # http://localhost:5173
```

In dev mode, requests carry `X-Dev-Tenant: acme`. Set `VITE_DEV_TENANT` to act as another
seeded tenant: `globex` (untrusted) or `internal` (trusted, which also sees internal-only GPUs).

In production, the static build is served by nginx (see `Dockerfile`), and the Gateway routes
`app.<domain>` here and `api.<domain>` to the control plane.
