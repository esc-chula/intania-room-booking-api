package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			path := chi.RouteContext(r.Context()).RoutePattern()
			if path == "" {
				path = r.URL.EscapedPath()
			}
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			logger.InfoContext(r.Context(), "http request", "method", r.Method, "path", path, "status", status, "latency", time.Since(start), "request_id", chimiddleware.GetReqID(r.Context()))
		})
	}
}
