package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/skybeam/mail/internal/auth"
)

type AdminHandler struct {
	deps *Deps
}

func NewAdminHandler(deps *Deps) *AdminHandler {
	return &AdminHandler{deps: deps}
}

// ── Domains ────────────────────────────────────────────────────────

func (h *AdminHandler) ListDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := h.deps.Pool.Query(r.Context(), `SELECT id, name, created_at FROM domains ORDER BY name`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to fetch domains")
		return
	}
	defer rows.Close()

	var domains []map[string]any
	for rows.Next() {
		var id, name string
		var ca string
		if err := rows.Scan(&id, &name, &ca); err == nil {
			domains = append(domains, map[string]any{"id": id, "name": name, "created_at": ca})
		}
	}
	if domains == nil {
		domains = []map[string]any{}
	}
	respond(w, http.StatusOK, map[string]any{"data": domains})
}

func (h *AdminHandler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "domain name required")
		return
	}

	_, err := h.deps.Pool.Exec(r.Context(), `INSERT INTO domains (name) VALUES ($1) ON CONFLICT DO NOTHING`, body.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to create domain")
		return
	}
	respond(w, http.StatusCreated, map[string]string{"message": "domain created"})
}

func (h *AdminHandler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.deps.Pool.Exec(r.Context(), `DELETE FROM domains WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to delete domain")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Accounts ───────────────────────────────────────────────────────

func (h *AdminHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.deps.Pool.Query(r.Context(), `
		SELECT id, email, display_name, is_admin, is_active, created_at 
		FROM accounts ORDER BY email`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to fetch accounts")
		return
	}
	defer rows.Close()

	var accounts []map[string]any
	for rows.Next() {
		var id, email, display string
		var isAdmin, isActive bool
		var ca string
		if err := rows.Scan(&id, &email, &display, &isAdmin, &isActive, &ca); err == nil {
			accounts = append(accounts, map[string]any{
				"id": id, "email": email, "display_name": display, 
				"is_admin": isAdmin, "is_active": isActive, "created_at": ca,
			})
		}
	}
	if accounts == nil {
		accounts = []map[string]any{}
	}
	respond(w, http.StatusOK, map[string]any{"data": accounts})
}

func (h *AdminHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		IsAdmin     bool   `json:"is_admin"`
	}
	if err := decode(r, &body); err != nil || body.Email == "" || body.Password == "" {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "email and password required")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "HASH_ERROR", "failed to hash password")
		return
	}

	_, err = h.deps.Pool.Exec(r.Context(), `
		INSERT INTO accounts (email, display_name, dovecot_user, password_hash, is_admin)
		VALUES ($1, $2, $3, $4, $5)
	`, body.Email, body.DisplayName, body.Email, hash, body.IsAdmin)
	
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "failed to create account")
		return
	}

	respond(w, http.StatusCreated, map[string]string{"message": "account created"})
}
