BINARY_API        := bin/api
BINARY_WATCHER    := bin/watcher
BINARY_RECONCILER := bin/reconciler

# --project-name: prevents Docker inferring 'deploy' from the -f path, which
#   breaks container names and causes env vars to not load from .env.
# --env-file: explicitly load .env from project root (not from deploy/ dir).
COMPOSE_BASE  := docker compose --project-name skybeam --env-file .env
COMPOSE_PROD  := $(COMPOSE_BASE) -f deploy/docker-compose.yml
COMPOSE_DEV   := $(COMPOSE_BASE) -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml

BUILD_FLAGS := -ldflags="-s -w"

.PHONY: all build api watcher reconciler test lint migrate dev prod down \
        logs ps restart compose-validate add-account backup clean tidy gen check help

all: build

# ── Build targets ──────────────────────────────────────────────
build: api watcher reconciler

api:
	go build $(BUILD_FLAGS) -o $(BINARY_API) ./cmd/api

watcher:
	go build $(BUILD_FLAGS) -o $(BINARY_WATCHER) ./cmd/watcher

reconciler:
	go build $(BUILD_FLAGS) -o $(BINARY_RECONCILER) ./cmd/reconciler

tidy:
	go mod tidy

## check: vet + build check (no binaries)
check:
	go vet ./...
	go build ./...

test:
	go test ./... -race -timeout 60s

lint:
	golangci-lint run ./...

## gen: regenerate sqlc queries
gen:
	sqlc generate

clean:
	rm -rf bin/

# ── Database ──────────────────────────────────────────────────
## migrate: run all pending SQL migrations against DATABASE_URL
migrate:
	@echo "Running migrations against $(DATABASE_URL)..."
	@for f in migrations/*.sql; do \
		echo "  → $$f"; \
		psql "$(DATABASE_URL)" -f "$$f"; \
	done
	@echo "Migrations complete."

## migrate-reset: DROP + recreate schema + run migrations (DEV ONLY)
migrate-reset:
	@echo "WARNING: Dropping and recreating public schema..."
	psql "$(DATABASE_URL)" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO CURRENT_USER;"
	$(MAKE) migrate

# ── Docker Compose — Development ─────────────────────────────
## dev: start full dev stack (all infra + services, hot-reload friendly)
dev:
	$(COMPOSE_DEV) up --build

## dev-infra: start only infra services in dev (Postgres, Dovecot, Postfix, Rspamd)
dev-infra:
	$(COMPOSE_DEV) up postgres dovecot postfix rspamd clamav

# ── Docker Compose — Production ──────────────────────────────
## prod: build and start the full production stack in detached mode
prod:
	$(COMPOSE_PROD) up -d --build

## down: stop and remove containers (volumes preserved)
down:
	$(COMPOSE_PROD) down

## down-volumes: stop and remove containers AND all volumes (DESTRUCTIVE)
down-volumes:
	@echo "WARNING: This will delete all data volumes!"
	$(COMPOSE_PROD) down -v

## logs: follow logs for all services
logs:
	$(COMPOSE_PROD) logs -f

## logs-api: follow API service logs only
logs-api:
	$(COMPOSE_PROD) logs -f api

## ps: show service status
ps:
	$(COMPOSE_PROD) ps

## restart: rolling restart of Go services only (no mail infra downtime)
restart:
	$(COMPOSE_PROD) restart api watcher reconciler

## compose-validate: syntax-check all Compose files
compose-validate:
	$(COMPOSE_PROD) config --quiet && echo "✓ docker-compose.yml is valid"
	$(COMPOSE_DEV) config --quiet && echo "✓ docker-compose.dev.yml is valid"

# ── Operations ────────────────────────────────────────────────
## add-account: add a new mailbox (usage: make add-account EMAIL=alice@example.com NAME="Alice")
add-account:
	@bash scripts/add-account.sh "$(EMAIL)" "$(NAME)"

## backup: backup PostgreSQL and Maildir to BACKUP_DIR (default: /var/backups/skybeam)
backup:
	@bash scripts/backup.sh "$(BACKUP_DIR)"

help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
