package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/chula-room-booking/internal/core/domain/user"
	"golang.org/x/crypto/bcrypt"
)

type memoryRepository struct {
	credentials Credentials
	err         error
}

func (r *memoryRepository) CreateUser(_ context.Context, name, email, hash string) (user.User, error) {
	if r.err != nil {
		return user.User{}, r.err
	}
	if r.credentials.User.Email == email {
		return user.User{}, ErrEmailExists
	}
	r.credentials = Credentials{User: user.User{ID: "user-id", Name: name, Email: email, Role: "USER_A", Active: true}, PasswordHash: hash}
	return r.credentials.User, nil
}
func (r *memoryRepository) FindCredentialsByEmail(_ context.Context, email string) (Credentials, error) {
	if r.err != nil {
		return Credentials{}, r.err
	}
	if r.credentials.User.Email != email {
		return Credentials{}, ErrNotFound
	}
	return r.credentials, nil
}
func (r *memoryRepository) FindUserByID(_ context.Context, id string) (user.User, error) {
	if r.err != nil {
		return user.User{}, r.err
	}
	if r.credentials.User.ID != id {
		return user.User{}, ErrNotFound
	}
	return r.credentials.User, nil
}

type testTokens struct{}

func (testTokens) Issue(id string) (string, error) { return id, nil }
func (testTokens) Verify(token string) (string, error) {
	if token == "invalid" {
		return "", ErrUnauthorized
	}
	return token, nil
}

func TestAuthenticationFlow(t *testing.T) {
	ctx := context.Background()
	repo := &memoryRepository{}
	s, err := New(repo, testTokens{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Register(ctx, " Test User ", " TEST@Example.COM ", " password with spaces ")
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Name != "Test User" || result.User.Email != "test@example.com" || result.User.Role != "USER_A" || result.ExpiresIn != 3600 || result.TokenType != "Bearer" || result.AccessToken == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.credentials.PasswordHash), []byte(" password with spaces ")); err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost([]byte(repo.credentials.PasswordHash)); err != nil || cost != 12 {
		t.Fatalf("wrong bcrypt cost: %d, %v", cost, err)
	}
	if _, err := s.Login(ctx, " TEST@example.com ", " password with spaces "); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(ctx, "Another", "test@example.com", "password123"); !errors.Is(err, ErrEmailExists) {
		t.Fatalf("duplicate: %v", err)
	}
	for _, tc := range []struct{ name, email, password string }{
		{"wrong password", "test@example.com", "incorrect-password"},
		{"unknown user", "other@example.com", "password123"},
		{"password preserved", "test@example.com", "password with spaces"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Login(ctx, tc.email, tc.password); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("want unauthorized, got %v", err)
			}
		})
	}
	repo.credentials.User.Role = "OS"
	u, err := s.Authenticate(ctx, result.AccessToken)
	if err != nil || u.Role != "OS" {
		t.Fatalf("current role: %+v %v", u, err)
	}
	repo.credentials.User.Active = false
	if _, err := s.Login(ctx, "test@example.com", " password with spaces "); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled login: %v", err)
	}
	if _, err := s.Authenticate(ctx, result.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled token: %v", err)
	}
	repo.credentials.User.Active = true
	repo.credentials.PasswordHash = ""
	if _, err := s.Login(ctx, "test@example.com", "password123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("passwordless login: %v", err)
	}
	for _, token := range []string{"invalid", "deleted-user"} {
		if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("invalid token: %v", err)
		}
	}
	repo.err = errors.New("database unavailable")
	if _, err := s.Login(ctx, "test@example.com", "password123"); !errors.Is(err, repo.err) {
		t.Fatalf("repository error: %v", err)
	}
	if _, err := s.Authenticate(ctx, result.AccessToken); !errors.Is(err, repo.err) {
		t.Fatalf("repository error: %v", err)
	}
}

func TestInputValidation(t *testing.T) {
	s, err := New(&memoryRepository{}, testTokens{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, email, password string }{
		{"", "test@example.com", "password123"}, {"   ", "test@example.com", "password123"},
		{"User", "bad-email", "password123"}, {"User", "User <test@example.com>", "password123"},
		{"User", "test@example.com", "short"}, {"User", "test@example.com", strings.Repeat("x", 73)},
		{"User", "test@example.com", strings.Repeat("ก", 25)},
	} {
		if _, err := s.Register(context.Background(), tc.name, tc.email, tc.password); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid registration accepted: %+v %v", tc, err)
		}
	}
	for _, password := range []string{"", "short", strings.Repeat("x", 73)} {
		if _, err := s.Login(context.Background(), "test@example.com", password); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid login password accepted: %v", err)
		}
	}
	if _, err := s.Login(context.Background(), "invalid", "password123"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
}
