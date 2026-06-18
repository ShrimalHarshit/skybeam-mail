#!/bin/bash
set -e

# SkyBeam Mail Platform - AWS Installation Script
# Usage: sudo ./install.sh

echo "========================================================"
echo "          SkyBeam Mail Platform Installer               "
echo "========================================================"

# 1. Check prerequisites
if ! command -v docker &> /dev/null; then
    echo "Error: docker is not installed."
    echo "Please install Docker first: curl -fsSL https://get.docker.com | sh"
    exit 1
fi

# 2. Gather Configuration
echo ""
echo "--- General Configuration ---"
read -p "Enter your primary mail domain (e.g., skybeam.live): " MAIL_DOMAIN
read -p "Enter API Hostname (e.g., api.skybeam.live): " API_HOSTNAME
read -p "Enter Mail Hostname (e.g., mail.skybeam.live): " MAIL_HOSTNAME
read -p "Enter an email for Let's Encrypt recovery (e.g., admin@skybeam.live): " ACME_EMAIL

echo ""
echo "--- Admin Account Setup ---"
read -p "Admin Email: " ADMIN_EMAIL
read -s -p "Admin Password: " ADMIN_PASS
echo ""

# 3. Generate Secrets
echo "Generating secure secrets..."
POSTGRES_PASSWORD=$(openssl rand -base64 32)
API_SECRET=$(openssl rand -hex 32)
DOVECOT_MASTER_PASS=$(openssl rand -base64 32)

# 4. Write .env
echo "Creating .env file..."
DATABASE_URL="postgres://skybeam:${POSTGRES_PASSWORD}@postgres:5432/skybeam?sslmode=disable"
cat > .env <<EOF
# SkyBeam Mail Platform — Environment Variables
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
API_ADDR=:8080
API_SECRET=${API_SECRET}
DOVECOT_MASTER_USER=skybeam_internal
DOVECOT_MASTER_PASS=${DOVECOT_MASTER_PASS}
API_HOSTNAME=${API_HOSTNAME}
MAIL_HOSTNAME=${MAIL_HOSTNAME}
MAIL_DOMAIN=${MAIL_DOMAIN}
SESSION_TTL_DAYS=30
RECONCILER_LIGHT_INTERVAL=5m
RECONCILER_FULL_INTERVAL=24h
LOG_LEVEL=info
LOG_FORMAT=json
ACME_EMAIL=${ACME_EMAIL}
DATABASE_URL=${DATABASE_URL}
DOVECOT_HOST=dovecot
EOF

# Update master-users file for Dovecot so watcher can auth
echo "skybeam_internal:{PLAIN}${DOVECOT_MASTER_PASS}" > deploy/dovecot/conf/master-users

# 5. Start Docker Stack
echo "Starting Docker Compose stack..."
docker compose -f deploy/docker-compose.yml up -d --build

# 6. Wait for Database & API
echo "Waiting for services to become healthy (this may take a minute)..."
attempt=0
while [ $attempt -le 30 ]; do
    if docker compose -f deploy/docker-compose.yml exec -T api wget -qO- http://localhost:8080/health > /dev/null 2>&1; then
        echo "API is up!"
        break
    fi
    echo -n "."
    sleep 2
    ((attempt++))
done

if [ $attempt -gt 30 ]; then
    echo -e "\nError: API failed to start. Please check logs: docker compose logs api"
    exit 1
fi

# 7. Create Admin Account
echo "Running database migrations..."
docker compose -f deploy/docker-compose.yml exec -T api goose -dir /migrations postgres "${DATABASE_URL}" up

echo "Creating Admin account..."
docker compose -f deploy/docker-compose.yml exec -T api api -setup-admin "${ADMIN_EMAIL}" -password "${ADMIN_PASS}"

# 8. Success Output
echo "========================================================"
echo "          Installation Complete!                        "
echo "========================================================"
echo ""
echo "Your SkyBeam Mail Platform is now running."
echo ""
echo "🌍 Admin & Web UI : https://${API_HOSTNAME}"
echo "🔒 Admin Email    : ${ADMIN_EMAIL}"
echo ""
echo "Next Steps:"
echo "1. Ensure DNS A records for ${API_HOSTNAME} and ${MAIL_HOSTNAME} point to this server's IP."
echo "2. Ensure MX record for ${MAIL_DOMAIN} points to ${MAIL_HOSTNAME}."
echo "3. Log in to the web interface to configure domains and mailboxes."
echo ""
echo "To view logs: docker compose -f deploy/docker-compose.yml logs -f"
echo "========================================================"
