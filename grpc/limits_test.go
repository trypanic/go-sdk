package grpc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/v2/grpc/internal/testsvc"
)

// With MaxConcurrentStreams = 1, a second stream on the same connection does
// not reach a handler while the first one is open.
func TestServerConfig_MaxConcurrentStreamsApplied(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32 // handlers ever started; never decremented
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	impl := &echoImpl{
		bidi: func(s grpc.ServerStream) error {
			calls.Add(1)
			started <- struct{}{}
			select {
			case <-release:
			case <-s.Context().Done():
			}
			return nil
		},
	}
	srv, lis := serve(t, ServerConfig{MaxConcurrentStreams: 1}, impl)
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{})
	t.Cleanup(func() { _ = cc.Close() })

	ctx := testCtx(t)
	first, err := cc.NewStream(ctx, testsvc.BidiStreamDesc, testsvc.BidiMethod)
	if err != nil {
		t.Fatalf("first NewStream: %v", err)
	}
	if err := first.SendMsg(wrapperspb.String("hold")); err != nil {
		t.Fatalf("first SendMsg: %v", err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first stream never reached the handler")
	}

	// The second stream waits for stream quota; give it a short deadline.
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	second, err := cc.NewStream(short, testsvc.BidiStreamDesc, testsvc.BidiMethod)
	if err == nil {
		err = second.RecvMsg(new(wrapperspb.StringValue))
	}
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("second stream: code = %s (err %v), want %s", status.Code(err), err, codes.DeadlineExceeded)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d handlers started on one connection, want 1", n)
	}
	close(release)
}
