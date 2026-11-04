package httprequest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// scriptedTransport answers with the scripted responses in order (the last
// one repeats) and records when each request arrived.
type scriptedTransport struct {
	mu        sync.Mutex
	responses []scriptedResponse
	arrivals  []time.Time
}

type scriptedResponse struct {
	status     int
	retryAfter string
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := min(len(s.arrivals), len(s.responses)-1)
	s.arrivals = append(s.arrivals, time.Now())
	r := s.responses[i]
	h := http.Header{}
	if r.retryAfter != "" {
		h.Set("Retry-After", r.retryAfter)
	}
	return &http.Response{
		StatusCode: r.status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    req,
	}, nil
}

func (s *scriptedTransport) gaps() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []time.Duration
	for i := 1; i < len(s.arrivals); i++ {
		out = append(out, s.arrivals[i].Sub(s.arrivals[i-1]))
	}
	return out
}

func newScripted(rs ...scriptedResponse) (*scriptedTransport, *http.Client) {
	tr := &scriptedTransport{responses: rs}
	return tr, &http.Client{Transport: tr}
}

var getReq = RequestConfig{Method: http.MethodGet, URL: "http://upstream.invalid/x"}

func TestBackoffUsesFullJitter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rc := RetryConfig{MaxRetries: 4, InitialDelay: 100 * time.Millisecond, MaxDelay: time.Second}
		caps := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

		belowCap := false
		for range 20 {
			tr, client := newScripted(scriptedResponse{status: http.StatusServiceUnavailable})
			_ = NewWithOptions(client, WithRetryConfig(rc)).Do(t.Context(), getReq, nil)

			gaps := tr.gaps()
			require.Len(t, gaps, len(caps))
			for i, gap := range gaps {
				require.LessOrEqual(t, gap, caps[i], "attempt %d waited longer than its backoff cap", i+2)
				if gap < caps[i] {
					belowCap = true
				}
			}
		}
		require.True(t, belowCap, "20 runs never waited less than the exponential cap: the delay has no jitter")
	})
}

func TestRetryAfterIsHonored(t *testing.T) {
	t.Parallel()

	rc := RetryConfig{MaxRetries: 2, InitialDelay: 10 * time.Millisecond, MaxDelay: 5 * time.Second}

	t.Run("delay in seconds", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			tr, client := newScripted(
				scriptedResponse{status: http.StatusTooManyRequests, retryAfter: "2"},
				scriptedResponse{status: http.StatusOK},
			)
			require.NoError(t, NewWithOptions(client, WithRetryConfig(rc)).Do(t.Context(), getReq, nil))
			require.Equal(t, []time.Duration{2 * time.Second}, tr.gaps())
		})
	})

	t.Run("HTTP date", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			when := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
			tr, client := newScripted(
				scriptedResponse{status: http.StatusServiceUnavailable, retryAfter: when},
				scriptedResponse{status: http.StatusOK},
			)
			require.NoError(t, NewWithOptions(client, WithRetryConfig(rc)).Do(t.Context(), getReq, nil))
			gaps := tr.gaps()
			require.Len(t, gaps, 1)
			require.InDelta(t, 3*time.Second, gaps[0], float64(time.Second))
		})
	})

	t.Run("beyond MaxDelay stops retrying", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			tr, client := newScripted(
				scriptedResponse{status: http.StatusTooManyRequests, retryAfter: "60"},
				scriptedResponse{status: http.StatusOK},
			)
			err := NewWithOptions(client, WithRetryConfig(rc)).Do(t.Context(), getReq, nil)

			_ = requireAppError(t, err, errorkit.ERR_CLIENT_RATE_LIMIT, true)
			require.Empty(t, tr.gaps(), "a retry before the advertised window fails by design")
		})
	})

	t.Run("beyond the context deadline stops retrying", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			tr, client := newScripted(
				scriptedResponse{status: http.StatusTooManyRequests, retryAfter: "2"},
				scriptedResponse{status: http.StatusOK},
			)
			start := time.Now()
			err := NewWithOptions(client, WithRetryConfig(rc)).Do(ctx, getReq, nil)

			_ = requireAppError(t, err, errorkit.ERR_CLIENT_RATE_LIMIT, true)
			require.Empty(t, tr.gaps())
			require.Zero(t, time.Since(start), "Do returns the upstream error at once instead of sleeping into a timeout")
		})
	})
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		header string
		want   time.Duration
	}{
		"absent":       {header: "", want: 0},
		"seconds":      {header: "7", want: 7 * time.Second},
		"zero":         {header: "0", want: 0},
		"negative":     {header: "-3", want: 0},
		"garbage":      {header: "soon", want: 0},
		"future date":  {header: now.Add(90 * time.Second).Format(http.TimeFormat), want: 90 * time.Second},
		"past date":    {header: now.Add(-time.Minute).Format(http.TimeFormat), want: 0},
		"huge seconds": {header: "99999999999999999999", want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, parseRetryAfter(tc.header, now))
		})
	}
}
