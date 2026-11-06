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
	s := New(rows, map[string]bool{"ENG 100": true, "ENG 3": true})
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
