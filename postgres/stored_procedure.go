package postgres

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/telemetry"
)

// StoredProcedurer runs a stored-procedure call and scans the result into T.
// The JSON variants marshal jsonArg and pass it as the last positional
// parameter ($N+1, after args).
type StoredProcedurer[T any] interface {
	QueryRow(ctx context.Context, query string, args ...any) (T, error)
	Query(ctx context.Context, query string, args ...any) ([]T, error)
	QueryRowJSON(ctx context.Context, query string, jsonArg any, args ...any) (T, error)
	QueryJSON(ctx context.Context, query string, jsonArg any, args ...any) ([]T, error)
}

// StoredProcedure is the untraced StoredProcedurer. Structs are scanned with
// pgx.RowToStructByNameLax (by column name), other types with pgx.RowTo. It
// holds no state besides Pool and is safe for concurrent use.
type StoredProcedure[T any] struct {
	Pool Querier
}

// NewStoredProcedure returns a StoredProcedure on pool wrapped with
// WrapWithTracing, so every call opens a span with the global tracer. Do not
// wrap the result again; use WrapWithInstrumenter on a StoredProcedure value
// to choose the tracer or disable tracing.
func NewStoredProcedure[T any](pool Querier) *TracedProcedure[T] {
	return &TracedProcedure[T]{inner: StoredProcedure[T]{Pool: pool}, instrumenter: telemetry.NewInstrumenter(telemetry.InstrumenterConfig{})}
}

// QueryRow runs query and scans the single row it returns. No row is
// ERR_DB_POSTGRES_NOT_FOUND.
func (s StoredProcedure[T]) QueryRow(ctx context.Context, query string, args ...any) (
	T,
	error,
) {
	// nosemgrep: defer-rows-close -- pgx.CollectOneRow closes rows automatically.
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		var e T
		return e, classifyPgError(err, "failed to execute query row")
	}

	result, err := pgx.CollectOneRow(rows, pgRowMapper[T]())
	if err != nil {
		var e T
		return e, classifyPgError(err, "failed to scan query row")
	}
	return result, nil
}

// Query runs query and scans every row it returns.
func (s StoredProcedure[T]) Query(ctx context.Context, query string, args ...any) (
	[]T,
	error,
) {
	// nosemgrep: defer-rows-close -- pgx.CollectRows closes rows automatically.
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, classifyPgError(err, "failed to execute query")
	}
	results, err := pgx.CollectRows(rows, pgRowMapper[T]())
	if err != nil {
		return nil, classifyPgError(err, "failed to collect query rows")
	}
	return results, nil
}

// QueryRowJSON is QueryRow with jsonArg marshaled to JSON and appended as
// the last positional parameter.
func (s StoredProcedure[T]) QueryRowJSON(
	ctx context.Context,
	query string,
	jsonArg any,
	args ...any,
) (T, error) {
	queryArgs, err := appendJSONArg(args, jsonArg)
	if err != nil {
		var e T
		return e, err
	}
	return s.QueryRow(ctx, query, queryArgs...)
}

// QueryJSON is Query with jsonArg marshaled to JSON and appended as the last
// positional parameter.
func (s StoredProcedure[T]) QueryJSON(
	ctx context.Context,
	query string,
	jsonArg any,
	args ...any,
) ([]T, error) {
	queryArgs, err := appendJSONArg(args, jsonArg)
	if err != nil {
		return nil, err
	}
	return s.Query(ctx, query, queryArgs...)
}

func pgRowMapper[T any]() pgx.RowToFunc[T] {
	if reflect.TypeFor[T]().Kind() == reflect.Struct {
		return pgx.RowToStructByNameLax[T]
	}
	return pgx.RowTo[T]
}

func appendJSONArg(args []any, jsonArg any) ([]any, error) {
	inBytes, err := json.Marshal(jsonArg)
	if err != nil {
		return nil, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("failed to marshal query JSON argument"),
			errorkit.WithWrapped(err),
		)
	}

	queryArgs := make([]any, 0, len(args)+1)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, string(inBytes))
	return queryArgs, nil
}
