package main

import (
	"context"
	"fmt"
	"os"

	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/infrastructure/database"
	"github.com/MadManJJ/room-reservation-api/internal/adapter/secondary/repository/postgres"
	"github.com/MadManJJ/room-reservation-api/internal/config"
	"github.com/MadManJJ/room-reservation-api/internal/core/domain/auth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap admin failed:", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := config.LoadBootstrapAdmin()
	if err != nil {
		return err
	}
	pool, err := database.Open(context.Background(), c)
	if err != nil {
		return err
	}
	defer pool.Close()
	created, err := auth.BootstrapAdmin(context.Background(), postgres.NewUsers(pool), c.BootstrapAdminEmail, c.GoogleAllowedDomain, c.GoogleAllowedSuffix)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintln(os.Stdout, "bootstrap administrator created")
		return nil
	}
	fmt.Fprintln(os.Stdout, "administrator already exists")
	return nil
}
