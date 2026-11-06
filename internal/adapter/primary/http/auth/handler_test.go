package auth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	domainauth "github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

type stubService struct {
	user        user.User
	token       string
	result      domainauth.Result
	signInCalls int
}

func (s *stubService) SignInGoogle(context.Context, domainauth.GoogleIdentity) (domainauth.Result, error) {
	s.signInCalls++
	return s.result, nil
}

func (s *stubService) Authenticate(_ context.Context, token string) (user.User, error) {
	s.token = token
	return s.user, nil
}

func TestHandlerAcceptsServicePort(t *testing.T) {
	service := &stubService{user: user.User{ID: "user-id"}}
	handler := New(service, slog.New(slog.NewJSONHandler(io.Discard, nil)), false, http.SameSiteLaxMode)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: "signed-token"})
	response := httptest.NewRecorder()
	handler.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := CurrentUser(r.Context())
		if !ok || u.ID != "user-id" {
			t.Errorf("authenticated user = %+v, present = %v", u, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || service.token != "signed-token" {
		t.Fatalf("status=%d token=%q", response.Code, service.token)
	}
}
