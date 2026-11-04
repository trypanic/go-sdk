package hertz

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/logger"
)

// testOptions returns the default profile without tracing, so tests do not
// depend on global OTel state.
func testOptions() httpserver.ServerOptions {
	o := httpserver.DefaultServerOptions()
	o.EnableTracing = false
	return o
}

func newTestServer(t *testing.T, opts httpserver.ServerOptions) *wrapperHertzServer {
	t.Helper()
	srv, ok := NewWithOptions(httpserver.ServerConfig{Host: "127.0.0.1", Port: 0}, opts).(*wrapperHertzServer)
	require.True(t, ok)
	return srv
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// logContext returns a context carrying a logger that writes to the
// returned buffer, so tests can assert SDK log lines without touching the
// global logger.
func logContext(t *testing.T) (context.Context, *syncBuffer) {
	t.Helper()
	buf := &syncBuffer{}
	zl := zerolog.New(buf)
	return logger.WithLogger(t.Context(), &zl), buf
}

type header struct{ key, value string }

// perform runs one in-memory request through the engine with the supplied
// context (ut.PerformRequest always uses context.Background).
func perform(ctx context.Context, s *wrapperHertzServer, method, uri string, body []byte, headers ...header) *protocol.Response {
	c := s.engine.NewContext()
	req := protocol.NewRequest(method, uri, nil)
	req.CopyTo(&c.Request)
	if body != nil {
		c.Request.SetBody(body)
	}
	for _, h := range headers {
		c.Request.Header.Set(h.key, h.value)
	}
	s.engine.ServeHTTP(ctx, c)
	return &c.Response
}

// stripTimestamp removes the wall-clock timestamp from a Reply body.
func stripTimestamp(t *testing.T, body []byte) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	delete(m, "timestamp")
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return string(out)
}
