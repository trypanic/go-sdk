package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/trypanic/go-sdk/logger/otlp"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/errorkit"
)

var (
	testTraceID = trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	testSpanID  = trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
)

// spanContext returns a context carrying a valid, sampled span context.
func spanContext(t *testing.T) context.Context {
	t.Helper()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    testTraceID,
		SpanID:     testSpanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(t.Context(), sc)
}

// decodeLines decodes every JSON line written to buf.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "line %q", line)
		lines = append(lines, m)
	}
	return lines
}

func TestGlobalHelpersWriteThroughGlobalLogger(t *testing.T) {
	buf := useGlobalBuffer(t)
	ctx := spanContext(t)
	appErr := errorkit.NewError(errorkit.ERR_VALIDATION).With(errorkit.WithReason("bad input"))

	Info("info %d", 1)
	Warn("warn %s", "two")
	require.Same(t, appErr, LogAndReturn(appErr, "returned"))
	ErrorCtx(ctx, appErr, "error ctx")
	WarnCtx(ctx, appErr, "warn ctx")
	LogInfoWithTrace(ctx, "info trace")
	LogErrorWithTrace(ctx, appErr, "error trace")
	LogWarnWithTrace(ctx, appErr, "warn trace")

	lines := decodeLines(t, buf)
	want := []struct {
		level, message string
		traced, coded  bool
	}{
		{level: "info", message: "info 1"},
		{level: "warn", message: "warn two"},
		{level: "error", message: "returned", coded: true},
		{level: "error", message: "error ctx", traced: true, coded: true},
		{level: "warn", message: "warn ctx", traced: true, coded: true},
		{level: "info", message: "info trace", traced: true},
		{level: "error", message: "error trace", traced: true, coded: true},
		{level: "warn", message: "warn trace", traced: true, coded: true},
	}
	require.Len(t, lines, len(want))
	for i, w := range want {
		line := lines[i]
		assert.Equal(t, w.level, line["level"], "line %d", i)
		assert.Equal(t, w.message, line["message"], "line %d", i)
		if w.traced {
			assert.Equal(t, testTraceID.String(), line["trace_id"], "line %d", i)
			assert.Equal(t, testSpanID.String(), line["span_id"], "line %d", i)
		} else {
			assert.NotContains(t, line, "trace_id", "line %d", i)
		}
		if w.coded {
			assert.Equal(t, string(errorkit.ERR_VALIDATION), line["error_code"], "line %d", i)
		}
	}
}

func TestCtxOrGlobalAndWithLogger(t *testing.T) {
	globalBuf := useGlobalBuffer(t)

	CtxOrGlobal(t.Context()).Info().Msg("to global")
	assert.Contains(t, globalBuf.String(), "to global")

	var ctxBuf bytes.Buffer
	ctxLogger := zerolog.New(&ctxBuf)
	ctx := WithLogger(t.Context(), &ctxLogger)
	CtxOrGlobal(ctx).Info().Msg("to ctx")
	assert.Contains(t, ctxBuf.String(), "to ctx")
	assert.NotContains(t, globalBuf.String(), "to ctx")
}

func TestTraceIDFromCtx(t *testing.T) {
	assert.Equal(t, testTraceID.String(), TraceIDFromCtx(spanContext(t)))
	assert.Empty(t, TraceIDFromCtx(t.Context()))
}

func TestLoggerInstanceWarnAndInfoCtx(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})

	l.Warn("careful %d", 3)
	l.InfoCtx(spanContext(t), "traced")
	l.WarnCtx(spanContext(t), errorkit.NewError(errorkit.ERR_INTERNAL), "warned")

	lines := decodeLines(t, &buf)
	require.Len(t, lines, 3)
	assert.Equal(t, "warn", lines[0]["level"])
	assert.Equal(t, "careful 3", lines[0]["message"])
	assert.Equal(t, testTraceID.String(), lines[1]["trace_id"])
	assert.Equal(t, "warn", lines[2]["level"])
	assert.Equal(t, string(errorkit.ERR_INTERNAL), lines[2]["error_code"])
}

// TestInitConfiguresGlobals checks the one function allowed to mutate
// zerolog globals: Init installs the package global and zerolog's
// log.Logger, sets the global level for the environment and the time format.
func TestInitConfiguresGlobals(t *testing.T) {
	previousGlobal, previousZlog := global.Load(), zlog.Logger
	previousLevel, previousFormat := zerolog.GlobalLevel(), zerolog.TimeFieldFormat
	t.Cleanup(func() {
		global.Store(previousGlobal)
		zlog.Logger = previousZlog
		zerolog.SetGlobalLevel(previousLevel)
		zerolog.TimeFieldFormat = previousFormat
	})
	t.Setenv("ENVIRONMENT", "production")

	l, closer := Init("svc", "1.0.0")

	require.NotNil(t, l)
	require.NoError(t, closer.Close())
	assert.Same(t, l, global.Load())
	assert.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
	assert.Equal(t, time.RFC3339Nano, zerolog.TimeFieldFormat)
	assert.Equal(t, zerolog.InfoLevel, zlog.Logger.GetLevel())
}

func TestInitWithOTLPWriterFansOut(t *testing.T) {
	previousGlobal, previousZlog := global.Load(), zlog.Logger
	previousLevel, previousFormat := zerolog.GlobalLevel(), zerolog.TimeFieldFormat
	t.Cleanup(func() {
		global.Store(previousGlobal)
		zlog.Logger = previousZlog
		zerolog.SetGlobalLevel(previousLevel)
		zerolog.TimeFieldFormat = previousFormat
	})
	t.Setenv("ENVIRONMENT", "production")
	exp := installRecordingProvider(t)

	l, _ := Init("svc", "1.0.0", otlp.NewOTLPWriter("svc"))
	l.Info().Msg("fan out")

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, "fan out", records[0].Body().AsString())
}
