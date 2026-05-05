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
import "github.com/trypanic/go-sdk/marshal"

type Payload struct {
    Query string `json:"query"`
}

p := Payload{Query: "SELECT * FROM items WHERE name = '<widget>'"}

// String variant
s, err := marshal.NoEscape(p)
// s == `{"query":"SELECT * FROM items WHERE name = '<widget>'"}`

// Byte slice variant
b, err := marshal.NoEscapeBytes(p)
```

---

## Configuration / Settings

This package has no configuration struct or environment variables. Both functions are stateless.

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

### LLM gateway — encoding user messages

`marshal.NoEscape` is typically used to serialise the structured payload sent to the LLM, ensuring URL-like strings and angle-bracket characters are not HTML-escaped before being embedded in the prompt body.

```go
userMessageJSON, err := marshal.NoEscape(userMessage)
if err != nil {
    return "", wrapInternal(err, "failed to marshal user message")
}
setup := llmclient.LLMRequestConfig{
    UserMessage: string(userMessageJSON),
    // ...
}
resp, err := l.client.Execute(ctx, setup)
```

### Internal ioutils printer

`marshal.NoEscape` is also the encoder used by `ioutils.PrintJSON` to dump debug values to stdout.

```go
// go-sdk/ioutils/printer.go
data, _ := marshal.NoEscape(v)
fmt.Println(string(data))
```

---

## Lifecycle / Concurrency Notes

Both functions are pure and stateless. They allocate a new `bytes.Buffer` and `json.Encoder` on every call and are safe for concurrent use without synchronisation.

---

## Dependencies

| Package | Role |
|---|---|
| `bytes` | In-memory buffer for the encoder |
| `encoding/json` | JSON encoding |
