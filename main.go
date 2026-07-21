package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jacek/agents-arena/internal/arena"
	"github.com/jacek/agents-arena/internal/store"
	"github.com/jacek/agents-arena/webui"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	database := flag.String("db", "arena.db", "SQLite database path")
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
	handler, err := webui.New(db, manager, logger)
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
