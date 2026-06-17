package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/skybeam/mail/internal/api/middleware"
	"github.com/skybeam/mail/internal/events"
)

// FolderHandler handles mailbox folder operations.
type FolderHandler struct{ deps *Deps }

func NewFolderHandler(deps *Deps) *FolderHandler { return &FolderHandler{deps: deps} }

// FolderView mirrors folder_view row.
type FolderView struct {
	FolderName  string `json:"name"`
	TotalCount  int    `json:"total"`
	UnreadCount int    `json:"unread"`
}

// List GET /api/v1/folders
func (h *FolderHandler) List(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())

	rows, err := h.deps.Pool.Query(r.Context(), `
		SELECT folder_name, total_count, unread_count
		FROM folder_view
		WHERE account_id = $1
		ORDER BY folder_name ASC
	`, acc.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to list folders")
		return
	}
	defer rows.Close()

	folders := make([]FolderView, 0)
	for rows.Next() {
		var f FolderView
		_ = rows.Scan(&f.FolderName, &f.TotalCount, &f.UnreadCount)
		folders = append(folders, f)
	}

	respond(w, http.StatusOK, map[string]any{"data": folders})
}

// Create POST /api/v1/folders
func (h *FolderHandler) Create(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())

	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "folder name is required")
		return
	}

	_, err := h.deps.Pool.Exec(r.Context(), `
		INSERT INTO folder_view (account_id, folder_name)
		VALUES ($1, $2)
		ON CONFLICT (account_id, folder_name) DO NOTHING
	`, acc.ID, body.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to create folder")
		return
	}

	_ = h.deps.EventWriter.EmitWith(r.Context(), acc.ID, events.FolderCreated,
		"", body.Name, map[string]string{"folder_name": body.Name})

	respond(w, http.StatusCreated, FolderView{FolderName: body.Name})
}

// Delete DELETE /api/v1/folders/:name
func (h *FolderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	acc := middleware.AccountFromContext(r.Context())
	name := chi.URLParam(r, "name")

	// Safety: prevent deleting system folders
	protected := map[string]bool{"INBOX": true, "Sent": true, "Trash": true, "Drafts": true, "Archive": true}
	if protected[name] {
		respondError(w, http.StatusForbidden, "PROTECTED_FOLDER", "cannot delete system folder")
		return
	}

	// Check folder is empty before deleting
	var count int
	_ = h.deps.Pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM message_view WHERE account_id = $1 AND folder = $2`,
		acc.ID, name).Scan(&count)

	if count > 0 {
		respondError(w, http.StatusConflict, "FOLDER_NOT_EMPTY", "folder must be empty before deletion")
		return
	}

	_, _ = h.deps.Pool.Exec(r.Context(),
		`DELETE FROM folder_view WHERE account_id = $1 AND folder_name = $2`, acc.ID, name)

	_ = h.deps.EventWriter.EmitWith(r.Context(), acc.ID, events.FolderDeleted,
		"", name, map[string]string{"folder_name": name})

	w.WriteHeader(http.StatusNoContent)
}
