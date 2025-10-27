package httpapi

import (
	"leads-system/internal/config"
	appErr "leads-system/internal/errors"
	"leads-system/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func RegisterRoutes(r *gin.Engine, log *zap.Logger, pool *pgxpool.Pool, cfg *config.Config) {
	u := usecase.NewLeadUsecase(pool, log)

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	r.POST("/leads", func(c *gin.Context) {
		var req usecase.CreateLeadRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			he := appErr.HTTPError{Code: appErr.QDE400INVALID, Message: err.Error()}
			c.JSON(he.Status(), he)
			return
		}
		res, err := u.CreateLead(c.Request.Context(), req, c.GetHeader("Idempotency-Key"))
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusCreated, res)
	})

	r.GET("/leads/:uuid", func(c *gin.Context) {
		res, err := u.GetLead(c.Request.Context(), c.Param("uuid"))
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.POST("/leads/:uuid/fde", func(c *gin.Context) {
		var req usecase.FDERequest
		if err := c.ShouldBindJSON(&req); err != nil {
			he := appErr.HTTPError{Code: appErr.QDE400INVALID, Message: err.Error()}
			c.JSON(he.Status(), he)
			return
		}
		res, err := u.CompleteFDE(c.Request.Context(), c.Param("uuid"), req)
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.POST("/leads/:uuid/scoring/callback", func(c *gin.Context) {
		var req usecase.ScoringCallbackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			he := appErr.HTTPError{Code: appErr.QDE400INVALID, Message: err.Error()}
			c.JSON(he.Status(), he)
			return
		}
		res, err := u.ScoringCallback(c.Request.Context(), c.Param("uuid"), req)
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.POST("/leads/:uuid/approval", func(c *gin.Context) {
		var req usecase.ApprovalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			he := appErr.HTTPError{Code: appErr.QDE400INVALID, Message: err.Error()}
			c.JSON(he.Status(), he)
			return
		}
		res, err := u.Approve(c.Request.Context(), c.Param("uuid"), req)
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	r.POST("/orders", func(c *gin.Context) {
		var req usecase.CreateOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			he := appErr.HTTPError{Code: appErr.QDE400INVALID, Message: err.Error()}
			c.JSON(he.Status(), he)
			return
		}
		res, err := u.CreateOrder(c.Request.Context(), req)
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusCreated, res)
	})

	r.POST("/orders/:agreement_no/disburse", func(c *gin.Context) {
		var req usecase.DisburseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			he := appErr.HTTPError{Code: appErr.QDE400INVALID, Message: err.Error()}
			c.JSON(he.Status(), he)
			return
		}
		req.AgreementNo = c.Param("agreement_no")
		res, err := u.Disburse(c.Request.Context(), req)
		if err != nil {
			he := usecase.ToHTTPError(err)
			c.JSON(he.Status(), he)
			return
		}
		c.JSON(http.StatusOK, res)
	})
}
