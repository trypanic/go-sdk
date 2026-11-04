package logger

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestNewDoesNotMutateGlobal(t *testing.T) {
	previous := global
	t.Cleanup(func() { global = previous })
	global = nil

	levelBefore := zerolog.GlobalLevel()
	timeFormatBefore := zerolog.TimeFieldFormat

	var buf bytes.Buffer
	l := New(Config{Env: Prod, Writer: &buf})
	l.Info("instance only")
	_ = New(Config{Env: Dev, Writer: io.Discard})

	if global != nil {
		t.Fatalf("logger.New must not set the package-level global")
	}
	if got := zerolog.GlobalLevel(); got != levelBefore {
		t.Fatalf("logger.New changed zerolog.GlobalLevel() from %v to %v", levelBefore, got)
	}
	if got := zerolog.TimeFieldFormat; got != timeFormatBefore {
		t.Fatalf("logger.New changed zerolog.TimeFieldFormat from %q to %q", timeFormatBefore, got)
	}
	if !strings.Contains(buf.String(), "instance only") {
		t.Fatalf("expected instance log output, got %q", buf.String())
	}
}

func TestNewLoggersKeepTheirOwnLevel(t *testing.T) {
	var devBuf, prodBuf bytes.Buffer
	dev := New(Config{Env: Dev, Writer: &devBuf})
	prod := New(Config{Env: Prod, Writer: &prodBuf})

	dev.Zerolog().Debug().Msg("debug-from-dev")
	prod.Zerolog().Debug().Msg("debug-from-prod")
	prod.Info("info-from-prod")

	if !strings.Contains(devBuf.String(), "debug-from-dev") {
		t.Fatalf("Dev logger must emit Debug after a Prod logger was built, got %q", devBuf.String())
	}
	if strings.Contains(prodBuf.String(), "debug-from-prod") {
		t.Fatalf("Prod logger must not emit Debug, got %q", prodBuf.String())
	}
	if !strings.Contains(prodBuf.String(), "info-from-prod") {
		t.Fatalf("Prod logger must emit Info, got %q", prodBuf.String())
	}
	if !strings.Contains(prodBuf.String(), `"time":`) {
		t.Fatalf("expected a time field on every line, got %q", prodBuf.String())
	}
}

// TestNewWhileLoggingIsRaceFree must run under -race: constructing loggers
// concurrently with logging must not touch shared zerolog state.
func TestNewWhileLoggingIsRaceFree(t *testing.T) {
	l := New(Config{Env: Prod, Writer: io.Discard})

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 200 {
			l.Info("tick")
		}
	})
	wg.Go(func() {
		for range 200 {
			_ = New(Config{Env: Prod, Writer: io.Discard})
		}
	})
	wg.Wait()
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
	l.ErrorCtx(t.Context(), appErr, "ctx error")

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
	ctx := l.IntoContext(t.Context())

	Ctx(ctx).Info().Msg("from-context")
	if !strings.Contains(buf.String(), "from-context") {
		t.Fatalf("expected logger from context to write to buffer, got %q", buf.String())
	}
}
