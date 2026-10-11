package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRequestLoggingAndRecovery(t *testing.T) {
	var output bytes.Buffer
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(RequestLogger(slog.New(slog.NewJSONHandler(&output, nil))))
	router.Use(chimiddleware.Recoverer)
	router.Get("/panic", func(http.ResponseWriter, *http.Request) { panic("test panic") })
	r := httptest.NewRequest(http.MethodGet, "/panic?token=query-secret", strings.NewReader("body-secret"))
	r.Header.Set("Cookie", "session=cookie-secret")
	r.Header.Set("Authorization", "Bearer auth-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["status"] != float64(500) || entry["method"] != "GET" || entry["path"] != "/panic" || entry["request_id"] == "" || entry["latency"] == nil {
		t.Fatalf("incomplete request log: %s", output.String())
	}
	if strings.Contains(output.String(), "secret") {
		t.Fatal("request secrets leaked into logs")
	}
}
