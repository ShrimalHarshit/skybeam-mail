-- 007_add_is_admin_flag.sql

ALTER TABLE accounts ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false;

-- Create domains table to support multi-domain setup
CREATE TABLE IF NOT EXISTS domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- We don't drop the static domain yet, but this prepares for the future.
