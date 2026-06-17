-- 002_create_sessions.sql
-- Stateful bearer token sessions. Token is stored as SHA-256 hash only.
-- Client receives the raw token once; server never sees it again.

CREATE TABLE IF NOT EXISTS sessions (
    id          UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    account_id  UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,   -- SHA-256(raw_token) as hex
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    last_seen   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS sessions_token_hash_idx  ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS sessions_account_id_idx  ON sessions(account_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx  ON sessions(expires_at);
