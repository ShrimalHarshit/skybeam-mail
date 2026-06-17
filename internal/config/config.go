// Package config loads all service configuration from environment variables.
//
// # Loading order (highest priority first)
//
//  1. Environment variables already set in the process (Docker, CI, shell exports).
//  2. Values from a .env file, searched upward from the working directory.
//
// This means Docker Compose / production environment variables always win over
// the .env file, which is intentional. The .env file is a developer convenience
// and is never required in production.
//
// # Usage
//
//	cfg := config.Load() // panics with a clear message on misconfiguration
//
// # .env file discovery
//
// The loader walks up the directory tree from the current working directory
// looking for a .env file. This lets you run binaries from either the project
// root or a nested cmd/* directory and always find the right file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// Config holds the full runtime configuration for all three binaries.
type Config struct {
	// Database
	DatabaseURL string

	// API Server
	APIAddr   string
	APISecret string // used to sign session tokens

	// Dovecot IMAP
	DovecotHost       string
	DovecotPort       int
	DovecotMasterUser string
	DovecotMasterPass string

	// Postfix SMTP Submission
	PostfixHost string
	PostfixPort int

	// Session
	SessionTTLDays int

	// Reconciler
	ReconcilerLightInterval time.Duration
	ReconcilerFullInterval  time.Duration

	// Logging
	LogLevel  string
	LogFormat string
}

// Load reads configuration from environment variables, optionally loading a
// .env file first. It returns a fully validated *Config or panics with a
// human-readable message so misconfiguration fails fast at startup.
//
// Secrets are never logged.
func Load() *Config {
	loadDotEnv() // must happen before any os.Getenv call

	var missing []string

	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg := &Config{
		// ── Required ──────────────────────────────────────────────────
		DatabaseURL:       get("DATABASE_URL"),
		APISecret:         get("API_SECRET"),
		DovecotMasterUser: get("DOVECOT_MASTER_USER"),
		DovecotMasterPass: get("DOVECOT_MASTER_PASS"),

		// ── Optional with defaults ────────────────────────────────────
		APIAddr:                 envOrDefault("API_ADDR", ":8080"),
		DovecotHost:             envOrDefault("DOVECOT_HOST", "localhost"),
		DovecotPort:             envIntOrDefault("DOVECOT_PORT", 143),
		PostfixHost:             envOrDefault("POSTFIX_HOST", "localhost"),
		PostfixPort:             envIntOrDefault("POSTFIX_PORT", 587),
		SessionTTLDays:          envIntOrDefault("SESSION_TTL_DAYS", 30),
		ReconcilerLightInterval: envDurationOrDefault("RECONCILER_LIGHT_INTERVAL", 5*time.Minute),
		ReconcilerFullInterval:  envDurationOrDefault("RECONCILER_FULL_INTERVAL", 24*time.Hour),
		LogLevel:                envOrDefault("LOG_LEVEL", "info"),
		LogFormat:               envOrDefault("LOG_FORMAT", "json"),
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "\n[skybeam] FATAL: missing required environment variables:\n")
		for _, k := range missing {
			fmt.Fprintf(os.Stderr, "  • %s\n", k)
		}
		fmt.Fprintf(os.Stderr, "\nEnsure these are set in your shell, Docker environment, or .env file.\n")
		fmt.Fprintf(os.Stderr, "See .env.example for a complete reference.\n\n")
		os.Exit(1)
	}

	return cfg
}

// loadDotEnv finds and loads a .env file, walking up from the working
// directory. It logs whether a file was found and from where.
// It is a no-op in Docker/production where real env vars are already set.
func loadDotEnv() {
	path, err := findDotEnv()
	if err != nil {
		// No .env file found — this is fine in Docker/CI.
		// Only log at debug so production logs stay clean.
		// (zerolog may not be initialised yet; use stderr directly.)
		fmt.Fprintf(os.Stderr, "[skybeam] .env not found — using process environment only\n")
		return
	}

	// godotenv.Overload would overwrite existing env vars; Load does not.
	// We use Load so Docker env vars always take priority over .env values.
	if err := godotenv.Load(path); err != nil {
		fmt.Fprintf(os.Stderr, "[skybeam] WARNING: found .env at %s but could not load it: %v\n", path, err)
		return
	}

	fmt.Fprintf(os.Stderr, "[skybeam] loaded .env from %s\n", path)
}

// findDotEnv walks upward from the current working directory, looking for a
// .env file. Returns the absolute path of the first one found, or an error.
// Stops at the filesystem root or after 6 levels (safety limit).
func findDotEnv() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}

	return "", errors.New(".env not found")
}

// LogSafeConfig emits a startup log of non-sensitive configuration values.
// Call this after setupLogger() so the output format matches the rest of the logs.
func LogSafeConfig(cfg *Config) {
	log.Info().
		Str("api_addr", cfg.APIAddr).
		Str("dovecot_host", cfg.DovecotHost).
		Int("dovecot_port", cfg.DovecotPort).
		Str("dovecot_master_user", cfg.DovecotMasterUser). // username is not a secret
		Str("postfix_host", cfg.PostfixHost).
		Int("postfix_port", cfg.PostfixPort).
		Int("session_ttl_days", cfg.SessionTTLDays).
		Str("reconciler_light", cfg.ReconcilerLightInterval.String()).
		Str("reconciler_full", cfg.ReconcilerFullInterval.String()).
		Str("log_level", cfg.LogLevel).
		Str("log_format", cfg.LogFormat).
		// DATABASE_URL, API_SECRET, DOVECOT_MASTER_PASS intentionally omitted.
		Msg("config loaded")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[skybeam] WARNING: %s=%q is not a valid integer, using default %d\n", key, v, fallback)
		return fallback
	}
	return n
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[skybeam] WARNING: %s=%q is not a valid duration, using default %s\n", key, v, fallback)
		return fallback
	}
	return d
}
