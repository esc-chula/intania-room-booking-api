package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	domainauth "example.com/chula-room-booking/internal/core/domain/auth"
	"example.com/chula-room-booking/internal/core/domain/user"
)

type Handler struct {
	service *domainauth.Service
	logger  *slog.Logger
}

type registerRequest struct {
	Name     string `json:"name" example:"Test User"`
	Email    string `json:"email" example:"test@example.com"`
	Password string `json:"password" example:"test-password-123" minLength:"8" maxLength:"72"`
}

type loginRequest struct {
	Email    string `json:"email" example:"test@example.com"`
	Password string `json:"password" example:"test-password-123" minLength:"8" maxLength:"72"`
}

type errorResponse struct {
	Error string `json:"error"`
}
type userContextKey struct{}

func New(service *domainauth.Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register creates an active USER_A account and issues an access token.
// @Summary Register with email and password
// @Description Name must be nonempty; email is trimmed and lowercased; password must be 8-72 bytes. Unknown fields are rejected. Available in all environments.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body registerRequest true "Registration credentials"
// @Success 201 {object} domainauth.Result
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/auth/register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var input registerRequest
	if err := decode(w, r, &input); err != nil {
		h.failure(w, r, domainauth.ErrInvalidInput)
		return
	}
	result, err := h.service.Register(r.Context(), input.Name, input.Email, input.Password)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, http.StatusCreated, result)
}

// Login verifies an active user's password and issues an access token.
// @Summary Log in with email and password
// @Description Returns a one-hour bearer JWT. Invalid credentials, disabled accounts, and accounts without passwords receive the same error.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body loginRequest true "Login credentials"
// @Success 200 {object} domainauth.Result
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := decode(w, r, &input); err != nil {
		h.failure(w, r, domainauth.ErrInvalidInput)
		return
	}
	result, err := h.service.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, http.StatusOK, result)
}

// Me returns the authenticated user's current public profile.
// @Summary Get current user
// @Description Validates the JWT and loads the current active user from PostgreSQL; role changes apply immediately.
// @Tags auth
// @Produce json
// @Security BearerAuth
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
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 {
			h.failure(w, r, domainauth.ErrUnauthorized)
			return
		}
		parts := strings.Fields(headers[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.failure(w, r, domainauth.ErrUnauthorized)
			return
		}
		u, err := h.service.Authenticate(r.Context(), parts[1])
		if err != nil {
			h.failure(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
	})
}

// RequireRoles authorizes a current authenticated user for a set of roles.
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

func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return domainauth.ErrInvalidInput
	}
	return nil
}

func (h *Handler) failure(w http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, domainauth.ErrInvalidInput):
		status, message = http.StatusBadRequest, "invalid input"
	case errors.Is(err, domainauth.ErrEmailExists):
		status, message = http.StatusConflict, "email already registered"
	case errors.Is(err, domainauth.ErrUnauthorized):
		status, message = http.StatusUnauthorized, "invalid credentials or token"
		w.Header().Set("WWW-Authenticate", "Bearer")
	default:
		// Driver errors may contain credentials; log only the type.
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
