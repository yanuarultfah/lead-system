package server

import (
	"fmt"
	"net/http"
	"strings"

	"leads-system/internal/config"
	"leads-system/internal/middleware"
	"leads-system/internal/transport/httpapi"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Server struct{ Engine *gin.Engine }

func New(cfg *config.Config, logger *zap.Logger, pool *pgxpool.Pool) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gzip.Gzip(gzip.BestSpeed))
	r.Use(middleware.TransactionID())
	r.Use(requestLogger(logger))
	r.Use(limitBody(cfg.App.MaxBodyBytes))
	if cfg.Traffic.InjectGlitch {
		r.Use(middleware.NetGlitch(cfg.Traffic.JitterPct, cfg.Traffic.GlitchErrorPct))
	}
	r.Use(middleware.MaskPIIResponse(cfg.Log.PIIMask))

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", strings.Join(cfg.API.AllowOrigins, ","))
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Transaction-Id,Idempotency-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	httpapi.RegisterRoutes(r, logger, pool, cfg)
	return &Server{Engine: r}
}

func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Info("req", zap.String("m", c.Request.Method), zap.String("p", c.FullPath()), zap.Int("s", c.Writer.Status()), zap.String("txn", c.GetHeader("X-Transaction-Id")))
	}
}
func limitBody(size string) gin.HandlerFunc {
	var n int64
	var unit string
	fmt.Sscanf(size, "%d%s", &n, &unit)
	switch unit {
	case "MiB":
		n *= 1024 * 1024
	case "KiB":
		n *= 1024
	}
	if n == 0 {
		n = 2 * 1024 * 1024
	}
	return func(c *gin.Context) { c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n); c.Next() }
}
