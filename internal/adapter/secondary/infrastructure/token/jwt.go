package token

import (
	"fmt"
	"regexp"
	"time"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/golang-jwt/jwt/v5"
)

const issuer = "chula-room-booking"
const audience = "chula-room-booking-api"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type JWT struct{ secret []byte }

var _ auth.Tokens = (*JWT)(nil)

func New(secret string) (*JWT, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &JWT{secret: []byte(secret)}, nil
}

func (j *JWT) Issue(id string) (string, error) {
	if !uuidPattern.MatchString(id) {
		return "", auth.ErrUnauthorized
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject: id, Issuer: issuer, Audience: jwt.ClaimStrings{audience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(auth.TokenLifetime)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
}

func (j *JWT) Verify(raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(),
		jwt.WithIssuer(issuer), jwt.WithAudience(audience), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || claims.IssuedAt == nil || !uuidPattern.MatchString(claims.Subject) {
		return "", auth.ErrUnauthorized
	}
	return claims.Subject, nil
}
