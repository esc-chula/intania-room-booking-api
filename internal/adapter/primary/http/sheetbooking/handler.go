package sheetbooking

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	domainbooking "github.com/MadManJJ/room-reservation-api/internal/core/domain/sheetbooking"
)

type Handler struct {
	service domainbooking.Service
	logger  *slog.Logger
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(service domainbooking.Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// List returns paginated booking rows from the OS spreadsheet.
// @Summary List public spreadsheet bookings
// @Description Reads a 60-second cached Sheet snapshot without authentication.
// @Tags sheet-bookings
// @Produce json
// @Param year query int false "Calendar year; requires month"
// @Param month query int false "Month 1-12; requires year"
// @Param building query string false "ENG building name"
// @Param status query string false "approved, cancelled, submitted, timeline, needs_revision, or conflicted"
// @Param page query int false "Page number, default 1"
// @Param page_size query int false "Page size, default 20, maximum 100"
// @Success 200 {object} domainbooking.Page
// @Failure 400 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Router /api/v1/admin/sheet-bookings [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := h.query(w, r, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := h.service.List(ctx, q)
	if err != nil {
		h.fetchError(w, r, err)
		return
	}
	respond(w, http.StatusOK, page)
}

// Calendar returns daily booking status counts for a month.
// @Summary Public monthly spreadsheet booking calendar
// @Description Conflicted counts active bookings overlapping the same room and daily time window. No authentication is required.
// @Tags sheet-bookings
// @Produce json
// @Param year query int true "Calendar year"
// @Param month query int true "Month 1-12"
// @Param building query string false "ENG building name"
// @Success 200 {object} domainbooking.CalendarResult
// @Failure 400 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Router /api/v1/admin/sheet-bookings/calendar [get]
func (h *Handler) Calendar(w http.ResponseWriter, r *http.Request) {
	q, ok := h.query(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := h.service.Calendar(ctx, q.Year, q.Month, q.Building)
	if err != nil {
		h.fetchError(w, r, err)
		return
	}
	respond(w, http.StatusOK, result)
}

// Weekly returns sheet bookings grouped under every catalog room in a date range.
// @Summary Public weekly spreadsheet room schedule
// @Description Uses the 60-second cached Sheet snapshot without authentication. Dates are inclusive and use Bangkok time.
// @Tags sheet-bookings
// @Produce json
// @Param start_date query string false "Start date YYYY-MM-DD; defaults to today in Bangkok"
// @Param end_date query string false "End date YYYY-MM-DD; defaults to six days after start_date"
// @Param building query string false "ENG 3, ENG 100, or ENG 4"
// @Success 200 {object} domainbooking.WeeklyResult
// @Failure 400 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Router /api/v1/admin/sheet-bookings/weekly [get]
func (h *Handler) Weekly(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	bad := func() { respond(w, http.StatusBadRequest, errorResponse{Error: "invalid query parameters"}) }
	building := v.Get("building")
	if building != "" && !h.service.ValidBuilding(building) || v.Has("end_date") && !v.Has("start_date") {
		bad()
		return
	}
	zone := time.FixedZone("ICT", 7*60*60)
	start := time.Now().In(zone)
	if v.Has("start_date") {
		var err error
		start, err = time.Parse("2006-01-02", v.Get("start_date"))
		if err != nil {
			bad()
			return
		}
	}
	end := start.AddDate(0, 0, 6)
	if v.Has("end_date") {
		var err error
		end, err = time.Parse("2006-01-02", v.Get("end_date"))
		if err != nil {
			bad()
			return
		}
	}
	if end.Before(start) || end.After(start.AddDate(0, 0, 6)) {
		bad()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := h.service.Weekly(ctx, start.Format("2006-01-02"), end.Format("2006-01-02"), building)
	if err != nil {
		h.fetchError(w, r, err)
		return
	}
	respond(w, http.StatusOK, result)
}
func (h *Handler) fetchError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "Google Sheets fetch failed", "error_type", fmt.Sprintf("%T", err))
	respond(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch sheet bookings"})
}
func (h *Handler) query(w http.ResponseWriter, r *http.Request, calendar bool) (domainbooking.Query, bool) {
	v := r.URL.Query()
	q := domainbooking.Query{Page: 1, PageSize: 20, Building: v.Get("building"), Status: v.Get("status")}
	bad := func() (domainbooking.Query, bool) {
		respond(w, http.StatusBadRequest, errorResponse{Error: "invalid query parameters"})
		return q, false
	}
	if y, m := v.Get("year"), v.Get("month"); (y == "") != (m == "") || calendar && y == "" {
		return bad()
	} else if y != "" {
		var err error
		q.Year, err = strconv.Atoi(y)
		if err != nil || q.Year < 1 || q.Year > 9999 {
			return bad()
		}
		q.Month, err = strconv.Atoi(m)
		if err != nil || q.Month < 1 || q.Month > 12 {
			return bad()
		}
	}
	if q.Building != "" && !h.service.ValidBuilding(q.Building) {
		return bad()
	}
	if calendar {
		if q.Status != "" || v.Has("page") || v.Has("page_size") {
			return bad()
		}
	} else {
		switch q.Status {
		case "", "approved", "cancelled", "submitted", "timeline", "needs_revision", "conflicted":
		default:
			return bad()
		}
		if v.Has("page") {
			var err error
			q.Page, err = strconv.Atoi(v.Get("page"))
			if err != nil || q.Page < 1 || q.Page > 1000000 {
				return bad()
			}
		}
		if v.Has("page_size") {
			var err error
			q.PageSize, err = strconv.Atoi(v.Get("page_size"))
			if err != nil || q.PageSize < 1 || q.PageSize > 100 {
				return bad()
			}
		}
	}
	return q, true
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
