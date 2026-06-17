package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

// Logger is a zerolog-based structured request logger middleware.
// Logs method, path, status, latency, and request ID on every response.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("latency_ms", time.Since(start)).
			Str("request_id", middleware.GetReqID(r.Context())).
			Str("remote_ip", r.RemoteAddr).
			Msg("request")
	})
}
