package llmclient

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/trypanic/go-sdk/v2/internal/redact"
)

// maskedKey returns the placeholder for a set key and "" for an unset one.
func maskedKey(key string) string {
	if key == "" {
		return ""
	}
	return redact.Placeholder
}

// String prints c with the API key masked.
func (c Config) String() string {
	return fmt.Sprintf("{APIKey:%s Endpoint:%s Timeout:%s AllowInsecureEndpoint:%t}",
		maskedKey(c.APIKey), c.Endpoint, c.Timeout, c.AllowInsecureEndpoint)
}

// GoString prints c for %#v with the API key masked.
func (c Config) GoString() string {
	return fmt.Sprintf("llmclient.Config{APIKey:%q, Endpoint:%q, Timeout:%d, AllowInsecureEndpoint:%t}",
		maskedKey(c.APIKey), c.Endpoint, c.Timeout, c.AllowInsecureEndpoint)
}

// MarshalJSON encodes c with the API key masked, so structured loggers
// that marshal values never write the key.
func (c Config) MarshalJSON() ([]byte, error) {
	type masked struct {
		APIKey   string `json:"APIKey"`
		Endpoint string `json:"Endpoint"`
		Timeout  int64  `json:"Timeout"`
		Insecure bool   `json:"AllowInsecureEndpoint"`
	}
	return json.Marshal(masked{
		APIKey: maskedKey(c.APIKey), Endpoint: c.Endpoint,
		Timeout: int64(c.Timeout), Insecure: c.AllowInsecureEndpoint,
	})
}

// LogValue implements slog.LogValuer with the API key masked.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("api_key", maskedKey(c.APIKey)),
		slog.String("endpoint", c.Endpoint),
		slog.Duration("timeout", c.Timeout),
	)
}

// String prints the client without its API key. fmt cannot call the
// methods of the unexported cfg field, so Client needs its own.
func (c Client) String() string { return "llmclient.Client" + c.cfg.String() }

// GoString prints the client for %#v without its API key.
func (c Client) GoString() string { return "llmclient.Client{cfg:" + c.cfg.GoString() + "}" }
