package httpserver

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestServerConfigAddress(t *testing.T) {
	t.Parallel()

	cfg := ServerConfig{Host: "0.0.0.0", Port: 8080}
	if got := cfg.Address(); got != "0.0.0.0:8080" {
		t.Fatalf("Address = %q, want 0.0.0.0:8080", got)
	}
}

func TestDefaultServerOptionsHasSafeDefaults(t *testing.T) {
	t.Parallel()

	o := DefaultServerOptions()
	if !o.EnableTracing || !o.EnableRecovery || !o.EnableHealth ||
		!o.EnableNoRoute || !o.EnableNoMethod {
		t.Fatalf("compatibility profile must enable tracing/recovery/health/no-route/no-method, got %+v", o)
	}
	if !o.EnableAudit {
		t.Fatalf("audit must be enabled in compatibility profile")
	}
	if o.Audit.CaptureBodies {
		t.Fatalf("audit must NOT capture bodies by default; got CaptureBodies=true")
	}
	if o.Audit.BodyRedactor == nil {
		t.Fatalf("audit body redactor must be non-nil by default")
	}
	if got := string(o.Audit.BodyRedactor([]byte("secret"))); got != "[REDACTED]" {
		t.Fatalf("default redactor must replace non-empty body with [REDACTED], got %q", got)
	}
	if got := o.Audit.BodyRedactor(nil); len(got) != 0 {
		t.Fatalf("default redactor must keep empty body empty, got %v", got)
	}
	if o.NoRouteStatus != 404 {
		t.Fatalf("NoRouteStatus default = %d, want 404", o.NoRouteStatus)
	}
	if o.NoMethodStatus != 405 {
		t.Fatalf("NoMethodStatus default = %d, want 405", o.NoMethodStatus)
	}
	if o.Reply.Layout == "" {
		t.Fatalf("ReplyOptions.Layout must default to a non-empty layout")
	}
	if o.Reply.Clock == nil {
		t.Fatalf("ReplyOptions.Clock must default to a non-nil clock")
	}
}

func TestRawBodyRedactorReturnsInput(t *testing.T) {
	t.Parallel()

	in := []byte("payload")
	if got := RawBodyRedactor(in); !bytes.Equal(got, in) {
		t.Fatalf("RawBodyRedactor must return its input verbatim")
	}
}

func TestReplyOptionApplicators(t *testing.T) {
	t.Parallel()

	clock := func() time.Time { return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC) }
	r := buildReply(ReplyOptions{Layout: "2006", Clock: clock},
		WithMessageOpt("hello"),
		WithErrorOpt(errors.New("boom")),
		WithMetadataOpt(map[string]int{"a": 1}),
		WithDataOpt("payload"),
	)

	if r.Message == nil || *r.Message != "hello" {
		t.Fatalf("Message not applied: %+v", r)
	}
	if r.Error == nil || *r.Error != "boom" {
		t.Fatalf("Error not applied: %+v", r)
	}
	if r.Metadata == nil {
		t.Fatalf("Metadata not applied: %+v", r)
	}
	if r.Data == nil {
		t.Fatalf("Data not applied: %+v", r)
	}
	if r.Timestamp != "2026" {
		t.Fatalf("Timestamp = %q, want layout-formatted '2026'", r.Timestamp)
	}
}
