package telemetry

import (
	"cmp"
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Span is a type alias for trace.Span.
//
// Callers that import telemetry do not need to import
// go.opentelemetry.io/otel/trace directly — use telemetry.Span wherever
// you would otherwise write trace.Span.
type Span = trace.Span

// compat is the instrumenter behind the package-level compatibility helpers
// (Start, Job, Batch, External, Messaging, DB) and the nil-Instrumenter
// fallback. InitTracer replaces it atomically, so it may race with span
// creation safely.
var compat atomic.Pointer[Instrumenter]

func init() {
	compat.Store(&Instrumenter{tracer: otel.Tracer("app")}) // safe fallback default
}

// Instrumenter creates spans from an explicit tracer.
type Instrumenter struct {
	tracer trace.Tracer
}

// InstrumenterConfig controls explicit telemetry construction.
type InstrumenterConfig struct {
	ScopeName      string
	TracerProvider trace.TracerProvider
}

// NewInstrumenter creates an explicit span factory. If no provider is supplied,
// it uses the currently configured global OpenTelemetry provider.
func NewInstrumenter(config InstrumenterConfig) *Instrumenter {
	provider := config.TracerProvider
	if provider == nil {
		provider = otel.GetTracerProvider()
	}

	return &Instrumenter{
		tracer: provider.Tracer(cmp.Or(config.ScopeName, "app")),
	}
}

// SpanFromContext retrieves the current span from the context.
//
// Use this instead of trace.SpanFromContext(ctx) directly to keep all OTel
// dependencies centralized in this package.
//
// The typical use case is enriching a span created by a higher-level
// abstraction (e.g. an HTTP server middleware or the SDK messaging package) with
// business attributes — without creating a new child span.
//
// Example:
//
//	span := telemetry.SpanFromContext(ctx)
//	telemetry.SetAttrString(span, "job.id", jobID)
//	telemetry.RecordError(span, err, "operation failed")
func SpanFromContext(ctx context.Context) Span {
	return trace.SpanFromContext(ctx)
}

// InitTracer initializes the global tracer for this service.
//
// Call it once during service startup, after the OpenTelemetry provider
// has been configured. It only affects the package-level compatibility
// helpers; prefer NewInstrumenter in new code.
//
// Example (in main):
//
//	telemetry.InitTracer(serviceName)
//
// NOTE:
// - The tracer name is NOT the service name in observability terms.
// - The actual service identity is defined via OpenTelemetry Resource attributes.
// - This tracer name is just the instrumentation scope.
func InitTracer(serviceName string) {
	compat.Store(&Instrumenter{tracer: otel.Tracer(serviceName)})
}

// Start creates a new span with the given name.
//
// This is the most generic span creation method.
// It should be used when none of the helper functions (Job, Batch, External, Messaging)
// fit your use case.
//
// Parameters:
// - ctx: parent context (MUST always be propagated)
// - name: span name (should follow naming convention)
// - opts: optional span start options (e.g. SpanKind)
//
// Returns:
// - Updated context containing the new span
// - The span itself (must call span.End())
//
// Example:
//
//	ctx, span := telemetry.Start(ctx, "job.import")
//	defer span.End()
func Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return compatibilityInstrumenter().Start(ctx, name, opts...)
}

// Start creates a new span with the given name.
func (i *Instrumenter) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, Span) {
	if i == nil || i.tracer == nil {
		return compatibilityInstrumenter().Start(ctx, name, opts...)
	}
	return i.tracer.Start(ctx, name, opts...)
}

// Job creates a span for a high-level job operation.
//
// Naming convention:
//
//	job.<operation>
//
// Use this for:
// - Long-running background jobs
// - Main business workflows
// - Asynchronous job processing
//
// Example:
//
//	ctx, span := telemetry.Job(ctx, "import")
//	defer span.End()
//
// Resulting span name:
//
//	"job.import"
func Job(ctx context.Context, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return compatibilityInstrumenter().Job(ctx, op, opts...)
}

// Job creates a span for a high-level job operation.
func (i *Instrumenter) Job(ctx context.Context, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return i.Start(ctx, "job."+op, opts...)
}

// Batch creates a span for a batch processing operation.
//
// Naming convention:
//
//	batch.<operation>
//
// Use this for:
// - Per-batch processing loops
// - Paginated external fetches
// - Chunk-based processing steps
//
// Example:
//
//	ctx, span := telemetry.Batch(ctx, "process")
//	defer span.End()
//
// Resulting span name:
//
//	"batch.process"
func Batch(ctx context.Context, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return compatibilityInstrumenter().Batch(ctx, op, opts...)
}

// Batch creates a span for a batch processing operation.
func (i *Instrumenter) Batch(ctx context.Context, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return i.Start(ctx, "batch."+op, opts...)
}

// External creates a span for calls to external systems.
//
// Naming convention:
//
//	external.<system>.<operation>
//
// Use this for:
// - Third-party APIs (payment providers, LLMs, partner services)
// - External HTTP calls
// - External service integrations
//
// Example:
//
//	ctx, span := telemetry.External(ctx, "payments", "charge")
//	defer span.End()
//
// Resulting span name:
//
//	"external.payments.charge"
//
// This makes filtering external dependencies very easy in observability tools.
func External(ctx context.Context, system string, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return compatibilityInstrumenter().External(ctx, system, op, opts...)
}

// External creates a span for calls to external systems.
func (i *Instrumenter) External(ctx context.Context, system string, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return i.Start(ctx, "external."+system+"."+op, opts...)
}

// Messaging creates a span for messaging operations (RabbitMQ, Kafka, etc).
//
// Naming convention:
//
//	messaging.<operation>
//
// Use this for:
// - Publishing messages
// - Consuming messages
// - Acknowledging messages
//
// You should also specify SpanKind when appropriate:
// - Producer for publish
// - Consumer for consume
//
// Example (producer):
//
//	ctx, span := telemetry.Messaging(
//		ctx,
//		"publish.channel-a",
//		trace.WithSpanKind(trace.SpanKindProducer),
//	)
//	defer span.End()
//
// Resulting span name:
//
//	"messaging.publish.channel-a"
func Messaging(ctx context.Context, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return compatibilityInstrumenter().Messaging(ctx, op, opts...)
}

// Messaging creates a span for messaging operations.
func (i *Instrumenter) Messaging(ctx context.Context, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return i.Start(ctx, "messaging."+op, opts...)
}

// DB creates a span for database operations.
//
// Naming convention:
//
//	<driver>.<resource>.<operation>
//
// Use this for:
// - MongoDB collection operations
// - PostgreSQL stored procedure calls
//
// Example (MongoDB):
//
//	ctx, span := telemetry.DB(ctx, "mongo", "listings", "insert")
//	defer span.End()
//
// Resulting span name:
//
//	"mongo.listings.insert"
//
// Example (PostgreSQL):
//
//	ctx, span := telemetry.DB(ctx, "postgres", "jobs", "checkOrCreate")
//	defer span.End()
//
// Resulting span name:
//
//	"postgres.jobs.checkOrCreate"
func DB(ctx context.Context, driver string, resource string, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return compatibilityInstrumenter().DB(ctx, driver, resource, op, opts...)
}

// DB creates a span for database operations.
func (i *Instrumenter) DB(ctx context.Context, driver string, resource string, op string, opts ...trace.SpanStartOption) (context.Context, Span) {
	return i.Start(ctx, driver+"."+resource+"."+op, opts...)
}

func compatibilityInstrumenter() *Instrumenter {
	return compat.Load()
}
