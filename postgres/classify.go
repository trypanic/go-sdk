package postgres

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trypanic/go-sdk/v2/errorkit"
)

// SQLSTATE values and classes used by classifyPgError. See
// https://www.postgresql.org/docs/current/errcodes-appendix.html.
const (
	sqlStateUniqueViolation      = "23505"
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
	sqlStateTooManyConnections   = "53300"
	sqlStateQueryCanceled        = "57014" // statement_timeout or a cancel request
	sqlStateLockNotAvailable     = "55P03" // lock_timeout or NOWAIT

	sqlClassIntegrityViolation = "23"
	sqlClassAuthorization      = "28"
	sqlClassConnection         = "08"
	sqlClassOperatorIntervened = "57P0" // 57P01..57P05: shutdown, crash, cannot connect now
)

// classifyPgError maps err to the most specific error code and wraps it, so
// errors.Is/errors.As keep reaching the driver error. reason describes the
// failed operation.
//
//	pgx.ErrNoRows                        → ERR_DB_POSTGRES_NOT_FOUND (404, not retriable)
//	context.Canceled                     → ERR_DB_POSTGRES_CANCELED (499, not retriable)
//	*pgconn.ParseConfigError             → errorkit.ERR_SYSTEM_CONFIG_INVALID
//	deadline / pgconn.Timeout / net timeout → ERR_DB_POSTGRES_TIMEOUT
//	SQLSTATE 23505                       → errorkit.ERR_VALIDATION_DUPLICATE
//	other SQLSTATE class 23              → errorkit.ERR_VALIDATION_INCONSISTENT
//	SQLSTATE 40001, 40P01                → ERR_DB_POSTGRES_DEADLOCK (retriable)
//	SQLSTATE 57014, 55P03                → ERR_DB_POSTGRES_TIMEOUT (retriable)
//	SQLSTATE class 28                    → ERR_DB_POSTGRES_AUTH_FAILED
//	SQLSTATE class 08, 53300, 57P0x      → ERR_DB_POSTGRES_UNAVAILABLE
//	*pgconn.ConnectError                 → ERR_DB_POSTGRES_CONNECTION_FAILED
//	pgconn.ErrConnClosed, net.Error      → ERR_DB_POSTGRES_UNAVAILABLE
//	anything else                        → ERR_DB_POSTGRES_ERROR
func classifyPgError(err error, reason string) *errorkit.AppError {
	return errorkit.NewError(pgErrorCode(err)).With(
		errorkit.WithReason("%s", reason),
		errorkit.WithWrapped(err),
	)
}

func pgErrorCode(err error) errorkit.ErrorCode {
	if errors.Is(err, pgx.ErrNoRows) {
		return ERR_DB_POSTGRES_NOT_FOUND
	}
	// Checked before timeouts: pgconn reports a cancelled context through
	// the same internal type that pgconn.Timeout recognizes.
	if errors.Is(err, context.Canceled) {
		return ERR_DB_POSTGRES_CANCELED
	}
	if _, ok := errors.AsType[*pgconn.ParseConfigError](err); ok {
		return errorkit.ERR_SYSTEM_CONFIG_INVALID
	}
	if isTimeout(err) {
		return ERR_DB_POSTGRES_TIMEOUT
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		if code, ok := sqlStateCode(pgErr.Code); ok {
			return code
		}
		return ERR_DB_POSTGRES_ERROR
	}
	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return ERR_DB_POSTGRES_CONNECTION_FAILED
	}
	if errors.Is(err, pgconn.ErrConnClosed) {
		return ERR_DB_POSTGRES_UNAVAILABLE
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return ERR_DB_POSTGRES_UNAVAILABLE
	}
	return ERR_DB_POSTGRES_ERROR
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) {
		return true
	}
	netErr, ok := errors.AsType[net.Error](err)
	return ok && netErr.Timeout()
}

func sqlStateCode(state string) (errorkit.ErrorCode, bool) {
	switch {
	case state == sqlStateUniqueViolation:
		return errorkit.ERR_VALIDATION_DUPLICATE, true
	case strings.HasPrefix(state, sqlClassIntegrityViolation):
		return errorkit.ERR_VALIDATION_INCONSISTENT, true
	case state == sqlStateSerializationFailure, state == sqlStateDeadlockDetected:
		return ERR_DB_POSTGRES_DEADLOCK, true
	case state == sqlStateQueryCanceled, state == sqlStateLockNotAvailable:
		return ERR_DB_POSTGRES_TIMEOUT, true
	case strings.HasPrefix(state, sqlClassAuthorization):
		return ERR_DB_POSTGRES_AUTH_FAILED, true
	case strings.HasPrefix(state, sqlClassConnection),
		state == sqlStateTooManyConnections,
		strings.HasPrefix(state, sqlClassOperatorIntervened):
		return ERR_DB_POSTGRES_UNAVAILABLE, true
	default:
		return "", false
	}
}
