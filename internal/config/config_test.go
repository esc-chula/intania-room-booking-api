package config

import (
	"os"
	"testing"
	"time"
)

func defaults(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_ENV", "HTTP_ADDR", "LOG_LEVEL", "DB_MAX_CONNS", "DB_MIN_CONNS", "DB_CONNECT_TIMEOUT", "SHUTDOWN_TIMEOUT"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost/test")
	t.Setenv("JWT_SECRET", "test-only-jwt-secret-at-least-32-bytes")
	t.Setenv("GOOGLE_SPREADSHEET_ID", "test-spreadsheet")
	t.Setenv("GOOGLE_SHEET_RANGE", "Bookings!A9:AM")
}

func TestDefaults(t *testing.T) {
	defaults(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AppEnv != "development" || c.HTTPAddr != ":8080" || c.DBMaxConns != 10 || c.DBMinConns != 0 || c.DBConnectTimeout != 5*time.Second || c.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestRejectInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"DATABASE_URL", ""}, {"DATABASE_URL", "  "},
		{"JWT_SECRET", ""}, {"JWT_SECRET", "short"}, {"JWT_SECRET", "                                "},
		{"GOOGLE_SPREADSHEET_ID", ""}, {"GOOGLE_SPREADSHEET_ID", "  "},
		{"GOOGLE_SHEET_RANGE", ""}, {"GOOGLE_SHEET_RANGE", "  "},
		{"APP_ENV", "staging"}, {"APP_ENV", ""},
		{"HTTP_ADDR", "localhost"}, {"HTTP_ADDR", ":0"}, {"HTTP_ADDR", ":65536"}, {"HTTP_ADDR", "bad host:8080"},
		{"LOG_LEVEL", "verbose"},
		{"DB_MAX_CONNS", "0"}, {"DB_MAX_CONNS", "-1"}, {"DB_MAX_CONNS", "abc"}, {"DB_MAX_CONNS", "2147483648"},
		{"DB_MIN_CONNS", "-1"}, {"DB_MIN_CONNS", "11"}, {"DB_MIN_CONNS", "abc"},
		{"DB_CONNECT_TIMEOUT", "0s"}, {"DB_CONNECT_TIMEOUT", "-1s"}, {"DB_CONNECT_TIMEOUT", "invalid"},
		{"SHUTDOWN_TIMEOUT", "0s"}, {"SHUTDOWN_TIMEOUT", "invalid"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			defaults(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, key := range []string{"DATABASE_URL", "GOOGLE_SPREADSHEET_ID", "GOOGLE_SHEET_RANGE"} {
		t.Run("missing "+key, func(t *testing.T) {
			defaults(t)
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("missing %s accepted", key)
			}
		})
	}
}

func TestOverrides(t *testing.T) {
	defaults(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DB_MAX_CONNS", "20")
	t.Setenv("DB_MIN_CONNS", "2")
	t.Setenv("DB_CONNECT_TIMEOUT", "3s")
	t.Setenv("SHUTDOWN_TIMEOUT", "15s")
	t.Setenv("GOOGLE_SPREADSHEET_ID", "another-spreadsheet")
	t.Setenv("GOOGLE_SHEET_RANGE", "Other!A1:AM")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AppEnv != "production" || c.HTTPAddr != "127.0.0.1:9000" || c.LogLevel.String() != "DEBUG" || c.SpreadsheetID != "another-spreadsheet" || c.SheetRange != "Other!A1:AM" || c.DBMaxConns != 20 || c.DBMinConns != 2 || c.DBConnectTimeout != 3*time.Second || c.ShutdownTimeout != 15*time.Second {
		t.Fatalf("overrides not applied: %+v", c)
	}
}
