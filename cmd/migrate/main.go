package main

import (
	"battery-storage-pandora/internal/config"
	"battery-storage-pandora/internal/database"
	"battery-storage-pandora/migrations"
	"context"
	"log/slog"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := migrations.Up(ctx, db); err != nil {
		return err
	}
	slog.Info("migrations applied")
	return nil
}
