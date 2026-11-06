package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	domainauth "github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

type googleRepository struct {
	u     user.User
	calls int
}

func (r *googleRepository) ResolveGoogleUser(_ context.Context, _, _, _, _ string) (user.User, error) {
	r.calls++
	return r.u, nil
}
func (r *googleRepository) CreateAdmin(context.Context, string) (user.User, error) {
	return user.User{}, nil
}
func (r *googleRepository) FindAdmin(context.Context) (user.User, error) {
	return user.User{}, domainauth.ErrNotFound
}
func (r *googleRepository) FindUserByID(context.Context, string) (user.User, error) {
	return r.u, nil
}

type googleTokens struct{}

func (googleTokens) Issue(string) (string, error)  { return "application-jwt", nil }
func (googleTokens) Verify(string) (string, error) { return "user-id", nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testGoogleHandler(t *testing.T) (*GoogleHandler, *googleRepository) {
	t.Helper()
	repo := &googleRepository{u: user.User{ID: "user-id", Email: "6500000021@student.chula.ac.th", Role: user.RoleUserA, Active: true}}
	h := &GoogleHandler{
		service: domainauth.New(repo, googleTokens{}, "student.chula.ac.th", "21"),
		logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
		config: oauth2.Config{
			ClientID: "client-id", ClientSecret: "client-secret",
			RedirectURL: "http://localhost:8080/auth/google/callback",
			Scopes:      []string{"openid", "email", "profile"},
			Endpoint:    oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth.test/token", AuthStyle: oauth2.AuthStyleInParams},
		},
		domain: "student.chula.ac.th", frontendURL: "http://localhost:3000", sameSite: http.SameSiteLaxMode,
		states: make(map[string]oauthTransaction),
	}
	h.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code_verifier") == "" {
			t.Errorf("missing PKCE verifier: %v", err)
		}
		return &http.Response{
			StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"access_token":"google-access","token_type":"Bearer","id_token":"signed-id-token"}`)),
		}, nil
	})}
	h.verify = func(_ context.Context, token, audience string) (*idtoken.Payload, error) {
		if token != "signed-id-token" || audience != "client-id" {
			t.Errorf("verifier input: token=%q audience=%q", token, audience)
		}
		return validPayload(), nil
	}
	return h, repo
}

func validPayload() *idtoken.Payload {
	return &idtoken.Payload{
		Issuer: "https://accounts.google.com", Audience: "client-id", Expires: time.Now().Add(time.Hour).Unix(), Subject: "google-sub",
		Claims: map[string]any{
			"email": "6500000021@student.chula.ac.th", "email_verified": true,
			"hd": "student.chula.ac.th", "name": "First Last", "given_name": "First", "family_name": "Last",
		},
	}
}

func startGoogleFlow(t *testing.T, h *GoogleHandler, login bool) (string, *http.Cookie) {
	t.Helper()
	path := "/auth/google/url"
	if login {
		path = "/auth/google/login"
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if login {
		h.Login(w, r)
	} else {
		h.URL(w, r)
	}
	want := http.StatusOK
	if login {
		want = http.StatusFound
	}
	if w.Code != want {
		t.Fatalf("start = %d: %s", w.Code, w.Body.String())
	}
	var authURL string
	if login {
		authURL = w.Header().Get("Location")
	} else {
		var body struct {
			AuthURL string `json:"auth_url"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		authURL = body.AuthURL
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	params := parsed.Query()
	if params.Get("state") == "" || params.Get("code_challenge_method") != "S256" || params.Get("code_challenge") == "" || params.Get("hd") != "student.chula.ac.th" || !strings.Contains(params.Get("scope"), "openid") || strings.Contains(authURL, "code_verifier") {
		t.Fatalf("bad authorization URL: %s", authURL)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != params.Get("state") || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/auth/google/callback" {
		t.Fatalf("state cookie: %+v", cookies)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("start response is cacheable")
	}
	return params.Get("state"), cookies[0]
}

func callback(h *GoogleHandler, state, code, oauthError string, cookie *http.Cookie) *httptest.ResponseRecorder {
	params := url.Values{}
	if state != "" {
		params.Set("state", state)
	}
	if code != "" {
		params.Set("code", code)
	}
	if oauthError != "" {
		params.Set("error", oauthError)
	}
	r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?"+params.Encode(), nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.Callback(w, r)
	return w
}

func TestGoogleFlowSuccessAndReplay(t *testing.T) {
	for _, login := range []bool{false, true} {
		h, repo := testGoogleHandler(t)
		state, cookie := startGoogleFlow(t, h, login)
		w := callback(h, state, "valid-code", "", cookie)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "http://localhost:3000" || repo.calls != 1 || strings.Contains(w.Body.String(), "application-jwt") || strings.Contains(w.Body.String(), "google-access") {
			t.Fatalf("callback: %d %s", w.Code, w.Body.String())
		}
		cookies := w.Result().Cookies()
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || len(cookies) != 2 || cookies[0].MaxAge != -1 || cookies[1].Name != authCookieName || cookies[1].Value != "application-jwt" || !cookies[1].HttpOnly || cookies[1].Path != "/api/v1" || cookies[1].MaxAge != 3600 {
			t.Fatal("sensitive callback headers or cookie clearing missing")
		}
		if replay := callback(h, state, "valid-code", "", cookie); replay.Code != http.StatusFound || !strings.Contains(replay.Header().Get("Location"), "invalid_callback") || len(replay.Result().Cookies()) != 1 {
			t.Fatalf("replay = %d", replay.Code)
		}
	}
}

func TestGoogleHandlerAcceptsServicePort(t *testing.T) {
	h, _ := testGoogleHandler(t)
	service := &stubService{result: domainauth.Result{AccessToken: "stub-jwt", ExpiresIn: 3600}}
	h.service = service
	state, cookie := startGoogleFlow(t, h, true)
	response := callback(h, state, "valid-code", "", cookie)
	if response.Code != http.StatusFound || service.signInCalls != 1 {
		t.Fatalf("status=%d sign-in calls=%d", response.Code, service.signInCalls)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == authCookieName && cookie.Value == "stub-jwt" {
			return
		}
	}
	t.Fatal("missing stub service token cookie")
}

func TestGoogleCookiesReachProtectedPath(t *testing.T) {
	h, _ := testGoogleHandler(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	callbackURL, _ := url.Parse("http://localhost:8080/auth/google/callback")
	apiURL, _ := url.Parse("http://localhost:8080/api/v1/auth/me")
	state, cookie := startGoogleFlow(t, h, true)
	jar.SetCookies(callbackURL, []*http.Cookie{cookie})
	w := callback(h, state, "valid-code", "", jar.Cookies(callbackURL)[0])
	jar.SetCookies(callbackURL, w.Result().Cookies())
	if cookies := jar.Cookies(apiURL); len(cookies) != 1 || cookies[0].Name != authCookieName || cookies[0].Value != "application-jwt" {
		t.Fatalf("protected request cookies: %+v", cookies)
	}
	if cookies := jar.Cookies(callbackURL); len(cookies) != 0 {
		t.Fatalf("callback cookies after sign-in: %+v", cookies)
	}
}

func TestGoogleCallbackLogsFailureStageWithoutSecrets(t *testing.T) {
	h, _ := testGoogleHandler(t)
	var logs bytes.Buffer
	h.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	state, cookie := startGoogleFlow(t, h, true)
	h.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("secret-upstream-details")
	})}
	callback(h, state, "secret-code", "", cookie)
	if !strings.Contains(logs.String(), `"stage":"code_exchange"`) {
		t.Fatalf("missing failure stage: %s", logs.String())
	}
	for _, secret := range []string{state, "secret-code", "secret-upstream-details"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("callback log contains sensitive data")
		}
	}
}

func TestProductionAuthCookie(t *testing.T) {
	h, _ := testGoogleHandler(t)
	h.secure = true
	h.sameSite = http.SameSiteNoneMode
	h.frontendURL = "https://frontend.example"
	state, cookie := startGoogleFlow(t, h, true)
	w := callback(h, state, "valid-code", "", cookie)
	if w.Code != http.StatusFound || w.Header().Get("Location") != h.frontendURL {
		t.Fatalf("redirect: %d %s", w.Code, w.Header().Get("Location"))
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == authCookieName && c.Value != "" {
			if c.Value != "application-jwt" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteNoneMode || c.MaxAge != 3600 {
				t.Fatalf("production cookie: %+v", c)
			}
			return
		}
	}
	t.Fatal("missing auth cookie")
}

func TestGoogleCallbackFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*GoogleHandler, string, **http.Cookie, *string, *string)
		want   int
	}{
		{"missing state", func(_ *GoogleHandler, _ string, _ **http.Cookie, state, _ *string) { *state = "" }, 400},
		{"wrong state", func(_ *GoogleHandler, _ string, _ **http.Cookie, state, _ *string) { *state = "wrong" }, 400},
		{"missing cookie", func(_ *GoogleHandler, _ string, cookie **http.Cookie, _, _ *string) { *cookie = nil }, 400},
		{"expired state", func(h *GoogleHandler, state string, _ **http.Cookie, _, _ *string) {
			h.states[state] = oauthTransaction{verifier: "verifier", expires: time.Now().Add(-time.Second)}
		}, 400},
		{"missing code", func(_ *GoogleHandler, _ string, _ **http.Cookie, _, code *string) { *code = "" }, 400},
		{"failed exchange", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("upstream failed") })}
		}, 400},
		{"missing ID token", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"access_token":"google-access","token_type":"Bearer"}`))}, nil
			})}
		}, 401},
		{"invalid token", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				return nil, errors.New("bad signature")
			}
		}, 401},
		{"wrong issuer", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				p.Issuer = "evil.example"
				return p, nil
			}
		}, 401},
		{"wrong audience", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				p.Audience = "other-client"
				return p, nil
			}
		}, 401},
		{"expired token", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				p.Expires = time.Now().Add(-time.Second).Unix()
				return p, nil
			}
		}, 401},
		{"missing subject", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				p.Subject = ""
				return p, nil
			}
		}, 401},
		{"unverified email", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				p.Claims["email_verified"] = false
				return p, nil
			}
		}, 403},
		{"missing email", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				delete(p.Claims, "email")
				return p, nil
			}
		}, 403},
		{"missing hosted domain", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				p := validPayload()
				delete(p.Claims, "hd")
				return p, nil
			}
		}, 403},
		{"ineligible user", func(h *GoogleHandler, _ string, _ **http.Cookie, _, _ *string) {
			h.verify = func(context.Context, string, string) (*idtoken.Payload, error) {
				return &idtoken.Payload{Issuer: "accounts.google.com", Audience: "client-id", Expires: time.Now().Add(time.Hour).Unix(), Subject: "sub", Claims: map[string]any{"email": "someone@gmail.com", "email_verified": true, "hd": "student.chula.ac.th"}}, nil
			}
		}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := testGoogleHandler(t)
			state, cookie := startGoogleFlow(t, h, false)
			code := "valid-code"
			tc.mutate(h, state, &cookie, &state, &code)
			w := callback(h, state, code, "", cookie)
			if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "http://localhost:3000/login?auth_error=") || repo.calls != 0 || strings.Contains(w.Header().Get("Location"), "google-access") {
				t.Fatalf("callback = %d, repo calls = %d: %s", w.Code, repo.calls, w.Body.String())
			}
		})
	}
	h, _ := testGoogleHandler(t)
	state, cookie := startGoogleFlow(t, h, false)
	if w := callback(h, state, "", "access_denied", cookie); w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "auth_error=access_denied") {
		t.Fatalf("Google error = %d", w.Code)
	}
}
