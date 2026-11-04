package httpadapter

import (
	"log/slog"
	"net/http"

	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/auth"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/health"
	requestmiddleware "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/middleware"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/sheetbooking"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type RouterConfig struct {
	Auth          *auth.Handler
	Health        *health.Handler
	SheetBookings *sheetbooking.Handler
	Logger        *slog.Logger
	AppEnv        string
}

func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestmiddleware.RequestLogger(config.Logger))
	r.Use(middleware.Recoverer)
	r.Get("/healthz", config.Health.Healthz)
	r.Get("/readyz", config.Health.Readyz)
	if config.Auth != nil {
		r.Route("/api/v1/auth", func(r chi.Router) {
			r.Post("/register", config.Auth.Register)
			r.Post("/login", config.Auth.Login)
			r.With(config.Auth.Authenticate).Get("/me", config.Auth.Me)
		})
	}
	if config.Auth != nil && config.SheetBookings != nil {
		r.With(config.Auth.Authenticate, config.Auth.RequireRoles(user.RoleOS)).Get("/api/v1/admin/sheet-bookings", config.SheetBookings.List)
	}
	if config.AppEnv == "development" {
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	}
	return r
}
