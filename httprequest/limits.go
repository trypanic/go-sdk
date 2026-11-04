package httprequest

import (
	"errors"
	"io"

	"github.com/trypanic/go-sdk/errorkit"
)

// DefaultMaxResponseBytes is the default cap on a successful response body,
// measured after transparent decompression. It is generous on purpose: it
// only exists to stop an upstream from exhausting the caller's memory.
const DefaultMaxResponseBytes int64 = 32 << 20 // 32 MiB

const (
	// maxErrorBodyBytes caps how much of a non-2xx body is read. The error
	// path only needs the provider's error code and message.
	maxErrorBodyBytes = 64 << 10 // 64 KiB
	// maxDrainBytes bounds the read of an unread body remainder before
	// Close, so small bodies leave the connection reusable and large ones
	// cost no more than this.
	maxDrainBytes = 256 << 10 // 256 KiB
)

// errBodyTooLarge is returned by limitedBody once more than its limit has
// been read.
var errBodyTooLarge = errors.New("response body exceeds the configured limit")

// WithMaxResponseBytes caps the size of a successful response body (after
// transparent gzip decompression). A body larger than n is rejected with a
// non-retriable errorkit.ERR_EXTERNAL_INVALID_RESPONSE. A non-positive n
// restores DefaultMaxResponseBytes.
func WithMaxResponseBytes(n int64) Option {
	return func(h *Request) { h.maxResponseBytes = normalizeMaxResponseBytes(n) }
}

func normalizeMaxResponseBytes(n int64) int64 {
	if n <= 0 {
		return DefaultMaxResponseBytes
	}
	return n
}

// limitedBody reads at most limit bytes from r and fails with
// errBodyTooLarge when r holds more.
type limitedBody struct {
	r        io.Reader
	limit    int64
	read     int64
	exceeded bool
}

func newLimitedBody(r io.Reader, limit int64) *limitedBody {
	return &limitedBody{r: io.LimitReader(r, limit+1), limit: limit}
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.exceeded {
		return 0, errBodyTooLarge
	}
	n, err := l.r.Read(p)
	l.read += int64(n)
	if l.read > l.limit {
		l.exceeded = true
		return n - int(l.read-l.limit), errBodyTooLarge
	}
	return n, err
}

// overLimit reports whether the body exceeds the limit, reading at most
// the remainder up to the limit to find out.
func (l *limitedBody) overLimit() bool {
	if !l.exceeded {
		_, _ = io.Copy(io.Discard, l)
	}
	return l.exceeded
}

// drainAndClose discards a bounded remainder of body and closes it, so the
// transport can reuse the connection for small bodies.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.CopyN(io.Discard, body, maxDrainBytes)
	_ = body.Close()
}

// buildBodyTooLargeError reports a response body over the configured limit.
func buildBodyTooLargeError(requestURL string, limit int64) error {
	appErr := errorkit.NewError(errorkit.ERR_EXTERNAL_INVALID_RESPONSE).
		With(
			errorkit.WithReason("Response body exceeds the limit of %d bytes", limit),
			errorkit.WithWrapped(errBodyTooLarge),
			errorkit.WithPayload(map[string]any{
				"url":                requestURL,
				"max_response_bytes": limit,
			}),
		)
	// A larger body will not shrink on retry.
	appErr.Metadata.Retriable = false
	return appErr
}
