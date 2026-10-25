-- The SQLite version of ../postgres/00004_local_accounts.sql.

-- +goose Up
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants (id),
    email         TEXT NOT NULL UNIQUE CHECK (email = lower(email) AND length(CAST(email AS BLOB)) BETWEEN 3 AND 254),
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    disabled_at   TEXT
) STRICT;
CREATE INDEX users_tenant_idx ON users (tenant_id);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
) STRICT;
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   TEXT NOT NULL,
    expires_at   TEXT,
    last_used_at TEXT,
    revoked_at   TEXT
) STRICT;
CREATE UNIQUE INDEX api_tokens_active_name_idx ON api_tokens (user_id, name) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE api_tokens;
DROP TABLE sessions;
DROP TABLE users;
