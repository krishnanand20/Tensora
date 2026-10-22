# AGENTS.md: web/ (TypeScript dashboard)

These rules add to the root [AGENTS.md](../AGENTS.md); read that first.

## Layout

```
src/api/types.ts      mirrors controlplane/internal/api/json.go; change both together
src/api/client.ts     fetch wrapper, ApiError, and auth headers
src/workloads.ts      pure UI rules (which actions apply to which state); unit-tested
src/pages/            one component per route
src/styles.css        plain CSS with light/dark tokens; no CSS framework
```

## Rules

- **The API is the only backend.** Never call Kubernetes, Keycloak admin APIs, or anything else directly.
- **Server state lives in TanStack Query.** Use `useState` for form and UI state only. Poll with `refetchInterval` only while something is mid-transition (`isSettling`).
- **Don't duplicate business rules.** The UI may hide buttons, as `availableActions` does, but the API decides. Show the API's error message to users verbatim.
- **Show what actually runs** (once ADR-0009 and ADR-0010 land): always display a workload's region and `placed_class`, and badge fallbacks clearly. Never hide a fallback. The launch form gets a region picker and an explicit fallback opt-in.
- **Money is USD.** Format prices and usage in USD.
- **Auth:** dev mode sends `X-Dev-Tenant`. OIDC (Keycloak with PKCE) arrives in Phase 3, in `client.ts` only. Never store tokens in `localStorage`.
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
