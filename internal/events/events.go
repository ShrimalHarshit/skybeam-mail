// Package events defines the event schema and provides the writer
// for appending events to the PostgreSQL event store.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventType is a strongly-typed event name.
type EventType string

const (
	MessageDelivered  EventType = "MessageDelivered"
	MessageRead       EventType = "MessageRead"
	MessageUnread     EventType = "MessageUnread"
	MessageMoved      EventType = "MessageMoved"
	MessageDeleted    EventType = "MessageDeleted"
	MessageArchived   EventType = "MessageArchived"
	MessageStarred    EventType = "MessageStarred"
	MessageUnstarred  EventType = "MessageUnstarred"
	MessageSent       EventType = "MessageSent"
	LabelAdded        EventType = "LabelAdded"
	LabelRemoved      EventType = "LabelRemoved"
	FolderCreated     EventType = "FolderCreated"
	FolderDeleted     EventType = "FolderDeleted"
	ReconcileComplete EventType = "ReconcileCompleted"
)

// Event is the canonical event envelope stored in the events table.
type Event struct {
	EventID    uuid.UUID       `json:"event_id"`
	AccountID  uuid.UUID       `json:"account_id"`
	EventType  EventType       `json:"event_type"`
	MessageUID string          `json:"message_uid,omitempty"`
	Folder     string          `json:"folder,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	EmittedAt  time.Time       `json:"emitted_at"`
}

// ── Payload types ─────────────────────────────────────────────

type DeliveredPayload struct {
	From      string `json:"from"`
	Subject   string `json:"subject"`
	SizeBytes int    `json:"size_bytes"`
	MessageID string `json:"message_id"`
}

type MovedPayload struct {
	FromFolder string `json:"from_folder"`
	ToFolder   string `json:"to_folder"`
}

type DeletedPayload struct {
	FromFolder string `json:"from_folder"`
	Permanent  bool   `json:"permanent"`
}

type SentPayload struct {
	ToAddrs   []string `json:"to_addrs"`
	MessageID string   `json:"message_id"`
	Subject   string   `json:"subject"`
}

type LabelPayload struct {
	Label string `json:"label"`
}

type ReconcilePayload struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Deleted  int `json:"deleted"`
}

// ── Writer ───────────────────────────────────────────────────

const insertEventSQL = `
INSERT INTO events (event_id, account_id, event_type, message_uid, folder, payload, emitted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`

// Writer appends events to the PostgreSQL event store.
type Writer struct {
	pool *pgxpool.Pool
}

// NewWriter creates a new event Writer.
func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{pool: pool}
}

// Emit writes a single event. payload must be JSON-serialisable.
func (w *Writer) Emit(ctx context.Context, e Event) error {
	if e.EventID == uuid.Nil {
		e.EventID = uuid.New()
	}
	if e.EmittedAt.IsZero() {
		e.EmittedAt = time.Now().UTC()
	}
	if e.Payload == nil {
		e.Payload = json.RawMessage("{}")
	}

	_, err := w.pool.Exec(ctx, insertEventSQL,
		e.EventID,
		e.AccountID,
		string(e.EventType),
		e.MessageUID,
		e.Folder,
		e.Payload,
		e.EmittedAt,
	)
	if err != nil {
		return fmt.Errorf("emit event %s: %w", e.EventType, err)
	}
	return nil
}

// EmitWith marshals payload and calls Emit.
func (w *Writer) EmitWith(ctx context.Context, accountID uuid.UUID, eventType EventType,
	messageUID, folder string, payload any) error {

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	return w.Emit(ctx, Event{
		AccountID:  accountID,
		EventType:  eventType,
		MessageUID: messageUID,
		Folder:     folder,
		Payload:    raw,
	})
}
