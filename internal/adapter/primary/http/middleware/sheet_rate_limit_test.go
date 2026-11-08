package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSheetRateLimitCloudRunUsesLastForwardedIP(t *testing.T) {
	t.Setenv("K_SERVICE", "room-booking-api")
	handler := SheetRateLimit()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := func(forwarded string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-For", forwarded)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < sheetBurst; i++ {
		if got := request("192.0.2.1, 203.0.113.10"); got != http.StatusOK {
			t.Fatalf("request %d: got %d", i, got)
		}
	}
	if got := request("198.51.100.2, 203.0.113.10"); got != http.StatusTooManyRequests {
		t.Fatalf("spoofed first IP: got %d, want 429", got)
	}
	if got := request("203.0.113.11"); got != http.StatusOK {
		t.Fatalf("different client IP: got %d, want 200", got)
	}
}
