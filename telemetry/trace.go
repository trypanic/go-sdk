package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Package telemetry centralizes span creation for the service.
//
// Why this exists:
// - Avoid repeating otel.Tracer(...) everywhere
// - Enforce consistent span naming conventions
// - Keep service identity (name/version) separate from span names
// - Reduce instrumentation mistakes
//
// IMPORTANT DESIGN DECISION:
// - Service name and version belong in OpenTelemetry Resource attributes
// - Span names describe operations, not service identity
//
// Level 1: Business Operations
// job.import
// batch.process
// content.enrich
//
// Level 2: External Operations
// external.taobao.importProductDetail
// external.warehouse.updateProduct
//
// Level 3: Messaging Operations
// messaging.publish.channel-a
// messaging.consume.channel-b
//
// What you'll see in signoz
// Service: taobao-ingestion (v1)
//
// job.import
//
//	└── batch.process
//	     ├── external.taobao.importProductsOverview
//	     ├── external.taobao.importProductDetail
//	     ├── external.taobao.importPurchaseOrderRender
//	     └── messaging.publish.channel-a

// Span is a type alias for trace.Span.
//
// Callers that import go-pkgs/telemetry do not need to import
// go.opentelemetry.io/otel/trace directly — use telemetry.Span wherever
// you would otherwise write trace.Span.
type Span = trace.Span

var tracer trace.Tracer = otel.Tracer("app") // safe fallback default

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
	scopeName := config.ScopeName
	if scopeName == "" {
		scopeName = "app"
	}

	provider := config.TracerProvider
	if provider == nil {
		provider = otel.GetTracerProvider()
	}

	return &Instrumenter{
		tracer: provider.Tracer(scopeName),
	}
}

// SpanFromContext retrieves the current span from the context.
//
// Use this instead of trace.SpanFromContext(ctx) directly to keep all OTel
// dependencies centralized in this package.
//
// The typical use case is enriching a span created by a higher-level
// abstraction (e.g. Hertz OTel middleware, go-pkgs/messaging) with
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
// This MUST be called once during service startup (typically in main.go)
// after the OpenTelemetry provider has been configured.
//
// Example (in main.go):
//
//	telemetry.InitTracer(serviceName)
//
// NOTE:
// - The tracer name is NOT the service name in observability terms.
// - The actual service identity is defined via OpenTelemetry Resource attributes.
// - This tracer name is just the instrumentation scope.
func InitTracer(serviceName string) {
	tracer = otel.Tracer(serviceName)
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
// - Third-party APIs (Taobao, LLM, Warehouse)
// - External HTTP calls
// - External service integrations
//
// Example:
//
//	ctx, span := telemetry.External(ctx, "taobao", "importProductDetail")
//	defer span.End()
//
// Resulting span name:
//
//	"external.taobao.importProductDetail"
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
//	ctx, span := telemetry.DB(ctx, "mongo", "taobao_listings_raw", "insert")
//	defer span.End()
//
// Resulting span name:
//
//	"mongo.taobao_listings_raw.insert"
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
	return &Instrumenter{tracer: tracer}
}
