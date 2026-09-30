package main

import (
	"battery-storage-pandora/internal/api"
	"battery-storage-pandora/internal/database"
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
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is required; see .env.example")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	db, err := database.Open(ctx, url)
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
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	// Corporate identity/policy is deliberately not guessed. This adapter is
	// enabled explicitly for the local MVP; a card still identifies the actor.
	if os.Getenv("DEV_MODE") != "true" || os.Getenv("DEV_API_TOKEN") == "" {
		return fmt.Errorf("local MVP requires DEV_MODE=true and DEV_API_TOKEN; enterprise authentication is not configured")
	}
	auth := api.StaticTokens(map[string]api.Principal{os.Getenv("DEV_API_TOKEN"): {Scope: "local-mvp", Read: true, Manage: true, Command: true}})
	server := &http.Server{Addr: addr, Handler: api.NewWithAuth(db, auth), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
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
