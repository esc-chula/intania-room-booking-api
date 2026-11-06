package sheetbooking

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/MadManJJ/room-reservation-api/internal/core/domain/sheetbooking"
)

type rows []domain.Booking

func (r rows) List(context.Context) ([]domain.Booking, error) { return []domain.Booking(r), nil }
func TestHandlers(t *testing.T) {
	h := New(domain.New(rows{{ID: "1", Room: "ENG 100/A", StartDate: "16/7/2026", EndDate: "16/7/2026", StartTime: "09:00", EndTime: "10:00", Status: "approved"}}, map[string]bool{"ENG 100": true}), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	for _, tc := range []struct {
		path     string
		calendar bool
		want     int
	}{
		{"/api/v1/admin/sheet-bookings?page=bad", false, 400},
		{"/api/v1/admin/sheet-bookings?month=7", false, 400},
		{"/api/v1/admin/sheet-bookings?building=Other", false, 400},
		{"/api/v1/admin/sheet-bookings/calendar?year=2026&month=13", true, 400},
		{"/api/v1/admin/sheet-bookings/calendar?year=2026&month=7&building=ENG%20100", true, 200},
		{"/api/v1/admin/sheet-bookings?page=1&page_size=1", false, 200},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.calendar {
			h.Calendar(w, r)
		} else {
			h.List(w, r)
		}
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d", tc.path, w.Code)
		}
		if w.Code == 200 {
			var body map[string]any
			if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
				t.Fatal(e)
			}
			if tc.calendar && body["days"] == nil || !tc.calendar && body["items"] == nil {
				t.Errorf("body: %v", body)
			}
		}
	}
}
