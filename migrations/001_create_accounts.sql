-- 001_create_accounts.sql
-- Accounts are the top-level identity. One account = one email address = one Dovecot mailbox.

CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- for gen_random_uuid()

-- ── PostgreSQL 15+ compatibility ─────────────────────────────────────────────
-- PG15 revoked CREATE on the public schema from all non-superuser roles.
-- This grant is idempotent and safe to run multiple times.
GRANT ALL ON SCHEMA public TO CURRENT_USER;

CREATE TABLE IF NOT EXISTS accounts (
    id           UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    email        TEXT        NOT NULL UNIQUE,
    display_name TEXT,
    dovecot_user TEXT        NOT NULL UNIQUE,  -- maps 1:1 to Dovecot username
    password_hash TEXT       NOT NULL,          -- bcrypt(cost=12)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_active    BOOLEAN     NOT NULL DEFAULT true
);

CREATE INDEX IF NOT EXISTS accounts_email_idx ON accounts(email);
