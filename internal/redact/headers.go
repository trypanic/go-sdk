package redact

import (
	"net/http"
	"strings"
)

// sensitiveHeaderNames lists credential headers by exact (lowercase) name.
// Most are also caught by sensitiveHeaderFragments; the list documents the
// well-known names and covers those without a matching fragment.
var sensitiveHeaderNames = map[string]struct{}{
	"authorization":        {},
	"proxy-authorization":  {},
	"cookie":               {},
	"set-cookie":           {},
	"x-api-key":            {},
	"api-key":              {},
	"x-auth-token":         {},
	"x-access-token":       {},
	"x-goog-api-key":       {},
	"x-amz-security-token": {},
	"x-csrf-token":         {},
	"x-xsrf-token":         {},
}

// sensitiveHeaderFragments masks vendor variants such as X-Foo-Token or
// X-Bar-Secret. A false positive only masks a harmless value.
var sensitiveHeaderFragments = []string{
	"token", "secret", "key", "auth", "password", "session", "cookie", "signature", "credential",
}

// IsSensitiveHeader reports whether the header name (any case) may carry a
// credential and must never be logged in clear.
func IsSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := sensitiveHeaderNames[lower]; ok {
		return true
	}
	for _, fragment := range sensitiveHeaderFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// HeaderValue returns Placeholder for a sensitive header and value
// otherwise.
func HeaderValue(name, value string) string {
	if IsSensitiveHeader(name) {
		return Placeholder
	}
	return value
}

// Headers returns a log-safe copy of h: multiple values are joined with
// ", " and the values of sensitive headers are replaced by Placeholder. The
// keys are kept, so the log still shows which headers were present.
func Headers(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		out[name] = HeaderValue(name, strings.Join(values, ", "))
	}
	return out
}
