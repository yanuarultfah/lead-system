package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	appErr "leads-system/internal/errors"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func isCommitRollbackError(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		// 25P02 is 'in failed sql transaction'
		if pe.Code == "25P02" {
			return true
		}
	}
	if err != nil && strings.Contains(err.Error(), "commit unexpectedly resulted in rollback") {
		return true
	}
	return false
}

type CreateLeadRequest struct {
	Channel      string `json:"channel" binding:"required"`
	Origin       string `json:"origin"  binding:"required"`
	SourceRef    string `json:"source_ref" binding:"required"`
	MerchantCode string `json:"merchant_code"`
	Customer     struct {
		FullName string `json:"full_name" binding:"required"`
		Mobile   string `json:"mobile"`
		Email    string `json:"email"`
	} `json:"customer"`
}
type FDERequest struct {
	KTPNumber      string          `json:"ktp_number" binding:"required"`
	NPWP           string          `json:"npwp"`
	EmploymentType string          `json:"employment_type"`
	MonthlyIncome  float64         `json:"monthly_income"`
	MonthlyExpense float64         `json:"monthly_expense"`
	BankAccount    string          `json:"bank_account"`
	DocFlags       map[string]bool `json:"doc_flags"`
	Additional     map[string]any  `json:"additional_data"`
}
type ScoringCallbackRequest struct {
	Score    float64        `json:"score"`
	Grade    string         `json:"grade"`
	Decision string         `json:"decision"` // APPROVE/REJECT/REVIEW
	Reason   string         `json:"reason"`
	Raw      map[string]any `json:"raw"`
}
type ApprovalRequest struct {
	Decision string `json:"decision" binding:"required"` // APPROVED/REJECTED
	Reason   string `json:"reason"`
}
type CreateOrderRequest struct {
	LeadUUID       string  `json:"lead_uuid" binding:"required"`
	AgreementNo    string  `json:"agreement_no" binding:"required"`
	ApprovedAmount float64 `json:"approved_amount" binding:"required"`
	Tenor          int32   `json:"tenor" binding:"required"`
	InterestPct    float64 `json:"interest_pct" binding:"required"`
	Installment    float64 `json:"installment" binding:"required"`
}
type DisburseRequest struct {
	AgreementNo string  `json:"-"`
	Amount      float64 `json:"amount" binding:"required"`
	Channel     string  `json:"channel" binding:"required"`
	BankAccount string  `json:"bank_account"`
}

type LeadResponse struct {
	UUID      string    `json:"uuid"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type LeadUsecase struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewLeadUsecase(pool *pgxpool.Pool, log *zap.Logger) *LeadUsecase {
	return &LeadUsecase{db: pool, log: log}
}

func validEmail(s string) bool {
	if s == "" {
		return true
	}
	re := regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
	return re.MatchString(s)
}
func validPhoneID(s string) bool {
	if s == "" {
		return true
	}
	re := regexp.MustCompile(`^(\+62|0)8[0-9]{8,12}$`)
	return re.MatchString(s)
}
func validNPWP(s string) bool {
	if s == "" {
		return true
	}
	digits := regexp.MustCompile(`\D`).ReplaceAllString(s, "")
	re := regexp.MustCompile(`^\d{15}$`)
	return re.MatchString(digits)
}

// CreateLead with idempotency header
func (u *LeadUsecase) CreateLead(ctx context.Context, req CreateLeadRequest, idemKey string) (*LeadResponse, error) {
	if !validEmail(req.Customer.Email) {
		return nil, appErr.HTTPError{Code: appErr.QDE400INVALID, Message: "invalid email format"}
	}
	if !validPhoneID(req.Customer.Mobile) {
		return nil, appErr.HTTPError{Code: appErr.QDE400INVALID, Message: "invalid phone format"}
	}

	var uuid string
	var status string
	var created time.Time

	for attempt := 0; attempt < 3; attempt++ {
		tx, err := u.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}
		committed := false
		// ensure rollback if not committed
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()

		err = func() error {
			// Idempotency: attempt to insert; RETURNING will yield no rows if conflict
			if idemKey != "" {
				row := tx.QueryRow(ctx, `INSERT INTO ops.idempotency(idempotency_key) VALUES ($1) ON CONFLICT DO NOTHING RETURNING idempotency_key`, idemKey)
				var k string
				if scanErr := row.Scan(&k); scanErr != nil {
					if errors.Is(scanErr, pgx.ErrNoRows) {
						// replayed key
						_ = tx.Rollback(ctx)
						return appErr.HTTPError{Code: appErr.IDEMP409REPLAY, Message: "replayed idempotency key"}
					}
					_ = tx.Rollback(ctx)
					return scanErr
				}
			}

			// If a lead with same origin+source_ref exists, return it.
			if err := tx.QueryRow(ctx, `SELECT lead_uuid, status, created_at FROM leads.lead WHERE origin=$1 AND source_ref=$2`, req.Origin, req.SourceRef).
				Scan(&uuid, &status, &created); err == nil {
				// existing lead found, commit and return its data
				if commitErr := tx.Commit(ctx); commitErr != nil {
					_ = tx.Rollback(ctx)
					return commitErr
				}
				committed = true
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				_ = tx.Rollback(ctx)
				return err
			}

			// Insert customer
			var customerID int64
			if err := tx.QueryRow(ctx, `INSERT INTO party.customer (full_name, mobile_no, email) VALUES ($1,$2,$3) RETURNING customer_id`,
				req.Customer.FullName, req.Customer.Mobile, req.Customer.Email).Scan(&customerID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			// Insert lead and return DB-generated uuid/status/created_at
			if err := tx.QueryRow(ctx, `
                INSERT INTO leads.lead (channel, origin, source_ref, merchant_code, customer_id, status, expired_at)
                VALUES ($1,$2,$3,$4,$5,'QDE_INIT', now() + interval '6 hours')
                RETURNING lead_uuid, status, created_at
            `, req.Channel, req.Origin, req.SourceRef, req.MerchantCode, customerID).Scan(&uuid, &status, &created); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			// Insert outbox event referencing the newly created lead
			if _, err := tx.Exec(ctx, `
                INSERT INTO ops.outbox (aggregate_type, aggregate_id, event_type, payload)
                SELECT 'lead', lead_id, 'LEAD_CREATED',
                       jsonb_build_object('uuid', lead_uuid, 'origin', origin, 'ts', now())
                FROM leads.lead WHERE lead_uuid=$1
            `, uuid); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			if err := tx.Commit(ctx); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			committed = true
			return nil
		}()
		// clear deferred rollback for this iteration (since we set committed or rolled back inside)
		if err == nil {
			break
		}
		var se *pgconn.PgError
		if (errors.As(err, &se) && se.Code == "40001") || isCommitRollbackError(err) {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return nil, err
	}

	return &LeadResponse{UUID: uuid, Status: status, CreatedAt: created}, nil
}

func (u *LeadUsecase) GetLead(ctx context.Context, uuid string) (*LeadResponse, error) {
	if uuid == "" {
		return nil, appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "lead not found"}
	}

	var status string
	var created time.Time

	err := u.db.QueryRow(ctx, `SELECT status, created_at FROM leads.lead WHERE lead_uuid=$1`, uuid).
		Scan(&status, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "lead not found"}
	}
	if err != nil {
		return nil, ToHTTPError(err)
	}

	return &LeadResponse{UUID: uuid, Status: status, CreatedAt: created}, nil
}

func (u *LeadUsecase) CompleteFDE(ctx context.Context, uuid string, r FDERequest) (*LeadResponse, error) {
	var status string
	var created time.Time

	for attempt := 0; attempt < 3; attempt++ {
		tx, err := u.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()

		err = func() error {
			// Load lead and merchant_code
			var leadID int64
			var merchantCode string
			if err := tx.QueryRow(ctx, `SELECT lead_id, status, created_at, COALESCE(merchant_code,'') FROM leads.lead WHERE lead_uuid=$1`, uuid).
				Scan(&leadID, &status, &created, &merchantCode); err != nil {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "lead not found"}
			}

			// basic validations
			if r.KTPNumber == "" {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.FDE422MISSING, Message: "KTP is required"}
			}
			if !validNPWP(r.NPWP) {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.FDE422MISSING, Message: "invalid NPWP format (15 digits required)"}
			}

			// fetch merchant rules if merchant_code present
			var ktpRegex string
			requiredDocs := []string{}
			if merchantCode != "" {
				if err := tx.QueryRow(ctx, `SELECT COALESCE(rule_profile->>'ktp_regex',''), COALESCE(ARRAY(SELECT jsonb_array_elements_text(rule_profile->'required_docs')), ARRAY[]::text[]) FROM ref.merchant WHERE merchant_code=$1`, merchantCode).
					Scan(&ktpRegex, &requiredDocs); err != nil && !errors.Is(err, pgx.ErrNoRows) {
					_ = tx.Rollback(ctx)
					return err
				}
			}

			ktpRegex = strings.TrimSpace(ktpRegex)
			if ktpRegex == "" {
				ktpRegex = `^\d{16}$`
			}
			re, compErr := regexp.Compile(ktpRegex)
			if compErr != nil {
				// fallback to default if the merchant regex is invalid
				re = regexp.MustCompile(`^\d{16}$`)
			}
			if !re.MatchString(r.KTPNumber) {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.FDE422MISSING, Message: "KTP format invalid"}
			}

			// ensure doc flags map is non-nil for checks and JSON marshalling
			if r.DocFlags == nil {
				r.DocFlags = map[string]bool{}
			}
			for _, d := range requiredDocs {
				if ok, exists := r.DocFlags[d]; !exists || !ok {
					_ = tx.Rollback(ctx)
					return appErr.HTTPError{Code: appErr.FDE422MISSING, Message: "missing doc: " + d}
				}
			}

			// ensure not already completed
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM leads.lead_detail WHERE lead_id=$1)`, leadID).Scan(&exists); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if exists {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.FDE409DONE, Message: "FDE already completed"}
			}

			// ensure additional is non-nil for JSON marshalling
			if r.Additional == nil {
				r.Additional = map[string]any{}
			}

			flagsJSON, err := json.Marshal(r.DocFlags)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			additionalJSON, err := json.Marshal(r.Additional)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			// insert lead_detail
			if _, err := tx.Exec(ctx, `
                INSERT INTO leads.lead_detail (lead_id, ktp_number, employment_type, monthly_income, monthly_expense, bank_account, doc_flags, additional_data, completed_at)
                VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now())
            `, leadID, r.KTPNumber, r.EmploymentType, r.MonthlyIncome, r.MonthlyExpense, r.BankAccount, flagsJSON, additionalJSON); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			// update lead status
			if _, err := tx.Exec(ctx, `UPDATE leads.lead SET status='FDE_COMPLETED', updated_at=now() WHERE lead_id=$1`, leadID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			// insert outbox
			if _, err := tx.Exec(ctx, `
                INSERT INTO ops.outbox (aggregate_type, aggregate_id, event_type, payload)
                VALUES ('lead',$1,'FDE_COMPLETED', jsonb_build_object('uuid',$2::text,'ts',now()))
            `, leadID, uuid); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			if err := tx.Commit(ctx); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			committed = true
			// reflect the new status
			status = "FDE_COMPLETED"
			return nil
		}()
		if err == nil {
			break
		}
		var se *pgconn.PgError
		if (errors.As(err, &se) && se.Code == "40001") || isCommitRollbackError(err) {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return nil, err
	}

	return &LeadResponse{UUID: uuid, Status: status, CreatedAt: created}, nil
}

func (u *LeadUsecase) ScoringCallback(ctx context.Context, uuid string, r ScoringCallbackRequest) (*LeadResponse, error) {
	var status string
	var created time.Time
	for attempt := 0; attempt < 3; attempt++ {
		tx, err := u.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}
		err = func() error {
			var leadID int64
			err := tx.QueryRow(ctx, `SELECT lead_id, status, created_at FROM leads.lead WHERE lead_uuid=$1`, uuid).
				Scan(&leadID, &status, &created)
			if err != nil {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "lead not found"}
			}

			var exists bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk.scoring WHERE lead_id=$1)`, leadID).Scan(&exists)
			if exists {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.SCOR409RECORDED, Message: "scoring already recorded"}
			}

			_, err = tx.Exec(ctx, `
                INSERT INTO risk.scoring (lead_id, score, grade, decision, reason, raw, requested_at, responded_at)
                VALUES ($1,$2,$3,$4,$5,$6, now(), now())
            `, leadID, r.Score, r.Grade, r.Decision, r.Reason, r.Raw)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			newStatus := "PREAPPROVAL_PENDING"
			if r.Decision == "APPROVE" {
				newStatus = "APPROVED"
			}
			if r.Decision == "REJECT" {
				newStatus = "REJECTED"
			}
			_, err = tx.Exec(ctx, `UPDATE leads.lead SET status=$2, updated_at=now() WHERE lead_id=$1`, leadID, newStatus)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			_, _ = tx.Exec(ctx, `
				INSERT INTO ops.outbox (aggregate_type, aggregate_id, event_type, payload)
				VALUES ('lead',$1,'SCORING_RECORDED', jsonb_build_object('uuid',$2::text,'decision',$3::text,'ts',now()))
			`, leadID, uuid, r.Decision)

			status = newStatus
			return tx.Commit(ctx)
		}()
		if err == nil {
			break
		}
		var se *pgconn.PgError
		if (errors.As(err, &se) && se.Code == "40001") || isCommitRollbackError(err) {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return nil, err
	}
	return &LeadResponse{UUID: uuid, Status: status, CreatedAt: created}, nil
}

func (u *LeadUsecase) Approve(ctx context.Context, uuid string, r ApprovalRequest) (*LeadResponse, error) {
	var status string
	var created time.Time
	for attempt := 0; attempt < 3; attempt++ {
		tx, err := u.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}
		err = func() error {
			var leadID int64
			err := tx.QueryRow(ctx, `SELECT lead_id, status, created_at FROM leads.lead WHERE lead_uuid=$1`, uuid).
				Scan(&leadID, &status, &created)
			if err != nil {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "lead not found"}
			}

			if status != "FDE_COMPLETED" && status != "PREAPPROVAL_PENDING" && status != "APPROVED" && status != "REJECTED" {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.APP409BADSTATE, Message: "invalid state for approval"}
			}
			newStatus := "APPROVED"
			if r.Decision == "REJECTED" {
				newStatus = "REJECTED"
			}
			_, err = tx.Exec(ctx, `UPDATE leads.lead SET status=$2, updated_at=now() WHERE lead_id=$1`, leadID, newStatus)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			_, _ = tx.Exec(ctx, `
				INSERT INTO ops.outbox (aggregate_type, aggregate_id, event_type, payload)
				VALUES ('lead',$1,'APPROVAL_' || $3, jsonb_build_object('uuid',$2::text,'ts',now()))
			`, leadID, uuid, newStatus)

			status = newStatus
			return tx.Commit(ctx)
		}()
		if err == nil {
			break
		}
		var se *pgconn.PgError
		if errors.As(err, &se) && se.Code == "40001" {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return nil, err
	}
	return &LeadResponse{UUID: uuid, Status: status, CreatedAt: created}, nil
}

type OrderResponse struct {
	AgreementNo string `json:"agreement_no"`
	LeadUUID    string `json:"lead_uuid"`
}

func (u *LeadUsecase) CreateOrder(ctx context.Context, r CreateOrderRequest) (*OrderResponse, error) {
	var agr string

	if r.AgreementNo == "" || r.LeadUUID == "" {
		return nil, appErr.HTTPError{Code: appErr.QDE400INVALID, Message: "missing required fields"}
	}

	for attempt := 0; attempt < 3; attempt++ {
		tx, err := u.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}

		err = func() error {
			var leadID int64
			var status string
			if err := tx.QueryRow(ctx, `SELECT lead_id, status FROM leads.lead WHERE lead_uuid=$1`, r.LeadUUID).
				Scan(&leadID, &status); err != nil {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "lead not found"}
			}

			if status != "APPROVED" && status != "ORDER_CREATED" {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.APP409BADSTATE, Message: "lead not approved"}
			}

			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "order".agreement WHERE lead_id=$1 OR agreement_no=$2)`, leadID, r.AgreementNo).Scan(&exists); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if exists {
				_ = tx.Rollback(ctx)
				return appErr.HTTPError{Code: appErr.ORD409EXISTS, Message: "agreement already exists"}
			}

			if err := tx.QueryRow(ctx, `
                INSERT INTO "order".agreement (lead_id, agreement_no, approved_amount, tenor, interest_pct, installment)
                VALUES ($1,$2,$3,$4,$5,$6) RETURNING agreement_no
            `, leadID, r.AgreementNo, r.ApprovedAmount, r.Tenor, r.InterestPct, r.Installment).Scan(&agr); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			if _, err := tx.Exec(ctx, `UPDATE leads.lead SET status='ORDER_CREATED', updated_at=now() WHERE lead_id=$1`, leadID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			if _, err := tx.Exec(ctx, `
                INSERT INTO ops.outbox (aggregate_type, aggregate_id, event_type, payload)
                VALUES ('lead',$1,'ORDER_CREATED', jsonb_build_object('lead_uuid',$2::text,'agreement_no',$3::text,'ts',now()))
            `, leadID, r.LeadUUID, agr); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			return tx.Commit(ctx)
		}()
		if err == nil {
			break
		}

		var se *pgconn.PgError
		if (errors.As(err, &se) && se.Code == "40001") || isCommitRollbackError(err) {
			// exponential-ish backoff with small base
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return nil, err
	}

	return &OrderResponse{AgreementNo: agr, LeadUUID: r.LeadUUID}, nil
}

type DisburseResponse struct {
	AgreementNo string `json:"agreement_no"`
	Status      string `json:"status"`
}

func (u *LeadUsecase) Disburse(ctx context.Context, r DisburseRequest) (*DisburseResponse, error) {
	if r.AgreementNo == "" {
		return nil, appErr.HTTPError{Code: appErr.QDE400INVALID, Message: "agreement_no required"}
	}

	var status string
	for attempt := 0; attempt < 3; attempt++ {
		tx, err := u.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()

		err = func() error {
			var agrID int64
			if err := tx.QueryRow(ctx, `SELECT agreement_id FROM "order".agreement WHERE agreement_no=$1`, r.AgreementNo).Scan(&agrID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return appErr.HTTPError{Code: appErr.GEN404NOTFOUND, Message: "agreement not found"}
				}
				return err
			}

			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "order".disbursement WHERE agreement_id=$1)`, agrID).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return appErr.HTTPError{Code: appErr.DISB409EXISTS, Message: "already disbursed"}
			}

			if _, err := tx.Exec(ctx, `
                INSERT INTO "order".disbursement (agreement_id, amount, channel, bank_account, status, requested_at, executed_at)
                VALUES ($1,$2,$3,$4,'SUCCESS', now(), now())
            `, agrID, r.Amount, r.Channel, r.BankAccount); err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, `
                INSERT INTO ops.outbox (aggregate_type, aggregate_id, event_type, payload)
                VALUES ('agreement',$1,'DISBURSED', jsonb_build_object('agreement_no',$2::text,'ts',now()))
            `, agrID, r.AgreementNo); err != nil {
				return err
			}

			status = "SUCCESS"
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			committed = true
			return nil
		}()
		if err == nil {
			break
		}
		var pe *pgconn.PgError
		if (errors.As(err, &pe) && pe.Code == "40001") || isCommitRollbackError(err) {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			continue
		}
		return nil, err
	}

	return &DisburseResponse{AgreementNo: r.AgreementNo, Status: status}, nil
}

func ToHTTPError(err error) appErr.HTTPError {
	var he appErr.HTTPError
	if errors.As(err, &he) {
		return he
	}
	return appErr.HTTPError{Code: appErr.GEN500UNKNOWN, Message: fmt.Sprintf("%v", err)}
}
