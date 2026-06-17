// Package auth handles password hashing, session token generation,
// and token verification against the sessions table.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// Session represents a row in the sessions table.
type Session struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	ExpiresAt time.Time
	LastSeen  time.Time
}

// Account is a minimal view of an accounts row.
type Account struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	DovecotUser string    `json:"-"`
	IsAdmin     bool      `json:"is_admin"`
}

// Service provides auth operations.
type Service struct {
	pool           *pgxpool.Pool
	sessionTTL     time.Duration
}

// NewService creates an auth Service.
func NewService(pool *pgxpool.Pool, sessionTTLDays int) *Service {
	return &Service{
		pool:       pool,
		sessionTTL: time.Duration(sessionTTLDays) * 24 * time.Hour,
	}
}

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Login verifies credentials and, on success, creates and returns a new bearer token.
// The returned token is the raw secret — it is stored only as a hash server-side.
func (s *Service) Login(ctx context.Context, email, password string) (token string, account *Account, err error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, email, display_name, dovecot_user, is_admin, password_hash
		 FROM accounts WHERE email = $1 AND is_active = true`, email)

	var acc Account
	var hash string
	if err := row.Scan(&acc.ID, &acc.Email, &acc.DisplayName, &acc.DovecotUser, &acc.IsAdmin, &hash); err != nil {
		if err == pgx.ErrNoRows {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("lookup account: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	token, tokenHash, err := generateToken()
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessions (account_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		acc.ID, tokenHash, expiresAt,
	)
	if err != nil {
		return "", nil, fmt.Errorf("create session: %w", err)
	}

	return token, &acc, nil
}

// Logout invalidates the session associated with the given token.
func (s *Service) Logout(ctx context.Context, token string) error {
	hash := hashToken(token)
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hash)
	return err
}

// VerifyToken looks up the session for the given token and returns the associated Account.
// It also updates last_seen on the session.
func (s *Service) VerifyToken(ctx context.Context, token string) (*Account, error) {
	hash := hashToken(token)
	now := time.Now().UTC()

	row := s.pool.QueryRow(ctx, `
		SELECT a.id, a.email, a.display_name, a.dovecot_user, a.is_admin, s.expires_at
		FROM sessions s
		JOIN accounts a ON a.id = s.account_id
		WHERE s.token_hash = $1 AND a.is_active = true
	`, hash)

	var acc Account
	var expiresAt time.Time
	if err := row.Scan(&acc.ID, &acc.Email, &acc.DisplayName, &acc.DovecotUser, &acc.IsAdmin, &expiresAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("verify token: %w", err)
	}

	if now.After(expiresAt) {
		return nil, ErrSessionExpired
	}

	// Update last_seen asynchronously — failure is non-fatal.
	go func() {
		_, _ = s.pool.Exec(context.Background(),
			`UPDATE sessions SET last_seen = $1 WHERE token_hash = $2`, now, hash)
	}()

	return &acc, nil
}

// ── Helpers ──────────────────────────────────────────────────

func generateToken() (raw, hashed string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateAdmin securely creates a new administrator account.
// If the email already exists, it is upgraded to an admin and the password updated.
func (s *Service) CreateAdmin(ctx context.Context, email, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	dovecotUser := email
	_, err = s.pool.Exec(ctx, `
		INSERT INTO accounts (email, display_name, dovecot_user, password_hash, is_admin)
		VALUES ($1, 'Admin', $2, $3, true)
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, is_admin = true
	`, email, dovecotUser, hash)
	return err
}

// ── Sentinel errors ───────────────────────────────────────────

var (
	ErrInvalidCredentials = fmt.Errorf("invalid email or password")
	ErrUnauthorized       = fmt.Errorf("unauthorized")
	ErrSessionExpired     = fmt.Errorf("session expired")
)
