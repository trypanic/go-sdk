package logger

import (
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// global holds the application logger instance
var global *zerolog.Logger

func globalLogger() *zerolog.Logger {
	if global != nil {
		return global
	}
	return fallbackLogger()
}

// fallbackLogger is the logger used before Init: plain JSON to stderr,
// built once and reused.
var fallbackLogger = sync.OnceValue(func() *zerolog.Logger {
	l := zerolog.New(os.Stderr).With().Timestamp().Logger()
	return &l
})

// Init creates and configures the logger for the environment DetectEnv
// returns. It sets the package-level global, zerolog's global log.Logger,
// zerolog's global level and zerolog.TimeFieldFormat: it is the only SDK
// function that mutates zerolog globals. Use New for an instance that
// touches no global state.
//
// An optional OTLPWriter ships every log event to an OpenTelemetry
// collector as well as to the console/stdout writer. The OTLP export uses
// its own connection, configured by NewLogProvider or InitLogProvider.
//
// Returns the logger and an io.Closer that does nothing (kept for forward
// compatibility); the OTLP lifecycle belongs to the LogProviderHandle.
//
// Usage without OTLP:
//
//	l, cleanup := logger.Init("my-service", "1.0.0")
//	defer cleanup.Close()
//
// Usage with OTLP:
//
//	lp := logger.InitLogProvider(ctx, "my-service", "otel-collector:4317")
//	defer lp.Shutdown(ctx)
//	l, cleanup := logger.Init("my-service", "1.0.0", logger.NewOTLPWriter("my-service"))
//	defer cleanup.Close()
func Init(appName, version string, otlpWriter ...*OTLPWriter) (*zerolog.Logger, io.Closer) {
	env := DetectEnv()
	applyGlobalSettings(env)

	var l zerolog.Logger

	if len(otlpWriter) > 0 && otlpWriter[0] != nil {
		// Fan out every log event to both the local console/stdout writer
		// AND the OTEL Collector. newWriter() returns the same io.Writer
		// that newLogger() would have used, so dev pretty-printing is preserved.
		baseWriter := newWriter(env, appName, version)
		multi := zerolog.MultiLevelWriter(baseWriter, otlpWriter[0])
		l = newLoggerWithWriter(multi, env)
	} else {
		l = newLogger(env, appName, version)
	}

	global = &l
	log.Logger = l
	return global, io.NopCloser(nil)
}
