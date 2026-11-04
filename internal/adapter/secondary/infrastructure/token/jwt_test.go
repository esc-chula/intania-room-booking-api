package token

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWT(t *testing.T) {
	secret := "test-only-secret-with-at-least-32-bytes"
	id := "11111111-1111-4111-8111-111111111111"
	j, err := New(secret)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := j.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := j.Verify(raw); err != nil || got != id {
		t.Fatalf("round trip: %q %v", got, err)
	}
	parts := strings.Split(raw, ".")
	parts[1] = "e30"
	for _, invalid := range []string{"", "garbage", strings.Join(parts, ".")} {
		if _, err := j.Verify(invalid); !errors.Is(err, auth.ErrUnauthorized) {
			t.Fatalf("invalid token accepted: %v", err)
		}
	}
	for _, name := range []string{"expired", "missing expiry", "missing issued at", "future issued at", "wrong issuer", "missing issuer", "wrong audience", "missing audience", "invalid subject", "missing subject", "wrong algorithm", "wrong secret", "unsigned"} {
		t.Run(name, func(t *testing.T) {
			claims := jwt.RegisteredClaims{Subject: id, Issuer: issuer, Audience: jwt.ClaimStrings{audience}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
			method := jwt.SigningMethodHS256
			key := any([]byte(secret))
			switch name {
			case "expired":
				claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
			case "missing expiry":
				claims.ExpiresAt = nil
			case "missing issued at":
				claims.IssuedAt = nil
			case "future issued at":
				claims.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
			case "wrong issuer":
				claims.Issuer = "other"
			case "missing issuer":
				claims.Issuer = ""
			case "wrong audience":
				claims.Audience = jwt.ClaimStrings{"other"}
			case "missing audience":
				claims.Audience = nil
			case "invalid subject":
				claims.Subject = "not-a-uuid"
			case "missing subject":
				claims.Subject = ""
			case "wrong algorithm":
				method = jwt.SigningMethodHS384
			case "wrong secret":
				key = []byte("other-secret")
			}
			var token *jwt.Token
			if name == "unsigned" {
				token = jwt.NewWithClaims(jwt.SigningMethodNone, claims)
				key = jwt.UnsafeAllowNoneSignatureType
			} else {
				token = jwt.NewWithClaims(method, claims)
			}
			raw, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := j.Verify(raw); !errors.Is(err, auth.ErrUnauthorized) {
				t.Fatalf("accepted %s: %v", name, err)
			}
		})
	}
	if _, err := New("short"); err == nil {
		t.Fatal("short secret accepted")
	}
	if _, err := j.Issue("invalid"); err == nil {
		t.Fatal("invalid subject issued")
	}
}
