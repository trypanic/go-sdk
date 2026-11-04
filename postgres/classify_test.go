package database

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trypanic/go-sdk/errorkit"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

type wantClass struct {
	code      errorkit.ErrorCode
	retriable bool
	http      int
}

var (
	wantNotFound     = wantClass{ERR_DB_POSTGRES_NOT_FOUND, false, 404}
	wantDuplicate    = wantClass{errorkit.ERR_VALIDATION_DUPLICATE, false, 409}
	wantInconsistent = wantClass{errorkit.ERR_VALIDATION_INCONSISTENT, false, 400}
	wantDeadlock     = wantClass{ERR_DB_POSTGRES_DEADLOCK, true, 409}
	wantAuth         = wantClass{ERR_DB_POSTGRES_AUTH_FAILED, false, 502}
	wantTimeout      = wantClass{ERR_DB_POSTGRES_TIMEOUT, true, 504}
	wantCanceled     = wantClass{ERR_DB_POSTGRES_CANCELED, false, 499}
	wantUnavailable  = wantClass{ERR_DB_POSTGRES_UNAVAILABLE, true, 503}
	wantConnFailed   = wantClass{ERR_DB_POSTGRES_CONNECTION_FAILED, true, 503}
	wantConfig       = wantClass{errorkit.ERR_SYSTEM_CONFIG_INVALID, false, 500}
	wantGeneric      = wantClass{ERR_DB_POSTGRES_ERROR, true, 503}
)

func assertClass(t *testing.T, err error, want wantClass, cause error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", want.code)
	}
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T: %v", err, err)
	}
	if appErr.Code() != want.code {
		t.Fatalf("code = %s, want %s (err: %v, wrapped: %v)", appErr.Code(), want.code, err, appErr.Wrapped)
	}
	if appErr.Metadata.Retriable != want.retriable {
		t.Fatalf("%s: retriable = %v, want %v", want.code, appErr.Metadata.Retriable, want.retriable)
	}
	if appErr.Metadata.HTTPStatus != want.http {
		t.Fatalf("%s: HTTP status = %d, want %d", want.code, appErr.Metadata.HTTPStatus, want.http)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, %v) = false; the cause must stay wrapped", cause)
	}
}

func pgErr(code string) error {
	return &pgconn.PgError{Code: code, Message: "server error " + code}
}

// TestStoredProcedureErrorMatrix is the regression test for C4-002: every
// failure used to be ERR_DB_POSTGRES_ERROR (retriable, 503).
func TestStoredProcedureErrorMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want wantClass
	}{
		{"unique violation 23505", pgErr("23505"), wantDuplicate},
		{"foreign key violation 23503", pgErr("23503"), wantInconsistent},
		{"not null violation 23502", pgErr("23502"), wantInconsistent},
		{"check violation 23514", pgErr("23514"), wantInconsistent},
		{"serialization failure 40001", pgErr("40001"), wantDeadlock},
		{"deadlock 40P01", pgErr("40P01"), wantDeadlock},
		{"invalid password 28P01", pgErr("28P01"), wantAuth},
		{"invalid authorization 28000", pgErr("28000"), wantAuth},
		{"too many connections 53300", pgErr("53300"), wantUnavailable},
		{"admin shutdown 57P01", pgErr("57P01"), wantUnavailable},
		{"connection exception 08006", pgErr("08006"), wantUnavailable},
		{"query canceled or statement timeout 57014", pgErr("57014"), wantTimeout},
		{"lock not available 55P03", pgErr("55P03"), wantTimeout},
		{"other server error 42883", pgErr("42883"), wantGeneric},
		{"context canceled", context.Canceled, wantCanceled},
		{"wrapped context canceled", fmt.Errorf("query: %w", context.Canceled), wantCanceled},
		{"deadline exceeded", context.DeadlineExceeded, wantTimeout},
		{"net timeout", timeoutNetError{}, wantTimeout},
		{"conn closed", pgconn.ErrConnClosed, wantUnavailable},
		{"no rows from driver", pgx.ErrNoRows, wantNotFound},
		{"generic", errors.New("boom"), wantGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Run("Query fails", func(t *testing.T) {
				sp := StoredProcedure[int]{Pool: fakeQuerier{err: tt.err}}
				// nosemgrep: defer-rows-close -- StoredProcedure.Query returns []T, not rows.
				_, err := sp.Query(t.Context(), "SELECT * FROM f()")
				assertClass(t, err, tt.want, tt.err)
				_, err = sp.QueryRow(t.Context(), "SELECT * FROM f()")
				assertClass(t, err, tt.want, tt.err)
			})

			t.Run("rows.Err mid-iteration", func(t *testing.T) {
				rows := &fakeRows{fieldDescriptions: []pgconn.FieldDescription{{Name: "n"}}, err: tt.err}
				sp := StoredProcedure[int]{Pool: fakeQuerier{rows: rows}}
				// nosemgrep: defer-rows-close -- StoredProcedure.Query returns []T, not rows.
				_, err := sp.Query(t.Context(), "SELECT * FROM f()")
				assertClass(t, err, tt.want, tt.err)
				if !rows.closed {
					t.Fatal("rows were not closed")
				}
			})
		})
	}
}

func TestStoredProcedureQueryRow_NoRowsIsNotFound(t *testing.T) {
	t.Parallel()

	rows := &fakeRows{fieldDescriptions: []pgconn.FieldDescription{{Name: "n"}}}
	sp := StoredProcedure[int]{Pool: fakeQuerier{rows: rows}}

	_, err := sp.QueryRow(t.Context(), "SELECT * FROM get_item($1)", 1)
	assertClass(t, err, wantNotFound, pgx.ErrNoRows)
	if !rows.closed {
		t.Fatal("rows were not closed")
	}
}

func TestNewPostgresClient_ErrorClassification(t *testing.T) {
	t.Run("parse failure is a configuration error", func(t *testing.T) {
		_, err := NewPostgresClient(Config{DSN: "postgres://u:p@host:notaport/db"})
		assertClass(t, err, wantConfig, nil)
		if _, ok := errors.AsType[*pgconn.ParseConfigError](err); !ok {
			t.Fatal("ParseConfigError must stay wrapped")
		}
	})

	t.Run("dial failure is a connection error", func(t *testing.T) {
		socketDir := filepath.Join(t.TempDir(), "nosocket")
		_, err := NewPostgresClient(Config{DSN: "postgres://u:p@/db?host=" + socketDir})
		assertClass(t, err, wantConnFailed, nil)
	})
}
