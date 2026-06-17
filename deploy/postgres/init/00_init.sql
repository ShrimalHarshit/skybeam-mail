-- PostgreSQL init script — runs once on first container boot
-- Applied automatically by the official postgres image from:
--   /docker-entrypoint-initdb.d/

-- Set sensible defaults
ALTER DATABASE skybeam SET timezone TO 'UTC';
ALTER DATABASE skybeam SET default_text_search_config TO 'pg_catalog.english';

-- Enable required extensions
\c skybeam

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- ── PostgreSQL 15+ schema permission fix ─────────────────────────────────────
-- PG15 revoked CREATE on the public schema from the PUBLIC role.
-- Grant it explicitly to our application user so migrations can run.
GRANT ALL ON SCHEMA public TO skybeam;
ALTER SCHEMA public OWNER TO skybeam;

-- pg_stat_statements: diagnose slow queries in production.
-- View with: SELECT * FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 20;
