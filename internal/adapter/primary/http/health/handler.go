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

type probeResponse struct {
	Status string `json:"status"`
}

func New(pinger Pinger, logger *slog.Logger) *Handler {
	return &Handler{pinger: pinger, logger: logger}
}

// Healthz reports whether the process is alive.
// @Summary Check liveness
// @Description Returns ok while the process is running; does not query PostgreSQL.
// @Tags health
// @Produce json
// @Success 200 {object} probeResponse "status: ok"
// @Router /healthz [get]
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, "ok")
}

// Readyz reports readiness, checking PostgreSQL only when it is configured.
// @Summary Check readiness
// @Description Returns ready when PostgreSQL is not configured; otherwise pings PostgreSQL with a two-second timeout.
// @Tags health
// @Produce json
// @Success 200 {object} probeResponse "status: ready"
// @Failure 503 {object} probeResponse "status: unavailable"
// @Router /readyz [get]
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.pinger == nil {
		respond(w, http.StatusOK, "ready")
		return
	}
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
	_ = json.NewEncoder(w).Encode(probeResponse{Status: status})
}
