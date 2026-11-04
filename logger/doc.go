// Package logger provides structured logging on zerolog with errorkit
// integration, OpenTelemetry trace correlation, built-in redaction and an
// optional OTLP export.
//
// # Global and instance APIs
//
// Init configures the package-level global logger used by Info, Warn,
// Error, ErrorCtx and the other package functions. It is the only SDK
// function that mutates zerolog's process-wide state (global level, time
// format, log.Logger); call it once, early in main. New builds an explicit
// *Logger from a Config without touching any global state; prefer it in
// libraries and tests.
//
// The environment (DetectEnv, the ENVIRONMENT variable) selects the output:
// Prod writes JSON to stdout at Info level and is the default; Dev writes a
// colored console format at Debug level.
//
// # Errors and traces
//
// ErrorCtx, WarnCtx and the Log*WithTrace helpers attach the trace_id and
// span_id of the active span in ctx (WithTrace) and the code, metadata and
// stack of an *errorkit.AppError (WithErrorKit). Error and Logger.Error
// accept any error: an *errorkit.AppError anywhere in the chain keeps its
// code, anything else is wrapped once in ERR_INTERNAL.
//
// # Redaction
//
// Every logger built by Init or New masks sensitive keys (passwords,
// tokens, secrets, authorization, cookies, API keys, DSNs), email domains
// and URL passwords before a line reaches any writer; Config.Redactor
// extends the policy. See the package README for the full rules.
//
// # OTLP
//
// logger/otlp.NewProvider creates the OTLP/gRPC log exporter (TLS by default) and
// registers it as the global OTel LoggerProvider; otlp.NewWriter then turns
// each log line into an OTel log record. Pass the writer to Init or
// Config.OTLPWriter and defer otlp.Provider.Close.
//
// # Concurrency
//
// Loggers are safe for concurrent use; the exporter has its own lifecycle.
// Init is not: call it before logging starts.
package logger
