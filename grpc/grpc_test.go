package grpc

import (
	"testing"
	"time"

	"github.com/cloudwego/kitex/pkg/serviceinfo"

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

func TestServerConfigAddress(t *testing.T) {
	cfg := ServerConfig{Host: "0.0.0.0", Port: 8888}
	if got := cfg.Address(); got != "0.0.0.0:8888" {
		t.Fatalf("Address() = %q, want %q", got, "0.0.0.0:8888")
	}
}

func TestDefaultOptions(t *testing.T) {
	s := DefaultServerOptions()
	if !s.EnableTracing || !s.EnableRecovery {
		t.Fatalf("DefaultServerOptions = %+v, want tracing+recovery on", s)
	}
	if !DefaultClientOptions().EnableTracing {
		t.Fatalf("DefaultClientOptions tracing should default on")
	}
}

func TestNewServerValidation(t *testing.T) {
	// nil svcInfo/handler -> config invalid
	_, err := New(ServerConfig{Host: "127.0.0.1", Port: 9000}, nil, nil)
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)

	// unresolvable address -> config invalid
	svcInfo := &serviceinfo.ServiceInfo{ServiceName: "t"}
	_, err = New(ServerConfig{Host: "bad host", Port: -1}, svcInfo, struct{}{})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
}

func TestNewClientValidation(t *testing.T) {
	_, err := NewClient(nil, ClientConfig{Hosts: []string{"127.0.0.1:9000"}})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)

	svcInfo := &serviceinfo.ServiceInfo{ServiceName: "t"}
	_, err = NewClient(svcInfo, ClientConfig{})
	wantCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
}

func TestDialOptions(t *testing.T) {
	// transport + hostports always present; tracing adds one.
	base := DialOptions(ClientConfig{Hosts: []string{"a:1"}}, ClientOptions{EnableTracing: false})
	if len(base) != 2 {
		t.Fatalf("base DialOptions len = %d, want 2", len(base))
	}
	full := DialOptions(
		ClientConfig{Hosts: []string{"a:1"}, RPCTimeout: time.Second, ConnectTimeout: time.Second},
		ClientOptions{EnableTracing: true},
	)
	if len(full) != 5 {
		t.Fatalf("full DialOptions len = %d, want 5", len(full))
	}
}
