package httprequest

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
)

// safeToResend reports whether a request whose attempt failed with err may
// be sent again without risking a duplicate side effect.
func safeToResend(config RequestConfig, err error, sent bool, rc RetryConfig) bool {
	return rc.RetryNonIdempotent || isIdempotent(config) || (!sent && neverSent(err))
}

// isIdempotent reports whether config is idempotent by method (RFC 9110
// §9.2.2) or by an idempotency key header, the same rule net/http applies
// to its own transparent retries.
func isIdempotent(config RequestConfig) bool {
	switch config.Method {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace,
		http.MethodPut, http.MethodDelete:
		return true
	}
	for key, value := range config.Headers {
		canonical := http.CanonicalHeaderKey(key)
		if (canonical == "Idempotency-Key" || canonical == "X-Idempotency-Key") && value != "" {
			return true
		}
	}
	return false
}

// neverSent reports whether err proves the request never left the process:
// the connection to the server could not be established.
func neverSent(err error) bool {
	opErr, ok := errors.AsType[*net.OpError](err)
	return ok && opErr.Op == "dial"
}

// doTracked observes the whole attempt, including redirects. A redirect is
// evidence of a response even when a custom transport emits no trace events.
func doTracked(client *http.Client, req *http.Request) (*http.Response, bool, error) {
	var sent atomic.Bool
	ctx := httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		WroteHeaders: func() { sent.Store(true) },
		WroteRequest: func(httptrace.WroteRequestInfo) { sent.Store(true) },
	})
	attemptClient := *client
	attemptClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		sent.Store(true)
		if client.CheckRedirect != nil {
			return client.CheckRedirect(next, via)
		}
		// Preserve net/http's default policy when the caller has no override.
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	res, err := attemptClient.Do(req.WithContext(ctx))
	return res, sent.Load() || res != nil, err
}
