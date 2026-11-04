package sheetbooking

import "context"

// Booking is a row from the existing OS spreadsheet, not a native booking request.
type Booking struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Room      string `json:"room"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type Reader interface {
	List(context.Context) ([]Booking, error)
}

type Service struct{ reader Reader }

func New(reader Reader) *Service { return &Service{reader: reader} }

func (s *Service) List(ctx context.Context) ([]Booking, error) { return s.reader.List(ctx) }
