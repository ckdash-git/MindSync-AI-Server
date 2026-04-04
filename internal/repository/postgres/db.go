package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/ckdash-git/MindSync-AI-Server/internal/config"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
)

// DB wraps sqlx.DB with application-specific functionality.
type DB struct {
	*sqlx.DB
	log *logger.Logger
}

// New creates a new database connection pool.
func New(cfg config.DatabaseConfig, log *logger.Logger) (*DB, error) {
	db, err := sqlx.Connect("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	log.Info("database connected",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Name,
		"max_open_conns", cfg.MaxOpenConns,
	)

	return &DB{DB: db, log: log}, nil
}

// HealthCheck verifies the database connection is alive.
func (db *DB) HealthCheck(ctx context.Context) error {
	return db.PingContext(ctx)
}

// Close gracefully closes the database connection pool.
func (db *DB) Close() error {
	db.log.Info("closing database connection pool")
	return db.DB.Close()
}
