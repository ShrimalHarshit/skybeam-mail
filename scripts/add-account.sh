#!/usr/bin/env bash
# scripts/add-account.sh — Add a new mailbox account to SkyBeam
#
# Usage:
#   ./scripts/add-account.sh alice@example.com "Alice Smith"
#
# This script:
#   1. Generates a bcrypt password hash and prompts you to save it
#   2. Adds the user to Dovecot's passwd-file
#   3. Inserts the account into PostgreSQL
#   4. Creates the Maildir structure
#
# Requirements: docker, psql (or run via `make add-account`)

set -euo pipefail

EMAIL="${1:-}"
DISPLAY_NAME="${2:-}"

if [[ -z "$EMAIL" ]]; then
  echo "Usage: $0 <email> [display_name]"
  exit 1
fi

# Derive dovecot user from email (local part + domain dir)
LOCAL="${EMAIL%%@*}"
DOMAIN="${EMAIL##*@}"
DOVECOT_USER="$EMAIL"
MAILDIR="/var/mail/vhosts/$DOMAIN/$LOCAL"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Adding account: $EMAIL"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 1. Generate Dovecot password hash
echo ""
echo "Enter password for $EMAIL (will be hashed with SHA512-CRYPT):"
HASH=$(docker compose -f deploy/docker-compose.yml exec -T dovecot \
  doveadm pw -s SHA512-CRYPT)

echo ""
echo "Password hash generated."

# 2. Add to Dovecot users file
USERS_FILE="deploy/dovecot/conf/users"
echo "$EMAIL:$HASH:5000:5000::$MAILDIR::" >> "$USERS_FILE"
echo "✓ Added to $USERS_FILE"

# 3. Create Maildir structure
docker compose -f deploy/docker-compose.yml exec -T dovecot \
  mkdir -p "$MAILDIR/Maildir"
docker compose -f deploy/docker-compose.yml exec -T dovecot \
  chown -R vmail:vmail "$MAILDIR"
echo "✓ Maildir created at $MAILDIR"

# 4. Insert into PostgreSQL
source .env
BCRYPT_HASH=$(docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U skybeam -d skybeam -tAc \
  "SELECT crypt('placeholder', gen_salt('bf', 12));")

docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U skybeam -d skybeam -c \
  "INSERT INTO accounts (email, display_name, dovecot_user, password_hash)
   VALUES ('$EMAIL', '$DISPLAY_NAME', '$DOVECOT_USER', '$BCRYPT_HASH')
   ON CONFLICT (email) DO NOTHING;"
echo "✓ Account inserted in PostgreSQL"

# 5. Seed system folders in folder_view
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U skybeam -d skybeam -c \
  "INSERT INTO folder_view (account_id, folder_name)
   SELECT id, unnest(ARRAY['INBOX','Sent','Drafts','Trash','Archive','Spam'])
   FROM accounts WHERE email = '$EMAIL'
   ON CONFLICT DO NOTHING;"
echo "✓ Default folders seeded"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Account $EMAIL created successfully."
echo "  IMPORTANT: The DB password is a placeholder bcrypt hash."
echo "  Set the real password via: POST /api/v1/auth/login"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
