package logger

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestLogLineTimestampHasSubSecondPrecision(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})

	before := time.Now()
	l.Info("precise")
	after := time.Now()

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	raw, ok := line["time"].(string)
	require.Truef(t, ok, "time field = %#v, want an RFC 3339 string", line["time"])

	ts, err := time.Parse(time.RFC3339Nano, raw)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, ts.Location(), "time must be written in UTC")
	// With whole-second precision the logged time is truncated and falls
	// before the instant captured just before the call.
	assert.False(t, ts.Before(before.Truncate(time.Microsecond)), "logged %s is before %s", ts, before)
	assert.False(t, ts.After(after), "logged %s is after %s", ts, after)
}

func TestOTLPWriterKeepsTimestampAndIntegerPrecision(t *testing.T) {
	exp := installRecordingProvider(t)
	w := NewOTLPWriter("svc")

	line := `{"level":"info","message":"m","time":"2026-11-03T10:00:00.123456789Z",` +
		`"snowflake":9007199254740993,"ratio":0.5,"negative":-42}` + "\n"
	_, err := w.Write([]byte(line))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	want := time.Date(2026, 11, 3, 10, 0, 0, 123456789, time.UTC)
	assert.True(t, records[0].Timestamp().Equal(want), "timestamp = %s, want %s", records[0].Timestamp(), want)

	attrs := map[string]attribute.Value{}
	records[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value
		return true
	})
	require.Contains(t, attrs, "snowflake")
	assert.Equal(t, attribute.INT64, attrs["snowflake"].Type())
	assert.Equal(t, int64(9007199254740993), attrs["snowflake"].AsInt64())
	assert.Equal(t, int64(-42), attrs["negative"].AsInt64())
	assert.Equal(t, attribute.FLOAT64, attrs["ratio"].Type())
	assert.InDelta(t, 0.5, attrs["ratio"].AsFloat64(), 0)
}

func TestOTLPWriterAcceptsLegacyUnixSeconds(t *testing.T) {
	exp := installRecordingProvider(t)
	w := NewOTLPWriter("svc")

	_, err := w.Write([]byte(`{"level":"info","message":"m","time":1793753281}`))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.True(t, records[0].Timestamp().Equal(time.Unix(1793753281, 0)))
}

func TestLoggerToOTLPKeepsSubSecondTimestamp(t *testing.T) {
	exp := installRecordingProvider(t)
	l := New(Config{Env: Prod, Writer: &bytes.Buffer{}, OTLPWriter: NewOTLPWriter("svc")})

	before := time.Now()
	l.Info("to otlp")

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.False(t, records[0].Timestamp().Before(before.Truncate(time.Microsecond)),
		"record timestamp %s lost sub-second precision (before %s)", records[0].Timestamp(), before)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestDevConsoleShowsClockTime guards the console writer: it must render the
// RFC 3339 time as HH:MM:SS whatever zerolog.TimeFieldFormat holds.
func TestDevConsoleShowsClockTime(t *testing.T) {
	previous := zerolog.TimeFieldFormat
	t.Cleanup(func() { zerolog.TimeFieldFormat = previous })

	for _, format := range []string{zerolog.TimeFormatUnix, time.RFC3339} {
		zerolog.TimeFieldFormat = format

		cw, ok := newWriter(Dev, "app", "1.0").(zerolog.ConsoleWriter)
		require.True(t, ok)
		var buf bytes.Buffer
		cw.Out = &buf

		l := newLoggerWithWriter(cw, Dev)
		l.Info().Msg("hello")

		plain := ansiEscape.ReplaceAllString(buf.String(), "")
		assert.Regexp(t, regexp.MustCompile(`^\d{2}:\d{2}:\d{2} `), plain,
			"TimeFieldFormat %q", format)
	}
}
