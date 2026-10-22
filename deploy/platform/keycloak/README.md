# Keycloak (Phase 3)

Keycloak is **global**: it runs once, in the global control plane, not in every region (ADR-0009).
Until the second region opens, that's the first region's cluster.

The operator is installed by helmfile when `features.keycloak` is true. These still have to be done:

1. **Database:** Keycloak needs Postgres. Use a separate database from Tensora's, with its own credentials stored in a Secret created out of band.
2. **Keycloak CR:** hostname `auth.<domain>`, routed through the `tensora` Gateway with an HTTPRoute.
3. **Realm `tensora`**, imported with a `KeycloakRealmImport`:
   - Client **`tensora-web`**: public, standard flow with PKCE (S256), redirect URI `https://app.<domain>/*`.
   - Client scope **`tensora-api`** with an *audience* mapper that adds `tensora-api` to access tokens. The API verifies `aud` (`TENSORA_OIDC_AUDIENCE`).
   - A *user attribute* or *organization* mapper that emits **`tensora_tenant`** (the tenant slug) in access tokens (`TENSORA_OIDC_TENANT_CLAIM`).
   - MFA required for anyone with admin roles.
4. **Point the API at it:** `TENSORA_AUTH_MODE=oidc` and `TENSORA_OIDC_ISSUER=https://auth.<domain>/realms/tensora`. The tensora chart sets these.

For local testing, `docker compose --profile auth up -d keycloak` starts Keycloak on :8180.
