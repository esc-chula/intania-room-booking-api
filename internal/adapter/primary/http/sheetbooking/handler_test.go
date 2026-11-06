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

type stubService struct {
	query domain.Query
}

func (*stubService) ValidBuilding(string) bool { return true }
func (s *stubService) List(_ context.Context, query domain.Query) (domain.Page, error) {
	s.query = query
	return domain.Page{Items: []domain.Booking{}, Page: query.Page, PageSize: query.PageSize}, nil
}
func (*stubService) Calendar(context.Context, int, int, string) (domain.CalendarResult, error) {
	return domain.CalendarResult{Days: []domain.Day{}}, nil
}
func (*stubService) Weekly(context.Context, string, string, string) (domain.WeeklyResult, error) {
	return domain.WeeklyResult{Rooms: []domain.WeeklyRoom{}}, nil
}

func TestHandlerAcceptsServicePort(t *testing.T) {
	service := &stubService{}
	handler := New(service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.List(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/sheet-bookings?building=ENG+100", nil))
	if response.Code != http.StatusOK || service.query.Building != "ENG 100" {
		t.Fatalf("status=%d query=%+v", response.Code, service.query)
	}
}

func TestHandlers(t *testing.T) {
	h := New(domain.New(rows{{ID: "1", Room: "ENG 100/A", StartDate: "16/7/2026", EndDate: "16/7/2026", StartTime: "09:00", EndTime: "10:00", Status: "approved"}}, map[string][]string{"ENG 100": {"A"}}), slog.New(slog.NewJSONHandler(io.Discard, nil)))
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

func TestWeeklyHandler(t *testing.T) {
	h := New(domain.New(rows{{ID: "1", Title: "Event", Room: "ENG 100/A", StartDate: "2026-11-02", EndDate: "2026-11-02", StartTime: "09:00", EndTime: "10:00", Status: "approved"}}, map[string][]string{"ENG 100": {"A", "B"}}), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"?start_date=2026-11-02&end_date=2026-11-02", 200},
		{"?start_date=2026-11-02", 200},
		{"", 200},
		{"?end_date=2026-11-02", 400},
		{"?start_date=2026-11-02&end_date=2026-11-09", 400},
		{"?start_date=2026-11-03&end_date=2026-11-02", 400},
		{"?start_date=wrong", 400},
		{"?building=unknown", 400},
	} {
		w := httptest.NewRecorder()
		h.Weekly(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/sheet-bookings/weekly"+tc.query, nil))
		if w.Code != tc.want {
			t.Errorf("%s: status %d", tc.query, w.Code)
		}
		if tc.query == "?start_date=2026-11-02&end_date=2026-11-02" {
			var result domain.WeeklyResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.StartDate != "2026-11-02" || len(result.Rooms) != 2 || len(result.Rooms[0].Bookings) != 1 || len(result.Rooms[1].Bookings) != 0 {
				t.Fatalf("response: %+v %v", result, err)
			}
		}
	}
}
