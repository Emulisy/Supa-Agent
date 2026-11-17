// Package config loads process settings at command startup.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Emulisy/Supa-Agent/internal/infra"
	"github.com/Emulisy/Supa-Agent/internal/llm"
	"github.com/joho/godotenv"
)

func LoadEnvironment() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: load .env: %w", err)
	}
	return nil
}

func ProviderCredentials() map[string]string {
	return map[string]string{llm.ProviderDeepSeek: os.Getenv("DEEPSEEK_API_KEY")}
}

type Server struct {
	HTTPAddr        string
	MySQL           infra.MySQLConfig
	Redis           infra.RedisConfig
	JWTSecret       string
	JWTTTL          time.Duration
	SessionDir      string
	LLMPath         string
	RunTimeout      time.Duration
	MaxConcurrent   int
	ShutdownTimeout time.Duration
}

func LoadServer() (Server, error) {
	if err := LoadEnvironment(); err != nil {
		return Server{}, err
	}
	cfg := Server{HTTPAddr: env("HTTP_ADDR", "127.0.0.1:8080"), MySQL: infra.MySQLConfig{DSN: os.Getenv("MYSQL_DSN")},
		Redis:     infra.RedisConfig{Addr: env("REDIS_ADDR", "127.0.0.1:6379"), Username: os.Getenv("REDIS_USERNAME"), Password: os.Getenv("REDIS_PASSWORD")},
		JWTSecret: os.Getenv("JWT_SECRET"), SessionDir: env("SESSION_DIR", "data/sessions"), LLMPath: env("LLM_CONFIG", "configs/llm.yaml")}
	var err error
	if _, _, err = net.SplitHostPort(cfg.HTTPAddr); err != nil {
		return Server{}, errors.New("config: invalid HTTP_ADDR")
	}
	if cfg.MySQL.DSN == "" {
		return Server{}, errors.New("config: MYSQL_DSN is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Server{}, errors.New("config: JWT_SECRET must contain at least 32 random bytes")
	}
	if strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")) == "" {
		return Server{}, errors.New("config: DEEPSEEK_API_KEY is required")
	}
	if cfg.Redis.DB, err = strconv.Atoi(env("REDIS_DB", "0")); err != nil || cfg.Redis.DB < 0 {
		return Server{}, errors.New("config: invalid REDIS_DB")
	}
	if cfg.MaxConcurrent, err = strconv.Atoi(env("RUN_MAX_CONCURRENT", "8")); err != nil || cfg.MaxConcurrent < 1 {
		return Server{}, errors.New("config: RUN_MAX_CONCURRENT must be positive")
	}
	for _, item := range []struct {
		key, fallback string
		target        *time.Duration
	}{
		{"JWT_ACCESS_TTL", "15m", &cfg.JWTTTL}, {"RUN_TIMEOUT", "5m", &cfg.RunTimeout}, {"SHUTDOWN_TIMEOUT", "10s", &cfg.ShutdownTimeout},
	} {
		*item.target, err = time.ParseDuration(env(item.key, item.fallback))
		if err != nil || *item.target < time.Second {
			return Server{}, fmt.Errorf("config: %s must be at least one second", item.key)
		}
	}
	if strings.TrimSpace(cfg.SessionDir) == "" {
		return Server{}, errors.New("config: SESSION_DIR is required")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
