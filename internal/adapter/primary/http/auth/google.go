package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainauth "github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

const (
	stateCookieName = "google_oauth_state"
	stateLifetime   = 10 * time.Minute
)

type OAuthStateStore interface {
	Save(context.Context, []byte, string, time.Time) error
	Consume(context.Context, []byte) (string, bool, error)
}

type GoogleHandler struct {
	service     domainauth.Service
	stateStore  OAuthStateStore
	logger      *slog.Logger
	config      oauth2.Config
	client      *http.Client
	verify      func(context.Context, string, string) (*idtoken.Payload, error)
	secure      bool
	sameSite    http.SameSite
	frontendURL string
	domain      string
}

func NewGoogle(ctx context.Context, service domainauth.Service, stateStore OAuthStateStore, logger *slog.Logger, clientID, clientSecret, redirectURL, frontendURL, domain string, sameSite http.SameSite) (*GoogleHandler, error) {
	if stateStore == nil {
		return nil, errors.New("OAuth state store is required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	validator, err := idtoken.NewValidator(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return nil, err
	}
	return &GoogleHandler{
		service: service, stateStore: stateStore,
		logger: logger,
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Scopes: []string{"openid", "email", "profile"}, Endpoint: google.Endpoint,
		},
		client: client, verify: validator.Validate, secure: parsed.Scheme == "https", sameSite: sameSite,
		frontendURL: frontendURL, domain: domain,
	}, nil
}

// URL returns a Google authorization URL and sets a browser-bound state cookie.
// @Summary Get Google authorization URL
// @Description Development only. The same browser must retain the temporary cookie when following auth_url.
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} errorResponse
// @Router /auth/google/url [get]
func (h *GoogleHandler) URL(w http.ResponseWriter, r *http.Request) {
	h.sensitiveHeaders(w)
	uri, err := h.start(r.Context(), w)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	respond(w, http.StatusOK, map[string]string{"auth_url": uri})
}

// Login redirects the browser to Google using the same flow as URL.
// @Summary Start Google sign-in
// @Description Redirects to the configured API host when needed, then sets a temporary state cookie and redirects to Google.
// @Tags auth
// @Success 302
// @Failure 503 {object} errorResponse
// @Router /auth/google/login [get]
func (h *GoogleHandler) Login(w http.ResponseWriter, r *http.Request) {
	h.sensitiveHeaders(w)
	callback, _ := url.Parse(h.config.RedirectURL) // Startup configuration validates this URL.
	if !strings.EqualFold(r.Host, callback.Host) {
		callback.Path = "/auth/google/login"
		http.Redirect(w, r, callback.String(), http.StatusFound)
		return
	}
	uri, err := h.start(r.Context(), w)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	http.Redirect(w, r, uri, http.StatusFound)
}

// Callback verifies Google's ID token, sets an application auth cookie, and redirects to the frontend.
// @Summary Complete Google sign-in
// @Description Redirects to FRONTEND_URL on success or /login?auth_error=code on failure. Never exposes a token in the URL or response body.
// @Tags auth
// @Success 302
// @Router /auth/google/callback [get]
func (h *GoogleHandler) Callback(w http.ResponseWriter, r *http.Request) {
	h.sensitiveHeaders(w)
	h.setStateCookie(w, "", -1)
	query := r.URL.Query()
	states := query["state"]
	cookie, err := r.Cookie(stateCookieName)
	if err != nil || len(states) != 1 || states[0] == "" || subtle.ConstantTimeCompare([]byte(states[0]), []byte(cookie.Value)) != 1 {
		h.redirectError(w, r, "invalid_callback", "state_cookie")
		return
	}
	verifier, ok, err := h.stateStore.Consume(r.Context(), hashState(states[0]))
	if err != nil {
		h.redirectError(w, r, "server_error", "state_storage")
		return
	}
	if !ok {
		h.redirectError(w, r, "invalid_callback", "state_expired_or_used")
		return
	}
	if len(query["error"]) > 0 {
		h.redirectError(w, r, "access_denied", "google_denied")
		return
	}
	codes := query["code"]
	if len(codes) != 1 || codes[0] == "" {
		h.redirectError(w, r, "invalid_callback", "missing_code")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, h.client)
	token, err := h.config.Exchange(ctx, codes[0], oauth2.VerifierOption(verifier))
	if err != nil {
		h.redirectError(w, r, "invalid_callback", "code_exchange")
		return
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok || idToken == "" {
		h.redirectError(w, r, "invalid_callback", "missing_id_token")
		return
	}
	payload, err := h.verify(ctx, idToken, h.config.ClientID)
	if err != nil || payload == nil || payload.Issuer != "accounts.google.com" && payload.Issuer != "https://accounts.google.com" || payload.Audience != h.config.ClientID || payload.Expires <= time.Now().Unix() || payload.Subject == "" {
		h.redirectError(w, r, "invalid_callback", "id_token_verification")
		return
	}
	verified, _ := payload.Claims["email_verified"].(bool)
	result, err := h.service.SignInGoogle(ctx, domainauth.GoogleIdentity{
		Subject: payload.Subject, Email: claim(payload, "email"), EmailVerified: verified,
		HostedDomain: claim(payload, "hd"), FirstName: claim(payload, "given_name"), LastName: claim(payload, "family_name"),
	})
	if err != nil {
		switch {
		case errors.Is(err, domainauth.ErrIneligible):
			h.redirectError(w, r, "ineligible", "eligibility")
		case errors.Is(err, domainauth.ErrUnauthorized):
			h.redirectError(w, r, "account_unavailable", "account_unavailable")
		case errors.Is(err, domainauth.ErrIdentityConflict):
			h.redirectError(w, r, "identity_conflict", "identity_conflict")
		default:
			h.logger.ErrorContext(r.Context(), "Google sign-in failed", "error_type", fmt.Sprintf("%T", err))
			h.redirectError(w, r, "server_error", "user_resolution")
		}
		return
	}
	setAuthCookie(w, result.AccessToken, h.secure, h.sameSite, result.ExpiresIn)
	http.Redirect(w, r, h.frontendURL, http.StatusFound)
}

func (h *GoogleHandler) redirectError(w http.ResponseWriter, r *http.Request, code, stage string) {
	h.logger.WarnContext(r.Context(), "Google sign-in callback failed", "stage", stage, "request_id", chimiddleware.GetReqID(r.Context()))
	http.Redirect(w, r, h.frontendURL+"/login?auth_error="+code, http.StatusFound)
}

func (h *GoogleHandler) start(ctx context.Context, w http.ResponseWriter) (string, error) {
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := h.stateStore.Save(ctx, hashState(state), verifier, time.Now().Add(stateLifetime)); err != nil {
		return "", err
	}
	h.setStateCookie(w, state, int(stateLifetime.Seconds()))
	uri := h.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("hd", h.domain))
	return uri, nil
}

func hashState(state string) []byte {
	hash := sha256.Sum256([]byte(state))
	return hash[:]
}

func (h *GoogleHandler) setStateCookie(w http.ResponseWriter, state string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: stateCookieName, Value: state, Path: "/auth/google/callback",
		HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})
}

func (h *GoogleHandler) sensitiveHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
}

func (h *GoogleHandler) failure(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "OAuth flow failed", "error_type", fmt.Sprintf("%T", err))
	respond(w, http.StatusServiceUnavailable, errorResponse{Error: "OAuth temporarily unavailable"})
}

func claim(payload *idtoken.Payload, key string) string {
	value, _ := payload.Claims[key].(string)
	return value
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
