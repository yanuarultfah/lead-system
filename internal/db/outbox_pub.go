package db

import (
	"context"
	"leads-system/internal/publish"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type OutboxItem struct {
	ID        int64
	AggType   string
	AggID     int64
	EventType string
	Payload   map[string]any
}

func PublishBatch(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger, pub *publish.Publisher, limit int) (int, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT outbox_id, aggregate_type, aggregate_id, event_type, payload
                                FROM ops.outbox WHERE published_at IS NULL
                                ORDER BY outbox_id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var items []OutboxItem
	for rows.Next() {
		var it OutboxItem
		if err := rows.Scan(&it.ID, &it.AggType, &it.AggID, &it.EventType, &it.Payload); err != nil {
			return 0, err
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return 0, tx.Commit(ctx)
	}

	for _, it := range items {
		if err := pub.PublishOutbox(it.EventType, it.Payload); err != nil {
			log.Error("publish event", zap.Int64("outbox_id", it.ID), zap.Error(err))
		}
	}
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	_, err = tx.Exec(ctx, `UPDATE ops.outbox SET published_at = now() WHERE outbox_id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	return len(items), tx.Commit(ctx)
}
