package database

import (
	"context"
	"errors"

	"github.com/MadManJJ/room-reservation-api/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, c config.Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL: check PostgreSQL connection settings")
	}
	pc.MaxConns = c.DBMaxConns
	pc.MinConns = c.DBMinConns
	pc.ConnConfig.ConnectTimeout = c.DBConnectTimeout
	startup, cancel := context.WithTimeout(ctx, c.DBConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(startup, pc)
	if err != nil {
		return nil, errors.New("cannot create PostgreSQL pool: check connection settings")
	}
	if err := pool.Ping(startup); err != nil {
		pool.Close()
		return nil, errors.New("PostgreSQL startup ping failed: check availability, credentials, and TLS settings")
	}
	return pool, nil
}
