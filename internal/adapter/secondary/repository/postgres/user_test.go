package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/user"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests use a temporary schema and never the application's tables.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("auth_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
		_ = admin.Close(ctx)
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, migration(t, "Up")); err != nil {
		t.Fatal(err)
	}
	return pool
}

func migration(t *testing.T, direction string) string {
	t.Helper()
	data, err := os.ReadFile("../../../../../db/migrations/00001_init_core_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, found := strings.Cut(string(data), "-- +goose Down")
	if !found {
		t.Fatal("missing Down migration")
	}
	if direction == "Down" {
		return down
	}
	return strings.TrimPrefix(up, "-- +goose Up")
}

func TestUsersIntegration(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewUsers(pool)
	u, err := repo.ResolveGoogleUser(ctx, "6500000021@student.chula.ac.th", "6500000021", "First", "Last")
	if err != nil || u.Role != user.RoleUserA || u.FirstName != "First" || u.LastName != "Last" || !u.Active {
		t.Fatalf("create Google user: %+v %v", u, err)
	}
	again, err := repo.ResolveGoogleUser(ctx, u.Email, "6500000021", "Changed", "Changed")
	if err != nil || again != u {
		t.Fatalf("repeat login changed user: %+v %v", again, err)
	}
	if _, err := repo.ResolveGoogleUser(ctx, u.Email, "9999999921", "", ""); !errors.Is(err, auth.ErrIdentityConflict) {
		t.Fatalf("mismatched existing student ID accepted: %v", err)
	}
	if found, err := repo.FindUserByID(ctx, u.ID); err != nil || found != u {
		t.Fatalf("find user: %+v %v", found, err)
	}
	if _, err := repo.ResolveGoogleUser(ctx, "john21@student.chula.ac.th", "", "John", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, role) VALUES (gen_random_uuid(), 'DUPLICATE@STUDENT.CHULA.AC.TH', 'USER_A')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, role) VALUES (gen_random_uuid(), 'duplicate@student.chula.ac.th', 'USER_A')`); err == nil {
		t.Fatal("normalized-email duplicate accepted")
	}
	if _, err := repo.FindUserByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
}

func TestPreloadedOSAndConflicts(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewUsers(pool)
	changed, err := repo.SeedOSUsers(ctx, []string{"6730000021"})
	if err != nil || changed != 1 {
		t.Fatalf("seed: %d %v", changed, err)
	}
	var preloadedID string
	var email *string
	if err := pool.QueryRow(ctx, `SELECT id::text, email FROM users WHERE student_id = '6730000021'`).Scan(&preloadedID, &email); err != nil || email != nil {
		t.Fatalf("preloaded profile is not null: %v %v", email, err)
	}
	u, err := repo.ResolveGoogleUser(ctx, "6730000021@student.chula.ac.th", "6730000021", "OS", "")
	if err != nil || u.ID != preloadedID || u.Role != user.RoleOS || u.FirstName != "OS" || u.LastName != "" || u.Email != "6730000021@student.chula.ac.th" {
		t.Fatalf("OS login: %+v %v", u, err)
	}
	u, err = repo.ResolveGoogleUser(ctx, u.Email, u.StudentID, "Changed", "Student")
	if err != nil || u.ID != preloadedID || u.Role != user.RoleOS || u.FirstName != "OS" || u.LastName != "Student" {
		t.Fatalf("later OS profile fill: %+v %v", u, err)
	}
	if changed, err := repo.SeedOSUsers(ctx, []string{"6730000021"}); err != nil || changed != 0 {
		t.Fatalf("repeat OS seed: %d %v", changed, err)
	}
	again, err := repo.ResolveGoogleUser(ctx, u.Email, u.StudentID, "Changed", "Changed")
	if err != nil || again != u {
		t.Fatalf("existing OS profile changed: %+v %v", again, err)
	}
	if changed, err := repo.SeedOSUsers(ctx, []string{"6740000021"}); err != nil || changed != 1 {
		t.Fatalf("second OS seed: %d %v", changed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET first_name = 'Existing' WHERE student_id = '6740000021'`); err != nil {
		t.Fatal(err)
	}
	partial, err := repo.ResolveGoogleUser(ctx, "6740000021@student.chula.ac.th", "6740000021", "Google", "Surname")
	if err != nil || partial.Role != user.RoleOS || partial.FirstName != "Existing" || partial.LastName != "Surname" {
		t.Fatalf("preloaded OS name overwritten: %+v %v", partial, err)
	}
	if _, err := repo.ResolveGoogleUser(ctx, "other21@student.chula.ac.th", "6730000021", "", ""); !errors.Is(err, auth.ErrIdentityConflict) {
		t.Fatalf("student ID mismatch accepted: %v", err)
	}
	other, err := repo.ResolveGoogleUser(ctx, "another21@student.chula.ac.th", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveGoogleUser(ctx, other.Email, "6730000021", "", ""); !errors.Is(err, auth.ErrIdentityConflict) {
		t.Fatalf("email/student split accepted: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET active = false WHERE id = $1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	if disabled, err := repo.FindUserByID(ctx, u.ID); err != nil || disabled.Active {
		t.Fatalf("disabled user: %+v %v", disabled, err)
	}
}

func TestOSBootstrapAndAdmin(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewUsers(pool)
	admin, err := repo.CreateAdmin(ctx, "6500000021@student.chula.ac.th")
	if err != nil || admin.Role != user.RoleAdmin {
		t.Fatalf("admin create: %+v %v", admin, err)
	}
	if found, err := repo.FindAdmin(ctx); err != nil || found != admin {
		t.Fatalf("find admin: %+v %v", found, err)
	}
	if _, err := repo.CreateAdmin(ctx, admin.Email); !errors.Is(err, auth.ErrIdentityConflict) {
		t.Fatalf("duplicate admin accepted: %v", err)
	}
	member, err := repo.ResolveGoogleUser(ctx, "6720000021@student.chula.ac.th", "6720000021", "", "")
	if err != nil || member.Role != user.RoleUserA {
		t.Fatalf("create member: %+v %v", member, err)
	}
	if changed, err := repo.SeedOSUsers(ctx, []string{"6500000021", "6720000021", "6730000021"}); err != nil || changed != 2 {
		t.Fatalf("OS seed: %d %v", changed, err)
	}
	if changed, err := repo.SeedOSUsers(ctx, []string{"6500000021", "6720000021", "6730000021"}); err != nil || changed != 0 {
		t.Fatalf("OS seed not idempotent: %d %v", changed, err)
	}
	if got, err := repo.FindUserByID(ctx, admin.ID); err != nil || got.Role != user.RoleAdmin {
		t.Fatalf("admin demoted: %+v %v", got, err)
	}
	if got, err := repo.FindUserByID(ctx, member.ID); err != nil || got.Role != user.RoleOS {
		t.Fatalf("member not promoted: %+v %v", got, err)
	}
}

func TestConcurrentFirstSignIn(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewUsers(pool)
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			<-start
			_, err := repo.ResolveGoogleUser(ctx, "6500000021@student.chula.ac.th", "6500000021", "First", "Last")
			results <- err
		})
	}
	close(start)
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent sign-in: %v", err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("user count: %d %v", count, err)
	}
}

func TestInitialMigrationDown(t *testing.T) {
	pool := testPool(t)
	if _, err := pool.Exec(context.Background(), migration(t, "Down")); err != nil {
		t.Fatalf("migration down: %v", err)
	}
}
