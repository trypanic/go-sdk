package redact

import (
	"net/http"
	"net/url"
	"strings"
)

// sensitiveHeaderNames lists credential headers by exact (lowercase) name.
// Most are also caught by sensitiveHeaderFragments; the list documents the
// well-known names and covers those without a matching fragment.
var sensitiveHeaderNames = map[string]struct{}{
	"link":                 {},
	"refresh":              {},
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

// HeaderValue masks credential and compound URL headers (Link and Refresh).
// Referer, Location and Content-Location use URL redaction; opaque URLs are
// masked in full. Other header values are preserved.
func HeaderValue(name, value string) string {
	if IsSensitiveHeader(name) {
		return Placeholder
	}
	switch strings.ToLower(name) {
	case "referer", "location", "content-location":
		parsed, err := url.Parse(value)
		if err != nil {
			return invalidURL
		}
		if parsed.Opaque != "" {
			return Placeholder
		}
		return URL(value)
	}
	return value
}

// Headers returns a log-safe copy of h: multiple values are joined with
// ", " after each is sanitized. Sensitive headers are replaced by Placeholder.
// Keys are kept, so the log still shows which headers were present.
func Headers(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		if IsSensitiveHeader(name) {
			out[name] = Placeholder
			continue
		}
		sanitized := make([]string, len(values))
		for i, value := range values {
			sanitized[i] = HeaderValue(name, value)
		}
		out[name] = strings.Join(sanitized, ", ")
	}
	return out
}
