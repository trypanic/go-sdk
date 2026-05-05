package database

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/trypanic/go-sdk/errorkit"
)

type StoredProcedurer[T any] interface {
	QueryRow(ctx context.Context, query string, args ...any) (T, error)
	Query(ctx context.Context, query string, args ...any) ([]T, error)
	QueryRowJSON(ctx context.Context, query string, jsonArg any, args ...any) (T, error)
	QueryJSON(ctx context.Context, query string, jsonArg any, args ...any) ([]T, error)
}

type StoredProcedure[T any] struct {
	Pool Querier
}

func NewStoredProcedure[T any](pool Querier) StoredProcedurer[T] {
	return WrapWithTracing(StoredProcedure[T]{Pool: pool})
}

func (s StoredProcedure[T]) QueryRow(ctx context.Context, query string, args ...any) (
	T,
	error,
) {
	// nosemgrep: defer-rows-close -- pgx.CollectOneRow closes rows automatically.
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		var e T
		return e, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("failed to execute query row"),
			errorkit.WithWrapped(err),
		)
	}

	result, err := pgx.CollectOneRow(rows, pgRowMapper[T]())
	if err != nil {
		var e T
		return e, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("failed to scan query row"),
			errorkit.WithWrapped(err),
		)
	}
	return result, nil
}

func (s StoredProcedure[T]) Query(ctx context.Context, query string, args ...any) (
	[]T,
	error,
) {
	// nosemgrep: defer-rows-close -- pgx.CollectRows closes rows automatically.
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("failed to execute query"),
			errorkit.WithWrapped(err),
		)
	}
	results, err := pgx.CollectRows(rows, pgRowMapper[T]())
	if err != nil {
		return nil, errorkit.NewError(ERR_DB_POSTGRES_ERROR).With(
			errorkit.WithReason("failed to collect query rows"),
			errorkit.WithWrapped(err),
		)
	}
	return results, nil
}

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
	if reflect.TypeOf((*T)(nil)).Elem().Kind() == reflect.Struct {
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
