// Package redact holds the redaction rules shared by the SDK's HTTP
// packages (httprequest, httpserver/hertz, llmclient), so that URLs and
// headers are sanitized the same way in error payloads, logs and spans.
package redact

import (
	"errors"
	"net/url"
	"strings"
)

// Placeholder replaces a redacted value.
const Placeholder = "[REDACTED]"

// queryPlaceholder replaces each query value. It avoids characters that
// url.URL.String would percent-encode, so the result stays readable.
const queryPlaceholder = "REDACTED"

// invalidURL is returned for input that cannot be parsed. The raw text is
// never echoed, because it may hold credentials.
const invalidURL = "[invalid URL]"

// URL returns raw without credentials: the userinfo and fragment are
// dropped and every query value is replaced while query keys are kept.
// Scheme, host and path are preserved. Unparseable input returns a fixed
// marker instead of the raw text.
func URL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return invalidURL
	}
	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""
	if u.RawQuery != "" {
		parts := strings.Split(u.RawQuery, "&")
		for i, part := range parts {
			key, _, _ := strings.Cut(part, "=")
			parts[i] = key + "=" + queryPlaceholder
		}
		u.RawQuery = strings.Join(parts, "&")
	}
	return u.String()
}

// URLError returns err with the URL of a top-level *url.Error sanitized by
// URL. The copy keeps Op and the wrapped error, so errors.Is and errors.As
// still see the cause (for example context.DeadlineExceeded or a
// *net.OpError). Any other error is returned unchanged.
func URLError(err error) error {
	urlErr, ok := errors.AsType[*url.Error](err)
	if !ok || urlErr != err {
		return err
	}
	sanitized := *urlErr
	sanitized.URL = URL(urlErr.URL)
	return &sanitized
}
