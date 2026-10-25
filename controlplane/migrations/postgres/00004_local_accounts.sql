-- Local accounts (roadmap Phase 1, step 5; ADR-0012): sign-in with an email and a password, browser
-- sessions, and API tokens. Only hashes of passwords and tokens are stored.

-- +goose Up
CREATE TABLE users (
    id            uuid        PRIMARY KEY,
    tenant_id     uuid        NOT NULL REFERENCES tenants (id),
    email         text        NOT NULL UNIQUE CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL,
    disabled_at   timestamptz
);
CREATE INDEX users_tenant_idx ON users (tenant_id);

-- A signed-in browser. token_hash is the SHA-256 of the cookie's value.
CREATE TABLE sessions (
    token_hash text        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- A credential for the CLI and scripts, shown once at creation. A revoked token keeps its row, so
-- its owner can see when it was revoked, but frees its name.
CREATE TABLE api_tokens (
    id           uuid        PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text        NOT NULL,
    prefix       text        NOT NULL,
    token_hash   text        NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz,
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE UNIQUE INDEX api_tokens_active_name_idx ON api_tokens (user_id, name) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE api_tokens;
DROP TABLE sessions;
DROP TABLE users;
