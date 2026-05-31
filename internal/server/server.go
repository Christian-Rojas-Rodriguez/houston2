package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nomenclator/houston2/internal/auth"
	"github.com/nomenclator/houston2/internal/handlers"
	"github.com/nomenclator/houston2/internal/middleware"
)

// Config holds the server configuration.
type Config struct {
	SupabaseURL string
	AnonKey     string
	JWTSecret   string
	Port        string
	LogLevel    string
}

// LoadConfig reads configuration from environment variables with safe defaults.
func LoadConfig() Config {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	return Config{
		SupabaseURL: os.Getenv("SUPABASE_URL"),
		AnonKey:     os.Getenv("SUPABASE_ANON_KEY"),
		JWTSecret:   os.Getenv("SUPABASE_JWT_SECRET"),
		Port:        port,
		LogLevel:    logLevel,
	}
}

// Server is the HTTP server for the Houston 2.0 orchestrator.
type Server struct {
	cfg     Config
	handler http.Handler
	logger  *slog.Logger
}

// New creates a Server with the given config and TenantQuerier.
// q is injectable so tests can provide a stub without a real Supabase connection.
func New(cfg Config, q middleware.TenantQuerier) *Server {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	s := &Server{cfg: cfg, logger: logger}
	s.handler = s.buildHandler(cfg, q, logger)
	return s
}

// ServeHTTP implements http.Handler so *Server can be used directly with httptest.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Run starts the HTTP server and blocks until ctx is cancelled or SIGINT/SIGTERM received.
// Performs a 30-second graceful shutdown drain before returning.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    ":" + s.cfg.Port,
		Handler: s,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case <-sigCh:
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (s *Server) buildHandler(cfg Config, q middleware.TenantQuerier, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	authCfg := auth.Config{
		SupabaseURL: cfg.SupabaseURL,
		AnonKey:     cfg.AnonKey,
		JWTSecret:   cfg.JWTSecret,
		Port:        cfg.Port,
	}

	// Public routes — no auth required.
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("GET /auth/login", auth.LoginHandler(authCfg))
	mux.Handle("GET /auth/callback", auth.CallbackHandler(authCfg))

	// Base protected chain for routes that require only RoleMember.
	protected := auth.AuthMiddleware(authCfg)(
		middleware.TenantMiddleware(q)(
			middleware.RequireRole(middleware.RoleMember)(
				http.HandlerFunc(stubHandler),
			),
		),
	)

	// POST /v1/agents requires RoleManager and uses the real handler (Task 0007).
	agentStore := handlers.NewSupabaseAgentStore(cfg.SupabaseURL, cfg.AnonKey)
	agentProtected := auth.AuthMiddleware(authCfg)(
		middleware.TenantMiddleware(q)(
			middleware.RequireRole(middleware.RoleManager)(
				handlers.CreateAgent(agentStore, ""),
			),
		),
	)

	mux.Handle("POST /v1/agents", agentProtected)
	mux.Handle("GET /v1/agents/{id}", protected)
	mux.Handle("POST /v1/runs", protected)
	mux.Handle("GET /v1/runs/{id}", protected)

	// Wrap everything: RequestID sets X-Request-ID header first, then logging.
	return RequestIDMiddleware(LoggingMiddleware(logger, mux))
}
