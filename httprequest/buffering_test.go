package httprequest

import (
	"bytes"
	"io"
	"net/http"
	"runtime"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/logger"
)

// staticTransport answers every request with body, without any network.
type staticTransport struct{ body []byte }

func (s staticTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(s.body)),
		Request:    req,
	}, nil
}

// allocPerOp returns the average heap bytes allocated by fn.
func allocPerOp(runs int, fn func()) uint64 {
	fn() // warm up lazy initialisation outside the measurement.
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		fn()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}

// TestDecodedResponseIsNotBufferedTwice checks that Do does not keep a
// second copy of a decoded body when no audit line needs its content.
// Not parallel: it measures process-wide allocation.
func TestDecodedResponseIsNotBufferedTwice(t *testing.T) {
	const size = 4 << 20
	body := bytes.Repeat([]byte("x"), size)
	client := &http.Client{Transport: staticTransport{body: body}}
	cfg := RequestConfig{Method: http.MethodGet, URL: "http://upstream.invalid/data"}

	var discard bytes.Buffer
	zl := zerolog.New(&discard)
	ctx := logger.WithLogger(t.Context(), &zl)

	h := &Request{maxResponseBytes: DefaultMaxResponseBytes}
	baseline := allocPerOp(4, func() {
		var out []byte
		_ = h.processResponse(newLimitedBody(bytes.NewReader(body), DefaultMaxResponseBytes), &out)
	})

	cases := map[string]Requester{
		"audit off":                      NewWithOptions(client),
		"audit on with default redactor": NewWithOptions(client, WithAuditLog()),
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			got := allocPerOp(4, func() {
				discard.Reset()
				var out []byte
				require.NoError(t, r.Do(ctx, cfg, &out))
				require.Len(t, out, size)
			})
			require.Less(t, got, baseline+size/2,
				"Do allocated %d bytes per call, decoding alone needs %d: the body is buffered twice", got, baseline)
		})
	}
}

func TestAuditLogStillSeesBodyForContentRedactors(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: staticTransport{body: []byte(`{"ok":true}`)}}
	cfg := RequestConfig{Method: http.MethodGet, URL: "http://upstream.invalid/data"}

	for name, tc := range map[string]struct {
		opts []Option
		want string
	}{
		"default redactor": {opts: []Option{WithAuditLog()}, want: `"response.body":"[REDACTED]"`},
		"raw bodies":       {opts: []Option{WithAuditLog(), WithRawBodies()}, want: `"response.body":"{\"ok\":true}"`},
		"custom redactor": {
			opts: []Option{WithAuditLog(), WithBodyRedactor(func(b []byte) []byte { return bytes.ToUpper(b) })},
			want: `"response.body":"{\"OK\":TRUE}"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logBuf bytes.Buffer
			zl := zerolog.New(&logBuf)
			ctx := logger.WithLogger(t.Context(), &zl)

			var out map[string]any
			require.NoError(t, NewWithOptions(client, tc.opts...).Do(ctx, cfg, &out))
			require.Equal(t, true, out["ok"])
			require.Contains(t, logBuf.String(), tc.want)
		})
	}
}

func BenchmarkDoDecodeAuditOff(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 1<<20)
	r := NewWithOptions(&http.Client{Transport: staticTransport{body: body}})
	cfg := RequestConfig{Method: http.MethodGet, URL: "http://upstream.invalid/data"}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		var out []byte
		if err := r.Do(b.Context(), cfg, &out); err != nil {
			b.Fatal(err)
		}
	}
}
