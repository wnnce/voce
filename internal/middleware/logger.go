package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Logger is a simple structured logging middleware using slog.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Map chi's response writer to capture status and size
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			latency := time.Since(start)

			// Group log fields
			slog.Log(r.Context(), slog.LevelInfo, "HTTP Request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("latency", latency),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
