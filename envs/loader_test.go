package envs

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestNewLoaderReturnsConfigErrorWithParseDetails(t *testing.T) {
	const requiredEnv = "ENVS_TEST_REQUIRED_VALUE"
	if err := os.Unsetenv(requiredEnv); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", requiredEnv, err)
	}

	var cfg struct {
		RequiredValue string `env:"ENVS_TEST_REQUIRED_VALUE,required"`
	}

	err := NewLoader(&cfg)
	if err == nil {
		t.Fatal("NewLoader() error = nil, want error")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("NewLoader() error type = %T, want *errorkit.AppError", err)
	}

	if got, want := appErr.Code(), errorkit.ERR_SYSTEM_CONFIG_INVALID; got != want {
		t.Fatalf("NewLoader() code = %s, want %s", got, want)
	}

	errorText := err.Error()
	for _, want := range []string{
		"failed to parse environment variables",
		requiredEnv,
	} {
		if !strings.Contains(errorText, want) {
			t.Errorf("NewLoader() error = %q, want substring %q", errorText, want)
		}
	}
}
