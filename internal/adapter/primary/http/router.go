package httpadapter

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/auth"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/health"
	requestmiddleware "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/middleware"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/sheetbooking"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type RouterConfig struct {
	Auth              *auth.Handler
	GoogleAuth        *auth.GoogleHandler
	Health            *health.Handler
	SheetBookings     *sheetbooking.Handler
	Logger            *slog.Logger
	AppEnv            string
	FrontendURL       string
	GoogleRedirectURL string
}

func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if config.FrontendURL != "" {
		apiURL := ""
		if config.AppEnv == "development" {
			apiURL = config.GoogleRedirectURL
		}
		r.Use(requestmiddleware.FrontendCORS(config.FrontendURL, apiURL))
	}
	r.Use(requestmiddleware.RequestLogger(config.Logger))
	r.Use(middleware.Recoverer)
	r.Get("/health", config.Health.Healthz)
	r.Get("/ready", config.Health.Readyz)
	if config.Auth != nil {
		r.Route("/api/v1/auth", func(r chi.Router) {
			r.With(config.Auth.Authenticate).Get("/me", config.Auth.Me)
			r.Post("/logout", config.Auth.Logout)
		})
	}
	if config.SheetBookings != nil {
		limitSheets := requestmiddleware.SheetRateLimit(func(ctx context.Context) (string, bool) {
			u, ok := auth.CurrentUser(ctx)
			return u.ID, ok
		})
		authenticate := authenticationUnavailable
		if config.Auth != nil {
			authenticate = config.Auth.Authenticate
		}
		r.With(authenticate, limitSheets).Get("/api/v1/admin/sheet-bookings", config.SheetBookings.List)
		r.With(authenticate, limitSheets).Get("/api/v1/admin/sheet-bookings/calendar", config.SheetBookings.Calendar)
		r.With(authenticate, limitSheets).Get("/api/v1/admin/sheet-bookings/weekly", config.SheetBookings.Weekly)
	}
	if config.GoogleAuth != nil {
		r.Get("/auth/google/login", config.GoogleAuth.Login)
		r.Get("/auth/google/callback", config.GoogleAuth.Callback)
	}
	if config.AppEnv == "development" {
		if config.GoogleAuth != nil {
			r.Get("/auth/google/url", config.GoogleAuth.URL)
		}
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))
	}
	return r
}

func authenticationUnavailable(_ http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"authentication unavailable"}`))
	})
}
