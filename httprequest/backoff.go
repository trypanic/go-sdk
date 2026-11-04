package httprequest

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	sharedbackoff "github.com/trypanic/go-sdk/v2/internal/backoff"
)

// fullJitter returns a random delay in [0, d] ("full jitter"), so clients
// that failed together do not retry in lockstep.
func fullJitter(d time.Duration) time.Duration {
	return sharedbackoff.Jitter(d)
}

// parseRetryAfter returns the wait a Retry-After header asks for, given as
// delay-seconds or an HTTP-date (RFC 9110 §10.2.3). It returns 0 when the
// header is absent, malformed or already in the past.
func parseRetryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(header, 10, 64); err == nil {
		if secs <= 0 || secs > int64(time.Duration(1<<63-1)/time.Second) {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// retryDelay picks the wait before the next attempt: the server's
// Retry-After when it sent one, otherwise the jittered backoff. It reports
// false when the next attempt should not happen: the server asks for more
// than maxDelay, or the wait would outlast the context deadline.
func retryDelay(ctx context.Context, backoff, retryAfter, maxDelay time.Duration) (time.Duration, bool) {
	delay := fullJitter(backoff)
	if retryAfter > 0 {
		if retryAfter > maxDelay {
			return 0, false
		}
		delay = retryAfter
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return 0, false
	}
	return delay, true
}
