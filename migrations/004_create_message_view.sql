-- 004_create_message_view.sql
-- Read-optimised message cache. Rebuilt from Dovecot by the reconciler.
-- On any conflict between this table and Dovecot, DOVECOT WINS.

CREATE TABLE IF NOT EXISTS message_view (
    id              UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    account_id      UUID        NOT NULL REFERENCES accounts(id),
    dovecot_uid     TEXT        NOT NULL,     -- IMAP UID (stringified for safety)
    folder          TEXT        NOT NULL,     -- Dovecot mailbox name e.g. "INBOX"
    message_id      TEXT,                     -- RFC 2822 Message-ID header
    thread_id       TEXT,                     -- derived: SHA-1 of root message_id
    subject         TEXT,
    from_addr       TEXT,
    to_addrs        JSONB,                    -- ["addr1","addr2"]
    cc_addrs        JSONB,
    reply_to        TEXT,
    date            TIMESTAMPTZ,
    size_bytes      INTEGER,
    is_read         BOOLEAN     NOT NULL DEFAULT false,
    is_starred      BOOLEAN     NOT NULL DEFAULT false,
    is_deleted      BOOLEAN     NOT NULL DEFAULT false,
    labels          TEXT[]      NOT NULL DEFAULT '{}',
    snippet         TEXT,                     -- first 200 chars of plain body
    body_text       TEXT,                     -- plain text body
    body_html       TEXT,                     -- HTML body
    has_attachments BOOLEAN     NOT NULL DEFAULT false,
    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(account_id, dovecot_uid, folder)
);

-- Folder + time: primary inbox query.
CREATE INDEX IF NOT EXISTS mv_account_folder_date_idx
    ON message_view(account_id, folder, date DESC);

-- Thread grouping.
CREATE INDEX IF NOT EXISTS mv_account_thread_idx
    ON message_view(account_id, thread_id);

-- Unread filter.
CREATE INDEX IF NOT EXISTS mv_account_unread_idx
    ON message_view(account_id, folder, is_read)
    WHERE is_read = false;

-- Full-text search index (GIN).
CREATE INDEX IF NOT EXISTS mv_fts_idx
    ON message_view USING GIN (
        to_tsvector('english',
            coalesce(subject,   '') || ' ' ||
            coalesce(from_addr, '') || ' ' ||
            coalesce(body_text, '')
        )
    );
