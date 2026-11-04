package llmclient

import (
	"os"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// Environment variables read by ConfigFromEnv.
const (
	// APIKeyEnvVar names the variable holding the provider API key (required).
	APIKeyEnvVar = "LLM_API_KEY"
	// EndpointEnvVar names the variable holding the chat-completions endpoint URL (optional).
	EndpointEnvVar = "LLM_ENDPOINT"
)

// ConfigFromEnv builds a Config from LLM_API_KEY (required) and
// LLM_ENDPOINT (optional; set Endpoint yourself when it is absent). It is
// the only place llmclient reads the environment. A missing key returns
// ERR_SYSTEM_CONFIG_INVALID. Timeout and AllowInsecureEndpoint keep their
// zero values; adjust the returned Config as needed.
func ConfigFromEnv() (Config, error) {
	key := os.Getenv(APIKeyEnvVar)
	if key == "" {
		return Config{}, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).
			With(errorkit.WithReason("environment variable %s is not set", APIKeyEnvVar))
	}
	return Config{APIKey: key, Endpoint: os.Getenv(EndpointEnvVar)}, nil
}
