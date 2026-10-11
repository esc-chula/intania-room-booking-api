package httpadapter

import (
	"log/slog"
	"net/http"

	"example.com/chula-room-booking/internal/adapter/primary/http/health"
	requestmiddleware "example.com/chula-room-booking/internal/adapter/primary/http/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func NewRouter(h *health.Handler, logger *slog.Logger, appEnv string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestmiddleware.RequestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)
	if appEnv == "development" {
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	}
	return r
}
