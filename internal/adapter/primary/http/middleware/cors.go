package middleware

import (
	"net/http"
	"net/url"
)

func FrontendCORS(frontendURL string) func(http.Handler) http.Handler {
	parsed, _ := url.Parse(frontendURL) // Configuration validates the URL at startup.
	allowedOrigin := parsed.Scheme + "://" + parsed.Host
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Add("Vary", "Origin")
			}
			if origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			} else if origin != "" && r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
