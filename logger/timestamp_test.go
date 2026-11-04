package logger

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/trypanic/go-sdk/logger/otlp"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestLoggerToOTLPKeepsSubSecondTimestamp(t *testing.T) {
	exp := installRecordingProvider(t)
	l := New(Config{Env: Prod, Writer: &bytes.Buffer{}, OTLPWriter: otlp.NewOTLPWriter("svc")})

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
