package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv              string
	HTTPAddr            string
	LogLevel            slog.Level
	DatabaseURL         string
	JWTSecret           string
	GoogleClientID      string
	GoogleClientSecret  string
	GoogleRedirectURL   string
	FrontendURL         string
	AuthCookieSameSite  http.SameSite
	GoogleAllowedDomain string
	GoogleAllowedSuffix string
	SpreadsheetID       string
	SheetRange          string
	BootstrapAdminEmail string
	OSStudentIDs        string
	DBMaxConns          int32
	DBMinConns          int32
	DBConnectTimeout    time.Duration
	ShutdownTimeout     time.Duration
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
	c.GoogleClientID = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	c.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	c.GoogleRedirectURL = value("GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback")
	c.FrontendURL = value("FRONTEND_URL", "http://localhost:3000")
	cookieMode := value("AUTH_COOKIE_SAME_SITE", "lax")
	if c.AppEnv == "production" {
		cookieMode = value("AUTH_COOKIE_SAME_SITE", "none")
	}
	switch strings.ToLower(cookieMode) {
	case "lax":
		c.AuthCookieSameSite = http.SameSiteLaxMode
	case "none":
		c.AuthCookieSameSite = http.SameSiteNoneMode
	default:
		return c, fmt.Errorf("AUTH_COOKIE_SAME_SITE must be lax or none")
	}
	c.GoogleAllowedDomain = value("GOOGLE_ALLOWED_DOMAIN", "student.chula.ac.th")
	c.GoogleAllowedSuffix = value("GOOGLE_ALLOWED_SUFFIX", "21")
	if strings.TrimSpace(c.GoogleAllowedDomain) == "" || strings.ContainsAny(c.GoogleAllowedDomain, "@/ \t\r\n") || c.GoogleAllowedSuffix == "" {
		return c, fmt.Errorf("GOOGLE_ALLOWED_DOMAIN and GOOGLE_ALLOWED_SUFFIX must be valid nonempty values")
	}
	if c.AppEnv == "production" && (c.GoogleClientID == "" || c.GoogleClientSecret == "") {
		return c, fmt.Errorf("Google OAuth client ID and secret are required in production")
	}
	if c.GoogleClientID != "" || c.GoogleClientSecret != "" {
		redirect, err := url.Parse(c.GoogleRedirectURL)
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" || err != nil || redirect.Host == "" || redirect.Path != "/auth/google/callback" || redirect.RawQuery != "" || redirect.Fragment != "" || redirect.User != nil || redirect.Scheme != "http" && redirect.Scheme != "https" || redirect.Scheme == "http" && (c.AppEnv == "production" || redirect.Hostname() != "localhost" && redirect.Hostname() != "127.0.0.1") || c.AuthCookieSameSite == http.SameSiteNoneMode && redirect.Scheme != "https" {
			return c, fmt.Errorf("Google OAuth needs a client ID, client secret, and valid callback URL")
		}
	}
	frontend, err := url.Parse(c.FrontendURL)
	if err != nil || frontend.Host == "" || frontend.User != nil || frontend.RawQuery != "" || frontend.Fragment != "" || frontend.Path != "" && frontend.Path != "/" || frontend.Scheme != "https" && (c.AppEnv == "production" || frontend.Scheme != "http" || frontend.Hostname() != "localhost" && frontend.Hostname() != "127.0.0.1") {
		return c, fmt.Errorf("FRONTEND_URL must be a frontend origin (HTTPS in production)")
	}
	c.FrontendURL = frontend.Scheme + "://" + frontend.Host
	c.SpreadsheetID = os.Getenv("GOOGLE_SPREADSHEET_ID")
	if strings.TrimSpace(c.SpreadsheetID) == "" {
		return c, fmt.Errorf("GOOGLE_SPREADSHEET_ID is required")
	}
	c.SheetRange = os.Getenv("GOOGLE_SHEET_RANGE")
	if strings.TrimSpace(c.SheetRange) == "" {
		return c, fmt.Errorf("GOOGLE_SHEET_RANGE is required")
	}
	c.BootstrapAdminEmail = os.Getenv("BOOTSTRAP_ADMIN_EMAIL")
	c.OSStudentIDs = os.Getenv("OS_STUDENT_IDS")
	c.ShutdownTimeout, err = duration("SHUTDOWN_TIMEOUT", "10s")
	if err != nil {
		return c, err
	}
	return c, loadDatabaseSettings(&c)
}

// LoadBootstrapAdmin reads only the settings required by the one-off admin command.
func LoadBootstrapAdmin() (Config, error) {
	c := Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		BootstrapAdminEmail: os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		GoogleAllowedDomain: value("GOOGLE_ALLOWED_DOMAIN", "student.chula.ac.th"),
		GoogleAllowedSuffix: value("GOOGLE_ALLOWED_SUFFIX", "21"),
	}
	return c, loadDatabaseSettings(&c)
}

func LoadBootstrapOS() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), OSStudentIDs: os.Getenv("OS_STUDENT_IDS"), GoogleAllowedSuffix: value("GOOGLE_ALLOWED_SUFFIX", "21")}
	if c.GoogleAllowedSuffix == "" {
		return c, fmt.Errorf("GOOGLE_ALLOWED_SUFFIX must be nonempty")
	}
	return c, loadDatabaseSettings(&c)
}

func loadDatabaseSettings(c *Config) error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	max, err := strconv.ParseInt(value("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || max < 1 {
		return fmt.Errorf("DB_MAX_CONNS must be a positive 32-bit integer")
	}
	min, err := strconv.ParseInt(value("DB_MIN_CONNS", "0"), 10, 32)
	if err != nil || min < 0 || min > max {
		return fmt.Errorf("DB_MIN_CONNS must be between zero and DB_MAX_CONNS")
	}
	c.DBMaxConns, c.DBMinConns = int32(max), int32(min)
	c.DBConnectTimeout, err = duration("DB_CONNECT_TIMEOUT", "5s")
	if err != nil {
		return err
	}
	return nil
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
