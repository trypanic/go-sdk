// Package telemetry centralizes OpenTelemetry span creation for services
// built on the SDK.
//
// It exists to:
//   - avoid repeating otel.Tracer(...) at every call site;
//   - enforce one span naming convention across services;
//   - keep service identity out of span names;
//   - keep the go.opentelemetry.io/otel/trace import in one place
//     (Span is an alias of trace.Span).
//
// # Service identity
//
// The service name and version belong in the OpenTelemetry Resource
// attributes configured on the TracerProvider. Span names describe
// operations, never the service. The scope name given to NewInstrumenter
// (or InitTracer) is only the instrumentation scope.
//
// # Span naming
//
// Use the naming helpers instead of ad-hoc names:
//
//	Job(ctx, "import")                      job.import
//	Batch(ctx, "process")                   batch.process
//	External(ctx, "payments", "charge")     external.payments.charge
//	Messaging(ctx, "publish.orders")        messaging.publish.orders
//	DB(ctx, "postgres", "jobs", "insert")   postgres.jobs.insert
//
// A trace for a background import job then reads:
//
//	job.import
//	└── batch.process
//	     ├── external.catalog.fetchProducts
//	     ├── postgres.products.upsert
//	     └── messaging.publish.products-updated
//
// Pass bounded values only (operation and resource names, never IDs), so
// the number of distinct span names stays small.
//
// # Instrumenter and compatibility helpers
//
// Prefer an explicit *Instrumenter built with NewInstrumenter and passed to
// the components that create spans. The package-level helpers (Start, Job,
// Batch, External, Messaging, DB) use a package-global instrumenter that
// InitTracer replaces; they remain for existing callers. A nil *Instrumenter
// falls back to that global instrumenter. Both are safe for concurrent use.
//
// # Attributes and errors
//
// WithString, WithInt, WithBool and WithSpanKind are span start options;
// SetAttrString, SetAttrInt, SetAttrInt64 and SetAttrStringSlice set
// attributes on a running span; RecordError records an error and marks the
// span failed in one call.
package telemetry
