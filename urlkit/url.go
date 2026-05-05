package urlkit

import (
	"net/url"
	"strings"

	"github.com/trypanic/go-sdk/errorkit"
)

// BuildURL constructs a URL by combining a base URL, path, and query parameters.
// Returns an error if the base URL or path is malformed.
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

	// If path is provided, combine with base
	if path != "" {
		// Parse the combined URL
		fullURL, err := url.Parse(base + path)
		if err != nil {
			return nil, errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).
				With(
					errorkit.WithReason("Invalid URL format after combining base and path"),
					errorkit.WithWrapped(err),
					errorkit.WithPayload(map[string]string{
						"base_url": base,
						"path":     path,
					}),
				)
		}
		baseURL = fullURL
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
