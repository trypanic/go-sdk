package postgres

import (
	"context"
	"regexp"

	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/v2/telemetry"
)

// reProcName extracts an unquoted, optionally schema-qualified routine from
// SELECT ... FROM routine(...) or CALL routine(...).
var reProcName = regexp.MustCompile(`(?i)\b(?:FROM|CALL)\s+([a-z_][a-z0-9_$]*(?:\.[a-z_][a-z0-9_$]*)?)\s*\(`)

// WrapWithTracing wraps a StoredProcedurer with automatic span creation.
//
// Each call to QueryRow, Query, QueryRowJSON, or QueryJSON opens a span
// named after the stored procedure extracted from the SQL string:
//
//	"SELECT * FROM check_or_create_job($1, ...)" → span: "postgres.check_or_create_job.query"
//
// The span becomes the parent of all pgx-level spans (prepare, query, pool.acquire)
// produced by the otelpgx tracer, making the trace tree immediately readable in
// observability tools without any instrumentation code in individual repositories.
//
// Usage (in bootstrap), on an untraced StoredProcedure value:
//
//	jobs.NewRepository(
//	    postgres.WrapWithTracing[domain.JobEntity](postgres.StoredProcedure[domain.JobEntity]{Pool: pool}),
//	)
//
// NewStoredProcedure already applies this wrapper; wrapping its result again
// opens two spans with the same name per call.
func WrapWithTracing[T any](inner StoredProcedurer[T]) StoredProcedurer[T] {
	return WrapWithInstrumenter(inner, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// WrapWithInstrumenter wraps a StoredProcedurer with an explicit instrumenter.
// A nil instrumenter disables tracing: inner is returned unchanged.
func WrapWithInstrumenter[T any](inner StoredProcedurer[T], instrumenter *telemetry.Instrumenter) StoredProcedurer[T] {
	if instrumenter == nil {
		return inner
	}
	return &TracedProcedure[T]{
		inner:        inner,
		instrumenter: instrumenter,
	}
}

// TracedProcedure adds telemetry to a stored procedure implementation.
type TracedProcedure[T any] struct {
	inner        StoredProcedurer[T]
	instrumenter *telemetry.Instrumenter
}

// QueryRow delegates to the wrapped procedure within a classified client span.
func (t *TracedProcedure[T]) QueryRow(ctx context.Context, query string, args ...any) (result T, err error) {
	ctx, span := t.pgSpan(ctx, query)
	defer func() { telemetry.End(span, err) }()
	return t.inner.QueryRow(ctx, query, args...)
}

// Query delegates to the wrapped procedure within a classified client span.
func (t *TracedProcedure[T]) Query(ctx context.Context, query string, args ...any) (result []T, err error) {
	ctx, span := t.pgSpan(ctx, query)
	defer func() { telemetry.End(span, err) }()
	return t.inner.Query(ctx, query, args...)
}

// QueryRowJSON delegates to the wrapped procedure within a classified client span.
func (t *TracedProcedure[T]) QueryRowJSON(ctx context.Context, query string, jsonArg any, args ...any) (result T, err error) {
	ctx, span := t.pgSpan(ctx, query)
	defer func() { telemetry.End(span, err) }()
	return t.inner.QueryRowJSON(ctx, query, jsonArg, args...)
}

// QueryJSON delegates to the wrapped procedure within a classified client span.
func (t *TracedProcedure[T]) QueryJSON(ctx context.Context, query string, jsonArg any, args ...any) (result []T, err error) {
	ctx, span := t.pgSpan(ctx, query)
	defer func() { telemetry.End(span, err) }()
	return t.inner.QueryJSON(ctx, query, jsonArg, args...)
}

func (t *TracedProcedure[T]) pgSpan(ctx context.Context, sql string) (context.Context, telemetry.Span) {
	return t.instrumenter.DB(ctx, "postgres", procName(sql), "query", telemetry.WithSpanKind(trace.SpanKindClient), telemetry.WithString("db.system", "postgresql"))
}

func procName(sql string) string {
	if m := reProcName.FindStringSubmatch(sql); len(m) > 1 {
		return m[1]
	}
	return "unknown"
}

func procSpanName(sql string) string { return "postgres." + procName(sql) + ".query" }

// Unwrap returns the stored procedure implementation enclosed by this decorator.
func (t *TracedProcedure[T]) Unwrap() StoredProcedurer[T] { return t.inner }
