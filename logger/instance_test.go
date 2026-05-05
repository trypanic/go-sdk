package logger

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestNewDoesNotMutateGlobal(t *testing.T) {
	previous := global
	t.Cleanup(func() { global = previous })
	global = nil

	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})
	l.Info("instance only")

	if global != nil {
		t.Fatalf("logger.New must not set the package-level global")
	}
	if !strings.Contains(buf.String(), "instance only") {
		t.Fatalf("expected instance log output, got %q", buf.String())
	}
}

func TestLoggerErrorReturnsOriginal(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})

	plain := errors.New("kaboom")
	if got := l.Error(plain, "failed"); got != plain {
		t.Fatalf("Logger.Error should return original error unchanged")
	}
	if !strings.Contains(buf.String(), "kaboom") {
		t.Fatalf("expected wrapped wrapped_error in output, got %q", buf.String())
	}
}

func TestLoggerErrorCtxIncludesAppError(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})

	appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithReason("boom"))
	l.ErrorCtx(context.Background(), appErr, "ctx error")

	out := buf.String()
	if !strings.Contains(out, "ERR_INTERNAL") {
		t.Fatalf("expected error_code in JSON output, got %q", out)
	}
	if !strings.Contains(out, "ctx error") {
		t.Fatalf("expected message in JSON output, got %q", out)
	}
}

func TestLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf}).
		WithFields(map[string]any{"request_id": "rid-42"})

	l.Info("ping")
	if !strings.Contains(buf.String(), "rid-42") {
		t.Fatalf("expected request_id field in output, got %q", buf.String())
	}
}

func TestLoggerIntoContextRetrievable(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})
	ctx := l.IntoContext(context.Background())

	Ctx(ctx).Info().Msg("from-context")
	if !strings.Contains(buf.String(), "from-context") {
		t.Fatalf("expected logger from context to write to buffer, got %q", buf.String())
	}
}
