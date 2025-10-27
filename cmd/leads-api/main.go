package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"leads-system/internal/config"
	"leads-system/internal/db"
	"leads-system/internal/observability"
	"leads-system/internal/server"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	tpShutdown := func() {}
	if cfg.OTEL.Enable {
		_, shutdown, err := observability.InitTracerProvider(cfg)
		if err == nil {
			tpShutdown = shutdown
		} else {
			logger.Warn("otel", zap.Error(err))
		}
	}
	defer tpShutdown()

	pool, err := db.NewPool(context.Background(), cfg, logger)
	if err != nil {
		logger.Fatal("db", zap.Error(err))
	}
	defer pool.Close()

	srv := server.New(cfg, logger, pool)
	httpSrv := &http.Server{
		Addr: cfg.App.ListenAddr, Handler: srv.Engine,
		ReadTimeout: cfg.App.ReadTimeout, WriteTimeout: cfg.App.WriteTimeout, IdleTimeout: cfg.App.IdleTimeout,
	}

	go func() {
		logger.Info("listen", zap.String("addr", cfg.App.ListenAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
