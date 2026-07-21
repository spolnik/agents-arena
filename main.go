package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jacek/agents-arena/internal/arena"
	"github.com/jacek/agents-arena/internal/store"
	"github.com/jacek/agents-arena/webui"
)

func main() {
	addr := flag.String("addr", defaultAddress(), "HTTP listen address")
	database := flag.String("db", defaultDatabasePath(), "SQLite database path")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	db, err := store.Open(*database)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.SeedExamples(); err != nil {
		logger.Error("seed example agents", "error", err)
		os.Exit(1)
	}

	manager := arena.NewManager(db, logger)
	handler, err := webui.NewWithBasicAuth(db, manager, logger, webui.BasicAuthConfig{
		Username: os.Getenv("ARENA_BASIC_AUTH_USERNAME"),
		Password: os.Getenv("ARENA_BASIC_AUTH_PASSWORD"),
	})
	if err != nil {
		logger.Error("create web server", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("agents arena listening", "address", *addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}
}

func defaultAddress() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

func defaultDatabasePath() string {
	if path := os.Getenv("ARENA_DATABASE_PATH"); path != "" {
		return path
	}
	if mount := os.Getenv("RAILWAY_VOLUME_MOUNT_PATH"); mount != "" {
		return filepath.Join(mount, "arena.db")
	}
	return "arena.db"
}
