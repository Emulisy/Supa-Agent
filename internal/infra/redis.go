package infra

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig contains the Redis address, credentials, and optional pool settings.
// Zero-valued pool settings and timeouts use the go-redis defaults.
type RedisConfig struct {
	Addr         string
	Username     string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// OpenRedis opens a Redis pool and verifies the connection before returning it.
// The caller closes the returned client when finished.
// Use a context with a deadline to bound initialization time.
func OpenRedis(ctx context.Context, cfg RedisConfig) (*redis.Client, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("redis: address is required")
	}
	if cfg.DB < 0 {
		return nil, fmt.Errorf("redis: database must be non-negative")
	}

	client := redis.NewClient(&redis.Options{
		Addr:                  cfg.Addr,
		Username:              cfg.Username,
		Password:              cfg.Password,
		DB:                    cfg.DB,
		PoolSize:              cfg.PoolSize,
		MinIdleConns:          cfg.MinIdleConns,
		DialTimeout:           cfg.DialTimeout,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		ContextTimeoutEnabled: true,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return client, nil
}
