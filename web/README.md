# web: Tensora dashboard

The dashboard, built with React, Vite, and TanStack Query. Users launch **GPU VMs**, **Jupyter
notebooks**, and **model endpoints** from a catalog-driven form, watch their state live, and stop,
start, or delete them.

```bash
make dev        # from the repo root: API on :8080
npm install
npm run dev     # http://localhost:5173
```

In dev mode, requests carry `X-Dev-Tenant: acme`. Set `VITE_DEV_TENANT` to act as another seeded
tenant: `globex` (untrusted), or `internal` (trusted, which also sees internal-only GPU classes).

How it's served:

- **Solo:** embedded in the `tensora` binary and served on localhost (planned).
- **Datacenter:** the static build is served by nginx (see `Dockerfile`). The Gateway routes `app.<domain>` here and `api.<domain>` to the control plane.

Planned (ADR-0009, 0010, 0013, 0014):

- a machines page showing Macs and Linux boxes with their GPUs, health, and sharing level
- a region or machine picker, and a fallback opt-in, on the launch form
- the placed class and a fallback badge on each workload
- sharing settings and host disclosures
- usage reports in hours (never money)
