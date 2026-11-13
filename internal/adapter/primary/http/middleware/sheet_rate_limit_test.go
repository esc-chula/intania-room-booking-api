package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testUserKey struct{}

func TestSheetRateLimitKeysByUser(t *testing.T) {
	handler := SheetRateLimit(func(ctx context.Context) (string, bool) {
		id, ok := ctx.Value(testUserKey{}).(string)
		return id, ok
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := func(id, ip string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = ip + ":1234"
		if id != "" {
			r = r.WithContext(context.WithValue(r.Context(), testUserKey{}, id))
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if got := request("", "203.0.113.10"); got != http.StatusUnauthorized {
		t.Fatalf("missing user: got %d, want 401", got)
	}
	for i := 0; i < sheetBurst; i++ {
		if got := request("user-a", "203.0.113.10"); got != http.StatusOK {
			t.Fatalf("request %d: got %d", i, got)
		}
	}
	if got := request("user-a", "203.0.113.11"); got != http.StatusTooManyRequests {
		t.Fatalf("same user from another IP: got %d, want 429", got)
	}
	if got := request("user-b", "203.0.113.10"); got != http.StatusOK {
		t.Fatalf("different user from same IP: got %d, want 200", got)
	}
}
