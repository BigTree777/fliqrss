package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fliqrss/backend/internal/api"
	"fliqrss/backend/internal/store"
)

func main() {
	addr := environment("ADDR", ":8080")
	allowedOrigin := environment("CORS_ORIGIN", "http://localhost:5173")
	repository, closeRepository, err := openRepository()
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer closeRepository()

	application := api.NewServer(repository, allowedOrigin)
	server := &http.Server{
		Addr:              addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      6 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("server shutdown failed", "error", err)
		}
	}()

	slog.Info("backend listening", "address", addr, "cors_origin", allowedOrigin)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func openRepository() (store.Repository, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Warn("DATABASE_URL is not set, using non-persistent in-memory storage")
		return store.NewMemory(), func() {}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, err := store.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	slog.Info("connected to PostgreSQL")
	return repository, func() {
		if err := repository.Close(); err != nil {
			slog.Error("database close failed", "error", err)
		}
	}, nil
}

func environment(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
