package main

import (
	"battery-storage-pandora/internal/config"
	"battery-storage-pandora/internal/database"
	"battery-storage-pandora/internal/handler"
	"battery-storage-pandora/internal/repository"
	"battery-storage-pandora/internal/service"
	"battery-storage-pandora/migrations"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("pandora stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode != "serve" && mode != "migrate" {
		return fmt.Errorf("usage: pandora [serve|migrate]")
	}
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
	if mode == "migrate" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := migrations.Up(ctx, db); err != nil {
			return err
		}
		slog.Info("migrations applied")
		return nil
	}
	addr := cfg.HTTPAddress
	server := &http.Server{
		Addr:              addr,
		Handler:           handler.New(service.New(repository.New(db))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	stop, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()
	finished := make(chan error, 1)
	go func() {
		slog.Info("listening", "address", addr, "swagger", "http://"+addr+"/swagger/")
		finished <- server.ListenAndServe()
	}()
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}
