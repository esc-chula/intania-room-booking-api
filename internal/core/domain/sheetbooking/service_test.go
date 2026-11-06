package sheetbooking

import (
	"context"
	"testing"
)

type fakeReader []Booking

func (f fakeReader) List(context.Context) ([]Booking, error) { return []Booking(f), nil }
func TestCalendarAndList(t *testing.T) {
	rows := fakeReader{
		{ID: "1", Room: "ENG 100/A", StartDate: "31/7/2026", EndDate: "2/8/2026", StartTime: "09:00", EndTime: "11:00", Status: "approved"},
		{ID: "2", Room: "ENG 100/A", StartDate: "2026-08-01", EndDate: "2026-08-01", StartTime: "10:00", EndTime: "12:00", Status: "submitted"},
		{ID: "3", Room: "ENG 100/A", StartDate: "2026-08-01", EndDate: "2026-08-01", StartTime: "12:00", EndTime: "13:00", Status: "cancelled"},
		{ID: "4", Room: "ENG 3/A", StartDate: "2026-08-01", EndDate: "2026-08-01", StartTime: "10:00", EndTime: "12:00", Status: "timeline"},
		{ID: "5", Room: "ENG 100/A", StartDate: "bad", EndDate: "bad", Status: "needs_revision"},
	}
	s := New(rows, map[string][]string{"ENG 100": {"A"}, "ENG 3": {"A"}})
	cal, e := s.Calendar(context.Background(), 2026, 8, "ENG 100")
	if e != nil {
		t.Fatal(e)
	}
	if len(cal.Days) != 2 || cal.Days[0].Date != "2026-08-01" || cal.Days[0].Counts.Approved != 1 || cal.Days[0].Counts.Submitted != 1 || cal.Days[0].Counts.Cancelled != 1 || cal.Days[0].Counts.Conflicted != 2 || cal.Days[1].Counts.Conflicted != 0 {
		t.Fatalf("calendar: %+v", cal)
	}
	page, e := s.List(context.Background(), Query{Year: 2026, Month: 8, Building: "ENG 100", Status: "conflicted", Page: 1, PageSize: 1})
	if e != nil {
		t.Fatal(e)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != "1" || !page.Items[0].Conflicted {
		t.Fatalf("page: %+v", page)
	}
	all, e := s.List(context.Background(), Query{Page: 1, PageSize: 20})
	if e != nil || all.Total != 5 || all.Items[4].ID != "5" {
		t.Fatalf("all: %+v %v", all, e)
	}
}

func TestWeekly(t *testing.T) {
	rows := fakeReader{
		{ID: "7", Title: "Event", Room: "ENG 3/301", StartDate: "2026-11-01", EndDate: "2026-11-03", StartTime: "09:00", EndTime: "11:00", Status: "approved"},
		{ID: "8", Title: "Other", Room: "ENG 3/301", StartDate: "2026-11-02", EndDate: "2026-11-02", StartTime: "10:00", EndTime: "12:00", Status: "submitted"},
		{ID: "", Room: "True Lab", StartDate: "2026-11-02", EndDate: "2026-11-02", StartTime: "08:00", EndTime: "09:00", Status: "unknown"},
		{ID: "9", Room: "ENG 100/301", StartDate: "bad", EndDate: "bad", StartTime: "09:00", EndTime: "10:00"},
		{ID: "10", Room: "missing", StartDate: "2026-11-02", EndDate: "2026-11-02", StartTime: "09:00", EndTime: "10:00"},
	}
	s := New(rows, map[string][]string{"ENG 3": {"301", "204"}, "ENG 100": {"301"}, "ห้องพิเศษ + ห้องภาค": {"True Lab"}})
	result, err := s.Weekly(context.Background(), "2026-11-02", "2026-11-02", "")
	if err != nil || len(result.Rooms) != 4 {
		t.Fatalf("weekly: %+v %v", result, err)
	}
	var eng3, eng100, special WeeklyRoom
	for _, room := range result.Rooms {
		switch {
		case room.Building == "ENG 3" && room.Name == "301":
			eng3 = room
		case room.Building == "ENG 100":
			eng100 = room
		case room.Name == "True Lab":
			special = room
		}
	}
	if len(eng3.Bookings) != 2 || !eng3.Bookings[0].HasConflict || !eng3.Bookings[1].HasConflict || eng3.Bookings[0].StartAt != "2026-11-02T09:00:00+07:00" || len(eng100.Bookings) != 0 || eng3.ID == eng100.ID {
		t.Fatalf("room grouping: %+v %+v", eng3, eng100)
	}
	if len(special.Bookings) != 1 || special.Bookings[0].ID != "-1" || special.Bookings[0].HasConflict {
		t.Fatalf("special room: %+v", special)
	}
	longer, err := s.Weekly(context.Background(), "2026-11-01", "2026-11-03", "ENG 3")
	if err != nil || len(longer.Rooms) != 2 || len(longer.Rooms[1].Bookings) != 4 {
		t.Fatalf("multi-day or building filter: %+v %v", longer, err)
	}
}
