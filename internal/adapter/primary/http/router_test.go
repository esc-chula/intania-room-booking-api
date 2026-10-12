package httpadapter

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "example.com/chula-room-booking/docs"
	"example.com/chula-room-booking/internal/adapter/primary/http/health"
)

type pinger struct{}

func (pinger) Ping(context.Context) error { return nil }

func TestRoutes(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			router := NewRouter(RouterConfig{
				Health: health.New(pinger{}, logger),
				Logger: logger,
				AppEnv: env,
			})
			for _, path := range []string{"/healthz", "/readyz", "/swagger/doc.json", "/swagger/index.html", "/unknown", "/api/v1/rooms"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				want := http.StatusOK
				if path == "/unknown" || path == "/api/v1/rooms" || env == "production" && strings.HasPrefix(path, "/swagger/") {
					want = http.StatusNotFound
				}
				if w.Code != want {
					t.Errorf("GET %s = %d, want %d", path, w.Code, want)
				}
				if path == "/swagger/doc.json" && want == http.StatusOK {
					var spec map[string]any
					if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
						t.Fatal(err)
					}
					if spec["swagger"] != "2.0" || spec["basePath"] != "/api/v1" {
						t.Fatalf("unexpected Swagger metadata: %v", spec)
					}
					if paths, ok := spec["paths"].(map[string]any); !ok || len(paths) != 0 {
						t.Fatal("Phase 0 must have no documented business operations")
					}
				}
				if path == "/swagger/index.html" && want == http.StatusOK && !strings.Contains(w.Body.String(), "swagger-ui") {
					t.Fatal("missing Swagger UI")
				}
			}
		})
	}
}
