// Package handlers contains HTTP handler types and shared infrastructure.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/skybeam/mail/internal/auth"
	"github.com/skybeam/mail/internal/config"
	"github.com/skybeam/mail/internal/events"
)

// Deps bundles shared dependencies injected into every handler.
type Deps struct {
	Pool        *pgxpool.Pool
	Cfg         *config.Config
	AuthSvc     *auth.Service
	EventWriter *events.Writer
}

// Health returns 200 OK with build info. No auth required.
func Health(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "skybeam-api",
	})
}

// ── JSON helpers ──────────────────────────────────────────────

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]string{
		"error": message,
		"code":  code,
	})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB limit
	return json.NewDecoder(r.Body).Decode(v)
}
