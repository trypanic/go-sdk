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
	// t.Setenv registers a restore of the previous state at cleanup;
	// Unsetenv then makes the variable absent for this test only.
	t.Setenv(requiredEnv, "")
	if err := os.Unsetenv(requiredEnv); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", requiredEnv, err)
	}

	var cfg struct {
		RequiredValue string `env:"ENVS_TEST_REQUIRED_VALUE,required"`
	}

	err := LoadInto(&cfg)
	if err == nil {
		t.Fatal("LoadInto() error = nil, want error")
	}

	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("LoadInto() error type = %T, want *errorkit.AppError", err)
	}

	if got, want := appErr.Code(), errorkit.ERR_SYSTEM_CONFIG_INVALID; got != want {
		t.Fatalf("LoadInto() code = %s, want %s", got, want)
	}

	errorText := err.Error()
	for _, want := range []string{
		"failed to parse environment variables",
		requiredEnv,
	} {
		if !strings.Contains(errorText, want) {
			t.Errorf("LoadInto() error = %q, want substring %q", errorText, want)
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
	err := LoadInto(&cfg)
	if err == nil {
		t.Fatal("LoadInto() error = nil, want error")
	}
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("LoadInto() error type = %T, want *errorkit.AppError", err)
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

type loadTestConfig struct {
	Host string `env:"ENVS_TEST_LOAD_HOST" envDefault:"0.0.0.0"`
	Port int    `env:"ENVS_TEST_LOAD_PORT,required"`
}

func TestLoad(t *testing.T) {
	t.Setenv("ENVS_TEST_LOAD_PORT", "8080")

	cfg, err := Load[loadTestConfig]()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := (loadTestConfig{Host: "0.0.0.0", Port: 8080}); cfg != want {
		t.Fatalf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadReturnsZeroValueAndConfigError(t *testing.T) {
	// t.Setenv registers the restore; Unsetenv makes the required var absent.
	t.Setenv("ENVS_TEST_LOAD_PORT", "")
	if err := os.Unsetenv("ENVS_TEST_LOAD_PORT"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENVS_TEST_LOAD_HOST", "partially-loaded")

	cfg, err := Load[loadTestConfig]()
	if appErr, ok := errors.AsType[*errorkit.AppError](err); !ok || appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("Load() error = %v, want ERR_SYSTEM_CONFIG_INVALID", err)
	}
	if cfg != (loadTestConfig{}) {
		t.Fatalf("Load() on error = %+v, want zero value", cfg)
	}
}

func TestLoadRejectsNonStructType(t *testing.T) {
	t.Parallel()

	_, err := Load[int]()
	if appErr, ok := errors.AsType[*errorkit.AppError](err); !ok || appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("Load[int]() error = %v, want ERR_SYSTEM_CONFIG_INVALID", err)
	}
}

// LoadWithOptions passes caarlos0/env options through, so tests of consumer
// code can inject an environment map instead of mutating the process.
func TestLoadWithOptions(t *testing.T) {
	t.Parallel()

	var cfg loadTestConfig
	err := LoadWithOptions(&cfg, env.Options{
		Prefix:      "APP_",
		Environment: map[string]string{"APP_ENVS_TEST_LOAD_PORT": "9090", "APP_ENVS_TEST_LOAD_HOST": "h"},
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if want := (loadTestConfig{Host: "h", Port: 9090}); cfg != want {
		t.Fatalf("LoadWithOptions() = %+v, want %+v", cfg, want)
	}

	err = LoadWithOptions(&cfg, env.Options{Environment: map[string]string{}})
	if appErr, ok := errors.AsType[*errorkit.AppError](err); !ok || appErr.Code() != errorkit.ERR_SYSTEM_CONFIG_INVALID {
		t.Fatalf("LoadWithOptions() missing required error = %v, want ERR_SYSTEM_CONFIG_INVALID", err)
	}
}
