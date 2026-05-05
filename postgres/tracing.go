package database

import (
	"context"
	"regexp"

	"github.com/trypanic/go-sdk/telemetry"
)

// reProcName matches the stored procedure name in a "SELECT * FROM <proc>(...)" call.
var reProcName = regexp.MustCompile(`(?i)\bFROM\s+(\w+)\s*\(`)

// WrapWithTracing wraps a StoredProcedurer with automatic span creation.
//
// Each call to QueryRow, Query, QueryRowJSON, or QueryJSON opens a span
// named after the stored procedure extracted from the SQL string:
//
//	"SELECT * FROM check_or_create_job($1, ...)" → span: "postgres.check_or_create_job"
//
// The span becomes the parent of all pgx-level spans (prepare, query, pool.acquire)
// produced by the otelpgx tracer, making the trace tree immediately readable in
// observability tools without any instrumentation code in individual repositories.
//
// Usage (in bootstrap):
//
//	jobs.NewRepository(
//	    database.WrapWithTracing(database.NewStoredProcedure[domain.JobEntity](pool)),
//	)
func WrapWithTracing[T any](inner StoredProcedurer[T]) StoredProcedurer[T] {
	return WrapWithInstrumenter(inner, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// WrapWithInstrumenter wraps a StoredProcedurer with an explicit instrumenter.
func WrapWithInstrumenter[T any](inner StoredProcedurer[T], instrumenter *telemetry.Instrumenter) StoredProcedurer[T] {
	return &tracingStoredProcedurer[T]{
		inner:        inner,
		instrumenter: instrumenter,
	}
}

type tracingStoredProcedurer[T any] struct {
	inner        StoredProcedurer[T]
	instrumenter *telemetry.Instrumenter
}

func (t *tracingStoredProcedurer[T]) QueryRow(ctx context.Context, query string, args ...any) (T, error) {
	ctx, span := t.pgSpan(ctx, query)
	defer span.End()
	return t.inner.QueryRow(ctx, query, args...)
}

func (t *tracingStoredProcedurer[T]) Query(ctx context.Context, query string, args ...any) ([]T, error) {
	ctx, span := t.pgSpan(ctx, query)
	defer span.End()
	return t.inner.Query(ctx, query, args...)
}

func (t *tracingStoredProcedurer[T]) QueryRowJSON(ctx context.Context, query string, jsonArg any, args ...any) (T, error) {
	ctx, span := t.pgSpan(ctx, query)
	defer span.End()
	return t.inner.QueryRowJSON(ctx, query, jsonArg, args...)
}

func (t *tracingStoredProcedurer[T]) QueryJSON(ctx context.Context, query string, jsonArg any, args ...any) ([]T, error) {
	ctx, span := t.pgSpan(ctx, query)
	defer span.End()
	return t.inner.QueryJSON(ctx, query, jsonArg, args...)
}

func (t *tracingStoredProcedurer[T]) pgSpan(ctx context.Context, sql string) (context.Context, telemetry.Span) {
	return t.instrumenter.Start(ctx, procSpanName(sql))
}

func procSpanName(sql string) string {
	if m := reProcName.FindStringSubmatch(sql); len(m) > 1 {
		return "postgres." + m[1]
	}
	return "postgres.query"
}
