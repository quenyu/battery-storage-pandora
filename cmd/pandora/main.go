package main

import (
	"battery-storage-pandora/internal/config"
	"battery-storage-pandora/internal/database"
	"battery-storage-pandora/internal/handler"
	"battery-storage-pandora/internal/repository"
	"battery-storage-pandora/internal/service"
	"context"
	"errors"
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	db, err := database.Open(connectCtx, cfg.DatabaseURL)
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
	return runHTTPServer(ctx, server)
}

func runHTTPServer(ctx context.Context, server *http.Server) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
		slog.Info("stopping server, waiting for active requests")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		server.Close()
		return err
	}
	if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	slog.Info("server stopped")
	return nil
}
