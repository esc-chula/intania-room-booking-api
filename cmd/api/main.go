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

	_ "example.com/chula-room-booking/docs"
	httpadapter "example.com/chula-room-booking/internal/adapter/primary/http"
	"example.com/chula-room-booking/internal/adapter/primary/http/health"
	"example.com/chula-room-booking/internal/adapter/secondary/infrastructure/database"
	"example.com/chula-room-booking/internal/config"
)

// @title Chula Room Booking API
// @version 0.1.0
// @description Phase 0 foundation; no business operations yet. Future authentication uses Chula SSO and PostgreSQL-backed server-side sessions with secure cookies and CSRF protection, not bearer JWT.
// @BasePath /api/v1
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
	router := httpadapter.NewRouter(httpadapter.RouterConfig{
		Health: healthHandler,
		Logger: logger,
		AppEnv: c.AppEnv,
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
