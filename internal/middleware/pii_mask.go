package middleware

import (
	"bytes"
	"io"
	"regexp"

	"github.com/gin-gonic/gin"
)

var rePhone = regexp.MustCompile(`\b(\+?62|0)8[0-9]{8,12}\b`)
var reEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
var reKTP = regexp.MustCompile(`\b[0-9]{16}\b`)

func MaskPIIResponse(enabled bool) gin.HandlerFunc {
	if !enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		blw := &bodyLogWriter{ResponseWriter: c.Writer, body: bytes.NewBuffer(nil)}
		c.Writer = blw
		c.Next()
		body := blw.body.Bytes()
		body = rePhone.ReplaceAll(body, []byte("08**********"))
		body = reEmail.ReplaceAll(body, []byte("***@***"))
		body = reKTP.ReplaceAll(body, []byte("**************"))
		c.Writer = blw.ResponseWriter
		_, _ = c.Writer.Write(body)
	}
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error)       { return w.body.Write(b) }
func (w *bodyLogWriter) WriteString(s string) (int, error) { return io.WriteString(w.body, s) }
