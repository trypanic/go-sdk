package httprequest

import (
	"errors"
	"net"
	"net/http"
)

// safeToResend reports whether a request whose attempt failed with err may
// be sent again without risking a duplicate side effect.
func safeToResend(config RequestConfig, err error, rc RetryConfig) bool {
	return rc.RetryNonIdempotent || isIdempotent(config) || neverSent(err)
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
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}
