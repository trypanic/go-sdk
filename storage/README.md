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

### Permissions

Stored items may contain personal data or tokens, so by default only the
owning account can read them: `NewFileStorage` creates `baseDir` with
`DefaultDirMode` (`0o700`) and writes files with `DefaultFileMode` (`0o600`).
Override them with options:

```go
fs, err := storage.NewFileStorage[Event]("./data",
    storage.WithDirMode(0o750),  // applied when the directory is created; umask applies
    storage.WithFileMode(0o640), // applied exactly to every storage file
)
```

An existing `baseDir` keeps its permissions. Files written before this
default changed keep their old mode until the next write of the key.

### Errors

Every method returns a typed `*errorkit.AppError`; filesystem causes are
wrapped, so `errors.Is` / `errors.As` reach them. `Delete` on a missing key is
a no-op (no error); `List` on a missing key returns `(nil, false, nil)`.

| Failure | Code | Retriable | HTTP |
|---|---|---|---|
| `Put(nil)` / `Append(nil)` (both stores) | `errorkit.ERR_VALIDATION_MISSING_FIELD` | no | 400 |
| `FileStorage` key longer than `MaxFileKeyLength` (125 bytes) | `errorkit.ERR_VALIDATION` | no | 400 |
| Permission denied (`fs.ErrPermission`) | `ERR_STORAGE_ACCESS_DENIED` | no | 502 |
| Undecodable or zero-length storage file | `ERR_STORAGE_CORRUPTED` | no | 500 |
| Any other I/O or encoding failure | `ERR_STORAGE_ERROR` | yes | 503 |

`FileStorage` keys are limited to `MaxFileKeyLength` bytes because the
hex-encoded file name (two characters per key byte plus `.json`) must fit the
255-byte file-name limit of common filesystems. `Append`, `List` and `Delete`
check the length before touching the disk.

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

### Atomicity and durability

Each write goes to a fresh temporary file in `baseDir`, created with
`os.CreateTemp` (`O_EXCL`, a random `.tmp-*` name, so an existing file or
symlink is never followed). The data is synced to disk, the temporary file is
renamed onto the final path, and the directory is synced so the rename
survives a crash. Any failure leaves the target file untouched and removes
the temporary file.

A zero-length storage file can only be the result of lost data (writes never
produce one), so reading it returns an error instead of an empty list.

### Concurrency

One `FileStorage` value is safe for concurrent use: an in-process
`sync.RWMutex` serializes its read-modify-write cycles. Use **one
`FileStorage` per directory per host**. Two values (or two processes) on the
same `baseDir` never corrupt a file, because they never share a temporary
file, but their read-modify-write cycles are not coordinated, so one
`Append` can overwrite another's concurrent update.

### Other methods

```go
all, err := fs.GetAll()     // map[original-key][]T
_ = fs.Delete("k")          // unlink (missing → no-op)
_ = fs.Clear()              // unlink every storage file of this store
```

`Clear` removes only the files the store owns: those whose names are
hex-encoded keys followed by `.json` (the files `GetAll` returns). Other files
in `baseDir`, such as `config.json`, are left untouched.

---

## Picking an implementation

| Need | Use |
|---|---|
| Process-local KV with overwrite semantics | `MemoryStorage[T]` (KVStorager) |
| Process-local append-log | `MemoryStorage[T]` (AppendLogStorager) |
| Persistent append-log across restarts | `FileStorage[T]` |

For ad-hoc dump-to-disk during development (one-shot file write, not a
running store), reach for `ioutils.WriteJSON` instead.
