package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// logOneLine logs a single Info event with one string field through a Prod
// logger and returns the decoded JSON line.
func logOneLine(t *testing.T, cfg Config, key, value, msg string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	cfg.Env = Prod
	cfg.Writer = &buf
	New(cfg).Zerolog().Info().Str(key, value).Msg(msg)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("output is not JSON: %v: %q", err, buf.String())
	}
	return line
}

func TestRedactionSensitiveKeys(t *testing.T) {
	const secret = "s3cr3t-value"
	keys := []string{
		"password", "db_password", "passwd", "pwd",
		"secret", "client_secret",
		"token", "access_token", "refresh_token", "id_token",
		"authorization", "Authorization", "Proxy-Authorization",
		"cookie", "Set-Cookie",
		"api_key", "apiKey", "X-Api-Key",
		"private_key",
		"dsn", "postgres_dsn",
		"credentials",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			line := logOneLine(t, Config{}, key, secret, "m")
			if got := line[key]; got != maskedPasswordValue {
				t.Fatalf("%s = %v, want %q", key, got, maskedPasswordValue)
			}
		})
	}
}

func TestRedactionKeepsNonSensitiveKeys(t *testing.T) {
	keys := []string{"user", "max_tokens", "token_count", "author", "status"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			line := logOneLine(t, Config{}, key, "plain", "m")
			if got := line[key]; got != "plain" {
				t.Fatalf("%s = %v, want plain", key, got)
			}
		})
	}
}

func TestRedactionMasksURLUserinfoInValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "dsn in field", value: "postgres://user:pa55@db:5432/app", want: "postgres://user:*****@db:5432/app"},
		{name: "url in free text", value: "dial amqp://guest:guest@mq:5672/ failed", want: "dial amqp://guest:*****@mq:5672/ failed"},
		{name: "no password", value: "https://user@example.com/x", want: "https://user@example.com/x"},
		{name: "no userinfo", value: "https://example.com/a@b", want: "https://example.com/a@b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := logOneLine(t, Config{}, "target", tc.value, tc.value)
			if got := line["target"]; got != tc.want {
				t.Fatalf("field = %v, want %q", got, tc.want)
			}
			if got := line["message"]; got != tc.want {
				t.Fatalf("message = %v, want %q", got, tc.want)
			}
		})
	}
}

func TestRedactionCustomRedactor(t *testing.T) {
	cfg := Config{
		Redactor: func(key string, value any) any {
			if key == "ssn" {
				return "[redacted]"
			}
			return value
		},
	}

	line := logOneLine(t, cfg, "ssn", "123-45-6789", "m")
	if got := line["ssn"]; got != "[redacted]" {
		t.Fatalf("ssn = %v, want [redacted]", got)
	}

	// The custom redactor runs after the built-in policy, so it cannot
	// un-mask a built-in sensitive key.
	line = logOneLine(t, cfg, "password", "hunter2", "m")
	if got := line["password"]; got != maskedPasswordValue {
		t.Fatalf("password = %v, want %q", got, maskedPasswordValue)
	}
	if strings.Contains(line["message"].(string), "hunter2") {
		t.Fatal("password leaked into the message")
	}
}
