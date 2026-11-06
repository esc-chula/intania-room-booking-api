package sheetbooking

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type service struct {
	reader    Reader
	buildings map[string]bool
}

var _ Service = (*service)(nil)

func New(reader Reader, buildings map[string]bool) Service {
	return &service{reader: reader, buildings: buildings}
}
func (s *service) ValidBuilding(name string) bool {
	return s.buildings[name] && strings.HasPrefix(name, "ENG ")
}

type parsed struct {
	booking                Booking
	start, end             time.Time
	startMinute, endMinute int
	validDate, validTime   bool
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{"2/1/2006", "2006-01-02"} {
		if d, e := time.Parse(layout, strings.TrimSpace(s)); e == nil {
			return d, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date: %q", s)
}
func parseMinute(s string) (int, bool) {
	t, e := time.Parse("15:04", strings.TrimSpace(s))
	if e != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}
func parse(rows []Booking) []parsed {
	out := make([]parsed, 0, len(rows))
	for _, b := range rows {
		p := parsed{booking: b}
		p.start, _ = parseDate(b.StartDate)
		p.end, _ = parseDate(b.EndDate)
		p.validDate = !p.start.IsZero() && !p.end.IsZero() && !p.end.Before(p.start)
		var a, c bool
		p.startMinute, a = parseMinute(b.StartTime)
		p.endMinute, c = parseMinute(b.EndTime)
		p.validTime = a && c && p.endMinute > p.startMinute
		out = append(out, p)
	}
	return out
}
func active(s string) bool { return s != "cancelled" && s != "unknown" }
func sameRoom(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b) && strings.TrimSpace(a) != ""
}
func overlap(a, b parsed) bool {
	return a.validDate && b.validDate && a.validTime && b.validTime && active(a.booking.Status) && active(b.booking.Status) && sameRoom(a.booking.Room, b.booking.Room) && !a.end.Before(b.start) && !b.end.Before(a.start) && a.startMinute < b.endMinute && b.startMinute < a.endMinute
}
func inMonth(p parsed, year, month int) bool {
	if !p.validDate {
		return false
	}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	next := first.AddDate(0, 1, 0)
	return p.start.Before(next) && !p.end.Before(first)
}
func buildingMatch(room, building string) bool {
	return building == "" || strings.HasPrefix(room, building+"/")
}
func markConflicts(rows []parsed, year, month int) {
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if overlap(rows[i], rows[j]) && (year == 0 || inMonth(rows[i], year, month) && inMonth(rows[j], year, month)) {
				rows[i].booking.Conflicted = true
				rows[j].booking.Conflicted = true
			}
		}
	}
}
func (s *service) List(ctx context.Context, q Query) (Page, error) {
	rows, e := s.reader.List(ctx)
	if e != nil {
		return Page{}, e
	}
	parsedRows := parse(rows)
	markConflicts(parsedRows, q.Year, q.Month)
	page := Page{Items: make([]Booking, 0), Page: q.Page, PageSize: q.PageSize}
	for _, p := range parsedRows {
		if q.Year != 0 && !inMonth(p, q.Year, q.Month) || !buildingMatch(p.booking.Room, q.Building) || q.Status != "" && q.Status != "conflicted" && p.booking.Status != q.Status || q.Status == "conflicted" && !p.booking.Conflicted {
			continue
		}
		page.Total++
		if page.Total > (q.Page-1)*q.PageSize && len(page.Items) < q.PageSize {
			page.Items = append(page.Items, p.booking)
		}
	}
	return page, nil
}
func (s *service) Calendar(ctx context.Context, year, month int, building string) (CalendarResult, error) {
	rows, e := s.reader.List(ctx)
	if e != nil {
		return CalendarResult{}, e
	}
	all := parse(rows)
	result := CalendarResult{Year: year, Month: month, Days: make([]Day, 0)}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, 0)
	for d := first; d.Before(last); d = d.AddDate(0, 0, 1) {
		counts := Counts{}
		todays := make([]int, 0)
		for i, p := range all {
			if !p.validDate || d.Before(p.start) || d.After(p.end) || !buildingMatch(p.booking.Room, building) {
				continue
			}
			switch p.booking.Status {
			case "approved":
				counts.Approved++
			case "cancelled":
				counts.Cancelled++
			case "submitted":
				counts.Submitted++
			case "timeline":
				counts.Timeline++
			case "needs_revision":
				counts.NeedsRevision++
			}
			if active(p.booking.Status) && p.validTime {
				todays = append(todays, i)
			}
		}
		conflicted := make(map[int]bool)
		for i, a := range todays {
			for _, b := range todays[i+1:] {
				if overlap(all[a], all[b]) {
					conflicted[a] = true
					conflicted[b] = true
				}
			}
		}
		counts.Conflicted = len(conflicted)
		if counts != (Counts{}) {
			result.Days = append(result.Days, Day{Date: d.Format("2006-01-02"), Counts: counts})
		}
	}
	sort.Slice(result.Days, func(i, j int) bool { return result.Days[i].Date < result.Days[j].Date })
	return result, nil
}
