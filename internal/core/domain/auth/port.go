package auth

import (
	"context"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
)

// Service is the inbound port used by authentication HTTP adapters.
type Service interface {
	SignInGoogle(context.Context, GoogleIdentity) (Result, error)
	Authenticate(context.Context, string) (user.User, error)
}

type UserRepository interface {
	ResolveGoogleUser(context.Context, string, string, string, string) (user.User, error)
	FindUserByID(context.Context, string) (user.User, error)
}

type AdminBootstrapRepository interface {
	CreateAdmin(context.Context, string) (user.User, error)
	FindAdmin(context.Context) (user.User, error)
}

type OSBootstrapRepository interface {
	SeedOSUsers(context.Context, []string) (int, error)
}

type Tokens interface {
	Issue(string) (string, error)
	Verify(string) (string, error)
}
