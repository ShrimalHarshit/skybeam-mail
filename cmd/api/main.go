package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/skybeam/mail/internal/api"
	"github.com/skybeam/mail/internal/auth"
	"github.com/skybeam/mail/internal/config"
	"github.com/skybeam/mail/internal/db"
)

func main() {
	cfg := config.Load()

	var setupAdmin string
	var adminPass string
	flag.StringVar(&setupAdmin, "setup-admin", "", "Email of the admin user to create")
	flag.StringVar(&adminPass, "password", "", "Password for the new admin user")
	flag.Parse()

	// ── Logging ──────────────────────────────────────────────
	setupLogger(cfg)
	config.LogSafeConfig(cfg) // logs non-sensitive config at INFO; secrets are omitted

	log.Info().Str("addr", cfg.APIAddr).Msg("SkyBeam API starting")

	// ── Database ─────────────────────────────────────────────
	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	if setupAdmin != "" {
		if adminPass == "" {
			log.Fatal().Msg("must provide -password with -setup-admin")
		}
		
		authSvc := auth.NewService(pool, cfg.SessionTTLDays)
		if err := authSvc.CreateAdmin(context.Background(), setupAdmin, adminPass); err != nil {
			log.Fatal().Err(err).Msg("failed to create admin user")
		}
		fmt.Println("Admin user created successfully.")
		return
	}

	// ── Router ───────────────────────────────────────────────
	router := api.NewRouter(cfg, pool)

	// ── HTTP Server ──────────────────────────────────────────
	srv := &http.Server{
		Addr:         cfg.APIAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Graceful Shutdown ─────────────────────────────────────
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	log.Info().Str("addr", cfg.APIAddr).Msg("API server listening")
	<-done
	log.Info().Msg("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
	}

	log.Info().Msg("server stopped")
}

func setupLogger(cfg *config.Config) {
	if cfg.LogFormat == "pretty" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
}
