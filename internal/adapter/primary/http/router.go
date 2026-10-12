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

type RouterConfig struct {
	Health *health.Handler
	Logger *slog.Logger
	AppEnv string
}

func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestmiddleware.RequestLogger(config.Logger))
	r.Use(middleware.Recoverer)
	r.Get("/healthz", config.Health.Healthz)
	r.Get("/readyz", config.Health.Readyz)
	if config.AppEnv == "development" {
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	}
	return r
}
