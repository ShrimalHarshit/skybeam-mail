package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/skybeam/mail/internal/api/middleware"
	"github.com/skybeam/mail/internal/events"
)

// MessageView mirrors the message_view table row.
type MessageView struct {
	ID             uuid.UUID  `json:"id"`
	DovecotUID     string     `json:"dovecot_uid"`
	Folder         string     `json:"folder"`
	MessageID      string     `json:"message_id"`
	ThreadID       string     `json:"thread_id"`
	Subject        string     `json:"subject"`
	FromAddr       string     `json:"from"`
	ToAddrs        []string   `json:"to"`
	Date           time.Time  `json:"date"`
	SizeBytes      int        `json:"size_bytes"`
	IsRead         bool       `json:"is_read"`
	IsStarred      bool       `json:"is_starred"`
	HasAttachments bool       `json:"has_attachments"`
	Labels         []string   `json:"labels"`
	Snippet        string     `json:"snippet"`
	BodyText       string     `json:"body_text,omitempty"`
	BodyHTML       string     `json:"body_html,omitempty"`
}

// MessageHandler handles message CRUD operations.
type MessageHandler struct{ deps *Deps }

func NewMessageHandler(deps *Deps) *MessageHandler { return &MessageHandler{deps: deps} }

// List GET /api/v1/messages
func (h *MessageHandler) List(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())
	q := r.URL.Query()

	folder := q.Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit
	unreadOnly := q.Get("unread_only") == "true"

	query := `
		SELECT id, dovecot_uid, folder, message_id, thread_id,
		       subject, from_addr, to_addrs, date, size_bytes,
		       is_read, is_starred, has_attachments, labels, snippet
		FROM message_view
		WHERE account_id = $1 AND folder = $2
		AND ($3 = false OR is_read = false)
		ORDER BY date DESC
		LIMIT $4 OFFSET $5`

	rows, err := h.deps.Pool.Query(r.Context(), query, acc.ID, folder, unreadOnly, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to list messages")
		return
	}
	defer rows.Close()

	msgs := make([]MessageView, 0, limit)
	for rows.Next() {
		var m MessageView
		if err := rows.Scan(&m.ID, &m.DovecotUID, &m.Folder, &m.MessageID, &m.ThreadID,
			&m.Subject, &m.FromAddr, &m.ToAddrs, &m.Date, &m.SizeBytes,
			&m.IsRead, &m.IsStarred, &m.HasAttachments, &m.Labels, &m.Snippet); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}

	respond(w, http.StatusOK, map[string]any{
		"data":  msgs,
		"page":  page,
		"limit": limit,
	})
}

// Get GET /api/v1/messages/:id
func (h *MessageHandler) Get(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid message id")
		return
	}

	var m MessageView
	err = h.deps.Pool.QueryRow(r.Context(), `
		SELECT id, dovecot_uid, folder, message_id, thread_id,
		       subject, from_addr, to_addrs, date, size_bytes,
		       is_read, is_starred, has_attachments, labels, snippet,
		       body_text, body_html
		FROM message_view
		WHERE id = $1 AND account_id = $2
	`, id, acc.ID).Scan(
		&m.ID, &m.DovecotUID, &m.Folder, &m.MessageID, &m.ThreadID,
		&m.Subject, &m.FromAddr, &m.ToAddrs, &m.Date, &m.SizeBytes,
		&m.IsRead, &m.IsStarred, &m.HasAttachments, &m.Labels, &m.Snippet,
		&m.BodyText, &m.BodyHTML,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "message not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to get message")
		return
	}

	respond(w, http.StatusOK, m)
}

// Raw GET /api/v1/messages/:id/raw
// TODO Week 5: fetch raw RFC 2822 bytes from Dovecot IMAP on demand.
func (h *MessageHandler) Raw(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "raw message fetch coming in week 5")
}

// Update PATCH /api/v1/messages/:id
func (h *MessageHandler) Update(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid message id")
		return
	}

	var body struct {
		IsRead    *bool    `json:"is_read"`
		IsStarred *bool    `json:"is_starred"`
		Folder    *string  `json:"folder"`
		Labels    []string `json:"labels"`
	}
	if err := decode(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	// Fetch current state
	var current MessageView
	err = h.deps.Pool.QueryRow(r.Context(),
		`SELECT id, dovecot_uid, folder, is_read, is_starred FROM message_view
		 WHERE id = $1 AND account_id = $2`, id, acc.ID).
		Scan(&current.ID, &current.DovecotUID, &current.Folder, &current.IsRead, &current.IsStarred)
	if err == pgx.ErrNoRows {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "message not found")
		return
	}

	// Apply updates to DB (Dovecot sync handled by watcher/reconciler in later weeks)
	if body.IsRead != nil {
		_, _ = h.deps.Pool.Exec(r.Context(),
			`UPDATE message_view SET is_read = $1 WHERE id = $2`, *body.IsRead, id)

		eventType := events.MessageRead
		if !*body.IsRead {
			eventType = events.MessageUnread
		}
		_ = h.deps.EventWriter.EmitWith(r.Context(), acc.ID, eventType,
			current.DovecotUID, current.Folder, map[string]any{})
	}

	if body.IsStarred != nil {
		_, _ = h.deps.Pool.Exec(r.Context(),
			`UPDATE message_view SET is_starred = $1 WHERE id = $2`, *body.IsStarred, id)

		eventType := events.MessageStarred
		if !*body.IsStarred {
			eventType = events.MessageUnstarred
		}
		_ = h.deps.EventWriter.EmitWith(r.Context(), acc.ID, eventType,
			current.DovecotUID, current.Folder, map[string]any{})
	}

	w.WriteHeader(http.StatusNoContent)
}

// Delete DELETE /api/v1/messages/:id
func (h *MessageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid message id")
		return
	}

	permanent := r.URL.Query().Get("permanent") == "true"

	var m MessageView
	err = h.deps.Pool.QueryRow(r.Context(),
		`SELECT dovecot_uid, folder FROM message_view WHERE id = $1 AND account_id = $2`,
		id, acc.ID).Scan(&m.DovecotUID, &m.Folder)
	if err == pgx.ErrNoRows {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "message not found")
		return
	}

	if permanent {
		_, _ = h.deps.Pool.Exec(r.Context(),
			`UPDATE message_view SET is_deleted = true WHERE id = $1`, id)
	} else {
		_, _ = h.deps.Pool.Exec(r.Context(),
			`UPDATE message_view SET folder = 'Trash' WHERE id = $1`, id)
	}

	_ = h.deps.EventWriter.EmitWith(r.Context(), acc.ID, events.MessageDeleted,
		m.DovecotUID, m.Folder, events.DeletedPayload{
			FromFolder: m.Folder,
			Permanent:  permanent,
		})

	w.WriteHeader(http.StatusNoContent)
}

// Thread GET /api/v1/threads/:threadID
func (h *MessageHandler) Thread(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())
	threadID := chi.URLParam(r, "threadID")

	rows, err := h.deps.Pool.Query(r.Context(), `
		SELECT id, dovecot_uid, folder, message_id, thread_id,
		       subject, from_addr, to_addrs, date, size_bytes,
		       is_read, is_starred, has_attachments, labels, snippet
		FROM message_view
		WHERE account_id = $1 AND thread_id = $2
		ORDER BY date ASC
	`, acc.ID, threadID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to get thread")
		return
	}
	defer rows.Close()

	msgs := make([]MessageView, 0)
	for rows.Next() {
		var m MessageView
		_ = rows.Scan(&m.ID, &m.DovecotUID, &m.Folder, &m.MessageID, &m.ThreadID,
			&m.Subject, &m.FromAddr, &m.ToAddrs, &m.Date, &m.SizeBytes,
			&m.IsRead, &m.IsStarred, &m.HasAttachments, &m.Labels, &m.Snippet)
		msgs = append(msgs, m)
	}

	respond(w, http.StatusOK, map[string]any{"data": msgs})
}

// Send POST /api/v1/messages/send
func (h *MessageHandler) Send(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "send coming in week 7")
}
