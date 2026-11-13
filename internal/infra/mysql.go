package infra

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Config contains the MySQL DSN and optional connection-pool settings.
// Positive pool settings override the database/sql defaults.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Client wraps a MySQL connection pool. The caller must close it when finished.
type Client struct {
	*sql.DB
}

// New opens a MySQL pool and verifies the connection before returning it.
// Use a context with a deadline to bound initialization time.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql: DSN is required")
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}

	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("mysql: ping: %w", err)
	}

	return &Client{DB: db}, nil
}


