package errors

import "net/http"

type Code string

const (
	GEN500DB       Code = "GEN_500_DB"
	GEN500UNKNOWN  Code = "GEN_500_UNKNOWN"
	GEN404NOTFOUND Code = "GEN_404_001"

	IDEMP409REPLAY Code = "IDEMP_409_REPLAY"

	QDE400INVALID Code = "QDE_400_001"
	QDE409DUP     Code = "QDE_409_001"

	FDE422MISSING Code = "FDE_422_001"
	FDE409DONE    Code = "FDE_409_001"

	SCOR409RECORDED Code = "SCOR_409_001"
	APP409BADSTATE  Code = "APP_409_001"
	ORD409EXISTS    Code = "ORD_409_001"
	DISB409EXISTS   Code = "DISB_409_001"
)

type HTTPError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e HTTPError) Status() int {
	switch e.Code {
	case QDE400INVALID:
		return http.StatusBadRequest
	case QDE409DUP, IDEMP409REPLAY, FDE409DONE, SCOR409RECORDED, APP409BADSTATE, ORD409EXISTS, DISB409EXISTS:
		return http.StatusConflict
	case GEN404NOTFOUND:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func (e HTTPError) Error() string { return e.Message }
