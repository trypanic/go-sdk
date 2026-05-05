# urlkit

URL construction helpers that safely combine a base URL, an optional path, and a map of query parameters into a validated `*url.URL` or plain string.

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `BuildURL` | Function | Combine base, path, and query params into a `*url.URL`, returning a structured error on invalid input |
| `BuildURLString` | Function | Convenience wrapper around `BuildURL` that returns the URL as a `string` |
| `MustBuildURL` | Function | Like `BuildURL` but panics instead of returning an error; use only for hard-coded, known-good URLs |
| `JoinPath` | Function | Append one or more path segments to a base URL, normalizing leading/trailing slashes between them |

---

## Quick Start

```go
import "github.com/trypanic/go-sdk/urlkit"

u, err := urlkit.BuildURL("https://api.example.com", "/users", map[string]string{
    "page":  "1",
    "limit": "10",
})
if err != nil {
    // handle errorkit.AppError
}
fmt.Println(u.String()) // https://api.example.com/users?limit=10&page=1

// String shortcut
s, err := urlkit.BuildURLString("https://api.example.com", "/items", nil)

// Panic variant for compile-time-known bases
base := urlkit.MustBuildURL("https://api.example.com", "/health", nil)

// Path-join: trims redundant slashes and skips empty segments
j, _ := urlkit.JoinPath("https://api.example.com/", "/v1/", "/users/")
// j == "https://api.example.com/v1/users"
```

---

## Configuration / Settings

This package has no configuration struct or environment variables. All inputs are passed directly as function arguments.

---

## API Reference

### `BuildURL`

```go
func BuildURL(base, path string, params map[string]string) (*url.URL, error)
```

Constructs a `*url.URL` by:
1. Validating that `base` is non-empty.
2. Parsing `base` with `url.Parse` to verify it is a well-formed URL.
3. If `path` is non-empty, concatenating `base + path` and parsing the result.
4. Encoding each entry from `params` into the query string using `url.Values.Set`.

**Edge cases:**
- `path` may be an empty string; the base URL is returned unchanged.
- `params` may be `nil` or empty; no query string is appended.
- Query parameters are URL-encoded by `url.Values.Encode`, which sorts keys lexicographically.
- The function does not join paths with a separator; if `base` has no trailing slash and `path` has no leading slash, the strings are concatenated directly (e.g., `"https://api.com"` + `"/v1"` → `"https://api.com/v1"`).

**Error conditions:**

| Condition | errorkit code |
|---|---|
| `base` is an empty string | `ERR_VALIDATION_MISSING_FIELD` — reason: `"Base URL is required"` |
| `base` cannot be parsed by `url.Parse` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"Invalid base URL format"`, payload: `{"base_url": base}` |
| `base + path` cannot be parsed by `url.Parse` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"Invalid URL format after combining base and path"`, payload: `{"base_url": base, "path": path}` |

---

### `BuildURLString`

```go
func BuildURLString(base, path string, params map[string]string) (string, error)
```

Calls `BuildURL` and returns `(*url.URL).String()`. Returns `""` and the same error on failure.

---

### `MustBuildURL`

```go
func MustBuildURL(base, path string, params map[string]string) *url.URL
```

Calls `BuildURL` and panics with the returned error if construction fails. Intended for package-level `var` blocks or `init()` functions where the URL is a hard-coded constant and a failure represents a programming error, not a runtime condition.

---

## Real-World Usage

### Vendor datasource — building signed API URLs

A typical ingestion caller uses `BuildURL` to construct both GET and POST endpoint URLs with dynamic parameters before signing them.

```go
// GET — product overview with scroll pagination
url, err := urlkit.BuildURL(t.cfg.APIBase, ProductOverviewEndpoint, params)
if err != nil {
    return "", err
}
return url.String(), nil

// POST — signed request; params are sent in the body, not the query string
url, err := urlkit.BuildURL(t.cfg.APIBase, endpoint, nil)
if err != nil {
    return err
}
cfg := httprequest.RequestConfig{
    URL:    url.String(),
    Method: "POST",
    Body:   body,
}
```

### OAuth use case — building the authorization redirect URL

An OAuth client typically uses `BuildURL` to construct the authorization redirect URL, embedding `response_type`, `redirect_uri`, `client_id`, and a CSRF state token as query parameters.

```go
params := map[string]string{
    "response_type": responseTypeCode,
    "redirect_uri":  a.cfg.RedirectURI,
    "force_auth":    forceAuthEnabled,
    "client_id":     a.cfg.ClientID,
    "state":         state,
}

authURL, err := urlkit.BuildURL(a.cfg.AuthURL, "", params)
if err != nil {
    return "", err
}
return authURL.String(), nil
```

---

## Lifecycle / Concurrency Notes

All three functions are pure and stateless. They are safe for concurrent use without additional synchronisation.

---

## Dependencies

| Package | Role |
|---|---|
| `net/url` | URL parsing, construction, and query-string encoding |
| `github.com/trypanic/go-sdk/errorkit` | Structured error wrapping |
