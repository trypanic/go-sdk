package grpc

import (
	"testing"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
)

func wantCode(t *testing.T, err error, code errorkit.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	appErr, ok := err.(*errorkit.AppError)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
	if appErr.Code() != code {
		t.Fatalf("error code = %s, want %s", appErr.Code(), code)
	}
}

func TestNew_EmptyAddressIsConfigError(t *testing.T) {
	_, err := New(ServerConfig{})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
}

func TestDial_EmptyTargetIsConfigError(t *testing.T) {
	_, err := Dial(ClientConfig{})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
}

func TestDefaultServerBuild_TracingAndRecoveryOn(t *testing.T) {
	b := defaultServerBuild()
	if !b.tracing || !b.recovery {
		t.Fatalf("defaultServerBuild = %+v, want tracing+recovery on", b)
	}
}

func TestDefaultClientBuild_TracingOn(t *testing.T) {
	if !defaultClientBuild().tracing {
		t.Fatal("defaultClientBuild tracing should default on")
	}
}

func TestServerOptions_Accumulate(t *testing.T) {
	b := defaultServerBuild()
	for _, o := range []ServerOption{
		WithServerTracing(false),
		WithServerRecovery(false),
		WithUnaryInterceptors(nil, nil),
		WithStreamInterceptors(nil),
		WithRawServerOptions(nil, nil, nil),
	} {
		o(b)
	}
	if b.tracing || b.recovery {
		t.Fatalf("toggles not applied: %+v", b)
	}
	if len(b.unary) != 2 || len(b.stream) != 1 || len(b.raw) != 3 {
		t.Fatalf("appends wrong: unary=%d stream=%d raw=%d", len(b.unary), len(b.stream), len(b.raw))
	}
}

func TestServerKeepalive_Builders(t *testing.T) {
	k := ServerKeepalive{
		Time:                10 * time.Second,
		Timeout:             3 * time.Second,
		MinTime:             5 * time.Second,
		PermitWithoutStream: true,
		MaxConnectionAge:    time.Hour,
	}
	if !k.hasParams() || !k.hasEnforcement() {
		t.Fatal("populated keepalive should report hasParams and hasEnforcement")
	}
	sp := k.serverParameters()
	if sp.Time != k.Time || sp.Timeout != k.Timeout || sp.MaxConnectionAge != k.MaxConnectionAge {
		t.Fatalf("serverParameters mismatch: %+v", sp)
	}
	ep := k.enforcementPolicy()
	if ep.MinTime != k.MinTime || !ep.PermitWithoutStream {
		t.Fatalf("enforcementPolicy mismatch: %+v", ep)
	}

	var zero ServerKeepalive
	if zero.hasParams() || zero.hasEnforcement() {
		t.Fatal("zero keepalive should report no params and no enforcement (preserve gRPC defaults)")
	}
}

func TestClientKeepalive_Builder(t *testing.T) {
	var zero ClientKeepalive
	if zero.isSet() {
		t.Fatal("zero client keepalive should report isSet=false")
	}
	k := ClientKeepalive{Time: 20 * time.Second, Timeout: 5 * time.Second, PermitWithoutStream: true}
	if !k.isSet() {
		t.Fatal("populated client keepalive should report isSet=true")
	}
	cp := k.clientParameters()
	if cp.Time != k.Time || cp.Timeout != k.Timeout || !cp.PermitWithoutStream {
		t.Fatalf("clientParameters mismatch: %+v", cp)
	}
}
