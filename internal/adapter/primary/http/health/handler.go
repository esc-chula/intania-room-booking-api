package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Pinger is an infrastructure-only dependency, not a business-domain port.
type Pinger interface{ Ping(context.Context) error }

type Handler struct {
	pinger Pinger
	logger *slog.Logger
}

func New(pinger Pinger, logger *slog.Logger) *Handler {
	return &Handler{pinger: pinger, logger: logger}
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, "ok")
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.pinger.Ping(ctx); err != nil {
		// Error messages from drivers can contain connection credentials.
		h.logger.WarnContext(ctx, "PostgreSQL readiness ping failed", "error_type", fmt.Sprintf("%T", err), "context_error", ctx.Err())
		respond(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	respond(w, http.StatusOK, "ready")
}

func respond(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{status})
}
