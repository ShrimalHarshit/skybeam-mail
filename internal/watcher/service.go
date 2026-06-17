// Package watcher implements the IMAP IDLE watcher service.
// It maintains one IMAP connection per active account, listens for
// new message delivery via IMAP IDLE, and emits events to PostgreSQL.
package watcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-message/mail"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"github.com/skybeam/mail/internal/config"
	"github.com/skybeam/mail/internal/events"
	imapclient "github.com/skybeam/mail/internal/imap"
)

// Service watches all active accounts via IMAP IDLE.
type Service struct {
	cfg         *config.Config
	pool        *pgxpool.Pool
	eventWriter *events.Writer
}

// NewService creates a watcher Service.
func NewService(cfg *config.Config, pool *pgxpool.Pool) *Service {
	return &Service{
		cfg:         cfg,
		pool:        pool,
		eventWriter: events.NewWriter(pool),
	}
}

// Run starts a watcher goroutine for each active account and blocks
// until ctx is cancelled. It re-polls the accounts list every 5 minutes
// to pick up new accounts.
func (s *Service) Run(ctx context.Context) error {
	var mu sync.Mutex
	watching := make(map[uuid.UUID]context.CancelFunc)

	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()

	refresh := func() {
		accounts, err := s.listAccounts(ctx)
		if err != nil {
			log.Error().Err(err).Msg("watcher: failed to list accounts")
			return
		}

		mu.Lock()
		defer mu.Unlock()

		for _, acc := range accounts {
			if _, ok := watching[acc.ID]; ok {
				continue // already watching
			}
			accCtx, cancel := context.WithCancel(ctx)
			watching[acc.ID] = cancel
			go s.watchAccount(accCtx, acc)
		}
	}

	refresh()

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, cancel := range watching {
				cancel()
			}
			mu.Unlock()
			return nil
		case <-tick.C:
			refresh()
		}
	}
}

type accountInfo struct {
	ID          uuid.UUID
	Email       string
	DovecotUser string
}

func (s *Service) listAccounts(ctx context.Context) ([]accountInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, email, dovecot_user FROM accounts WHERE is_active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []accountInfo
	for rows.Next() {
		var a accountInfo
		if err := rows.Scan(&a.ID, &a.Email, &a.DovecotUser); err == nil {
			accounts = append(accounts, a)
		}
	}
	return accounts, nil
}

// watchAccount runs IMAP IDLE for a single account indefinitely.
// On any error it backs off and reconnects.
func (s *Service) watchAccount(ctx context.Context, acc accountInfo) {
	log.Info().Str("user", acc.DovecotUser).Msg("watcher: starting watch")

	backoff := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := s.idleLoop(ctx, acc)
		if err != nil {
			log.Warn().Err(err).Str("user", acc.DovecotUser).
				Dur("backoff", backoff).Msg("watcher: reconnecting")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Minute {
				backoff *= 2
			}
		} else {
			backoff = 5 * time.Second
		}
	}
}

// idleLoop opens an IMAP connection and loops IDLE forever.
func (s *Service) idleLoop(ctx context.Context, acc accountInfo) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.DovecotHost, s.cfg.DovecotPort)
	c, err := imapclient.DialNoTLS(addr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer c.Logout()

	loginUser := fmt.Sprintf("%s*%s", acc.DovecotUser, s.cfg.DovecotMasterUser)
	if err := c.Login(loginUser, s.cfg.DovecotMasterPass); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	if _, err := c.Select("INBOX"); err != nil {
		return fmt.Errorf("select INBOX: %w", err)
	}

	// Initial scan to catch anything delivered while we were offline
	if err := s.scanNewMessages(ctx, c, acc); err != nil {
		log.Warn().Err(err).Str("user", acc.DovecotUser).Msg("watcher: initial scan error")
	}

	log.Debug().Str("user", acc.DovecotUser).Msg("watcher: entering IDLE")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// IDLE for up to 25 minutes (RFC 2177 recommends < 29 min).
		if err := c.Idle(25 * time.Minute); err != nil {
			return fmt.Errorf("idle: %w", err)
		}

		// IDLE woke up — something changed. Scan for new messages.
		if err := s.scanNewMessages(ctx, c, acc); err != nil {
			log.Warn().Err(err).Str("user", acc.DovecotUser).Msg("watcher: scan error")
		}
	}
}

// scanNewMessages fetches all messages in INBOX, upserts them into
// message_view, and emits MessageDelivered events for genuinely new messages.
// It is idempotent — safe to call after every IDLE wake-up.
func (s *Service) scanNewMessages(ctx context.Context, c *imapclient.Client, acc accountInfo) error {
	log.Debug().Str("user", acc.DovecotUser).Msg("watcher: scan triggered")

	// Fetch envelope + flags + size for all messages in INBOX (1:*).
	allRange := imap.SeqSet{}
	allRange.AddRange(1, 0) // 1:* — 0 means "*" in go-imap/v2
	msgs, err := c.FetchHeaders(allRange)
	if err != nil {
		return fmt.Errorf("fetch headers: %w", err)
	}

	if len(msgs) == 0 {
		log.Debug().Str("user", acc.DovecotUser).Msg("watcher: no messages in INBOX")
		return nil
	}

	// Collect UIDs already tracked in message_view.
	rows, err := s.pool.Query(ctx,
		`SELECT dovecot_uid FROM message_view WHERE account_id = $1 AND folder = 'INBOX'`,
		acc.ID)
	if err != nil {
		return fmt.Errorf("query existing: %w", err)
	}
	known := make(map[string]struct{})
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err == nil {
			known[uid] = struct{}{}
		}
	}
	rows.Close()

	newCount := 0
	for _, m := range msgs {
		uid := fmt.Sprintf("%d", m.UID)

		// Extract envelope fields safely.
		var (
			subject   string
			fromAddr  string
			toAddrs   []string
			msgID     string
			threadID  string
			date      time.Time
		)

		if m.Envelope != nil {
			subject = m.Envelope.Subject
			msgID   = m.Envelope.MessageID
			date    = m.Envelope.Date

			if len(m.Envelope.From) > 0 {
				addr := m.Envelope.From[0]
				if addr.Name != "" {
					fromAddr = fmt.Sprintf("%s <%s@%s>", addr.Name, addr.Mailbox, addr.Host)
				} else {
					fromAddr = fmt.Sprintf("%s@%s", addr.Mailbox, addr.Host)
				}
			}
			for _, addr := range m.Envelope.To {
				toAddrs = append(toAddrs, fmt.Sprintf("%s@%s", addr.Mailbox, addr.Host))
			}
		}

		// Thread ID: normalise the Message-ID into a stable grouping key.
		// A proper implementation would follow In-Reply-To chains; for now
		// each message is its own thread root (overridden by reconciler later).
		if msgID != "" {
			threadID = msgID
		} else {
			threadID = uid // fallback: isolated thread
		}

		isRead := false
		for _, f := range m.Flags {
			if f == imap.FlagSeen {
				isRead = true
			}
		}

		// If we already know this message, just upsert headers (updates sync time/read state).
		if _, alreadyKnown := known[uid]; alreadyKnown {
			_, err := s.pool.Exec(ctx, `
				INSERT INTO message_view
					(account_id, dovecot_uid, folder, message_id, thread_id,
					 subject, from_addr, to_addrs, date, size_bytes, is_read, last_synced_at)
				VALUES ($1,$2,'INBOX',$3,$4,$5,$6,$7::jsonb,$8,$9,$10,now())
				ON CONFLICT (account_id, dovecot_uid, folder)
				DO UPDATE SET last_synced_at = now(), is_read = EXCLUDED.is_read`,
				acc.ID, uid, msgID, threadID,
				subject, fromAddr, toJSONB(toAddrs),
				nullTime(date), int(m.SizeBytes), isRead,
			)
			if err != nil {
				log.Warn().Err(err).Str("uid", uid).Msg("watcher: upsert headers failed")
			}
			continue
		}

		// New message: Fetch the full body to extract text, html, and snippet.
		var bodyText, bodyHTML, snippet string
		bodyBytes, err := c.FetchBody(m.UID)
		if err == nil {
			bodyText, bodyHTML = extractBody(bodyBytes)
			
			// Generate snippet: first 200 plain text characters
			cleanText := strings.ReplaceAll(strings.ReplaceAll(bodyText, "\n", " "), "\r", " ")
			cleanText = strings.Join(strings.Fields(cleanText), " ") // collapse multiple spaces
			if len(cleanText) > 200 {
				snippet = cleanText[:200]
			} else {
				snippet = cleanText
			}
		} else {
			log.Warn().Err(err).Str("uid", uid).Msg("watcher: failed to fetch body")
		}

		// Upsert full message
		_, err = s.pool.Exec(ctx, `
			INSERT INTO message_view
				(account_id, dovecot_uid, folder, message_id, thread_id,
				 subject, from_addr, to_addrs, date, size_bytes, is_read, last_synced_at,
				 body_text, body_html, snippet)
			VALUES ($1,$2,'INBOX',$3,$4,$5,$6,$7::jsonb,$8,$9,$10,now(),$11,$12,$13)
			ON CONFLICT (account_id, dovecot_uid, folder)
			DO UPDATE SET last_synced_at = now()`,
			acc.ID, uid, msgID, threadID,
			subject, fromAddr, toJSONB(toAddrs),
			nullTime(date), int(m.SizeBytes), isRead,
			bodyText, bodyHTML, snippet,
		)
		if err != nil {
			log.Warn().Err(err).Str("uid", uid).Msg("watcher: upsert full message failed")
			continue
		}

		_ = s.eventWriter.EmitWith(ctx, acc.ID, events.MessageDelivered, uid, "INBOX",
			map[string]any{"subject": subject, "from": fromAddr, "snippet": snippet})
		newCount++
	}

	log.Info().
		Str("user", acc.DovecotUser).
		Int("total_imap", len(msgs)).
		Int("new", newCount).
		Msg("watcher: scan complete")

	return nil
}

// toJSONB encodes a string slice to a Postgres JSONB literal.
func toJSONB(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	b := []byte{'['}
	for i, s := range ss {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = append(b, []byte(s)...)
		b = append(b, '"')
	}
	b = append(b, ']')
	return string(b)
}

// nullTime returns nil if t is zero (avoids storing zero timestamps).
func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// extractBody parses a raw RFC 2822 message and extracts text and HTML parts.
func extractBody(raw []byte) (textBody, htmlBody string) {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return "", ""
	}

	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			continue
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := h.ContentType()
			b, _ := io.ReadAll(p.Body)
			
			if contentType == "text/plain" {
				textBody += string(b)
			} else if contentType == "text/html" {
				htmlBody += string(b)
			}
		}
	}

	return strings.TrimSpace(textBody), strings.TrimSpace(htmlBody)
}
