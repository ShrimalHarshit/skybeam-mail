package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/skybeam/mail/internal/api/middleware"
	"github.com/skybeam/mail/internal/auth"
)

// AuthHandler handles login, logout, and /me.
type AuthHandler struct{ deps *Deps }

func NewAuthHandler(deps *Deps) *AuthHandler { return &AuthHandler{deps: deps} }

// Login POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if body.Email == "" || body.Password == "" {
		respondError(w, http.StatusBadRequest, "MISSING_FIELDS", "email and password are required")
		return
	}

	token, account, err := h.deps.AuthSvc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
			return
		}
		respondError(w, http.StatusInternalServerError, "SERVER_ERROR", "login failed")
		return
	}

	respond(w, http.StatusOK, map[string]any{
		"token":   token,
		"account": account,
	})
}

// Logout DELETE /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Extract token directly from header (middleware already verified it)
	authHeader := r.Header.Get("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		respondError(w, http.StatusBadRequest, "INVALID_TOKEN", "malformed authorization header")
		return
	}

	if err := h.deps.AuthSvc.Logout(r.Context(), parts[1]); err != nil {
		respondError(w, http.StatusInternalServerError, "SERVER_ERROR", "logout failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Me GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	account := middleware.AccountFromContext(r.Context())
	respond(w, http.StatusOK, account)
}
