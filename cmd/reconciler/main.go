package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/skybeam/mail/internal/config"
	"github.com/skybeam/mail/internal/db"
	"github.com/skybeam/mail/internal/reconciler"
)

func main() {
	cfg := config.Load()
	setupLogger(cfg)
	config.LogSafeConfig(cfg)

	log.Info().Msg("SkyBeam Reconciler starting")

	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	svc := reconciler.NewService(cfg, pool)
	go func() {
		if err := svc.Run(ctx); err != nil {
			log.Error().Err(err).Msg("reconciler exited with error")
		}
	}()

	log.Info().
		Str("light_interval", cfg.ReconcilerLightInterval.String()).
		Str("full_interval", cfg.ReconcilerFullInterval.String()).
		Msg("reconciler running")

	<-done
	log.Info().Msg("shutting down reconciler...")
	cancel()
	time.Sleep(3 * time.Second)
	log.Info().Msg("reconciler stopped")
}

func setupLogger(cfg *config.Config) {
	if cfg.LogFormat == "pretty" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	zerolog.SetGlobalLevel(level)
}
