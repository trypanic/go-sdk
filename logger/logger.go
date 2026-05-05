package logger

import (
	"io"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// global holds the application logger instance
var global *zerolog.Logger

func globalLogger() *zerolog.Logger {
	if global != nil {
		return global
	}

	l := zerolog.New(os.Stderr).With().Timestamp().Logger()
	return &l
}

// Init creates and configures the logger for the given environment.
// It sets both the package-level global AND zerolog's global log.Logger.
//
// An optional OTLPWriter can be passed to ship logs directly to SigNoz
// over the same gRPC connection used for traces. When provided, each log
// event is written to both the console/stdout AND the OTEL Collector.
//
// Returns the logger and a cleanup function (no-op for zerolog itself;
// the OTLPWriter lifecycle is managed by the LoggerProvider in main.go).
//
// Usage without OTLP (dev / no SigNoz):
//
//	l, cleanup := logger.Init("my-service", "1.0.0")
//	defer cleanup.Close()
//
// Usage with OTLP (production + SigNoz):
//
//	lp := logger.InitLogProvider(ctx, "my-service", "otel-collector:4317")
//	defer lp.Shutdown(ctx)
//	l, cleanup := logger.Init("my-service", "1.0.0", logger.NewOTLPWriter("my-service"))
//	defer cleanup.Close()
func Init(appName, version string, otlpWriter ...*OTLPWriter) (*zerolog.Logger, io.Closer) {
	env := DetectEnv()

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
