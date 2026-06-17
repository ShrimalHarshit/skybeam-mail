#!/usr/bin/env bash
# scripts/backup.sh — Backup PostgreSQL and Maildir
#
# Usage:
#   ./scripts/backup.sh [output_dir]
#   ./scripts/backup.sh /mnt/backups
#
# Requirements:
#   - docker (postgres container must be running)
#   - restic (for Maildir backup) — or swap for rsync

set -euo pipefail

BACKUP_DIR="${1:-/var/backups/skybeam}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$BACKUP_DIR/postgres" "$BACKUP_DIR/maildir"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  SkyBeam Backup — $TIMESTAMP"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ── PostgreSQL ────────────────────────────────────────
PG_DUMP="$BACKUP_DIR/postgres/skybeam_${TIMESTAMP}.sql.gz"
echo "Dumping PostgreSQL → $PG_DUMP"
docker compose -f deploy/docker-compose.yml exec -T postgres \
  pg_dump -U skybeam skybeam | gzip > "$PG_DUMP"
echo "✓ PostgreSQL dump: $(du -h "$PG_DUMP" | cut -f1)"

# ── Maildir (via restic) ──────────────────────────────
if command -v restic &>/dev/null && [[ -n "${RESTIC_REPOSITORY:-}" ]]; then
  echo "Backing up Maildir via restic → $RESTIC_REPOSITORY"
  restic backup \
    --tag skybeam-mail \
    --tag "$TIMESTAMP" \
    /var/lib/docker/volumes/skybeam_mail_data/_data   # adjust if volume path differs
  echo "✓ Maildir restic backup complete"
else
  # Fallback: copy volume data locally (container must be paused for consistency)
  MAIL_DUMP="$BACKUP_DIR/maildir/maildir_${TIMESTAMP}.tar.gz"
  echo "Fallback: tarballing Maildir → $MAIL_DUMP"
  docker run --rm \
    -v skybeam_mail_data:/data:ro \
    -v "$BACKUP_DIR/maildir":/backup \
    alpine tar czf "/backup/maildir_${TIMESTAMP}.tar.gz" /data
  echo "✓ Maildir tarball: $(du -h "$MAIL_DUMP" | cut -f1)"
fi

# ── Prune old local backups (keep 30 days) ────────────
find "$BACKUP_DIR/postgres" -name "*.sql.gz" -mtime +30 -delete
echo "✓ Pruned postgres backups older than 30 days"

echo ""
echo "Backup complete: $BACKUP_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
