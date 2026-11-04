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
2. Parsing `base` once with `url.Parse` to verify it is a well-formed URL.
3. If `path` is non-empty, appending it to the path of `base` as a **path**
   (never as URL syntax).
4. Merging each entry from `params` into the query string of `base` using `url.Values.Set`.

**Path rules:**
- `path` can never change the scheme, host, query or fragment of `base`.
  `BuildURL("https://api.good.com", "@evil.com/x", nil)` yields
  `https://api.good.com/@evil.com/x`.
- A missing leading slash is added: `"users"` is treated as `"/users"`.
- `path` is appended to the path of `base`; a trailing slash on `base` is not
  doubled (`"https://api.com/v1/"` + `"/users"` → `"https://api.com/v1/users"`).
  A trailing slash on `path` is kept.
- `path` may contain percent-escapes (`"/files/a%2Fb"` is kept as is).
- `path` is rejected with `ERR_VALIDATION_INVALID_FORMAT` when it contains
  `?` or `#`, a `.` or `..` segment (also percent-encoded, e.g. `%2e%2e`), or
  an invalid escape sequence. Pass query values through `params` instead.

**Edge cases:**
- `path` may be an empty string; the base URL is returned unchanged.
- `params` may be `nil` or empty; no query string is appended.
- A query string on `base` (API keys, signed URLs) is kept; `params` are
  merged into it and win on equal keys.
- Query parameters are URL-encoded by `url.Values.Encode`, which sorts keys lexicographically.

**Error conditions:**

| Condition | errorkit code |
|---|---|
| `base` is an empty string | `ERR_VALIDATION_MISSING_FIELD` — reason: `"Base URL is required"` |
| `base` cannot be parsed by `url.Parse` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"Invalid base URL format"`, payload: `{"base_url": base}` |
| `path` contains `?` or `#` | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"Path must not contain a query or fragment"`, payload: `{"base_url": base, "path": path}` |
| `path` contains an invalid escape sequence | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"Path contains an invalid escape sequence"`, payload as above |
| `path` contains a `.` or `..` segment | `ERR_VALIDATION_INVALID_FORMAT` — reason: `"Path must not contain dot segments"`, payload as above |

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
