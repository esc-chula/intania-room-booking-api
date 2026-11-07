package health

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakePinger struct {
	calls int
	ping  func(context.Context) error
}

func (p *fakePinger) Ping(ctx context.Context) error {
	p.calls++
	return p.ping(ctx)
}

func TestProbes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ready  bool
		ping   func(context.Context) error
		code   int
		status string
	}{
		{"liveness without database", false, func(context.Context) error { return errors.New("offline") }, 200, "ok"},
		{"readiness success", true, func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 2*time.Second {
				t.Error("readiness must bound the ping to two seconds")
			}
			return nil
		}, 200, "ready"},
		{"readiness failure", true, func(context.Context) error { return errors.New("secret DSN") }, 503, "unavailable"},
		{"readiness timeout", true, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 503, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePinger{ping: tc.ping}
			h := New(p, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			path := "/healthz"
			if tc.ready {
				path = "/readyz"
			}
			r := httptest.NewRequest(http.MethodGet, path, nil)
			if tc.name == "readiness timeout" {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Millisecond)
				defer cancel()
				r = r.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			if tc.ready {
				h.Readyz(w, r)
			} else {
				h.Healthz(w, r)
			}
			if w.Code != tc.code {
				t.Fatalf("status = %d, want %d", w.Code, tc.code)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body["status"] != tc.status {
				t.Fatalf("unexpected body: %s", w.Body.String())
			}
			if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing probe headers")
			}
			if tc.ready && p.calls != 1 || !tc.ready && p.calls != 0 {
				t.Fatalf("unexpected ping count: %d", p.calls)
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal error leaked")
			}
		})
	}
}

func TestReadinessWithoutDatabase(t *testing.T) {
	h := New(nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	h.Readyz(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK || w.Body.String() != "{\"status\":\"ready\"}\n" {
		t.Fatalf("readiness without database: %d %s", w.Code, w.Body.String())
	}
}
