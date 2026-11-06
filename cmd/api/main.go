package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/MadManJJ/room-reservation-api/docs"
	httpadapter "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http"
	httpauth "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/auth"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/health"
	httpsheetbooking "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/sheetbooking"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/database"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/googlesheets"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/token"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/repository/postgres"
	"github.com/MadManJJ/room-reservation-api/internal/config"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/sheetbooking"
)

// @title Chula Room Booking API
// @version 0.1.0
// @description Google OAuth sign-in sets a one-hour HttpOnly authentication cookie. OS-only live spreadsheet bookings are available to authenticated users.
// @BasePath /
func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: c.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	healthHandler := health.New(pool, logger)
	tokens, err := token.New(c.JWTSecret)
	if err != nil {
		return err
	}
	authService := auth.New(postgres.NewUsers(pool), tokens, c.GoogleAllowedDomain, c.GoogleAllowedSuffix)
	var googleAuth *httpauth.GoogleHandler
	if c.GoogleClientID != "" {
		googleAuth, err = httpauth.NewGoogle(ctx, authService, logger, c.GoogleClientID, c.GoogleClientSecret, c.GoogleRedirectURL, c.FrontendURL, c.GoogleAllowedDomain, c.AuthCookieSameSite)
		if err != nil {
			return err
		}
	}
	mapping, err := os.ReadFile("rooms.json")
	if err != nil {
		return err
	}
	var rooms map[string][]string
	if err := json.Unmarshal(mapping, &rooms); err != nil {
		return err
	}
	buildings := make(map[string]bool)
	for name := range rooms {
		buildings[name] = true
	}
	sheetReader, err := googlesheets.New(ctx, c.SpreadsheetID, c.SheetRange)
	if err != nil {
		return err
	}
	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Auth:              httpauth.New(authService, logger, strings.HasPrefix(c.GoogleRedirectURL, "https://"), c.AuthCookieSameSite),
		GoogleAuth:        googleAuth,
		Health:            healthHandler,
		SheetBookings:     httpsheetbooking.New(sheetbooking.New(sheetReader, buildings), logger),
		Logger:            logger,
		AppEnv:            c.AppEnv,
		FrontendURL:       c.FrontendURL,
		GoogleRedirectURL: c.GoogleRedirectURL,
	})
	server := &http.Server{
		Addr:              c.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	served := make(chan error, 1)
	go func() { served <- server.ListenAndServe() }()
	logger.Info("HTTP server starting", "address", c.HTTPAddr, "environment", c.AppEnv)
	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stop()
		logger.Info("HTTP server shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		logger.Info("HTTP server stopped")
		return nil
	}
}
