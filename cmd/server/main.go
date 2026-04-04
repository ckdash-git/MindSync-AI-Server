package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/internal/config"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/go-chi/chi/v5"
)

func main() {
	// ── Load Configuration ──────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// ── Initialize Logger ───────────────────────────────────────────
	log := logger.New(cfg.Log.Level)
	log.Info("starting MindSync AI Server",
		"env", cfg.Server.Env,
		"port", cfg.Server.Port,
	)

	// ── Initialize Router ───────────────────────────────────────────
	r := chi.NewRouter()

	// Health check — always available, no middleware
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"mindsync-ai-server"}`))
	})

	// API v1 routes (will be expanded in subsequent phases)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":"pong"}`))
		})
	})

	// ── Initialize HTTP Server ──────────────────────────────────────
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  2 * time.Minute,
	}

	// ── Graceful Shutdown ───────────────────────────────────────────
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-shutdownCh:
		log.Info("received shutdown signal", "signal", sig.String())
	case err := <-errCh:
		log.Error("server error", "error", err)
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	log.Info("shutting down server", "timeout", cfg.Server.ShutdownTimeout)
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	log.Info("server stopped gracefully")
}
