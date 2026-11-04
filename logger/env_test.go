package logger

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestDetectEnv(t *testing.T) {
	cases := []struct {
		name  string
		value string
		unset bool
		want  Environment
	}{
		{name: "unset defaults to Prod", unset: true, want: Prod},
		{name: "empty defaults to Prod", value: "", want: Prod},
		{name: "production", value: "production", want: Prod},
		{name: "prod", value: "prod", want: Prod},
		{name: "upper case PRODUCTION", value: "PRODUCTION", want: Prod},
		{name: "mixed case Production", value: "Production", want: Prod},
		{name: "unrecognised value", value: "prd", want: Prod},
		{name: "staging", value: "staging", want: Prod},
		{name: "development", value: "development", want: Dev},
		{name: "dev", value: "dev", want: Dev},
		{name: "local", value: "local", want: Dev},
		{name: "upper case DEVELOPMENT with spaces", value: "  DEVELOPMENT ", want: Dev},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", tc.value)
			if tc.unset {
				if err := os.Unsetenv("ENVIRONMENT"); err != nil {
					t.Fatal(err)
				}
			}
			if got := DetectEnv(); got != tc.want {
				t.Errorf("DetectEnv() with ENVIRONMENT=%q (unset=%v) = %q, want %q", tc.value, tc.unset, got, tc.want)
			}
		})
	}
}

// TestConsoleWriterReadsEnvironmentOnce checks that the Dev console writer
// decides whether to print service metadata when it is built, not on every
// log line.
func TestConsoleWriterReadsEnvironmentOnce(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	writer, ok := newWriter(Dev, "test-service", "0.1.0").(zerolog.ConsoleWriter)
	if !ok {
		t.Fatal("newWriter(Dev) did not return a zerolog.ConsoleWriter")
	}
	var out bytes.Buffer
	writer.Out = &out
	writer.NoColor = true

	t.Setenv("ENVIRONMENT", "development")
	l := zerolog.New(writer)
	l.Info().Str("k", "v").Msg("hello")

	if got := stripConsoleANSI(out.String()); strings.Contains(got, "service_name=") {
		t.Fatalf("writer built with ENVIRONMENT=local printed service metadata after the variable changed: %q", got)
	}
}
