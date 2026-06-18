# ─── Stage 1: Build all binaries ──────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api        ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/watcher    ./cmd/watcher
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/reconciler ./cmd/reconciler

RUN go install github.com/pressly/goose/v3/cmd/goose@latest

# ─── Stage 2: api ─────────────────────────────────────────────
FROM alpine:3.20 AS api
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/api /usr/local/bin/api
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY migrations /migrations
EXPOSE 8080
ENTRYPOINT ["api"]

# ─── Stage 3: watcher ─────────────────────────────────────────
FROM alpine:3.20 AS watcher
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/watcher /usr/local/bin/watcher
ENTRYPOINT ["watcher"]

# ─── Stage 4: reconciler ──────────────────────────────────────
FROM alpine:3.20 AS reconciler
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/reconciler /usr/local/bin/reconciler
ENTRYPOINT ["reconciler"]
