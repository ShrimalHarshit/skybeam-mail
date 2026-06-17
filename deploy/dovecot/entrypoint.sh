#!/bin/sh
set -e

# Auth files are bind-mounted. We can't chown them (read-only mount).
# Instead, just verify they exist and warn if missing.
for f in /etc/dovecot/users /etc/dovecot/master-users; do
    if [ -f "$f" ]; then
        echo "[dovecot] auth file present: $f"
    else
        echo "[dovecot] WARNING: $f not found — authentication will fail"
    fi
done

# Ensure mail storage has correct ownership on the volume.
# The volume is writable, so this is fine.
chown -R vmail:vmail /var/mail/vhosts 2>/dev/null || true

echo "[dovecot] Starting Dovecot..."
exec dovecot -F
