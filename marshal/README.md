# marshal

JSON encoding helpers that disable HTML escaping, preventing `<`, `>`, and `&` from being mangled into `\u003c`/`\u003e`/`\u0026` sequences in API payloads and LLM messages.

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `NoEscape` | Function | Encode any value to a JSON string with HTML escaping disabled (trailing `\n`) |
| `NoEscapeBytes` | Function | Encode any value to a JSON byte slice with HTML escaping disabled (trailing `\n`) |
| `NoEscapeNoNewline` | Function | Same as `NoEscape` with the trailing newline stripped |
| `NoEscapeBytesNoNewline` | Function | Same as `NoEscapeBytes` with the trailing newline stripped |

---

## Quick Start

```go
import "github.com/trypanic/go-sdk/v2/marshal"

type Payload struct {
    Query string `json:"query"`
}

p := Payload{Query: "SELECT * FROM items WHERE name = '<widget>'"}

// String variant
s, err := marshal.NoEscape(p)
// s == "{\"query\":\"SELECT * FROM items WHERE name = '<widget>'\"}\n"

// Byte slice variant
b, err := marshal.NoEscapeBytes(p)
```

---

## Configuration / Settings

This package has no configuration struct or environment variables. All four functions are stateless.

---

## API Reference

### `NoEscape`

```go
func NoEscape(v any) (string, error)
```

Encodes `v` to JSON using `json.NewEncoder` with `SetEscapeHTML(false)` and returns the result as a `string`. The encoder appends a trailing newline (standard `json.Encoder` behaviour); the returned string includes it.

**Edge cases:**
- If `v` cannot be marshalled by `encoding/json` (e.g., contains a channel or a function), the call returns `""` and a wrapped error.
- `nil` is encoded as `"null\n"`.

**Error conditions:** Encoding failures are wrapped as
`*errorkit.AppError` with code `errorkit.ERR_INTERNAL` and reason
`"failed to marshal JSON without escaping"`.

---

### `NoEscapeBytes`

```go
func NoEscapeBytes(v any) ([]byte, error)
```

Identical to `NoEscape` but returns `[]byte` instead of `string`. Use this variant when the result will be passed directly to an API that accepts `[]byte` to avoid an extra allocation.

**Edge cases:** Same as `NoEscape`. Returns `nil` on error.

---

### `NoEscapeNoNewline` / `NoEscapeBytesNoNewline`

```go
func NoEscapeNoNewline(v any) (string, error)
func NoEscapeBytesNoNewline(v any) ([]byte, error)
```

Same encoding as `NoEscape` / `NoEscapeBytes` but with the trailing
newline byte appended by `json.Encoder` removed. Use these when the
consumer treats the entire payload as a single JSON document and an extra
`\n` would be invalid (e.g. a database column, a hashed payload, or a
prompt assembled by string concatenation).

**Error conditions:** Same as the newline-keeping variants.

---

## Real-World Usage

### LLM requests — encoding a structured user message

`marshal.NoEscape` is useful to serialise a structured payload that is
embedded in an LLM prompt, so URL-like strings and angle brackets are not
HTML-escaped.

```go
userMessageJSON, err := marshal.NoEscapeNoNewline(userMessage)
if err != nil {
    return "", err // already an *errorkit.AppError (ERR_INTERNAL)
}
answer, err := client.Execute(ctx, llmclient.RequestConfig{
    UserMessage: userMessageJSON,
    // ...
})
```

### `ioutils` debug dumps

`ioutils.WriteJSON` (and therefore `ioutils.PrintJSON` and `ioutils.SaveJSON`)
encodes with `marshal.NoEscapeBytes`, so debug dumps use the same encoding as
this package.

---

## Lifecycle / Concurrency Notes

All four functions are pure and stateless. They allocate a new `bytes.Buffer` and `json.Encoder` on every call and are safe for concurrent use without synchronisation.

---

## Dependencies

| Package | Role |
|---|---|
| `bytes` | In-memory buffer for the encoder |
| `encoding/json` | JSON encoding |
