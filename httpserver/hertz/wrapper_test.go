package hertz

import (
	"context"
	"testing"

	"github.com/trypanic/go-sdk/httpserver"
)

// Compile-time assertion: Hertz adapter satisfies the SDK HTTPServer
// interface, including the new Shutdown method.
var _ httpserver.HTTPServer = (*wrapperHertzServer)(nil)

func TestNewWithOptionsBackfillsZeroFields(t *testing.T) {
	t.Parallel()

	srv := NewWithOptions(httpserver.ServerConfig{Host: "127.0.0.1", Port: 0},
		httpserver.ServerOptions{
			EnableHealth: true,
		},
	)
	w, ok := srv.(*wrapperHertzServer)
	if !ok {
		t.Fatalf("NewWithOptions returned %T, want *wrapperHertzServer", srv)
	}
	if w.replyOpts.Layout == "" || w.replyOpts.Clock == nil {
		t.Fatalf("Reply layout/clock must be backfilled, got %+v", w.replyOpts)
	}
}

func TestShutdownIsExposedOnHTTPServer(t *testing.T) {
	t.Parallel()

	var srv httpserver.HTTPServer = NewWithOptions(
		httpserver.ServerConfig{Host: "127.0.0.1", Port: 0},
		httpserver.ServerOptions{},
	)
	// Shutdown on an un-started engine returns "engine is not running"; the
	// guarantee tested here is that Shutdown is reachable through the SDK
	// interface and does not panic.
	_ = srv.Shutdown(context.Background())
}
