package db

import (
	"context"
	"fmt"

	"leads-system/internal/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewPool(ctx context.Context, cfg *config.Config, log *zap.Logger) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(cfg.DB.URI)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	conf.MaxConns = cfg.DB.MaxConns
	conf.MinConns = cfg.DB.MinConns
	conf.MaxConnLifetime = cfg.DB.MaxConnLifetime
	conf.MaxConnIdleTime = cfg.DB.MaxConnIdleTime
	conf.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if cfg.DB.StatementTimeoutMs > 0 {
			_, err := c.Exec(ctx, fmt.Sprintf("set statement_timeout=%d", cfg.DB.StatementTimeoutMs))
			return err
		}
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
