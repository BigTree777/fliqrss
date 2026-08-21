package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fliqrss/backend/internal/api"
	"fliqrss/backend/internal/collector"
	"fliqrss/backend/internal/feed"
	"fliqrss/backend/internal/store"
)

func main() {
	addr := environment("ADDR", ":8080")
	allowedOrigin := environment("CORS_ORIGIN", "http://localhost:5173")
	refreshInterval, err := durationEnvironment("FEED_REFRESH_INTERVAL", 15*time.Minute)
	if err != nil {
		slog.Error("invalid feed refresh interval", "error", err)
		os.Exit(1)
	}
	repository, closeRepository, err := openRepository()
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer closeRepository()

	feedLoader := feed.NewClient()
	application := api.NewServerWithFeedLoader(repository, allowedOrigin, feedLoader)
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
	collectionDone := make(chan struct{})
	go func() {
		defer close(collectionDone)
		collector.New(repository, feedLoader, collector.DefaultConcurrency, slog.Default()).Run(shutdownContext, refreshInterval)
	}()

	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("server shutdown failed", "error", err)
		}
	}()

	slog.Info("backend listening", "address", addr, "cors_origin", allowedOrigin)
	err = server.ListenAndServe()
	stop()
	<-collectionDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func durationEnvironment(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 15m: %w", key, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return duration, nil
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
