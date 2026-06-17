-- 003_create_events.sql
-- Append-only event store. Rows are NEVER updated or deleted.
-- Events describe what happened but are NOT the source of truth.
-- Dovecot is the source of truth.

CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL   NOT NULL PRIMARY KEY,       -- monotonic, used as cursor
    event_id    UUID        NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    account_id  UUID        NOT NULL REFERENCES accounts(id),
    event_type  TEXT        NOT NULL,
    message_uid TEXT,                                    -- Dovecot UID (per-mailbox)
    folder      TEXT,                                    -- Dovecot mailbox name
    payload     JSONB       NOT NULL DEFAULT '{}',
    emitted_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary access patterns:
CREATE INDEX IF NOT EXISTS events_account_time_idx  ON events(account_id, emitted_at DESC);
CREATE INDEX IF NOT EXISTS events_account_type_idx  ON events(account_id, event_type);
CREATE INDEX IF NOT EXISTS events_message_uid_idx   ON events(message_uid) WHERE message_uid IS NOT NULL;
