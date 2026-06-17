package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/skybeam/mail/internal/api/middleware"
	"github.com/skybeam/mail/internal/events"
)

// EventHandler exposes the raw event log (debug/internal use only).
type EventHandler struct{ deps *Deps }

func NewEventHandler(deps *Deps) *EventHandler { return &EventHandler{deps: deps} }

type eventRow struct {
	ID        int64           `json:"id"`
	EventID   uuid.UUID       `json:"event_id"`
	EventType events.EventType `json:"event_type"`
	MessageUID string         `json:"message_uid"`
	Folder    string          `json:"folder"`
	EmittedAt time.Time       `json:"emitted_at"`
}

// List GET /api/v1/events?limit=100&after_id=0
func (h *EventHandler) List(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64)

	rows, err := h.deps.Pool.Query(r.Context(), `
		SELECT id, event_id, event_type, coalesce(message_uid,''), coalesce(folder,''), emitted_at
		FROM events
		WHERE account_id = $1 AND id > $2
		ORDER BY id ASC
		LIMIT $3
	`, acc.ID, afterID, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to list events")
		return
	}
	defer rows.Close()

	evts := make([]eventRow, 0, limit)
	for rows.Next() {
		var e eventRow
		_ = rows.Scan(&e.ID, &e.EventID, &e.EventType, &e.MessageUID, &e.Folder, &e.EmittedAt)
		evts = append(evts, e)
	}

	nextID := int64(0)
	if len(evts) > 0 {
		nextID = evts[len(evts)-1].ID
	}

	respond(w, http.StatusOK, map[string]any{
		"data":    evts,
		"next_id": nextID,
	})
}
