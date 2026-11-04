package postgres

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

const leakedPassword = "S3cr3tPw"

// TestNewPostgresClient_ParseError_DoesNotLeakPassword is the regression test
// for C4-001/C4-003: a DSN that fails to parse must not expose the password
// through the payload, PrettyJSON or a log line, and the driver error must be
// reachable through errors.As.
func TestNewPostgresClient_ParseError_DoesNotLeakPassword(t *testing.T) {
	_, err := NewPostgresClient(Config{DSN: "postgres://svc:" + leakedPassword + "@db.internal:notaport/app"})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}

	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}

	if _, ok := errors.AsType[*pgconn.ParseConfigError](err); !ok {
		t.Fatalf("errors.AsType[*pgconn.ParseConfigError] = false; unwrap = %v", errors.Unwrap(err))
	}

	assertNoSecret(t, "PrettyJSON", appErr.PrettyJSON())
	assertNoSecret(t, "log line", logLine(t, appErr))
	if payload, ok := appErr.Payload.(string); !ok || payload == "" {
		t.Fatalf("payload = %#v, want the redacted driver message", appErr.Payload)
	}
}

// TestNewPostgresClient_PingError_KeepsCause covers the connect/ping path of
// C4-003. The DSN points at a Unix socket directory that has no server, so the
// dial fails locally without any network traffic.
func TestNewPostgresClient_PingError_KeepsCause(t *testing.T) {
	socketDir := filepath.Join(t.TempDir(), "nosocket")
	_, err := NewPostgresClient(Config{DSN: "postgres://svc:" + leakedPassword + "@/app?host=" + socketDir + "&connect_timeout=1"})
	if err == nil {
		t.Fatal("expected connect error, got nil")
	}

	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}

	if _, ok := errors.AsType[*pgconn.ConnectError](err); !ok {
		t.Fatalf("errors.AsType[*pgconn.ConnectError] = false; unwrap = %v", errors.Unwrap(err))
	}

	pretty := appErr.PrettyJSON()
	if strings.Contains(pretty, "marshal_failed") {
		t.Fatalf("PrettyJSON fell back to marshal_failed: %s", pretty)
	}
	assertNoSecret(t, "PrettyJSON", pretty)

	line := logLine(t, appErr)
	if strings.Contains(line, "marshaling error") {
		t.Fatalf("log line could not serialize the payload: %s", line)
	}
	assertNoSecret(t, "log line", line)
}

func logLine(t *testing.T, appErr *errorkit.AppError) string {
	t.Helper()
	var buf bytes.Buffer
	logger.New(logger.Config{Env: logger.Prod, Writer: &buf}).ErrorCtx(t.Context(), appErr, "postgres init failed")
	return buf.String()
}

func assertNoSecret(t *testing.T, where, text string) {
	t.Helper()
	if strings.Contains(text, leakedPassword) {
		t.Fatalf("%s contains the DSN password: %s", where, text)
	}
}
