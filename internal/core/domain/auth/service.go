package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"example.com/chula-room-booking/internal/core/domain/user"
	"golang.org/x/crypto/bcrypt"
)

const TokenLifetime = time.Hour
const passwordCost = 12

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrEmailExists  = errors.New("email already registered")
	ErrNotFound     = errors.New("user not found")
	ErrUnauthorized = errors.New("invalid credentials or token")
)

type Credentials struct {
	User         user.User
	PasswordHash string
}

type Repository interface {
	CreateUser(context.Context, string, string, string) (user.User, error)
	FindCredentialsByEmail(context.Context, string) (Credentials, error)
	FindUserByID(context.Context, string) (user.User, error)
}

type Tokens interface {
	Issue(string) (string, error)
	Verify(string) (string, error)
}

type Result struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	User        user.User `json:"user"`
}

type Service struct {
	repository Repository
	tokens     Tokens
	dummyHash  []byte
}

func New(repository Repository, tokens Tokens) (*Service, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), passwordCost)
	if err != nil {
		return nil, err
	}
	return &Service{repository: repository, tokens: tokens, dummyHash: dummy}, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return "", ErrInvalidInput
	}
	return email, nil
}

func validPassword(password string) bool { return len(password) >= 8 && len(password) <= 72 }

func (s *Service) Register(ctx context.Context, name, email, password string) (Result, error) {
	name = strings.TrimSpace(name)
	email, err := normalizeEmail(email)
	if err != nil || name == "" || !validPassword(password) {
		return Result{}, ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return Result{}, err
	}
	u, err := s.repository.CreateUser(ctx, name, email, string(hash))
	if err != nil {
		return Result{}, err
	}
	return s.result(u)
}

func (s *Service) Login(ctx context.Context, email, password string) (Result, error) {
	email, err := normalizeEmail(email)
	if err != nil || !validPassword(password) {
		return Result{}, ErrInvalidInput
	}
	credentials, err := s.repository.FindCredentialsByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Result{}, err
	}
	hash := []byte(credentials.PasswordHash)
	if len(hash) == 0 {
		hash = s.dummyHash
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if err != nil || credentials.PasswordHash == "" || !credentials.User.Active || passwordErr != nil {
		return Result{}, ErrUnauthorized
	}
	return s.result(credentials.User)
}

func (s *Service) Authenticate(ctx context.Context, token string) (user.User, error) {
	id, err := s.tokens.Verify(token)
	if err != nil {
		return user.User{}, ErrUnauthorized
	}
	u, err := s.repository.FindUserByID(ctx, id)
	if errors.Is(err, ErrNotFound) || err == nil && !u.Active {
		return user.User{}, ErrUnauthorized
	}
	return u, err
}

func (s *Service) result(u user.User) (Result, error) {
	token, err := s.tokens.Issue(u.ID)
	if err != nil {
		return Result{}, err
	}
	return Result{AccessToken: token, TokenType: "Bearer", ExpiresIn: int(TokenLifetime.Seconds()), User: u}, nil
}
