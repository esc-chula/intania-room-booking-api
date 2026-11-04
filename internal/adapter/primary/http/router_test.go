package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/MadManJJ/room-reservation-api/docs"
	httpauth "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/auth"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/health"
	httpsheetbooking "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/sheetbooking"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/token"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/sheetbooking"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

type pinger struct{}

func (pinger) Ping(context.Context) error { return nil }

func TestRoutes(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			router := NewRouter(RouterConfig{
				Health: health.New(pinger{}, logger),
				Logger: logger,
				AppEnv: env,
			})
			for _, path := range []string{"/healthz", "/readyz", "/swagger/doc.json", "/swagger/index.html", "/unknown", "/api/v1/rooms"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				want := http.StatusOK
				if path == "/unknown" || path == "/api/v1/rooms" || env == "production" && strings.HasPrefix(path, "/swagger/") {
					want = http.StatusNotFound
				}
				if w.Code != want {
					t.Errorf("GET %s = %d, want %d", path, w.Code, want)
				}
				if path == "/swagger/doc.json" && want == http.StatusOK {
					var spec map[string]any
					if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
						t.Fatal(err)
					}
					if spec["swagger"] != "2.0" || spec["basePath"] != "/" {
						t.Fatalf("unexpected Swagger metadata: %v", spec)
					}
					paths, ok := spec["paths"].(map[string]any)
					if !ok || len(paths) != 6 {
						t.Fatalf("want six documented paths, got %v", spec["paths"])
					}
					for _, probe := range []string{"/healthz", "/readyz"} {
						methods, ok := paths[probe].(map[string]any)
						if !ok || len(methods) != 1 {
							t.Fatalf("want one operation for %s, got %v", probe, paths[probe])
						}
						get, ok := methods["get"].(map[string]any)
						if !ok {
							t.Fatalf("missing GET %s operation", probe)
						}
						responses, ok := get["responses"].(map[string]any)
						if !ok || responses["200"] == nil {
							t.Fatalf("missing GET %s 200 response", probe)
						}
						if probe == "/readyz" && responses["503"] == nil {
							t.Fatal("missing GET /readyz 503 response")
						}
					}
					for path, method := range map[string]string{"/api/v1/auth/register": "post", "/api/v1/auth/login": "post", "/api/v1/auth/me": "get"} {
						operations, ok := paths[path].(map[string]any)
						if !ok || operations[method] == nil {
							t.Fatalf("missing %s %s documentation", method, path)
						}
					}
					operations, ok := paths["/api/v1/admin/sheet-bookings"].(map[string]any)
					if !ok || operations["get"] == nil {
						t.Fatal("missing sheet bookings documentation")
					}
					sheetGet := operations["get"].(map[string]any)
					responses, ok := sheetGet["responses"].(map[string]any)
					if !ok {
						t.Fatal("missing sheet bookings responses")
					}
					for _, status := range []string{"200", "401", "403", "500", "502"} {
						if responses[status] == nil {
							t.Fatalf("missing sheet bookings %s response", status)
						}
					}
					if sheetGet["security"] == nil {
						t.Fatal("missing sheet bookings bearer security")
					}
				}
				if path == "/swagger/index.html" && want == http.StatusOK && !strings.Contains(w.Body.String(), "swagger-ui") {
					t.Fatal("missing Swagger UI")
				}
			}
		})
	}
}

type authRepository struct {
	credentials auth.Credentials
	admin       user.User
	err         error
}

func (r *authRepository) CreateUser(_ context.Context, name, email, hash string) (user.User, error) {
	if r.err != nil {
		return user.User{}, r.err
	}
	if r.credentials.User.Email == email {
		return user.User{}, auth.ErrEmailExists
	}
	r.credentials = auth.Credentials{User: user.User{ID: "11111111-1111-4111-8111-111111111111", Name: name, Email: email, Role: "USER_A", Active: true}, PasswordHash: hash}
	return r.credentials.User, nil
}
func (r *authRepository) CreateAdmin(_ context.Context, name, email, _ string) (user.User, error) {
	if r.err != nil {
		return user.User{}, r.err
	}
	if r.credentials.User.Email == email || r.admin.ID != "" {
		return user.User{}, auth.ErrEmailExists
	}
	r.admin = user.User{ID: "admin-id", Name: name, Email: email, Role: user.RoleAdmin, Active: true}
	return r.admin, nil
}
func (r *authRepository) FindCredentialsByEmail(_ context.Context, email string) (auth.Credentials, error) {
	if r.err != nil {
		return auth.Credentials{}, r.err
	}
	if email != r.credentials.User.Email {
		return auth.Credentials{}, auth.ErrNotFound
	}
	return r.credentials, nil
}
func (r *authRepository) FindUserByID(_ context.Context, id string) (user.User, error) {
	if r.err != nil {
		return user.User{}, r.err
	}
	if id != r.credentials.User.ID {
		return user.User{}, auth.ErrNotFound
	}
	return r.credentials.User, nil
}
func (r *authRepository) FindAdmin(_ context.Context) (user.User, error) {
	if r.err != nil {
		return user.User{}, r.err
	}
	if r.admin.ID == "" {
		return user.User{}, auth.ErrNotFound
	}
	return r.admin, nil
}

func TestAuthRoutes(t *testing.T) {
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			var logs strings.Builder
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			repo := &authRepository{}
			tokens, err := token.New("test-only-secret-with-at-least-32-bytes")
			if err != nil {
				t.Fatal(err)
			}
			service, err := auth.New(repo, tokens)
			if err != nil {
				t.Fatal(err)
			}
			router := NewRouter(RouterConfig{Health: health.New(pinger{}, logger), Auth: httpauth.New(service, logger), Logger: logger, AppEnv: env})
			request := func(method, path, body, bearer string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				if bearer != "" {
					r.Header.Set("Authorization", bearer)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("missing JSON/no-store headers")
				}
				if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "active") {
					t.Fatalf("private fields exposed: %s", w.Body.String())
				}
				return w
			}
			body := `{"name":" Test User ","email":" TEST@example.com ","password":"test-password-123"}`
			w := request("POST", "/api/v1/auth/register", body, "")
			if w.Code != 201 {
				t.Fatalf("register: %d %s", w.Code, w.Body.String())
			}
			var registered auth.Result
			if err := json.Unmarshal(w.Body.Bytes(), &registered); err != nil {
				t.Fatal(err)
			}
			if registered.User.Role != "USER_A" || registered.User.Email != "test@example.com" || registered.ExpiresIn != 3600 || registered.TokenType != "Bearer" || registered.AccessToken == "" {
				t.Fatalf("unexpected registration: %+v", registered)
			}
			w = request("POST", "/api/v1/auth/login", `{"email":"test@example.com","password":"test-password-123"}`, "")
			if w.Code != 200 {
				t.Fatalf("login: %d %s", w.Code, w.Body.String())
			}
			var loggedIn auth.Result
			if err := json.Unmarshal(w.Body.Bytes(), &loggedIn); err != nil {
				t.Fatal(err)
			}
			w = request("GET", "/api/v1/auth/me", "", "Bearer "+loggedIn.AccessToken)
			if w.Code != 200 || !strings.Contains(w.Body.String(), registered.User.ID) {
				t.Fatalf("me: %d %s", w.Code, w.Body.String())
			}
			for _, tc := range []struct {
				path, body, bearer string
				status             int
			}{
				{"register", body, "", 409},
				{"register", `{"name":"User","email":"other@example.com","password":"test-password-123","role":"OS"}`, "", 400},
				{"register", `{"name":"User","email":"other@example.com","password":"test-password-123","id":"client-id"}`, "", 400},
				{"register", `{"name":"User","email":"other@example.com","password":"test-password-123","active":true}`, "", 400},
				{"register", `{"name":"User","email":"bad-email","password":"test-password-123"}`, "", 400},
				{"register", `{"name":"User","email":"other@example.com","password":"short"}`, "", 400},
				{"register", `{`, "", 400}, {"register", `null`, "", 400},
				{"register", body + ` {}`, "", 400}, {"register", strings.Repeat(" ", 16<<10) + body, "", 400},
				{"login", `{"email":"test@example.com","password":"incorrect-password"}`, "", 401},
				{"login", `{"email":"unknown@example.com","password":"incorrect-password"}`, "", 401},
				{"login", `{"email":"test@example.com","password":"test-password-123","unknown":1}`, "", 400},
				{"me", "", "", 401}, {"me", "", "Bearer", 401}, {"me", "", "Bearer invalid", 401},
				{"me", "", "Basic " + loggedIn.AccessToken, 401}, {"me", "", "Bearer " + loggedIn.AccessToken + " extra", 401},
			} {
				method := "POST"
				if tc.path == "me" {
					method = "GET"
				}
				w := request(method, "/api/v1/auth/"+tc.path, tc.body, tc.bearer)
				if w.Code != tc.status {
					t.Fatalf("%s: got %d want %d: %s", tc.path, w.Code, tc.status, w.Body.String())
				}
			}
			repo.credentials.User.Role = user.RoleOS
			w = request("GET", "/api/v1/auth/me", "", "Bearer "+loggedIn.AccessToken)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"OS"`) {
				t.Fatal("role change not reflected")
			}
			repo.credentials.User.Active = false
			if w := request("GET", "/api/v1/auth/me", "", "Bearer "+loggedIn.AccessToken); w.Code != 401 {
				t.Fatal("disabled user accepted")
			}
			repo.credentials.User.Active = true
			repo.credentials.User.ID = "deleted"
			if w := request("GET", "/api/v1/auth/me", "", "Bearer "+loggedIn.AccessToken); w.Code != 401 {
				t.Fatal("deleted user accepted")
			}
			repo.err = errors.New("database-password-must-not-appear")
			if w := request("POST", "/api/v1/auth/login", `{"email":"test@example.com","password":"test-password-123"}`, ""); w.Code != 500 {
				t.Fatal("wrong database error status")
			}
			for _, secret := range []string{"test-password-123", "database-password-must-not-appear", loggedIn.AccessToken} {
				if strings.Contains(logs.String(), secret) {
					t.Fatal("sensitive data logged")
				}
			}
		})
	}
}

type sheetReaderFunc func(context.Context) ([]sheetbooking.Booking, error)

func (f sheetReaderFunc) List(ctx context.Context) ([]sheetbooking.Booking, error) { return f(ctx) }

func TestSheetBookingRoutes(t *testing.T) {
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			var logs strings.Builder
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			repo := &authRepository{credentials: auth.Credentials{User: user.User{
				ID: "11111111-1111-4111-8111-111111111111", Role: "OS", Active: true,
			}}}
			tokens, err := token.New("test-only-secret-with-at-least-32-bytes")
			if err != nil {
				t.Fatal(err)
			}
			accessToken, err := tokens.Issue(repo.credentials.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			authService, err := auth.New(repo, tokens)
			if err != nil {
				t.Fatal(err)
			}
			fetches := 0
			upstreamError := false
			reader := sheetReaderFunc(func(context.Context) ([]sheetbooking.Booking, error) {
				fetches++
				if upstreamError {
					return nil, errors.New("private-upstream-detail")
				}
				return []sheetbooking.Booking{{ID: "123", Name: "Requester"}}, nil
			})
			router := NewRouter(RouterConfig{
				Auth:          httpauth.New(authService, logger),
				Health:        health.New(pinger{}, logger),
				SheetBookings: httpsheetbooking.New(sheetbooking.New(reader), logger),
				Logger:        logger,
				AppEnv:        env,
			})
			request := func(bearer string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sheet-bookings", nil)
				if bearer != "" {
					r.Header.Set("Authorization", bearer)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("missing JSON/no-store headers")
				}
				return w
			}
			for _, bearer := range []string{"", "Bearer invalid"} {
				if w := request(bearer); w.Code != http.StatusUnauthorized {
					t.Fatalf("unauthorized request: %d %s", w.Code, w.Body.String())
				}
			}
			if fetches != 0 {
				t.Fatal("fetched sheet before authorization")
			}
			repo.credentials.User.Role = user.RoleUserA
			if w := request("Bearer " + accessToken); w.Code != http.StatusForbidden {
				t.Fatalf("non-OS request: %d %s", w.Code, w.Body.String())
			}
			if fetches != 0 {
				t.Fatal("fetched sheet for non-OS user")
			}
			repo.credentials.User.Role = user.RoleOS
			for i := 1; i <= 2; i++ {
				w := request("Bearer " + accessToken)
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"Requester"`) || fetches != i {
					t.Fatalf("OS request: %d %s, fetches=%d", w.Code, w.Body.String(), fetches)
				}
			}
			repo.credentials.User.Role = user.RoleAdmin
			if w := request("Bearer " + accessToken); w.Code != http.StatusOK || fetches != 3 {
				t.Fatalf("admin request: %d %s, fetches=%d", w.Code, w.Body.String(), fetches)
			}
			upstreamError = true
			w := request("Bearer " + accessToken)
			if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"error":`) || strings.Contains(w.Body.String(), "private-upstream-detail") || strings.Contains(logs.String(), "private-upstream-detail") {
				t.Fatalf("upstream failure leaked details: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
