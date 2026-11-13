package httpadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/MadManJJ/room-reservation-api/docs"
	httpauth "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/auth"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/health"
	httpsheetbooking "github.com/MadManJJ/room-reservation-api/internal/adapter/primary/http/sheetbooking"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/token"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	domainbooking "github.com/MadManJJ/room-reservation-api/internal/core/domain/sheetbooking"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

type pinger struct{}

func (pinger) Ping(context.Context) error { return nil }

type routerOAuthStates struct{}

func (routerOAuthStates) Save(context.Context, []byte, string, time.Time) error { return nil }
func (routerOAuthStates) Consume(context.Context, []byte) (string, bool, error) {
	return "", false, nil
}

type sheetService struct{}

func (sheetService) ValidBuilding(string) bool { return true }
func (sheetService) List(context.Context, domainbooking.Query) (domainbooking.Page, error) {
	return domainbooking.Page{Items: []domainbooking.Booking{}}, nil
}

func (sheetService) Calendar(context.Context, int, int, string) (domainbooking.CalendarResult, error) {
	return domainbooking.CalendarResult{Days: []domainbooking.Day{}}, nil
}

func (sheetService) Weekly(context.Context, string, string, string) (domainbooking.WeeklyResult, error) {
	return domainbooking.WeeklyResult{Rooms: []domainbooking.WeeklyRoom{}}, nil
}

type authRepository struct {
	current user.User
	users   map[string]user.User
}

func (r *authRepository) ResolveGoogleUser(context.Context, string, string, string, string) (user.User, error) {
	return r.current, nil
}

func (r *authRepository) CreateAdmin(context.Context, string) (user.User, error) {
	return user.User{}, nil
}

func (r *authRepository) FindAdmin(context.Context) (user.User, error) {
	return user.User{}, auth.ErrNotFound
}

func (r *authRepository) FindUserByID(_ context.Context, id string) (user.User, error) {
	if r.users != nil {
		u, ok := r.users[id]
		if !ok {
			return user.User{}, auth.ErrNotFound
		}
		return u, nil
	}
	return r.current, nil
}

func sheetAuthenticationUsers(t *testing.T, logger *slog.Logger, count int) (*httpauth.Handler, []string) {
	t.Helper()
	tokens, err := token.New("test-only-jwt-secret-at-least-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	users := make(map[string]user.User, count)
	jwts := make([]string, count)
	for i := range count {
		id := fmt.Sprintf("11111111-1111-4111-8111-%012d", i+1)
		users[id] = user.User{ID: id, Role: user.RoleUserA, Active: true}
		jwts[i], err = tokens.Issue(id)
		if err != nil {
			t.Fatal(err)
		}
	}
	service := auth.New(&authRepository{users: users}, tokens, "student.chula.ac.th", "21")
	return httpauth.New(service, logger, false, http.SameSiteLaxMode), jwts
}

func sheetAuthentication(t *testing.T, logger *slog.Logger, role string) (*httpauth.Handler, string) {
	t.Helper()
	tokens, err := token.New("test-only-jwt-secret-at-least-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	u := user.User{ID: "11111111-1111-4111-8111-111111111111", Email: "6500000021@student.chula.ac.th", Role: role, Active: true}
	jwt, err := tokens.Issue(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return httpauth.New(auth.New(&authRepository{current: u}, tokens, "student.chula.ac.th", "21"), logger, false, http.SameSiteLaxMode), jwt
}

func authenticatedRequest(method, path, jwt string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(&http.Cookie{Name: "room_auth", Value: jwt})
	return request
}

func TestRoutesAndSwagger(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sheets := httpsheetbooking.New(sheetService{}, logger)
	for _, env := range []string{"development", "production"} {
		t.Run(env, func(t *testing.T) {
			router := NewRouter(RouterConfig{Health: health.New(pinger{}, logger), SheetBookings: sheets, Logger: logger, AppEnv: env})
			for _, oldPath := range []string{"/healthz", "/readyz"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, oldPath, nil))
				if w.Code != http.StatusNotFound {
					t.Errorf("GET %s = %d, want 404", oldPath, w.Code)
				}
			}
			for _, path := range []string{"/health", "/ready", "/swagger/doc.json", "/swagger/index.html", "/api/v1/auth/me", "/api/v1/auth/register", "/api/v1/auth/login", "/api/v1/admin/sheet-bookings", "/api/v1/admin/sheet-bookings/calendar?year=2026&month=11", "/api/v1/admin/sheet-bookings/weekly?start_date=2026-11-02", "/auth/google/login", "/auth/google/callback"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				want := http.StatusOK
				if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/") || env == "production" && strings.HasPrefix(path, "/swagger/") {
					want = http.StatusNotFound
				}
				if strings.HasPrefix(path, "/api/v1/admin/sheet-bookings") {
					want = http.StatusServiceUnavailable
				}
				if w.Code != want {
					t.Errorf("GET %s = %d, want %d", path, w.Code, want)
				}
				if path == "/api/v1/admin/sheet-bookings" && w.Code == http.StatusServiceUnavailable && w.Body.String() != `{"error":"authentication unavailable"}` {
					t.Fatalf("unavailable authentication response: %d %s", w.Code, w.Body.String())
				}
				if path == "/swagger/doc.json" && want == http.StatusOK {
					var spec struct {
						BasePath            string                       `json:"basePath"`
						Paths               map[string]map[string]any    `json:"paths"`
						SecurityDefinitions map[string]map[string]string `json:"securityDefinitions"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
						t.Fatal(err)
					}
					if spec.BasePath != "/" {
						t.Fatalf("base path: %q", spec.BasePath)
					}
					for _, documented := range []string{"/health", "/ready", "/api/v1/auth/me", "/api/v1/admin/sheet-bookings", "/api/v1/admin/sheet-bookings/calendar", "/api/v1/admin/sheet-bookings/weekly", "/auth/google/url", "/auth/google/login", "/auth/google/callback"} {
						if spec.Paths[documented]["get"] == nil {
							t.Fatalf("missing GET %s documentation", documented)
						}
					}
					cookieAuth := spec.SecurityDefinitions["CookieAuth"]
					if cookieAuth["type"] != "apiKey" || cookieAuth["in"] != "header" || cookieAuth["name"] != "Cookie" {
						t.Fatalf("cookie auth definition: %+v", cookieAuth)
					}
					for _, path := range []string{"/api/v1/admin/sheet-bookings", "/api/v1/admin/sheet-bookings/calendar", "/api/v1/admin/sheet-bookings/weekly"} {
						operation, ok := spec.Paths[path]["get"].(map[string]any)
						if !ok || operation["security"] == nil {
							t.Fatalf("missing cookie security for %s", path)
						}
						responses, ok := operation["responses"].(map[string]any)
						if !ok || responses["401"] == nil || responses["503"] == nil {
							t.Fatalf("missing auth responses for %s: %+v", path, responses)
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

func TestSheetRoutesRequireAuthenticationForEveryRole(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sheets := httpsheetbooking.New(sheetService{}, logger)
	paths := []string{
		"/api/v1/admin/sheet-bookings",
		"/api/v1/admin/sheet-bookings/calendar?year=2026&month=11",
		"/api/v1/admin/sheet-bookings/weekly?start_date=2026-11-02",
	}
	authHandler, jwt := sheetAuthentication(t, logger, user.RoleUserA)
	router := NewRouter(RouterConfig{Auth: authHandler, Health: health.New(pinger{}, logger), SheetBookings: sheets, Logger: logger})
	for _, path := range paths {
		for _, request := range []*http.Request{
			httptest.NewRequest(http.MethodGet, path, nil),
			authenticatedRequest(http.MethodGet, path, "invalid"),
		} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, request)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("GET %s without valid auth = %d, want 401", path, w.Code)
			}
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, authenticatedRequest(http.MethodGet, path, jwt))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s with valid auth = %d, want 200", path, w.Code)
		}
	}
	for _, role := range []string{user.RoleUserA, user.RoleUserB, user.RoleOS, user.RoleAdmin} {
		t.Run(role, func(t *testing.T) {
			authHandler, jwt := sheetAuthentication(t, logger, role)
			router := NewRouter(RouterConfig{Auth: authHandler, Health: health.New(pinger{}, logger), SheetBookings: sheets, Logger: logger})
			for _, path := range paths {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, authenticatedRequest(http.MethodGet, path, jwt))
				if w.Code != http.StatusOK {
					t.Errorf("GET %s as %s = %d, want 200", path, role, w.Code)
				}
			}
		})
	}
}

func TestSheetRateLimitSharedAcrossRoutes(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	authHandler, jwts := sheetAuthenticationUsers(t, logger, 2)
	router := NewRouter(RouterConfig{
		Auth: authHandler, Health: health.New(pinger{}, logger), SheetBookings: httpsheetbooking.New(sheetService{}, logger), Logger: logger,
	})
	paths := []string{
		"/api/v1/admin/sheet-bookings",
		"/api/v1/admin/sheet-bookings/calendar?year=2026&month=11",
		"/api/v1/admin/sheet-bookings/weekly?start_date=2026-11-02",
	}
	request := func(path, ip, jwt string) *httptest.ResponseRecorder {
		r := authenticatedRequest(http.MethodGet, path, jwt)
		r.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 600; i++ {
		if got := request(paths[i%len(paths)], "203.0.113.10", jwts[0]).Code; got != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, got)
		}
	}
	w := request(paths[1], "203.0.113.11", jwts[0])
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "1" || w.Body.String() != `{"error":"rate limit exceeded"}` {
		t.Fatalf("rate limit response: %d %s", w.Code, w.Body.String())
	}
	if got := request(paths[0], "203.0.113.10", jwts[1]).Code; got != http.StatusOK {
		t.Fatalf("second user on same IP: got %d, want 200", got)
	}
	if got := request("/health", "203.0.113.10", jwts[0]).Code; got != http.StatusOK {
		t.Fatalf("health: got %d, want 200", got)
	}
}

func TestSheetRateLimitAllowsCampusBurst(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	authHandler, jwts := sheetAuthenticationUsers(t, logger, 200)
	router := NewRouter(RouterConfig{
		Auth: authHandler, Health: health.New(pinger{}, logger), SheetBookings: httpsheetbooking.New(sheetService{}, logger), Logger: logger,
	})
	paths := []string{
		"/api/v1/admin/sheet-bookings",
		"/api/v1/admin/sheet-bookings/calendar?year=2026&month=11",
		"/api/v1/admin/sheet-bookings/weekly?start_date=2026-11-02",
	}
	var rejected atomic.Int64
	var wg sync.WaitGroup
	for _, jwt := range jwts {
		wg.Add(1)
		go func(jwt string) {
			defer wg.Done()
			for _, path := range paths {
				r := authenticatedRequest(http.MethodGet, path, jwt)
				r.RemoteAddr = "203.0.113.10:1234" // One campus NAT IP.
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					rejected.Add(1)
				}
			}
		}(jwt)
	}
	wg.Wait()
	if got := rejected.Load(); got != 0 {
		t.Fatalf("200 concurrent users across 3 routes: %d rejected", got)
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

func TestLogoutOrigins(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, tc := range []struct {
		env, origin string
		want        int
	}{
		{"development", "http://localhost:8080", http.StatusNoContent},
		{"development", "http://localhost:3000", http.StatusNoContent},
		{"development", "http://evil.example", http.StatusForbidden},
		{"production", "http://localhost:8080", http.StatusForbidden},
	} {
		router := NewRouter(RouterConfig{Auth: httpauth.New(nil, logger, false, http.SameSiteLaxMode), Health: health.New(pinger{}, logger), Logger: logger, AppEnv: tc.env, FrontendURL: "http://localhost:3000", GoogleRedirectURL: "http://localhost:8080/auth/google/callback"})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		request.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		if w.Code != tc.want {
			t.Fatalf("%s origin %s: got %d, want %d", tc.env, tc.origin, w.Code, tc.want)
		}
		if tc.want == http.StatusNoContent {
			cookies := w.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != "room_auth" || cookies[0].Path != "/api/v1" || cookies[0].MaxAge != -1 {
				t.Fatalf("logout cookie: %+v", cookies)
			}
		}
	}
}

func TestGoogleRoutesAndCORS(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	repo := &authRepository{current: user.User{ID: "11111111-1111-4111-8111-111111111111", Active: true}}
	authService := auth.New(repo, nil, "student.chula.ac.th", "21")
	google, err := httpauth.NewGoogle(context.Background(), authService, routerOAuthStates{}, logger, "client", "secret", "https://api.example.com/auth/google/callback", "https://frontend.other.example", "student.chula.ac.th", http.SameSiteNoneMode)
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
