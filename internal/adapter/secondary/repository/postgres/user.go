package postgres

import (
	"context"
	"errors"

	"example.com/chula-room-booking/internal/core/domain/auth"
	"example.com/chula-room-booking/internal/core/domain/user"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

func (r *Users) CreateUser(ctx context.Context, name, email, hash string) (user.User, error) {
	return r.createUser(ctx, name, email, hash, user.RoleUserA)
}

func (r *Users) CreateAdmin(ctx context.Context, name, email, hash string) (user.User, error) {
	return r.createUser(ctx, name, email, hash, user.RoleAdmin)
}

func (r *Users) createUser(ctx context.Context, name, email, hash, role string) (user.User, error) {
	var u user.User
	err := r.pool.QueryRow(ctx, `INSERT INTO users
		(id, name, email, password_hash, role, active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4::user_role, true, CURRENT_TIMESTAMP AT TIME ZONE 'UTC', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		RETURNING id::text, name, email, role::text, active`, name, email, hash, role).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && (pgErr.ConstraintName == "users_email_key" || pgErr.ConstraintName == "users_email_normalized_idx") {
		return user.User{}, auth.ErrEmailExists
	}
	return u, err
}

func (r *Users) FindAdmin(ctx context.Context) (user.User, error) {
	var u user.User
	err := r.pool.QueryRow(ctx, `SELECT id::text, COALESCE(name, ''), COALESCE(email, ''), role::text,
		COALESCE(active, false) FROM users WHERE role = 'ADMIN' ORDER BY created_at NULLS LAST, id LIMIT 1`).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, auth.ErrNotFound
	}
	return u, err
}

func (r *Users) FindCredentialsByEmail(ctx context.Context, email string) (auth.Credentials, error) {
	var c auth.Credentials
	err := r.pool.QueryRow(ctx, `SELECT id::text, COALESCE(name, ''), COALESCE(email, ''), role::text,
		COALESCE(active, false), COALESCE(password_hash, '') FROM users WHERE lower(btrim(email)) = $1`, email).
		Scan(&c.User.ID, &c.User.Name, &c.User.Email, &c.User.Role, &c.User.Active, &c.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Credentials{}, auth.ErrNotFound
	}
	return c, err
}

func (r *Users) FindUserByID(ctx context.Context, id string) (user.User, error) {
	var u user.User
	err := r.pool.QueryRow(ctx, `SELECT id::text, COALESCE(name, ''), COALESCE(email, ''), role::text,
		COALESCE(active, false) FROM users WHERE id = $1::uuid`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, auth.ErrNotFound
	}
	return u, err
}
