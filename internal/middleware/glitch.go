package middleware

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func NetGlitch(jitterPct, glitchErrorPct int) gin.HandlerFunc {
	rand.Seed(time.Now().UnixNano())
	return func(c *gin.Context) {
		if jitterPct > 0 && rand.Intn(100) < jitterPct {
			time.Sleep(time.Duration(50+rand.Intn(200)) * time.Millisecond)
		}
		if glitchErrorPct > 0 && rand.Intn(100) < glitchErrorPct {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "injected_glitch"})
			return
		}
		c.Next()
	}
}
