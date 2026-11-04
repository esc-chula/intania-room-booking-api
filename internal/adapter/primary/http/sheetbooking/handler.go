package sheetbooking

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"example.com/chula-room-booking/internal/adapter/primary/http/auth"
	domainbooking "example.com/chula-room-booking/internal/core/domain/sheetbooking"
)

type Handler struct {
	service *domainbooking.Service
	logger  *slog.Logger
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(service *domainbooking.Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// List returns current booking rows from the OS spreadsheet.
// @Summary List OS spreadsheet bookings
// @Description Requires a currently active OS user. Reads the configured Google Sheet on every request and returns seven string fields per row.
// @Tags sheet-bookings
// @Produce json
// @Security BearerAuth
// @Success 200 {array} domainbooking.Booking
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Router /api/v1/admin/sheet-bookings [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.CurrentUser(r.Context())
	if !ok {
		respond(w, http.StatusUnauthorized, errorResponse{Error: "authentication required"})
		return
	}
	if u.Role != "OS" {
		respond(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	bookings, err := h.service.List(ctx)
	if err != nil {
		// Google API errors can include request details; log only the type.
		h.logger.ErrorContext(r.Context(), "Google Sheets fetch failed", "error_type", fmt.Sprintf("%T", err))
		respond(w, http.StatusBadGateway, errorResponse{Error: "failed to fetch sheet bookings"})
		return
	}
	respond(w, http.StatusOK, bookings)
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
