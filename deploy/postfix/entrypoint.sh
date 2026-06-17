#!/bin/sh
set -e

echo "[postfix] Running postmap on lookup tables..."

# Generate hash maps from text files (must be done before postfix starts)
for table in /etc/postfix/virtual /etc/postfix/transport; do
    if [ -f "$table" ]; then
        postmap "$table"
        echo "[postfix] postmap: $table -> $table.db"
    fi
done

echo "[postfix] Starting Postfix..."
exec postfix start-fg
