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
	"github.com/ckdash-git/MindSync-AI-Server/internal/handler"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/middleware"
	"github.com/ckdash-git/MindSync-AI-Server/internal/openrouter"
	"github.com/ckdash-git/MindSync-AI-Server/internal/privacy"
	"github.com/ckdash-git/MindSync-AI-Server/internal/repository/postgres"
	"github.com/ckdash-git/MindSync-AI-Server/internal/security"
	"github.com/ckdash-git/MindSync-AI-Server/internal/service"
	"github.com/go-chi/chi/v5"
)

func main() {
	// ── Load Configuration ─────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// ── Initialize Logger ──────────────────────────────────────────────
	log := logger.New(cfg.Log.Level)
	log.Info("starting MindSync AI Server",
		"env", cfg.Server.Env,
		"port", cfg.Server.Port,
	)

	// ── Initialize Database ────────────────────────────────────────────
	db, err := postgres.New(cfg.Database, log)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// ── Initialize Security ────────────────────────────────────────────
	jwtManager := security.NewJWTManager(
		cfg.JWT.Secret,
		cfg.JWT.AccessExpiry,
		cfg.JWT.RefreshExpiry,
	)

	// ── Initialize Repositories ────────────────────────────────────────
	userRepo := postgres.NewUserRepo(db)
	sessionRepo := postgres.NewSessionRepo(db)
	chatRepo := postgres.NewChatRepo(db)
	messageRepo := postgres.NewMessageRepo(db)

	// ── Initialize OpenRouter Client ───────────────────────────────────
	orClient := openrouter.NewClient(openrouter.ClientConfig{
		BaseURL:    cfg.OpenRouter.BaseURL,
		Timeout:    cfg.OpenRouter.Timeout,
		MaxRetries: cfg.OpenRouter.MaxRetries,
	}, log)

	// ── Initialize Privacy Processor ───────────────────────────────────
	privacyProc := privacy.NewProcessor(privacy.PrivacyMode(cfg.Privacy.Mode), log)

	// ── Initialize Services ────────────────────────────────────────────
	authService := service.NewAuthService(userRepo, sessionRepo, jwtManager, log)
	chatService := service.NewChatService(chatRepo, messageRepo, orClient, privacyProc, log)
	streamService := service.NewStreamService(orClient, privacyProc, log)

	// ── Initialize Middleware ──────────────────────────────────────────
	authMiddleware := middleware.NewAuthMiddleware(jwtManager, log)
	middleware.SetAuthMiddleware(authMiddleware)

	rateLimiter := middleware.NewRateLimiter(middleware.RateLimitConfig{
		AuthRPM:   cfg.RateLimit.AuthRPM,
		ChatRPM:   cfg.RateLimit.ChatRPM,
		StreamRPM: cfg.RateLimit.StreamRPM,
	})

	// ── Initialize Handlers ────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authService, log)
	chatHandler := handler.NewChatHandler(chatService, log)
	streamHandler := handler.NewStreamHandler(streamService, cfg.OpenRouter.DefaultModel, log)
	facadeHandler := handler.NewFacadeHandler(chatService, cfg.OpenRouter.DefaultModel, rateLimiter, log)

	// ── Initialize Router ──────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware (applied to all routes)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins: cfg.CORS.AllowedOrigins,
		AllowedMethods: cfg.CORS.AllowedMethods,
		AllowedHeaders: cfg.CORS.AllowedHeaders,
	}))

	// Health check — no middleware beyond global
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		dbErr := db.HealthCheck(r.Context())
		status := "healthy"
		if dbErr != nil {
			status = "degraded"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":%q,"service":"mindsync-ai-server"}`, status)
	})

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes (with auth-specific rate limiting)
		r.Group(func(r chi.Router) {
			r.Use(rateLimiter.Handler("auth"))
			r.Use(middleware.Timeout(cfg.Timeout.Default))
			authHandler.RegisterRoutes(r)
		})

		// Chat routes (with chat-specific rate limiting)
		r.Group(func(r chi.Router) {
			r.Use(rateLimiter.Handler("chat"))
			r.Use(middleware.Timeout(cfg.Timeout.Default))
			chatHandler.RegisterRoutes(r)
		})

		// Streaming routes (with stream-specific rate limiting and timeout)
		r.Group(func(r chi.Router) {
			r.Use(rateLimiter.Handler("stream"))
			r.Use(middleware.StreamTimeout(cfg.Timeout.Stream))
			streamHandler.RegisterRoutes(r)
		})

		// Façade routes (simplified client-facing endpoints)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(cfg.Timeout.Default))
			facadeHandler.RegisterRoutes(r)
		})
	})

	// ── Initialize HTTP Server ─────────────────────────────────────────
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout + cfg.Timeout.Stream, // Account for streaming
		IdleTimeout:  2 * time.Minute,
	}

	// ── Graceful Shutdown ──────────────────────────────────────────────
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case sig := <-shutdownCh:
		log.Info("received shutdown signal", "signal", sig.String())
	case err := <-errCh:
		log.Error("server error", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	log.Info("shutting down server", "timeout", cfg.Server.ShutdownTimeout)
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	log.Info("server stopped gracefully")
}
