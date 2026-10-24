# AGENTS.md: web/ (TypeScript dashboard)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

The dashboard is the same in every profile. For Solo, it will be embedded in the `tensora` binary
(ADR-0012), so it must work when served from `127.0.0.1` with no separate web server.

## Layout

```
src/api/types.ts      mirrors controlplane/internal/api/json.go; change both together
src/api/client.ts     fetch wrapper, ApiError (with requestId), auth headers, and file downloads
src/workloads.ts      pure UI rules (which actions apply to which state); unit-tested
src/pages/            one component per route
src/styles.css        plain CSS with light/dark tokens; no CSS framework
```

## Rules

- **The API is the only backend.** Never call Kubernetes, agents, IdP admin APIs, or anything else directly.
- **Server state lives in TanStack Query.** Use `useState` for form and UI state only. Poll with `refetchInterval` only while something is mid-transition (`isSettling`).
- **Don't duplicate business rules.** The UI may hide buttons, as `availableActions` does, but the API decides. Show the API's error message to users verbatim.
- **Show what actually runs** (ADR-0009, ADR-0010): always display a workload's region and what it runs on with `classShown`, which names the requested class whenever a fallback placed it elsewhere. Add the machine once agents place workloads. Never hide a fallback. The launch form gets a region or machine picker and an explicit fallback opt-in.
- **Be honest about sharing** (ADR-0014): before a user launches on someone else's machine, show that the host can see their data. Show owners exactly what they're sharing, and when.
- **Nothing hidden** (rule 14, ADR-0015): show every timeline reason in full, never truncated or summarized away. Keep **Download full log** on the workload page, and include anything new the API exports. Show the request ID with every error (`ApiError` already appends it). Take download file names from `filenameFromDisposition`, which strips directories. Deleted workloads stay reachable: the workload page loads with `include_deleted`, and the list has **Show deleted**. Show notes (`from` equals `to`) as notes, via `describeChange`.
- **Build workload URLs with `workloadPath`**, which encodes the ID, so an ID can never point at a different API path.
- **No money anywhere** (rule 12): no prices, plans, upgrade prompts, or payment screens. Usage pages show hours, never currency.
- **Auth:** dev mode sends `X-Dev-Tenant`. Planned: local accounts (Solo and Lab) and OIDC with PKCE (Datacenter), in `client.ts` only. Never store tokens in `localStorage`.
- **Relative URLs only:** call `/v1/...`, never hard-coded hosts, so the embedded build works on any address.
- **Dependencies:** keep the list short. Ask before adding a UI kit, state library, or CSS framework.
- **Strict TypeScript:** `any` needs a comment explaining why.

## Commands

```bash
npm install
npm run dev         # :5173, proxies /v1 to the API on :8080 (run `make dev` first)
npm run typecheck
npm test
npm run build
```
