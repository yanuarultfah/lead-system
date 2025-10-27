package main

import (
	"context"
	"leads-system/internal/config"
	"leads-system/internal/db"
	"leads-system/internal/publish"
	"time"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log, _ := zap.NewProduction()
	defer log.Sync()

	pool, err := db.NewPool(context.Background(), cfg, log)
	if err != nil {
		log.Fatal("db", zap.Error(err))
	}
	defer pool.Close()

	pub, err := publish.NewPublisher(context.Background(), cfg, log, pool)
	if err != nil {
		log.Fatal("publisher", zap.Error(err))
	}
	defer pub.Close()

	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		n, err := db.PublishBatch(context.Background(), pool, log, pub, 50)
		if err != nil {
			log.Error("batch", zap.Error(err))
		}
		if n == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}
