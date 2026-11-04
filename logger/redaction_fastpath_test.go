package logger

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestRedactingWriterPassesThroughLinesWithoutSensitiveContent(t *testing.T) {
	line := `{"level":"info","zeta":"1","alpha":2,"message":"plain"}` + "\n"
	var out bytes.Buffer
	w := newRedactingWriter(&out, nil)

	n, err := w.Write([]byte(line))
	if err != nil || n != len(line) {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(line))
	}
	if out.String() != line {
		t.Fatalf("a line without sensitive content must pass through unchanged:\n got %q\nwant %q", out.String(), line)
	}
}

func TestRedactingWriterSlowPathTriggers(t *testing.T) {
	tests := []struct {
		name string
		line string
		leak string
	}{
		{name: "mixed case key", line: `{"Proxy-Authorization":"Bearer abc"}`, leak: "Bearer abc"},
		{name: "separated key", line: `{"API_Key":"k-123"}`, leak: "k-123"},
		{name: "url userinfo", line: `{"message":"dial postgres://u:pw1@db/app"}`, leak: "pw1"},
		{name: "email", line: `{"contact_email":"jane@example.com"}`, leak: "example.com"},
		{name: "escaped key in nested JSON", line: `{"body":"{\"\\u0070assword\":\"hunter2\"}"}`, leak: "hunter2"},
		{name: "escaped unicode lower-cases to ascii", line: `{"to\u212Aen":"tok-1"}`, leak: "tok-1"},
		{name: "raw unicode lower-cases to ascii", line: "{\"to\u212Aen\":\"tok-2\"}", leak: "tok-2"},
		{name: "tab escape inside key", line: `{"to\tken":"tok-3"}`, leak: "tok-3"},
		{name: "carriage return escape inside key", line: `{"api\rkey":"k-456"}`, leak: "k-456"},
		{name: "newline escape inside key", line: `{"pass\nword":"hunter3"}`, leak: "hunter3"},
		{name: "backspace and form feed escapes inside key", line: `{"sec\bret":"s-1","to\fken":"tok-4"}`, leak: "s-1"},
		{name: "escaped slashes hide the URL scheme separator", line: `{"payload":"{\"url\":\"postgres:\\/\\/u:s3cr3tpw@h/db\"}"}`, leak: "s3cr3tpw"},
		{name: "escaped slashes in a plain value", line: `{"message":"dial postgres:\/\/u:pw9@db/app"}`, leak: "pw9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if _, err := newRedactingWriter(&out, nil).Write([]byte(tc.line)); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if strings.Contains(out.String(), tc.leak) {
				t.Fatalf("sensitive value leaked: %q", out.String())
			}
		})
	}
}

// TestLoggerRedactsKeysWithEscapedControlCharacters goes through a real
// logger: zerolog writes control characters in keys as two-character JSON
// escapes (\t, \r, \n, \b, \f) and the decoded key drops them, so these
// keys are sensitive and must be masked.
func TestLoggerRedactsKeysWithEscapedControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	l := newLoggerWithWriter(&buf, Prod)

	l.Info().
		Str("to\tken", "tok-e2e").
		Str("api\rkey", "key-e2e").
		Str("pass\nword", "pw-e2e").
		Str("sec\bret", "sec-e2e").
		Str("x_to\fken", "ff-e2e").
		Str("payload", `{"url":"postgres:\/\/u:s3cr3tpw@h/db"}`).
		Msg("escaped keys")

	out := buf.String()
	for _, leak := range []string{"tok-e2e", "key-e2e", "pw-e2e", "sec-e2e", "ff-e2e", "s3cr3tpw"} {
		if strings.Contains(out, leak) {
			t.Errorf("value %q leaked: %s", leak, out)
		}
	}
	if !strings.Contains(out, "escaped keys") {
		t.Fatalf("log line missing: %q", out)
	}
}

func TestCustomRedactorAlwaysRuns(t *testing.T) {
	var out bytes.Buffer
	w := newRedactingWriter(&out, func(key string, value any) any {
		if key == "plain" {
			return "masked"
		}
		return value
	})
	if _, err := w.Write([]byte(`{"plain":"visible"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(out.String(), `"plain":"masked"`) {
		t.Fatalf("custom redactor did not run: %q", out.String())
	}
}

func TestFallbackLoggerIsCached(t *testing.T) {
	previous := global.Load()
	t.Cleanup(func() { global.Store(previous) })
	global.Store(nil)

	first, second := globalLogger(), globalLogger()
	if first != second {
		t.Fatal("globalLogger must reuse one fallback logger when Init was not called")
	}
}

func BenchmarkLoggerInfoPlain(b *testing.B) {
	l := New(Config{Env: Prod, Writer: io.Discard})
	b.ReportAllocs()
	for b.Loop() {
		l.Zerolog().Info().Str("request_id", "rid-42").Int("status", 200).Msg("request served")
	}
}

func BenchmarkLoggerInfoSensitive(b *testing.B) {
	l := New(Config{Env: Prod, Writer: io.Discard})
	b.ReportAllocs()
	for b.Loop() {
		l.Zerolog().Info().Str("request_id", "rid-42").Str("token", "tok-secret").Msg("request served")
	}
}

func BenchmarkLoggerErrorCtx(b *testing.B) {
	l := New(Config{Env: Prod, Writer: io.Discard})
	appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithReason("boom"))
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		l.ErrorCtx(ctx, appErr, "operation failed")
	}
}
