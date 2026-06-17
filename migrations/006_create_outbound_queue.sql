-- 006_create_outbound_queue.sql
-- Outbound email queue. API enqueues messages here; a background goroutine
-- in the API service delivers them via Postfix SMTP submission.

CREATE TYPE outbound_status AS ENUM ('pending', 'sent', 'failed');

CREATE TABLE IF NOT EXISTS outbound_queue (
    id          UUID            NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    account_id  UUID            NOT NULL REFERENCES accounts(id),
    status      outbound_status NOT NULL DEFAULT 'pending',
    raw_message TEXT            NOT NULL,    -- RFC 2822 formatted message
    to_addrs    JSONB           NOT NULL,    -- ["addr@example.com"]
    attempt_count INTEGER       NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    sent_at     TIMESTAMPTZ,
    error_msg   TEXT
);

CREATE INDEX IF NOT EXISTS oq_pending_idx ON outbound_queue(status, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS oq_account_idx ON outbound_queue(account_id, created_at DESC);
