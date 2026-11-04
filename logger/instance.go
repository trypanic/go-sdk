package logger

import (
	"context"
	"io"

	"github.com/rs/zerolog"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// Config controls how an SDK logger is constructed. Zero values pick safe
// defaults so callers can pass `Config{}` and still get a working logger.
type Config struct {
	// AppName and Version are stamped on every Dev console log line as
	// service_name / service_version. They are required when Env == Dev to
	// match the legacy Init behavior.
	AppName string
	Version string

	// Env selects log level + writer. Empty string means DetectEnv().
	Env Environment

	// Writer overrides the default writer derived from Env. When nil the SDK
	// uses newWriter(Env, AppName, Version). Useful for tests or for fanning
	// out to a MultiLevelWriter.
	Writer io.Writer

	// OTLPWriter, when non-nil, is composed with Writer through
	// zerolog.MultiLevelWriter. The OTLP path requires InitProvider to
	// have run first.
	OTLPWriter io.Writer

	// Redactor, when non-nil, extends the built-in redaction policy (see
	// the package README, "Redaction"). It is called for every JSON object
	// field of each log line, at every nesting level, after the built-in
	// policy, with the already-redacted value; its return value is logged
	// instead. It cannot undo built-in masking. It must be safe for
	// concurrent use.
	Redactor func(key string, value any) any
}

// Logger is the explicit instance API. It owns a *zerolog.Logger and exposes
// the same logging surface as the global helpers, without touching package
// state. SDK consumers that do not want global side effects should use this.
type Logger struct {
	zl zerolog.Logger
}

// New builds a Logger from cfg. It does not mutate the package-level global.
func New(cfg Config) *Logger {
	env := cfg.Env
	if env == "" {
		env = DetectEnv()
	}

	base := cfg.Writer
	if base == nil {
		base = newWriter(env, cfg.AppName, cfg.Version)
	}

	w := base
	if cfg.OTLPWriter != nil {
		w = zerolog.MultiLevelWriter(base, cfg.OTLPWriter)
	}

	return &Logger{zl: newLoggerWithRedactor(w, env, cfg.Redactor)}
}

// Zerolog exposes the underlying zerolog.Logger for low-level event building.
func (l *Logger) Zerolog() *zerolog.Logger { return &l.zl }

// Info logs a printf-style Info message.
func (l *Logger) Info(msg string, args ...any) {
	l.zl.Info().Msgf(msg, args...)
}

// Warn logs a printf-style Warn message.
func (l *Logger) Warn(msg string, args ...any) {
	l.zl.Warn().Msgf(msg, args...)
}

// Error logs an errorkit.AppError or wraps a plain error before logging.
// An *errorkit.AppError anywhere in appErr's chain keeps its own code; a nil
// error is logged as ERR_UNKNOWN. Returns the original error unchanged for
// chaining.
func (l *Logger) Error(appErr error, msg ...string) error {
	errorMsg := ""
	if len(msg) > 0 {
		errorMsg = msg[0]
	}
	withError(l.zl.Error(), appErr).Msg(errorMsg)
	return appErr
}

// InfoCtx logs Info enriched with OTel trace_id/span_id from ctx.
func (l *Logger) InfoCtx(ctx context.Context, msg string) {
	WithTrace(ctx, l.zl.Info()).Msg(msg)
}

// ErrorCtx logs an AppError at Error level with OTel trace fields.
func (l *Logger) ErrorCtx(ctx context.Context, appErr *errorkit.AppError, msg string) {
	event := WithTrace(ctx, l.zl.Error())
	WithErrorKit(event, appErr).Msg(msg)
}

// WarnCtx logs an AppError at Warn level with OTel trace fields.
func (l *Logger) WarnCtx(ctx context.Context, appErr *errorkit.AppError, msg string) {
	event := WithTrace(ctx, l.zl.Warn())
	WithErrorKit(event, appErr).Msg(msg)
}

// WithFields returns a child logger with the supplied static fields attached.
func (l *Logger) WithFields(fields map[string]any) *Logger {
	ctx := l.zl.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{zl: ctx.Logger()}
}

// IntoContext stores the underlying zerolog.Logger into ctx so downstream
// code can retrieve it via Ctx / CtxOrGlobal.
func (l *Logger) IntoContext(ctx context.Context) context.Context {
	return l.zl.WithContext(ctx)
}
