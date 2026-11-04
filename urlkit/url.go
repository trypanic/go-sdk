package urlkit

import (
	"net/url"
	"strings"

	"github.com/trypanic/go-sdk/errorkit"
)

// BuildURL constructs a URL by combining a base URL, path, and query parameters.
// Returns an error if the base URL or path is malformed.
//
// path is always treated as a URL path, never as URL syntax: it cannot change
// the scheme, host, query or fragment of base. A missing leading slash is
// added, and path is appended to the path of base (a trailing slash on base
// is not doubled). path may contain percent-escapes. A path that contains
// '?' or '#', a "." or ".." segment (also when percent-encoded) or an
// invalid escape is rejected with ERR_VALIDATION_INVALID_FORMAT. The query of
// base is kept; params are merged into it and win on equal keys.
//
// Example:
//
//	url, err := BuildURL("https://api.example.com", "/users", map[string]string{
//	    "page": "1",
//	    "limit": "10",
//	})
//	// Result: https://api.example.com/users?page=1&limit=10
func BuildURL(base, path string, params map[string]string) (*url.URL, error) {
	// Validate base URL is not empty
	if base == "" {
		return nil, errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
			With(errorkit.WithReason("Base URL is required"))
	}

	// Parse base URL first to validate it
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).
			With(
				errorkit.WithReason("Invalid base URL format"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(map[string]string{
					"base_url": base,
				}),
			)
	}

	if path != "" {
		if err := appendPath(baseURL, base, path); err != nil {
			return nil, err
		}
	}

	// Add query parameters if provided
	if len(params) > 0 {
		query := baseURL.Query()
		for key, value := range params {
			query.Set(key, value)
		}
		baseURL.RawQuery = query.Encode()
	}

	return baseURL, nil
}

// BuildURLString is a convenience wrapper that returns the URL as a string
// instead of a *url.URL pointer.
//
// Example:
//
//	urlStr, err := BuildURLString("https://api.example.com", "/users", map[string]string{
//	    "page": "1",
//	})
//	// Result: "https://api.example.com/users?page=1"
func BuildURLString(base, path string, params map[string]string) (string, error) {
	parsedURL, err := BuildURL(base, path, params)
	if err != nil {
		return "", err
	}
	return parsedURL.String(), nil
}

// MustBuildURL is like BuildURL but panics if an error occurs.
// Use this only when you're certain the inputs are valid (e.g., hard-coded URLs).
//
// Example:
//
//	url := MustBuildURL("https://api.example.com", "/users", nil)
func MustBuildURL(base, path string, params map[string]string) *url.URL {
	parsedURL, err := BuildURL(base, path, params)
	if err != nil {
		panic(err) // nosemgrep: no-panic-outside-init -- Must* stdlib idiom; caller opts in by name
	}
	return parsedURL
}

// JoinPath joins a base URL with one or more path segments, normalizing
// any combination of leading/trailing slashes between them. Empty segments
// are skipped. The base URL is parsed for validity; query strings on the
// base are preserved.
//
// Example:
//
//	JoinPath("https://api.example.com/", "/v1/", "/users/")
//	// → "https://api.example.com/v1/users"
func JoinPath(base string, segments ...string) (string, error) {
	if base == "" {
		return "", errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
			With(errorkit.WithReason("Base URL is required"))
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("Invalid base URL format"),
			errorkit.WithWrapped(err),
			errorkit.WithPayload(map[string]string{"base_url": base}),
		)
	}

	parts := []string{strings.TrimRight(u.Path, "/")}
	for _, s := range segments {
		s = strings.Trim(s, "/")
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	u.Path = strings.Join(parts, "/")
	return u.String(), nil
}

// appendPath appends path to the path of u in place, following the rules
// documented on BuildURL.
func appendPath(u *url.URL, base, path string) error {
	invalid := func(reason string, cause error) error {
		opts := []errorkit.ErrorOption{
			errorkit.WithReason("%s", reason),
			errorkit.WithPayload(map[string]string{
				"base_url": base,
				"path":     path,
			}),
		}
		if cause != nil {
			opts = append(opts, errorkit.WithWrapped(cause))
		}
		return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(opts...)
	}

	if strings.ContainsAny(path, "?#") {
		return invalid("Path must not contain a query or fragment", nil)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	rawPath := strings.TrimRight(u.EscapedPath(), "/") + path
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		return invalid("Path contains an invalid escape sequence", err)
	}
	if hasDotSegment(decoded) {
		return invalid("Path must not contain dot segments", nil)
	}
	u.Path = decoded
	u.RawPath = rawPath
	return nil
}

// hasDotSegment reports whether p, split on '/', has a "." or ".." segment.
func hasDotSegment(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}
