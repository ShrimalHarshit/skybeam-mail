# SkyBeam Mail Platform

Self-hosted, API-first email platform. Postfix + Dovecot underneath. Event-driven architecture above. REST API for clients.

## Architecture

```
Internet → Postfix → Rspamd → ClamAV → Dovecot (source of truth)
                                              ⇓ IMAP IDLE
                                        imap-watcher
                                              ⇓ events
                                         PostgreSQL
                                    (events + materialized views)
                                              ⇓
                                          REST API
                                              ⇓
                                    Caddy (automatic TLS)
                                              ⇓ HTTPS
                                           Clients
```

## Services

| Binary | Purpose |
|---|---|
| `skybeam-api` | REST API server |
| `imap-watcher` | IMAP IDLE → event emitter |
| `reconciler` | Dovecot ↔ DB drift repair |

## Quick Start (Development)

```bash
# 1. Clone and configure
cp .env.example .env
# Edit .env — set DATABASE_URL, API_SECRET, DOVECOT_MASTER_PASS

# 2. Start infrastructure (Postgres + Dovecot + Postfix + Rspamd)
make dev

# 3. Run migrations (new terminal, after postgres is up)
export DATABASE_URL=postgres://skybeam:devpassword@localhost:5432/skybeam?sslmode=disable
make migrate

# 4. Run API server locally
go run ./cmd/api

# 5. Run watcher locally (separate terminal)
go run ./cmd/watcher

# 6. Run reconciler locally (separate terminal)
go run ./cmd/reconciler
```

## Production Deploy

```bash
# 1. Copy and configure
cp .env.example .env
# Set: POSTGRES_PASSWORD, API_SECRET, DOVECOT_MASTER_PASS
#      API_HOSTNAME, MAIL_HOSTNAME, MAIL_DOMAIN, ACME_EMAIL

# 2. Point DNS before starting (Caddy needs to reach Let's Encrypt)
#   A  api.skybeam.studio   → <your server IP>
#   A  mail.skybeam.studio  → <your server IP>

# 3. Build and start (Caddy obtains TLS certs automatically on first boot)
make prod

# 4. Run migrations
make migrate

# 5. Add first mailbox
make add-account EMAIL=alice@skybeam.studio NAME="Alice"
```

No cert files. No Certbot. No cron jobs. Caddy handles everything.

## Database Migrations

Migrations are plain SQL files in `migrations/`. Run in order:

```bash
# Apply all
make migrate

# Reset (dev only — DESTROYS ALL DATA)
make migrate-reset
```

## Build

```bash
make build          # build all three binaries to bin/
make api            # api only
make watcher        # watcher only
make reconciler     # reconciler only
```

## Testing

```bash
make test           # go test ./... -race
make lint           # golangci-lint
make check          # go vet + build check
```

## Environment Variables

See `.env.example` for full documentation. Required variables:

| Variable | Description | Example |
|---|---|---|
| `POSTGRES_PASSWORD` | PostgreSQL password | `openssl rand -base64 32` |
| `API_SECRET` | 32-byte token signing secret | `openssl rand -hex 32` |
| `DOVECOT_MASTER_USER` | Internal IMAP impersonation user | `skybeam_internal` |
| `DOVECOT_MASTER_PASS` | Internal IMAP password | `openssl rand -base64 32` |
| `API_HOSTNAME` | Caddy gets a cert for this | `api.skybeam.studio` |
| `MAIL_HOSTNAME` | Postfix MX hostname | `mail.skybeam.studio` |
| `MAIL_DOMAIN` | Domain to receive mail for | `skybeam.studio` |
| `ACME_EMAIL` | Let's Encrypt contact email | `ops@skybeam.studio` |

## API

Base: `https://your-domain/api/v1`

Auth: `Authorization: Bearer <token>`

| Method | Path | Description |
|---|---|---|
| POST | `/auth/login` | Login, get bearer token |
| DELETE | `/auth/logout` | Invalidate session |
| GET | `/auth/me` | Current account info |
| GET | `/folders` | List folders with counts |
| POST | `/folders` | Create folder |
| DELETE | `/folders/:name` | Delete empty folder |
| GET | `/messages` | List messages (folder, page, limit) |
| GET | `/messages/:id` | Get full message |
| GET | `/messages/:id/raw` | Raw RFC 2822 source |
| PATCH | `/messages/:id` | Update (read, star, move) |
| DELETE | `/messages/:id` | Move to Trash |
| DELETE | `/messages/:id?permanent=true` | Expunge |
| POST | `/messages/send` | Send email |
| GET | `/threads/:id` | Get thread |
| GET | `/search?q=...` | Full-text search |
| GET | `/events` | Event log (debug) |
| GET | `/health` | Health check |

## Backup

```bash
# Database
pg_dump -U skybeam skybeam | gzip > backup_$(date +%Y%m%d).sql.gz

# Mail (Maildir — no downtime required)
restic -r s3:your-bucket/skybeam-mail backup /var/mail
```

## Roadmap

See [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) for the full 12-week plan.
