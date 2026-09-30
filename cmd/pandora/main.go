package main

import (
	"battery-storage-pandora/internal/config"
	"battery-storage-pandora/internal/database"
	"battery-storage-pandora/internal/handler"
	"battery-storage-pandora/internal/repository"
	"battery-storage-pandora/internal/service"
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("pandora stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	db, err := database.Open(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	repo := repository.New(db)
	storage := service.New(repo)
	addr := cfg.HTTPAddress
	server := &http.Server{
		Addr:              addr,
		Handler:           handler.New(storage),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("listening", "address", addr, "swagger", "http://"+addr+"/swagger/")
	return server.ListenAndServe()
}
