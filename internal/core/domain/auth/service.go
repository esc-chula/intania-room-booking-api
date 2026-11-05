package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

const TokenLifetime = time.Hour

var (
	ErrInvalidInput              = errors.New("invalid input")
	ErrIneligible                = errors.New("Google account is not eligible")
	ErrIdentityConflict          = errors.New("email and student ID belong to different users")
	ErrNotFound                  = errors.New("user not found")
	ErrUnauthorized              = errors.New("invalid credentials or token")
	ErrBootstrapAdminEmailExists = errors.New("bootstrap admin email belongs to a non-admin user")
)

type Repository interface {
	ResolveGoogleUser(context.Context, string, string, string, string) (user.User, error)
	CreateAdmin(context.Context, string) (user.User, error)
	FindUserByID(context.Context, string) (user.User, error)
	FindAdmin(context.Context) (user.User, error)
}

type Tokens interface {
	Issue(string) (string, error)
	Verify(string) (string, error)
}

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

type Service struct {
	repository    Repository
	tokens        Tokens
	allowedDomain string
	allowedSuffix string
}

func New(repository Repository, tokens Tokens, allowedDomain, allowedSuffix string) *Service {
	return &Service{repository: repository, tokens: tokens, allowedDomain: allowedDomain, allowedSuffix: allowedSuffix}
}

func EligibleGoogleEmail(email, hostedDomain string, verified bool, allowedDomain, allowedSuffix string) bool {
	if !verified || !strings.EqualFold(hostedDomain, allowedDomain) {
		return false
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || !strings.EqualFold(domain, allowedDomain) || !strings.HasSuffix(local, allowedSuffix) {
		return false
	}
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email && len(email) <= 254
}

func (s *Service) SignInGoogle(ctx context.Context, identity GoogleIdentity) (Result, error) {
	if identity.Subject == "" || !EligibleGoogleEmail(identity.Email, identity.HostedDomain, identity.EmailVerified, s.allowedDomain, s.allowedSuffix) {
		return Result{}, ErrIneligible
	}
	email := strings.ToLower(identity.Email)
	local, _, _ := strings.Cut(email, "@")
	studentID := ""
	if numericStudentID(local) {
		studentID = local
	}
	u, err := s.repository.ResolveGoogleUser(ctx, email, studentID, strings.TrimSpace(identity.FirstName), strings.TrimSpace(identity.LastName))
	if err != nil {
		return Result{}, err
	}
	if !u.Active {
		return Result{}, ErrUnauthorized
	}
	return s.result(u)
}

func numericStudentID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func BootstrapAdmin(ctx context.Context, repository Repository, email, allowedDomain, allowedSuffix string) (bool, error) {
	if _, err := repository.FindAdmin(ctx); err == nil {
		return false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return false, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !EligibleGoogleEmail(email, allowedDomain, true, allowedDomain, allowedSuffix) {
		return false, ErrInvalidInput
	}
	if _, err := repository.CreateAdmin(ctx, email); err != nil {
		if errors.Is(err, ErrIdentityConflict) {
			if _, findErr := repository.FindAdmin(ctx); findErr == nil {
				return false, nil
			}
			return false, ErrBootstrapAdminEmailExists
		}
		return false, err
	}
	return true, nil
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
