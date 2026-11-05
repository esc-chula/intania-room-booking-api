package auth

import "net/http"

const authCookieName = "room_auth"

func setAuthCookie(w http.ResponseWriter, token string, secure bool, sameSite http.SameSite, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: authCookieName, Value: token, Path: "/api/v1", MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: sameSite,
	})
}
