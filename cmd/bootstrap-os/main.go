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
		fmt.Fprintln(os.Stderr, "bootstrap OS failed:", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := config.LoadBootstrapOS()
	if err != nil {
		return err
	}
	ids, err := auth.ParseOSStudentIDs(c.OSStudentIDs, c.GoogleAllowedSuffix)
	if err != nil {
		return fmt.Errorf("OS_STUDENT_IDS must be comma-separated numeric student IDs ending in %s", c.GoogleAllowedSuffix)
	}
	if len(ids) == 0 {
		fmt.Fprintln(os.Stdout, "OS_STUDENT_IDS is empty; nothing to add")
		return nil
	}
	pool, err := database.Open(context.Background(), c)
	if err != nil {
		return err
	}
	defer pool.Close()
	changed, err := postgres.NewUsers(pool).SeedOSUsers(context.Background(), ids)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "OS bootstrap complete: %d rows inserted or promoted\n", changed)
	return nil
}
