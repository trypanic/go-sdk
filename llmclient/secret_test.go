package llmclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const testSecret = "sk-live-SECRET"

func requireNoKey(t *testing.T, where, s string) {
	t.Helper()
	if strings.Contains(s, testSecret) {
		t.Fatalf("%s leaks the API key: %s", where, s)
	}
}

func TestConfigNeverPrintsTheAPIKey(t *testing.T) {
	t.Parallel()

	cfg := Config{APIKey: testSecret, Endpoint: "https://llm.example/v1/chat"}
	client := NewClient(nil, cfg)

	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		requireNoKey(t, "Config "+format, fmt.Sprintf(format, cfg))
		requireNoKey(t, "*Config "+format, fmt.Sprintf(format, &cfg))
		requireNoKey(t, "*Client "+format, fmt.Sprintf(format, client))
		requireNoKey(t, "Client "+format, fmt.Sprintf(format, *client))
	}
	if s := fmt.Sprintf("%+v", cfg); !strings.Contains(s, "llm.example") || !strings.Contains(s, "[REDACTED]") {
		t.Fatalf("Config should still show the endpoint and a masked key, got %s", s)
	}

	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	requireNoKey(t, "json.Marshal(Config)", string(b))

	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("cfg", "config", cfg)
	requireNoKey(t, "slog", buf.String())
}

func TestConfigShowsAnEmptyKeyAsEmpty(t *testing.T) {
	t.Parallel()

	if s := fmt.Sprintf("%+v", Config{}); strings.Contains(s, "[REDACTED]") {
		t.Fatalf("an unset key must not look set, got %s", s)
	}
}
