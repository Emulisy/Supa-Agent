// Command server assembles one API process with shared infrastructure.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Emulisy/Supa-Agent/internal/agent"
	"github.com/Emulisy/Supa-Agent/internal/api"
	"github.com/Emulisy/Supa-Agent/internal/config"
	"github.com/Emulisy/Supa-Agent/internal/infra"
	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/Emulisy/Supa-Agent/internal/repo"
	"github.com/Emulisy/Supa-Agent/internal/security"
	"github.com/Emulisy/Supa-Agent/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadServer()
	if err != nil {
		return err
	}
	llmConfig, err := llm.LoadConfigFile(cfg.LLMPath)
	if err != nil {
		return err
	}
	tokens, err := security.NewJWTManager(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	initCtx, cancelInit := context.WithTimeout(ctx, 10*time.Second)
	defer cancelInit()
	db, err := infra.OpenMySQL(initCtx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer db.Close()
	rdb, err := infra.OpenRedis(initCtx, cfg.Redis)
	if err != nil {
		return err
	}
	defer rdb.Close()
	gateway, err := llm.New(initCtx, llmConfig, config.ProviderCredentials())
	if err != nil {
		return err
	}
	users, err := repo.NewUserRepository(db)
	if err != nil {
		return err
	}
	agentRepo, err := repo.NewAgentRepository(db)
	if err != nil {
		return err
	}
	sessions, err := repo.NewSessionRepository(cfg.SessionDir)
	if err != nil {
		return err
	}
	runs, err := repo.NewRunRepository(rdb)
	if err != nil {
		return err
	}
	runtime, err := agent.NewRuntime(gateway)
	if err != nil {
		return err
	}
	auth, err := service.NewAuthService(users, tokens)
	if err != nil {
		return err
	}
	agents, err := service.NewAgentService(agentRepo, runtime)
	if err != nil {
		return err
	}
	chat, err := service.NewChatService(ctx, agents, runtime, sessions, runs, service.ChatConfig{RunTimeout: cfg.RunTimeout, MaxConcurrent: cfg.MaxConcurrent})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = chat.Shutdown(cleanup)
	}()
	router, err := api.NewRouter(auth, agents, chat, tokens)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("http: listen: %w", err)
	}
	slog.Info("server started", "address", listener.Addr().String())
	server := &http.Server{Handler: router, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
	return serve(ctx, server, listener, chat, cfg.ShutdownTimeout)
}

type runManager interface{ Shutdown(context.Context) error }

func serve(ctx context.Context, server *http.Server, listener net.Listener, runs runManager, timeout time.Duration) error {
	defer server.Close()
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	var serveErr error
	select {
	case <-ctx.Done():
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	}
	slog.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// Cancel generation first. SSE handlers then replay its terminal event and exit.
	runErr := runs.Shutdown(shutdownCtx)
	httpErr := server.Shutdown(shutdownCtx)
	return errors.Join(serveErr, runErr, httpErr)
}
