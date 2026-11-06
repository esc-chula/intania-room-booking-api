package sheetbooking

import "context"

// Service is the inbound port used by the sheet-booking HTTP adapter.
type Service interface {
	ValidBuilding(string) bool
	List(context.Context, Query) (Page, error)
	Calendar(context.Context, int, int, string) (CalendarResult, error)
}

type Reader interface {
	List(context.Context) ([]Booking, error)
}
