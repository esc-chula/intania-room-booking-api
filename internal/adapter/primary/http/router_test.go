package httpadapter

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/MadManJJ/room-reservation-api/docs"
	httpauth "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/auth"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/health"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/token"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

type pinger struct{}

func (pinger) Ping(context.Context) error { return nil }

type authRepository struct{ current user.User }

func (r *authRepository) ResolveGoogleUser(context.Context, string, string, string, string) (user.User, error) {
	return r.current, nil
}
func (r *authRepository) CreateAdmin(context.Context, string) (user.User, error) {
	return user.User{}, nil
}
func (r *authRepository) FindAdmin(context.Context) (user.User, error) {
	return user.User{}, auth.ErrNotFound
}
func (r *authRepository) FindUserByID(context.Context, string) (user.User, error) {
	return r.current, nil
}

func TestRoutesAndSwagger(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			router := NewRouter(RouterConfig{Health: health.New(pinger{}, logger), Logger: logger, AppEnv: env})
			for _, path := range []string{"/healthz", "/readyz", "/swagger/doc.json", "/swagger/index.html", "/api/v1/auth/register", "/api/v1/auth/login", "/auth/google/login"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				want := http.StatusOK
				if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/") || env == "production" && strings.HasPrefix(path, "/swagger/") {
					want = http.StatusNotFound
				}
				if w.Code != want {
					t.Errorf("GET %s = %d, want %d", path, w.Code, want)
				}
				if path == "/swagger/doc.json" && want == http.StatusOK {
					var spec struct {
						BasePath string                    `json:"basePath"`
						Paths    map[string]map[string]any `json:"paths"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
						t.Fatal(err)
					}
					if spec.BasePath != "/" {
						t.Fatalf("base path: %q", spec.BasePath)
					}
					for _, documented := range []string{"/healthz", "/readyz", "/api/v1/auth/me", "/api/v1/admin/sheet-bookings", "/auth/google/url", "/auth/google/login", "/auth/google/callback"} {
						if spec.Paths[documented]["get"] == nil {
							t.Fatalf("missing GET %s documentation", documented)
						}
					}
					if spec.Paths["/api/v1/auth/logout"]["post"] == nil {
						t.Fatal("missing logout documentation")
					}
					for _, removed := range []string{"/api/v1/auth/register", "/api/v1/auth/login"} {
						if spec.Paths[removed] != nil {
							t.Fatalf("stale password route %s", removed)
						}
					}
				}
			}
		})
	}
}

func TestMeUsesCurrentUser(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tokens, err := token.New("test-only-jwt-secret-at-least-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	u := user.User{ID: "11111111-1111-4111-8111-111111111111", FirstName: "Student", Email: "6500000021@student.chula.ac.th", Role: user.RoleUserA, Active: true}
	repo := &authRepository{current: u}
	authService := auth.New(repo, tokens, "student.chula.ac.th", "21")
	jwt, err := tokens.Issue(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(RouterConfig{Auth: httpauth.New(authService, logger, false, http.SameSiteLaxMode), Health: health.New(pinger{}, logger), Logger: logger, AppEnv: "development", FrontendURL: "http://localhost:3000"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: "room_auth", Value: jwt})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"first_name":"Student"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	repo.current.Active = false
	w = httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user: %d", w.Code)
	}
	headerOnly := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	headerOnly.Header.Set("Authorization", "Bearer "+jwt)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, headerOnly)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Bearer header accepted: %d", w.Code)
	}
	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, logout)
	if w.Code != http.StatusNoContent || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout: %d %+v", w.Code, w.Result().Cookies())
	}
}

func TestGoogleRoutesAndCORS(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	repo := &authRepository{current: user.User{ID: "11111111-1111-4111-8111-111111111111", Active: true}}
	authService := auth.New(repo, nil, "student.chula.ac.th", "21")
	google, err := httpauth.NewGoogle(context.Background(), authService, logger, "client", "secret", "https://api.example.com/auth/google/callback", "https://frontend.other.example", "student.chula.ac.th", http.SameSiteNoneMode)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"development", "production"} {
		router := NewRouter(RouterConfig{GoogleAuth: google, Health: health.New(pinger{}, logger), Logger: logger, AppEnv: env, FrontendURL: "https://frontend.other.example"})
		for _, path := range []string{"/auth/google/login", "/auth/google/callback", "/auth/google/url"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Header.Set("Origin", "https://frontend.other.example")
			router.ServeHTTP(w, r)
			if path == "/auth/google/url" && env == "production" {
				if w.Code != http.StatusNotFound {
					t.Fatalf("production URL helper: %d", w.Code)
				}
			} else if w.Code != http.StatusFound && w.Code != http.StatusOK {
				t.Fatalf("%s %s: %d", env, path, w.Code)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "https://frontend.other.example" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatalf("missing credentialed CORS headers: %v", w.Header())
			}
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/me", nil)
		r.Header.Set("Origin", "https://evil.example")
		router.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("wrong origin allowed: %d %v", w.Code, w.Header())
		}
	}
}
