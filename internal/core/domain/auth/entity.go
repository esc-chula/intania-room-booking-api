package auth

import "github.com/MadManJJ/room-reservation-api/internal/core/domain/user"

type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	HostedDomain  string
	FirstName     string
	LastName      string
}

type Result struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	User        user.User `json:"user"`
}
