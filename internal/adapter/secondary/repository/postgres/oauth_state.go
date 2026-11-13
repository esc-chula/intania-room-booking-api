package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OAuthStates struct{ pool *pgxpool.Pool }

func NewOAuthStates(pool *pgxpool.Pool) *OAuthStates { return &OAuthStates{pool: pool} }

func (r *OAuthStates) Save(ctx context.Context, stateHash []byte, verifier string, expiresAt time.Time) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM oauth_states WHERE expires_at <= now()`); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO oauth_states (state_hash, verifier, expires_at) VALUES ($1, $2, $3)`, stateHash, verifier, expiresAt)
	return err
}

func (r *OAuthStates) Consume(ctx context.Context, stateHash []byte) (string, bool, error) {
	var verifier string
	err := r.pool.QueryRow(ctx, `DELETE FROM oauth_states WHERE state_hash = $1 AND expires_at > now() RETURNING verifier`, stateHash).Scan(&verifier)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return verifier, err == nil, err
}
