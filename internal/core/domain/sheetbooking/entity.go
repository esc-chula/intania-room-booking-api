package sheetbooking

// Booking is a row from the existing OS spreadsheet, not a native booking request.
type Booking struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Room       string `json:"room"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	Status     string `json:"status"`
	Conflicted bool   `json:"conflicted"`
}

type Query struct {
	Year, Month, Page, PageSize int
	Building, Status            string
}

type Page struct {
	Items    []Booking `json:"items"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Total    int       `json:"total"`
}

type Counts struct {
	Approved      int `json:"approved"`
	Cancelled     int `json:"cancelled"`
	Submitted     int `json:"submitted"`
	Timeline      int `json:"timeline"`
	NeedsRevision int `json:"needs_revision"`
	Conflicted    int `json:"conflicted"`
}

type Day struct {
	Date   string `json:"date"`
	Counts Counts `json:"counts"`
}

type CalendarResult struct {
	Year  int   `json:"year"`
	Month int   `json:"month"`
	Days  []Day `json:"days"`
}
