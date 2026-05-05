package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestConsoleWriterFormatsExtraFieldsWithoutDuplicatingCoreParts(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	writer := newWriter(Dev, "test-service", "0.1.0")
	consoleWriter, ok := writer.(zerolog.ConsoleWriter)
	if !ok {
		t.Fatalf("newWriter() returned %T, want zerolog.ConsoleWriter", writer)
	}

	var out bytes.Buffer
	consoleWriter.Out = &out
	consoleWriter.NoColor = true

	logger := zerolog.New(consoleWriter).With().Timestamp().Caller().Logger()
	logger.Info().
		Str("account_id", "acc_123").
		Str("account_label", "prime").
		Msg("create complete")

	got := stripConsoleANSI(out.String())

	if !strings.Contains(got, "service_name=test-service service_version=0.1.0") {
		t.Fatalf("expected service metadata to be separated, got %q", got)
	}

	if !strings.Contains(got, "account_id=acc_123 account_label=prime") {
		t.Fatalf("expected extra fields to be separated, got %q", got)
	}

	if strings.Contains(got, "message=create complete") {
		t.Fatalf("expected message not to be duplicated in extras, got %q", got)
	}

	if strings.Contains(got, "time=") {
		t.Fatalf("expected timestamp not to be duplicated in extras, got %q", got)
	}
}

func TestConsoleWriterRedactsSensitiveDirectFields(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	writer := newWriter(Dev, "test-service", "0.1.0")
	consoleWriter, ok := writer.(zerolog.ConsoleWriter)
	if !ok {
		t.Fatalf("newWriter() returned %T, want zerolog.ConsoleWriter", writer)
	}

	var out bytes.Buffer
	consoleWriter.Out = &out
	consoleWriter.NoColor = true

	logger := zerolog.New(consoleWriter).With().Timestamp().Caller().Logger()
	logger.Info().
		Str("email", "fanny-lou@proton.me").
		Str("password", "SuperSecretP@ss123").
		Msg("login attempt")

	got := stripConsoleANSI(out.String())

	if !strings.Contains(got, "email=fanny-lou@*****") {
		t.Fatalf("expected email domain to be masked, got %q", got)
	}

	if !strings.Contains(got, "password=*****") {
		t.Fatalf("expected password to be masked, got %q", got)
	}

	if strings.Contains(got, "fanny-lou@proton.me") || strings.Contains(got, "SuperSecretP@ss123") {
		t.Fatalf("expected sensitive direct fields not to leak, got %q", got)
	}
}

func TestJSONWriterRedactsSensitiveFieldsBeforeWriting(t *testing.T) {
	var out bytes.Buffer

	logger := newLoggerWithWriter(&out, Prod)
	logger.Info().
		Str("email", "fanny-lou@proton.me").
		Str("password", "SuperSecretP@ss123").
		Str("request.body", "{\"email\":\"owner@example.org\",\"password\":\"AnotherSecret123\"}").
		Msg("login attempt")

	got := out.String()

	if !strings.Contains(got, "\"email\":\"fanny-lou@*****\"") {
		t.Fatalf("expected top-level email to be masked in JSON output, got %q", got)
	}

	if !strings.Contains(got, "\"password\":\"*****\"") {
		t.Fatalf("expected top-level password to be masked in JSON output, got %q", got)
	}

	if !strings.Contains(got, "\"request.body\":\"{\\\"email\\\":\\\"owner@*****\\\",\\\"password\\\":\\\"*****\\\"}\"") {
		t.Fatalf("expected nested request body JSON string to be redacted in JSON output, got %q", got)
	}

	if strings.Contains(got, "fanny-lou@proton.me") || strings.Contains(got, "owner@example.org") || strings.Contains(got, "SuperSecretP@ss123") || strings.Contains(got, "AnotherSecret123") {
		t.Fatalf("expected sensitive values not to leak in JSON output, got %q", got)
	}
}

func TestFormatConsoleExtrasCompactsAndRedactsSensitiveJSONFields(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("http: inbound audit")

	err := formatConsoleExtras(
		map[string]any{
			"request.body":  "{\n  \"email\": \"fanny-lou@proton.me\",\n  \"password\": \"SuperSecretP@ss123\",\n  \"profile\": {\"contact_email\": \"owner@example.org\"}\n}",
			"response.body": "{\n  \"status\": \"healthy\"\n}",
			"response.headers": map[string]any{
				"Content-Type": "application/json; charset=utf-8",
				"Server":       "go-http",
			},
			zerolog.MessageFieldName:   "http: inbound audit",
			zerolog.TimestampFieldName: "2026-04-10T08:11:53-05:00",
		},
		&buf,
		"test-service",
		"0.1.0",
		true,
	)
	if err != nil {
		t.Fatalf("formatConsoleExtras() error = %v", err)
	}

	got := stripConsoleANSI(buf.String())

	if !strings.Contains(got, "http: inbound audit | service_name=test-service service_version=0.1.0") {
		t.Fatalf("expected inline extras to stay readable, got %q", got)
	}

	if !strings.Contains(got, "request.body={\"email\":\"fanny-lou@*****\",\"password\":\"*****\",\"profile\":{\"contact_email\":\"owner@*****\"}}") {
		t.Fatalf("expected request body JSON to be compacted inline and redacted, got %q", got)
	}

	if !strings.Contains(got, "response.body={\"status\":\"healthy\"}") {
		t.Fatalf("expected response body JSON to be compacted inline, got %q", got)
	}

	if !strings.Contains(got, "response.headers={\"Content-Type\":\"application/json; charset=utf-8\",\"Server\":\"go-http\"}") {
		t.Fatalf("expected response headers JSON to stay inline, got %q", got)
	}

	if strings.Contains(got, "\n") {
		t.Fatalf("expected all extras on a single line, got %q", got)
	}

	if strings.Contains(got, "fanny-lou@proton.me") || strings.Contains(got, "owner@example.org") || strings.Contains(got, "SuperSecretP@ss123") {
		t.Fatalf("expected sensitive JSON fields not to leak, got %q", got)
	}
}

func stripConsoleANSI(s string) string {
	replacer := strings.NewReplacer(Blue, "", Reset, "")
	return replacer.Replace(s)
}
