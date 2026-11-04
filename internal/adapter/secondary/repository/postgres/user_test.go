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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests use an isolated schema, never the application's users table.
// Opt in with TEST_DATABASE_URL pointing to a disposable local PostgreSQL DB.
func testPool(t *testing.T, legacySchema bool) *pgxpool.Pool {
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
	initialSchema := migration(t, "00001_init_core_schema.sql", "Up")
	if legacySchema {
		initialSchema = strings.Replace(initialSchema, ", 'ADMIN'", "", 1)
	}
	if _, err := pool.Exec(ctx, initialSchema); err != nil {
		t.Fatal(err)
	}
	return pool
}

func migration(t *testing.T, filename, direction string) string {
	t.Helper()
	data, err := os.ReadFile("../../../../../db/migrations/" + filename)
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
	pool := testPool(t, false)
	ctx := context.Background()
	var legacyID string
	if err := pool.QueryRow(ctx, `INSERT INTO users (id, email, active) VALUES (gen_random_uuid(), ' LEGACY@Example.com ', NULL) RETURNING id::text`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	up := migration(t, "00002_add_password_auth.sql", "Up")
	down := migration(t, "00002_add_password_auth.sql", "Down")
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	repo := NewUsers(pool)
	legacy, err := repo.FindCredentialsByEmail(ctx, "legacy@example.com")
	if err != nil || legacy.User.ID != legacyID || legacy.User.Active || legacy.PasswordHash != "" || legacy.User.Name != "" {
		t.Fatalf("legacy user: %+v %v", legacy, err)
	}
	if _, err := repo.CreateUser(ctx, "Duplicate", "legacy@example.com", "hash"); !errors.Is(err, auth.ErrEmailExists) {
		t.Fatalf("legacy normalized duplicate: %v", err)
	}
	u, err := repo.CreateUser(ctx, "Test User", "test@example.com", "test-hash")
	if err != nil || u.Role != "USER_A" || !u.Active || u.ID == "" {
		t.Fatalf("create: %+v %v", u, err)
	}
	c, err := repo.FindCredentialsByEmail(ctx, u.Email)
	if err != nil || c.User != u || c.PasswordHash != "test-hash" {
		t.Fatalf("credentials: %+v %v", c, err)
	}
	if got, err := repo.FindUserByID(ctx, u.ID); err != nil || got != u {
		t.Fatalf("find: %+v %v", got, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET active = false, role = 'OS' WHERE id = $1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.FindUserByID(ctx, u.ID); err != nil || got.Active || got.Role != "OS" {
		t.Fatalf("updated user: %+v %v", got, err)
	}
	if _, err := repo.FindCredentialsByEmail(ctx, "missing@example.com"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := repo.FindUserByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatal(err)
	}
	var missingTimestamps bool
	if err := pool.QueryRow(ctx, `SELECT created_at IS NULL OR updated_at IS NULL FROM users WHERE id = $1::uuid`, u.ID).Scan(&missingTimestamps); err != nil || missingTimestamps {
		t.Fatalf("timestamps: %v %v", missingTimestamps, err)
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			<-start
			_, err := repo.CreateUser(ctx, "Concurrent", "concurrent@example.com", "hash")
			results <- err
		})
	}
	close(start)
	group.Wait()
	close(results)
	successes, duplicates := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, auth.ErrEmailExists) {
			duplicates++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent registration: %d successes, %d duplicates", successes, duplicates)
	}
	if _, err := pool.Exec(ctx, down); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("Down removed users: %d %v", count, err)
	}
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	if legacy, err := repo.FindCredentialsByEmail(ctx, "legacy@example.com"); err != nil || legacy.PasswordHash != "" {
		t.Fatalf("reapplied migration: %+v %v", legacy, err)
	}
}

func TestNormalizedEmailCollisionMigration(t *testing.T) {
	pool := testPool(t, false)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES (gen_random_uuid(), 'Test@example.com'), (gen_random_uuid(), 'test@example.com')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, migration(t, "00002_add_password_auth.sql", "Up")); err == nil {
		t.Fatal("normalized-email collision accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("collision changed users: %d %v", count, err)
	}
}

func TestAdminRoleMigrations(t *testing.T) {
	t.Run("fresh schema includes admin", func(t *testing.T) {
		pool := testPool(t, false)
		var role string
		if err := pool.QueryRow(context.Background(), `SELECT 'ADMIN'::user_role::text`).Scan(&role); err != nil || role != "ADMIN" {
			t.Fatalf("admin role missing: %q %v", role, err)
		}
	})
	t.Run("upgrade adds admin", func(t *testing.T) {
		pool := testPool(t, true)
		ctx := context.Background()
		if _, err := pool.Exec(ctx, migration(t, "00002_add_password_auth.sql", "Up")); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, migration(t, "00003_add_admin_role.sql", "Up")); err != nil {
			t.Fatal(err)
		}
		repo := NewUsers(pool)
		created, err := repo.CreateAdmin(ctx, "Admin", auth.BootstrapAdminEmail, "hash")
		if err != nil || created.Role != "ADMIN" || !created.Active {
			t.Fatalf("create admin: %+v %v", created, err)
		}
		if found, err := repo.FindAdmin(ctx); err != nil || found != created {
			t.Fatalf("find admin: %+v %v", found, err)
		}
		if _, err := pool.Exec(ctx, migration(t, "00003_add_admin_role.sql", "Down")); err != nil {
			t.Fatalf("irreversible down migration should be a no-op: %v", err)
		}
	})
}
