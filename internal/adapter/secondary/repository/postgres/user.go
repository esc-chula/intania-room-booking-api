package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

const userColumns = `id::text, COALESCE(first_name, ''), COALESCE(last_name, ''), COALESCE(email, ''), COALESCE(student_id, ''), role::text, COALESCE(active, false)`

func scanUser(row pgx.Row) (user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Email, &u.StudentID, &u.Role, &u.Active)
	return u, err
}

func (r *Users) ResolveGoogleUser(ctx context.Context, email, studentID, firstName, lastName string) (user.User, error) {
	for attempt := 0; attempt < 3; attempt++ {
		u, err := r.resolveGoogleUser(ctx, email, studentID, firstName, lastName)
		if !uniqueViolation(err) {
			return u, err
		}
	}
	return user.User{}, auth.ErrIdentityConflict
}

func (r *Users) resolveGoogleUser(ctx context.Context, email, studentID, firstName, lastName string) (user.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return user.User{}, err
	}
	defer tx.Rollback(ctx)
	byEmail, emailErr := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(btrim(email)) = $1`, email))
	if emailErr != nil && !errors.Is(emailErr, pgx.ErrNoRows) {
		return user.User{}, emailErr
	}
	if emailErr == nil && studentID != "" && byEmail.StudentID != "" && byEmail.StudentID != studentID {
		return user.User{}, auth.ErrIdentityConflict
	}
	var byStudent user.User
	studentErr := pgx.ErrNoRows
	if studentID != "" {
		byStudent, studentErr = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE student_id = $1 FOR UPDATE`, studentID))
		if studentErr != nil && !errors.Is(studentErr, pgx.ErrNoRows) {
			return user.User{}, studentErr
		}
	}
	if emailErr == nil && studentErr == nil && byEmail.ID != byStudent.ID {
		return user.User{}, auth.ErrIdentityConflict
	}
	var u user.User
	switch {
	case emailErr == nil:
		u = byEmail // Existing email is deliberately never updated.
	case studentErr == nil:
		if byStudent.Email != "" {
			if !strings.EqualFold(byStudent.Email, email) {
				return user.User{}, auth.ErrIdentityConflict
			}
			u = byStudent // A concurrent login may have filled this row after the email lookup.
		} else {
			u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET first_name = $2, last_name = $3, email = $4,
				updated_at = CURRENT_TIMESTAMP AT TIME ZONE 'UTC' WHERE id = $1::uuid RETURNING `+userColumns,
				byStudent.ID, nullable(firstName), nullable(lastName), email))
		}
	case studentID != "":
		u, err = scanUser(tx.QueryRow(ctx, `INSERT INTO users
			(id, student_id, first_name, last_name, email, role, active, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, 'USER_A', true, CURRENT_TIMESTAMP AT TIME ZONE 'UTC', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
			RETURNING `+userColumns, studentID, nullable(firstName), nullable(lastName), email))
	default:
		u, err = scanUser(tx.QueryRow(ctx, `INSERT INTO users
			(id, first_name, last_name, email, role, active, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, 'USER_A', true, CURRENT_TIMESTAMP AT TIME ZONE 'UTC', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
			RETURNING `+userColumns, nullable(firstName), nullable(lastName), email))
	}
	if err != nil {
		return user.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return user.User{}, err
	}
	return u, nil
}

func (r *Users) CreateAdmin(ctx context.Context, email string) (user.User, error) {
	local, _, _ := strings.Cut(email, "@")
	var studentID any
	if numeric(local) {
		studentID = local
	}
	u, err := scanUser(r.pool.QueryRow(ctx, `INSERT INTO users
		(id, student_id, first_name, email, role, active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'Admin', $2, 'ADMIN', true, CURRENT_TIMESTAMP AT TIME ZONE 'UTC', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		RETURNING `+userColumns, studentID, email))
	if uniqueViolation(err) {
		return user.User{}, auth.ErrIdentityConflict
	}
	return u, err
}

func (r *Users) FindAdmin(ctx context.Context) (user.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE role = 'ADMIN' ORDER BY created_at NULLS LAST, id LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, auth.ErrNotFound
	}
	return u, err
}

func (r *Users) FindUserByID(ctx context.Context, id string) (user.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, auth.ErrNotFound
	}
	return u, err
}

func (r *Users) SeedOSUsers(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	changed := 0
	for _, id := range ids {
		// Existing accounts created through OAuth already carry numeric student IDs.
		tag, err := tx.Exec(ctx, `INSERT INTO users
			(id, student_id, role, active, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, 'OS', true, CURRENT_TIMESTAMP AT TIME ZONE 'UTC', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
			ON CONFLICT (student_id) DO UPDATE SET role = 'OS', updated_at = CURRENT_TIMESTAMP AT TIME ZONE 'UTC'
			WHERE users.role NOT IN ('OS', 'ADMIN')`, id)
		if err != nil {
			return 0, err
		}
		changed += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return changed, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
