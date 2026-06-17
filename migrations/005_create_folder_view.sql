-- 005_create_folder_view.sql
-- Per-account folder summary. Refreshed by the reconciler after each sync.
-- Counts are derived from message_view, NOT computed on-the-fly per request.

CREATE TABLE IF NOT EXISTS folder_view (
    id             UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    account_id     UUID        NOT NULL REFERENCES accounts(id),
    folder_name    TEXT        NOT NULL,
    total_count    INTEGER     NOT NULL DEFAULT 0,
    unread_count   INTEGER     NOT NULL DEFAULT 0,
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(account_id, folder_name)
);

CREATE INDEX IF NOT EXISTS fv_account_idx ON folder_view(account_id);

-- Seed system folders on account creation (done by application, not trigger).
-- System folders: INBOX, Sent, Drafts, Trash, Archive, Spam
