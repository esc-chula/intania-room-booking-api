package main

import (
	"context"
	"fmt"
	"os"

	"example.com/chula-room-booking/internal/adapter/secondary/infrastructure/database"
	"example.com/chula-room-booking/internal/adapter/secondary/repository/postgres"
	"example.com/chula-room-booking/internal/config"
	"example.com/chula-room-booking/internal/core/domain/auth"
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
	created, err := auth.BootstrapAdmin(context.Background(), postgres.NewUsers(pool), c.BootstrapAdminName, c.BootstrapAdminPassword)
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
