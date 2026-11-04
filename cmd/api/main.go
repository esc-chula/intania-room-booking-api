package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
// @description Email/password authentication with one-hour stateless bearer JWTs and OS-only live spreadsheet bookings. OAuth integration is planned.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter Bearer followed by a space and your access token.
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
	authService, err := auth.New(postgres.NewUsers(pool), tokens)
	if err != nil {
		return err
	}
	sheetReader, err := googlesheets.New(ctx, c.SpreadsheetID, c.SheetRange)
	if err != nil {
		return err
	}
	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Auth:          httpauth.New(authService, logger),
		Health:        healthHandler,
		SheetBookings: httpsheetbooking.New(sheetbooking.New(sheetReader), logger),
		Logger:        logger,
		AppEnv:        c.AppEnv,
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
