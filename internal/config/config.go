package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv           string
	HTTPAddr         string
	LogLevel         slog.Level
	DatabaseURL      string
	JWTSecret        string
	DBMaxConns       int32
	DBMinConns       int32
	DBConnectTimeout time.Duration
	ShutdownTimeout  time.Duration
}

func Load() (Config, error) {
	c := Config{AppEnv: value("APP_ENV", "development"), HTTPAddr: value("HTTP_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL")}
	if c.AppEnv != "development" && c.AppEnv != "production" {
		return c, fmt.Errorf("APP_ENV must be development or production")
	}
	host, port, err := net.SplitHostPort(c.HTTPAddr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 || strings.ContainsAny(host, " \t\r\n/\\") {
		return c, fmt.Errorf("HTTP_ADDR must be a valid host:port with port 1-65535")
	}
	switch strings.ToLower(value("LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		return c, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	c.JWTSecret = os.Getenv("JWT_SECRET")
	if len(strings.TrimSpace(c.JWTSecret)) < 32 {
		return c, fmt.Errorf("JWT_SECRET must contain at least 32 non-padding bytes")
	}
	max, err := strconv.ParseInt(value("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || max < 1 {
		return c, fmt.Errorf("DB_MAX_CONNS must be a positive 32-bit integer")
	}
	min, err := strconv.ParseInt(value("DB_MIN_CONNS", "0"), 10, 32)
	if err != nil || min < 0 || min > max {
		return c, fmt.Errorf("DB_MIN_CONNS must be between zero and DB_MAX_CONNS")
	}
	c.DBMaxConns, c.DBMinConns = int32(max), int32(min)
	c.DBConnectTimeout, err = duration("DB_CONNECT_TIMEOUT", "5s")
	if err != nil {
		return c, err
	}
	c.ShutdownTimeout, err = duration("SHUTDOWN_TIMEOUT", "10s")
	return c, err
}

func value(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func duration(key, fallback string) (time.Duration, error) {
	d, err := time.ParseDuration(value(key, fallback))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return d, nil
}
