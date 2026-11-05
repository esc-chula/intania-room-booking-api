package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	domainauth "github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

type Handler struct {
	service  *domainauth.Service
	logger   *slog.Logger
	secure   bool
	sameSite http.SameSite
}

type errorResponse struct {
	Error string `json:"error"`
}
type userContextKey struct{}

func New(service *domainauth.Service, logger *slog.Logger, secure bool, sameSite http.SameSite) *Handler {
	return &Handler{service: service, logger: logger, secure: secure, sameSite: sameSite}
}

// Me returns the authenticated user's current public profile.
// @Summary Get current user
// @Description Validates the JWT and loads the current active user from PostgreSQL; role changes apply immediately.
// @Tags auth
// @Produce json
// @Success 200 {object} user.User
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/auth/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		h.failure(w, r, domainauth.ErrUnauthorized)
		return
	}
	respond(w, http.StatusOK, u)
}

func CurrentUser(ctx context.Context) (user.User, bool) {
	u, ok := ctx.Value(userContextKey{}).(user.User)
	return u, ok
}

func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil || cookie.Value == "" {
			h.failure(w, r, domainauth.ErrUnauthorized)
			return
		}
		u, err := h.service.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			h.failure(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
	})
}

// Logout clears the browser authentication cookie.
// @Summary Log out
// @Description Clears the application auth cookie; an already issued JWT expires at its original time.
// @Tags auth
// @Success 204
// @Router /api/v1/auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	setAuthCookie(w, "", h.secure, h.sameSite, -1)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RequireRoles(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := CurrentUser(r.Context())
			if !ok {
				h.failure(w, r, domainauth.ErrUnauthorized)
				return
			}
			if u.Role == user.RoleAdmin {
				next.ServeHTTP(w, r)
				return
			}
			for _, role := range roles {
				if u.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			respond(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
		})
	}
}

func (h *Handler) failure(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, domainauth.ErrUnauthorized):
		status, message = http.StatusUnauthorized, "invalid authentication cookie"
	default:
		h.logger.ErrorContext(r.Context(), "authentication operation failed", "error_type", fmt.Sprintf("%T", err))
	}
	respond(w, status, errorResponse{Error: message})
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
