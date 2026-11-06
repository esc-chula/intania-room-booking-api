package sheetbooking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type service struct {
	reader Reader
	rooms  map[string][]string
}

var _ Service = (*service)(nil)

func New(reader Reader, rooms map[string][]string) Service {
	return &service{reader: reader, rooms: rooms}
}
func (s *service) ValidBuilding(name string) bool {
	_, ok := s.rooms[name]
	return ok && (name == "ENG 3" || name == "ENG 100" || name == "ENG 4")
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

func (s *service) Weekly(ctx context.Context, startDate, endDate, building string) (WeeklyResult, error) {
	rows, err := s.reader.List(ctx)
	if err != nil {
		return WeeklyResult{}, err
	}
	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)
	parsedRows := parse(rows)
	markConflicts(parsedRows, 0, 0)
	result := WeeklyResult{StartDate: startDate, EndDate: endDate, Rooms: make([]WeeklyRoom, 0)}
	buildings := make([]string, 0, len(s.rooms))
	for name := range s.rooms {
		if building == "" || building == name {
			buildings = append(buildings, name)
		}
	}
	sort.Strings(buildings)
	zone := time.FixedZone("ICT", 7*60*60)
	for _, name := range buildings {
		labels := append([]string(nil), s.rooms[name]...)
		sort.Strings(labels)
		for _, label := range labels {
			key := label
			if strings.HasPrefix(name, "ENG ") {
				key = name + "/" + label
			}
			hash := sha256.Sum256([]byte(name + "\x00" + label))
			room := WeeklyRoom{ID: "room-" + hex.EncodeToString(hash[:16]), Building: name, Name: label, Bookings: make([]WeeklyBooking, 0)}
			for _, p := range parsedRows {
				if !p.validDate || !p.validTime || strings.TrimSpace(p.booking.Room) != key {
					continue
				}
				for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
					if day.Before(p.start) || day.After(p.end) {
						continue
					}
					at := func(minute int) string {
						return time.Date(day.Year(), day.Month(), day.Day(), minute/60, minute%60, 0, 0, zone).Format(time.RFC3339)
					}
					id := strings.TrimSpace(p.booking.ID)
					if id == "" {
						id = "-1"
					}
					room.Bookings = append(room.Bookings, WeeklyBooking{ID: id, Title: p.booking.Title, StartAt: at(p.startMinute), EndAt: at(p.endMinute), Status: p.booking.Status, HasConflict: p.booking.Conflicted})
				}
			}
			sort.Slice(room.Bookings, func(i, j int) bool {
				a, b := room.Bookings[i], room.Bookings[j]
				if a.StartAt != b.StartAt {
					return a.StartAt < b.StartAt
				}
				if a.ID != b.ID {
					return a.ID < b.ID
				}
				return a.Title < b.Title
			})
			result.Rooms = append(result.Rooms, room)
		}
	}
	return result, nil
}
