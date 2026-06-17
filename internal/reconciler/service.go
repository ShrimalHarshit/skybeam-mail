// Package reconciler implements the Dovecot ↔ PostgreSQL drift repair service.
// It runs on a schedule and ensures message_view and folder_view are consistent
// with what Dovecot actually holds. Dovecot always wins on conflict.
package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/skybeam/mail/internal/config"
	"github.com/skybeam/mail/internal/events"
	imapclient "github.com/skybeam/mail/internal/imap"
)

// Service runs periodic reconcile jobs for all active accounts.
type Service struct {
	cfg         *config.Config
	pool        *pgxpool.Pool
	eventWriter *events.Writer
}

// NewService creates a reconciler Service.
func NewService(cfg *config.Config, pool *pgxpool.Pool) *Service {
	return &Service{
		cfg:         cfg,
		pool:        pool,
		eventWriter: events.NewWriter(pool),
	}
}

// Run starts the reconciler loop. It blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	lightTicker := time.NewTicker(s.cfg.ReconcilerLightInterval)
	fullTicker := time.NewTicker(s.cfg.ReconcilerFullInterval)
	defer lightTicker.Stop()
	defer fullTicker.Stop()

	// Run a light sync immediately on startup.
	s.runAll(ctx, false)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-lightTicker.C:
			s.runAll(ctx, false)
		case <-fullTicker.C:
			s.runAll(ctx, true)
		}
	}
}

// runAll reconciles all active accounts.
func (s *Service) runAll(ctx context.Context, full bool) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, email, dovecot_user FROM accounts WHERE is_active = true`)
	if err != nil {
		log.Error().Err(err).Msg("reconciler: failed to list accounts")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var email, dovecotUser string
		if err := rows.Scan(&id, &email, &dovecotUser); err != nil {
			continue
		}

		go func(accountID uuid.UUID, user string) {
			if err := s.reconcileAccount(ctx, accountID, user, full); err != nil {
				log.Error().Err(err).Str("account", user).Msg("reconcile failed")
			}
		}(id, dovecotUser)
	}
}

// reconcileAccount performs the full reconcile algorithm for one account.
func (s *Service) reconcileAccount(ctx context.Context, accountID uuid.UUID, dovecotUser string, full bool) error {
	log.Debug().Str("user", dovecotUser).Bool("full", full).Msg("reconcile: starting")

	addr := fmt.Sprintf("%s:%d", s.cfg.DovecotHost, s.cfg.DovecotPort)
	c, err := imapclient.DialNoTLS(addr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	// Use Dovecot master credentials with user impersonation.
	loginUser := fmt.Sprintf("%s*%s", dovecotUser, s.cfg.DovecotMasterUser)
	if err := c.Login(loginUser, s.cfg.DovecotMasterPass); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	folders, err := c.ListFolders()
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}

	stats := events.ReconcilePayload{}

	for _, folder := range folders {
		sel, err := c.Select(folder.Name)
		if err != nil {
			log.Warn().Str("folder", folder.Name).Err(err).Msg("reconcile: skip folder")
			continue
		}
		if sel.NumMessages == 0 {
			continue
		}

		// TODO Week 9: implement full UID diff + insert/update/delete logic.
		// Placeholder counts for now.
		log.Debug().Str("folder", folder.Name).
			Uint32("messages", sel.NumMessages).
			Msg("reconcile: folder scanned")
	}

	// Refresh folder_view counts from message_view.
	if err := s.refreshFolderCounts(ctx, accountID); err != nil {
		log.Error().Err(err).Msg("reconcile: folder count refresh failed")
	}

	_ = s.eventWriter.EmitWith(ctx, accountID, events.ReconcileComplete, "", "", stats)
	log.Info().Str("user", dovecotUser).Msg("reconcile: complete")
	return nil
}

// refreshFolderCounts updates folder_view totals from message_view.
func (s *Service) refreshFolderCounts(ctx context.Context, accountID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO folder_view (account_id, folder_name, total_count, unread_count, last_synced_at)
		SELECT
			account_id,
			folder,
			COUNT(*) AS total_count,
			COUNT(*) FILTER (WHERE is_read = false) AS unread_count,
			now()
		FROM message_view
		WHERE account_id = $1
		  AND is_deleted = false
		GROUP BY account_id, folder
		ON CONFLICT (account_id, folder_name)
		DO UPDATE SET
			total_count  = EXCLUDED.total_count,
			unread_count = EXCLUDED.unread_count,
			last_synced_at = now()
	`, accountID)
	return err
}
