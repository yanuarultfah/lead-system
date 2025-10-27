package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const HeaderTxnID = "X-Transaction-Id"

func TransactionID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader(HeaderTxnID)
		if tid == "" {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			tid = hex.EncodeToString(b)
			c.Request.Header.Set(HeaderTxnID, tid)
		}
		c.Writer.Header().Set(HeaderTxnID, tid)
		c.Next()
	}
}
