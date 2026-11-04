package logger

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// useGlobalBuffer points the package-level global logger at a buffer for the
// duration of the test.
func useGlobalBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := global.Load()
	t.Cleanup(func() { global.Store(previous) })
	var buf bytes.Buffer
	l := newLoggerWithWriter(&buf, Prod)
	global.Store(&l)
	return &buf
}

func TestErrorKeepsCodeOfWrappedAppError(t *testing.T) {
	appErr := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	wrapped := fmt.Errorf("repo: %w", appErr)

	t.Run("instance", func(t *testing.T) {
		var buf bytes.Buffer
		l := New(Config{Env: Prod, Writer: &buf})
		if got := l.Error(wrapped, "lookup failed"); got != wrapped {
			t.Fatalf("Logger.Error must return the original error")
		}
		assertWrappedAppErrorLogged(t, buf.String())
	})

	t.Run("global", func(t *testing.T) {
		buf := useGlobalBuffer(t)
		if got := Error(wrapped, "lookup failed"); got != wrapped {
			t.Fatalf("Error must return the original error")
		}
		assertWrappedAppErrorLogged(t, buf.String())
	})
}

func TestErrorSkipsTypedNilAppError(t *testing.T) {
	var nilAppErr *errorkit.AppError
	appErr := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	joined := errors.Join(nilAppErr, appErr)

	t.Run("instance", func(t *testing.T) {
		var buf bytes.Buffer
		l := New(Config{Env: Prod, Writer: &buf})
		if got := l.Error(joined, "lookup failed"); got != joined {
			t.Fatalf("Logger.Error must return the original error")
		}
		assertJoinedAppErrorLogged(t, buf.String())
	})

	t.Run("global", func(t *testing.T) {
		buf := useGlobalBuffer(t)
		if got := Error(joined, "lookup failed"); got != joined {
			t.Fatalf("Error must return the original error")
		}
		assertJoinedAppErrorLogged(t, buf.String())
	})
}

func assertJoinedAppErrorLogged(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, `"error_code":"ERR_CLIENT_NOT_FOUND"`) {
		t.Fatalf("expected the joined AppError code, got %q", out)
	}
	if strings.Contains(out, "ERR_UNKNOWN") || strings.Contains(out, "ERR_INTERNAL") {
		t.Fatalf("a joined AppError after a typed nil must not be replaced, got %q", out)
	}
}

func assertWrappedAppErrorLogged(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, `"error_code":"ERR_CLIENT_NOT_FOUND"`) {
		t.Fatalf("expected the wrapped AppError code, got %q", out)
	}
	if strings.Contains(out, "ERR_INTERNAL") {
		t.Fatalf("a wrapped AppError must not be re-wrapped as ERR_INTERNAL, got %q", out)
	}
	if !strings.Contains(out, "repo: ") {
		t.Fatalf("expected the outer error message as context, got %q", out)
	}
}

func TestNilAppErrorDoesNotPanic(t *testing.T) {
	var nilAppErr *errorkit.AppError

	cases := []struct {
		name string
		log  func(l *Logger)
	}{
		{name: "ErrorCtx typed nil", log: func(l *Logger) { l.ErrorCtx(t.Context(), nilAppErr, "nil") }},
		{name: "WarnCtx typed nil", log: func(l *Logger) { l.WarnCtx(t.Context(), nilAppErr, "nil") }},
		{name: "Error typed nil", log: func(l *Logger) { _ = l.Error(nilAppErr, "nil") }},
		{name: "Error nil interface", log: func(l *Logger) { _ = l.Error(nil, "nil") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := New(Config{Env: Prod, Writer: &buf})
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("logging a nil error panicked: %v", r)
				}
			}()
			tc.log(l)
			if !strings.Contains(buf.String(), `"error_code":"ERR_UNKNOWN"`) {
				t.Fatalf("expected error_code ERR_UNKNOWN for a nil error, got %q", buf.String())
			}
		})
	}
}

func TestWithErrorKitNil(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf)
	WithErrorKit(zl.Error(), nil).Msg("nil")
	if !strings.Contains(buf.String(), `"error_code":"ERR_UNKNOWN"`) {
		t.Fatalf("expected error_code ERR_UNKNOWN, got %q", buf.String())
	}
}

func TestErrorPlainErrorStillWrapped(t *testing.T) {
	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})
	_ = l.Error(errors.New("kaboom"), "failed")
	if !strings.Contains(buf.String(), `"error_code":"ERR_INTERNAL"`) {
		t.Fatalf("a plain error must be logged as ERR_INTERNAL, got %q", buf.String())
	}
}
