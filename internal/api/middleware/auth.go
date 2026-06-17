package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/skybeam/mail/internal/auth"
)

type contextKey string

const AccountContextKey contextKey = "account"

// RequireAuth validates the Bearer token on every request and injects
// the Account into the request context. Returns 401 on any failure.
func RequireAuth(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				respondUnauthorized(w)
				return
			}

			account, err := svc.VerifyToken(r.Context(), token)
			if err != nil {
				respondUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), AccountContextKey, account)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AccountFromContext extracts the authenticated account from the context.
// Panics if RequireAuth middleware was not applied — this is intentional
// to catch mis-wired routes during development.
func AccountFromContext(ctx context.Context) *auth.Account {
	acc, ok := ctx.Value(AccountContextKey).(*auth.Account)
	if !ok || acc == nil {
		panic("AccountFromContext: no account in context — RequireAuth middleware missing")
	}
	return acc
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}

func respondUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="skybeam"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","code":"UNAUTHORIZED"}`))
}
