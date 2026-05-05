# storage

Generic key-based storage with two implementations:

- `MemoryStorage[T]` — in-memory, satisfies both `KVStorager[T]` and `AppendLogStorager[T]`.
- `FileStorage[T]` — JSON-file backed, satisfies `AppendLogStorager[T]` only.

The two interfaces are deliberately separate. `Put` always means *overwrite*;
`Append` always means *append*. No method does both.

---

## Interfaces

```go
type KVStorager[T any] interface {
    Put(key string, value *T) error
    Get(key string) (*T, bool, error)
    Delete(key string) error
    Clear() error
}

type AppendLogStorager[T any] interface {
    Append(key string, item *T) (*[]T, error)
    List(key string) (*[]T, bool, error)
    Delete(key string) error
    Clear() error
}
```

`MemoryStorage[T]` keeps its KV map and append-log map in disjoint namespaces:
`Put("k", ...)` does not touch what `Append("k", ...)` wrote, and vice versa.
`Delete("k")` and `Clear()` clear both namespaces.

---

## MemoryStorage

```go
ms := storage.NewMemory[string]()

v := "hello"
_ = ms.Put("k", &v)             // overwrite
got, ok, _ := ms.Get("k")       // → &"hello", true, nil

_, _ = ms.Append("log", &v)
list, ok, _ := ms.List("log")   // → &["hello"], true, nil

_ = ms.Delete("k")              // clears both KV and log entries at "k"
_ = ms.Clear()                  // wipes everything
```

All methods are guarded by `sync.RWMutex`. Read methods (`Get`, `List`)
allow concurrent readers; write methods take the exclusive lock.

---

## FileStorage

Persistent append-log. Each key maps to a single JSON file containing the
list of items appended to that key.

```go
fs, err := storage.NewFileStorage[string]("./data")
if err != nil {
    return err
}

a := "first"
b := "second"
_, _ = fs.Append("k", &a)       // file now: ["first"]
_, _ = fs.Append("k", &b)       // file now: ["first", "second"]

list, ok, err := fs.List("k")   // → &["first","second"], true, nil
```

### Errors

Every method returns a typed `*errorkit.AppError` with code
`ERR_STORAGE_ERROR` for I/O or decoding failures. `Delete` on a missing
key is a no-op (no error); `List` on a missing key returns `(nil, false, nil)`.

### Filename encoding

Keys are hex-encoded so that two keys that differ only in unsafe characters
cannot collide on the filesystem:

| Key         | File name (under `baseDir`) |
|-------------|------------------------------|
| `"a/b"`     | `612f62.json`                |
| `"a:b"`     | `613a62.json`                |
| `"abc"`     | `616263.json`                |

The encoding is reversible. `GetAll` decodes the filename back to the
original key, so the returned map is keyed by what the caller originally
passed to `Append`.

### Atomicity

Each write goes to `<file>.tmp` first and is then renamed onto the final
path. A failed rename leaves the target file untouched and removes the
temporary file.

### Other methods

```go
all, err := fs.GetAll()     // map[original-key][]T
_ = fs.Delete("k")          // unlink (missing → no-op)
_ = fs.Clear()              // unlink every *.json in baseDir
```

---

## Picking an implementation

| Need | Use |
|---|---|
| Process-local KV with overwrite semantics | `MemoryStorage[T]` (KVStorager) |
| Process-local append-log | `MemoryStorage[T]` (AppendLogStorager) |
| Persistent append-log across restarts | `FileStorage[T]` |

For ad-hoc dump-to-disk during development (one-shot file write, not a
running store), reach for `ioutils.WriteJSON` instead.
