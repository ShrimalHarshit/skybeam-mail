// Package api wires together the HTTP router, middleware, and handlers.
package api

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/skybeam/mail/internal/api/handlers"
	"github.com/skybeam/mail/internal/api/middleware"
	"github.com/skybeam/mail/internal/auth"
	"github.com/skybeam/mail/internal/config"
	"github.com/skybeam/mail/internal/events"
)

// NewRouter builds and returns the fully-configured chi router.
func NewRouter(cfg *config.Config, pool *pgxpool.Pool) http.Handler {
	authSvc := auth.NewService(pool, cfg.SessionTTLDays)
	eventWriter := events.NewWriter(pool)

	// ── Shared handler dependencies ───────────────────────────
	deps := &handlers.Deps{
		Pool:        pool,
		Cfg:         cfg,
		AuthSvc:     authSvc,
		EventWriter: eventWriter,
	}

	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.CORS)
	r.Use(chimiddleware.Timeout(30_000_000_000)) // 30s

	// ── Unauthenticated routes ────────────────────────────────
	r.Get("/health", handlers.Health)

	r.Route("/api/v1", func(r chi.Router) {
		// Auth — no bearer token required
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", handlers.NewAuthHandler(deps).Login)
		})

		// All routes below require a valid session
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(authSvc))

			r.Delete("/auth/logout", handlers.NewAuthHandler(deps).Logout)
			r.Get("/auth/me", handlers.NewAuthHandler(deps).Me)

			r.Route("/folders", func(r chi.Router) {
				fh := handlers.NewFolderHandler(deps)
				r.Get("/", fh.List)
				r.Post("/", fh.Create)
				r.Delete("/{name}", fh.Delete)
			})

			r.Route("/messages", func(r chi.Router) {
				mh := handlers.NewMessageHandler(deps)
				r.Get("/", mh.List)
				r.Post("/send", mh.Send)
				r.Get("/{id}", mh.Get)
				r.Get("/{id}/raw", mh.Raw)
				r.Patch("/{id}", mh.Update)
				r.Delete("/{id}", mh.Delete)
			})

			r.Route("/threads", func(r chi.Router) {
				mh := handlers.NewMessageHandler(deps)
				r.Get("/{threadID}", mh.Thread)
			})

			r.Route("/search", func(r chi.Router) {
				sh := handlers.NewSearchHandler(deps)
				r.Get("/", sh.Search)
			})

			r.Route("/events", func(r chi.Router) {
				eh := handlers.NewEventHandler(deps)
				r.Get("/", eh.List)
			})
		})
		// ── Admin Routes (Requires authentication + Admin role) ─────
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(authSvc))
			r.Use(middleware.AdminOnly)

			adminHandler := handlers.NewAdminHandler(deps)
			r.Get("/admin/domains", adminHandler.ListDomains)
			r.Post("/admin/domains", adminHandler.CreateDomain)
			r.Delete("/admin/domains/{id}", adminHandler.DeleteDomain)

			r.Get("/admin/accounts", adminHandler.ListAccounts)
			r.Post("/admin/accounts", adminHandler.CreateAccount)
		})
	})

	// ── Static Frontend ───────────────────────────────────────
	// Try /ui (Docker) or ./ui (local dev)
	uiPath := "./ui"
	if _, err := os.Stat("/ui"); err == nil {
		uiPath = "/ui"
	}
	r.Handle("/*", http.FileServer(http.Dir(uiPath)))

	return r
}
