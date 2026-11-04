package envs

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/caarlos0/env/v11"

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

// A secret mis-assigned to a non-string field must not be copied into the
// error text, the pretty output or the JSON output (they end up in logs),
// while the cause stays inspectable with errors.As.
func TestNewLoaderErrorDoesNotLeakValues(t *testing.T) {
	const secret = "hunter2-secret"
	t.Setenv("ENVS_TEST_PORT", secret)
	t.Setenv("ENVS_TEST_TIMEOUT", secret+"-duration")

	var cfg struct {
		Port    int           `env:"ENVS_TEST_PORT"`
		Timeout time.Duration `env:"ENVS_TEST_TIMEOUT"`
	}
	err := NewLoader(&cfg)
	if err == nil {
		t.Fatal("NewLoader() error = nil, want error")
	}
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("NewLoader() error type = %T, want *errorkit.AppError", err)
	}
	if got, want := appErr.Code(), errorkit.ERR_SYSTEM_CONFIG_INVALID; got != want {
		t.Fatalf("code = %s, want %s", got, want)
	}

	for name, out := range map[string]string{
		"Error":         err.Error(),
		"Pretty":        appErr.Pretty(),
		"PrettyJSON":    appErr.PrettyJSON(),
		"Wrapped.Error": appErr.Wrapped.Error(),
	} {
		if strings.Contains(out, secret) {
			t.Errorf("%s leaks the env value: %q", name, out)
		}
	}
	for _, want := range []string{"failed to parse environment variables", `"Port"`, `"Timeout"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Error() = %q, want substring %q", err.Error(), want)
		}
	}

	if _, ok := errors.AsType[env.ParseError](err); !ok {
		t.Errorf("errors.AsType[env.ParseError] failed; cause must stay inspectable")
	}
}
