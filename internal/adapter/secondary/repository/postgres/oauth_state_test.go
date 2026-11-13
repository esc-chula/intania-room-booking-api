package postgres

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuthStatesIntegration(t *testing.T) {
	pool := testPool(t)
	data, err := os.ReadFile("../../../../../db/migrations/00002_oauth_states.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, _, ok := strings.Cut(string(data), "-- +goose Down")
	if !ok {
		t.Fatal("missing Down migration")
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx, strings.TrimPrefix(up, "-- +goose Up")); err != nil {
		t.Fatal(err)
	}
	first, second := NewOAuthStates(pool), NewOAuthStates(pool)
	hash := bytes.Repeat([]byte{1}, 32)
	if err := first.Save(ctx, hash, "first-verifier", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if verifier, found, err := second.Consume(ctx, hash); err != nil || !found || verifier != "first-verifier" {
		t.Fatalf("consume from another repository: %q %t %v", verifier, found, err)
	}
	if _, found, err := first.Consume(ctx, hash); err != nil || found {
		t.Fatalf("replayed state: found=%t err=%v", found, err)
	}
	expired := bytes.Repeat([]byte{2}, 32)
	if err := first.Save(ctx, expired, "expired", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, found, err := second.Consume(ctx, expired); err != nil || found {
		t.Fatalf("expired state: found=%t err=%v", found, err)
	}
	if err := first.Save(ctx, hash, "concurrent", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var expiredRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM oauth_states WHERE state_hash = $1`, expired).Scan(&expiredRows); err != nil || expiredRows != 0 {
		t.Fatalf("expired state cleanup: rows=%d err=%v", expiredRows, err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			verifier, found, err := second.Consume(ctx, hash)
			if err != nil {
				t.Errorf("concurrent consume: %v", err)
			} else if found {
				if verifier != "concurrent" {
					t.Errorf("verifier = %q", verifier)
				}
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := winners.Load(); got != 1 {
		t.Fatalf("concurrent consumers = %d, want 1", got)
	}
}
