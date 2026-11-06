package googlesheets

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/sheetbooking"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Reader struct {
	service       *sheets.Service
	spreadsheetID string
	readRange     string
	mu            sync.Mutex
	cached        []sheetbooking.Booking
	expires       time.Time
}

var _ sheetbooking.Reader = (*Reader)(nil)

func New(ctx context.Context, spreadsheetID, readRange string) (*Reader, error) {
	service, err := sheets.NewService(ctx, option.WithScopes(sheets.SpreadsheetsReadonlyScope))
	if err != nil {
		return nil, err
	}
	return &Reader{service: service, spreadsheetID: spreadsheetID, readRange: readRange}, nil
}

func (r *Reader) List(ctx context.Context) ([]sheetbooking.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != nil && time.Now().Before(r.expires) {
		return append([]sheetbooking.Booking{}, r.cached...), nil
	}
	response, err := r.service.Spreadsheets.Values.Get(r.spreadsheetID, r.readRange).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	bookings := make([]sheetbooking.Booking, 0, len(response.Values))
	for _, row := range response.Values {
		bookings = append(bookings, sheetbooking.Booking{
			ID:        cell(row, 0),
			Title:     cell(row, 2),
			Name:      cell(row, 5),
			Room:      cell(row, 13),
			StartDate: cell(row, 15),
			EndDate:   cell(row, 16),
			StartTime: cell(row, 17),
			EndTime:   cell(row, 18),
			Status:    status(cell(row, 30)),
		})
	}
	r.cached = bookings
	r.expires = time.Now().Add(time.Minute)
	return append([]sheetbooking.Booking{}, bookings...), nil
}

func cell(row []interface{}, index int) string {
	if index >= len(row) {
		return ""
	}
	return fmt.Sprint(row[index])
}

func status(value string) string {
	switch strings.TrimSpace(value) {
	case "อนุมัติแล้ว":
		return "approved"
	case "ยกเลิกเอกสาร":
		return "cancelled"
	case "ส่งเอกสารแล้ว":
		return "submitted"
	case "ลง TIMELINE แล้ว":
		return "timeline"
	case "แก้ไขเอกสาร":
		return "needs_revision"
	default:
		return "unknown"
	}
}
