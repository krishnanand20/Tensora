# web: Tensora dashboard

The customer dashboard, built with React, Vite, and TanStack Query. Customers launch **GPU VMs**,
**Jupyter notebooks**, and **model endpoints** from a catalog-driven form, see them with live state,
and stop, start, or delete them.

```bash
make dev        # from the repo root: API on :8080
npm install
npm run dev     # http://localhost:5173
```

In dev mode, requests carry `X-Dev-Tenant: acme`. Set `VITE_DEV_TENANT` to act as another
seeded tenant: `globex` (untrusted) or `internal` (trusted, which also sees internal-only GPUs).

In production, the static build is served by nginx (see `Dockerfile`), and the Gateway routes
`app.<domain>` here and `api.<domain>` to the control plane.

Planned (ADR-0009 and ADR-0010): a region picker and a fallback opt-in on the launch form, the
placed class and a fallback badge in the workload list, and USD usage and billing pages.
