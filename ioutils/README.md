# ioutils

> **Dev-only package.** These helpers exist for ad-hoc debugging and local
> testing. Production data paths should use `storage` (for persistence) or
> `marshal` (for wire encoding) directly.

JSON dump utilities built around an `io.Writer`-oriented core, plus thin
convenience wrappers for `os.Stdout` and named files.

---

## API

| Symbol | Purpose |
|---|---|
| `WriteJSON(w io.Writer, v any) error` | Encode `v` to `w` with HTML escaping disabled. |
| `SaveJSON(filename string, v any) error` | Truncating file write via `WriteJSON`. |
| `PrintJSON(v any)` | Dump `v` to `os.Stdout` between two banner lines. Errors are silently dropped. |
| `SaveProductsToJSON(filename, v any) error` | Deprecated alias for `SaveJSON`. |

---

## WriteJSON

```go
var buf bytes.Buffer
if err := ioutils.WriteJSON(&buf, payload); err != nil {
    return err
}
```

Uses `json.Encoder` with `SetEscapeHTML(false)`. The encoder writes a
trailing newline, matching the standard library. On encode failure the
error is wrapped with `errorkit.ERR_INTERNAL`.

## SaveJSON

```go
if err := ioutils.SaveJSON("dump.json", payload); err != nil {
    return err
}
```

Creates (or truncates) the file, encodes via `WriteJSON`, and closes.
A close failure is reported only when no encode error preceded it.

## PrintJSON

```go
ioutils.PrintJSON(payload)
// ================================
// {"id":1,"name":"x"}
// ================================
```

Convenience for REPL-style inspection. Encode errors are swallowed; banners
are always printed. Do not use in production code.

---

## Concurrency

All functions are pure with respect to package state. `WriteJSON` is safe
for concurrent use as long as the supplied `io.Writer` is. `SaveJSON`
opens its own file handle per call. `PrintJSON` writes to `os.Stdout`,
which the Go runtime serializes per-write but interleaves across calls.
