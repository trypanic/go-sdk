package llmclient

import (
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestConfigFromEnvReadsKeyAndEndpoint(t *testing.T) {
	t.Setenv(APIKeyEnvVar, "sk-from-env")
	t.Setenv(EndpointEnvVar, "https://llm.example/v1/chat")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-from-env" || cfg.Endpoint != "https://llm.example/v1/chat" {
		t.Fatalf("ConfigFromEnv() = %+v", cfg)
	}
}

func TestConfigFromEnvEndpointIsOptional(t *testing.T) {
	t.Setenv(APIKeyEnvVar, "sk-from-env")
	t.Setenv(EndpointEnvVar, "")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Endpoint != "" {
		t.Fatalf("Endpoint = %q, want empty when %s is unset", cfg.Endpoint, EndpointEnvVar)
	}
}

func TestConfigFromEnvRequiresKey(t *testing.T) {
	t.Setenv(APIKeyEnvVar, "")

	_, err := ConfigFromEnv()
	appErr, ok := err.(*errorkit.AppError)
	if !ok {
		t.Fatalf("want *errorkit.AppError, got %T (%v)", err, err)
	}
	if appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("code = %s, want ERR_SYSTEM_CONFIG_INVALID", appErr.Code())
	}
}
