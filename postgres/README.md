# `database` — PostgreSQL Pool & Generic Stored-Procedure Runner

Package `database` provides:

1. A pgx/v5 connection-pool factory with explicit DSN handling and pool tuning.
2. A generic stored-procedure runner that scans rows into `T` via `pgx.RowToStructByNameLax`
   for structs or `pgx.RowTo` for scalars.
3. An optional, opt-in tracing wrapper that creates one span per stored-procedure call.

The package itself does **not** install pgx tracing on the pool — telemetry is opt-in.
Callers that want pgx-driver tracing wire it up themselves (see [Adding pgx tracing](#adding-pgx-tracing)).

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Constructors](#constructors)
  - [NewPostgresClient](#newpostgresclient)
  - [NewPostgresClientFromEnv](#newpostgresclientfromenv)
- [DSN Normalization](#dsn-normalization)
- [Stored Procedures](#stored-procedures)
  - [Interface](#interface)
  - [Constructors](#stored-procedure-constructors)
  - [Method Reference](#method-reference)
- [Tracing](#tracing)
  - [Stored-procedure span wrapper](#stored-procedure-span-wrapper)
  - [Adding pgx tracing](#adding-pgx-tracing)
- [Real-World Usage](#real-world-usage)
- [Lifecycle](#lifecycle)
- [Dependencies](#dependencies)

---

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `Config` | struct | DSN + pool tuning fields |
| `PostgresEnvVar` | const | `"POSTGRES_DSN"` — the only env var the SDK reads, and only via `NewPostgresClientFromEnv` |
| `NewPostgresClient` | func | Builds a `*pgxpool.Pool` from an explicit `Config`; never reads env |
| `NewPostgresClientFromEnv` | func | Reads `POSTGRES_DSN`, then forwards to `NewPostgresClient` |
| `normalizePostgresDSN` | func (unexported) | Re-encodes `@` characters in passwords so pgx can parse the DSN |
| `Querier` | interface | `Query` + `QueryRow`; `*pgxpool.Pool` and `pgx.Tx` both implement it |
| `Execer` | interface | `Exec` |
| `ClientPort` | interface | `Querier` + `Execer` + `Begin` + `Close` |
| `StoredProcedurer[T]` | interface | Generic stored-procedure runner: `Query`, `QueryRow`, `QueryJSON`, `QueryRowJSON` |
| `StoredProcedure[T]` | struct | Concrete implementation backed by a `Querier` |
| `NewStoredProcedure[T]` | func | Returns a `StoredProcedurer[T]` wrapped with the global tracer (compatibility) |
| `WrapWithTracing[T]` | func | Wraps a `StoredProcedurer[T]` using the global tracer |
| `WrapWithInstrumenter[T]` | func | Wraps a `StoredProcedurer[T]` using an explicit `*telemetry.Instrumenter` |

---

## Quick Start

```go
import (
    "context"
    "log"
    "time"

    "github.com/trypanic/go-sdk/database"
)

pool, err := database.NewPostgresClient(database.Config{
    DSN:             "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
    MaxConnLifetime: time.Hour,
    MaxConnIdleTime: time.Minute,
    MaxConns:        10,
    MinConns:        2,
})
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

type Item struct {
    ID   int64  `db:"id"`
    Name string `db:"name"`
}

sp := database.NewStoredProcedure[Item](pool)

ctx := context.Background()

// Plain positional args
item, err := sp.QueryRow(ctx, "SELECT * FROM get_item($1)", 42)

// Many rows
items, err := sp.Query(ctx, "SELECT * FROM list_items($1)", "active")

// JSON argument appended after positional args
created, err := sp.QueryJSON(
    ctx,
    "SELECT * FROM insert_items($1::jsonb)",
    []Item{{Name: "alpha"}, {Name: "beta"}},
)
```

---

## Configuration

```go
type Config struct {
    DSN             string        // postgres://user:pass@host:port/db
    MaxConnLifetime time.Duration // max time a conn may be reused
    MaxConnIdleTime time.Duration // max idle time before a conn is closed
    MaxConns        int32         // pool ceiling
    MinConns        int32         // pool floor (pre-warmed connections)
}
```

The package reads no environment variables in `NewPostgresClient`. The convenience helper
`NewPostgresClientFromEnv` reads `POSTGRES_DSN` once at construction and forwards the rest of
`Config` unchanged. SDK consumers that prefer a different env var name should read it themselves
and assign `Config.DSN`.

---

## Constructors

### NewPostgresClient

```go
func NewPostgresClient(cfg Config) (*pgxpool.Pool, error)
```

Behavior:

1. Returns `ERR_SYSTEM_CONFIG_INVALID` if `cfg.DSN` is empty.
2. Calls `normalizePostgresDSN(cfg.DSN)` so passwords containing `@` parse correctly.
3. Parses with `pgxpool.ParseConfig`, applies the four pool-tuning fields, then connects with a
   5-second timeout context.
4. Pings the database. If the ping fails the pool is closed before returning the error.

All errors are wrapped with `errorkit.ERR_DB_POSTGRES_ERROR` and carry the underlying error
in the payload.

### NewPostgresClientFromEnv

```go
func NewPostgresClientFromEnv(cfg Config) (*pgxpool.Pool, error)
```

Reads `os.Getenv(PostgresEnvVar)` (`"POSTGRES_DSN"`) and assigns it to `cfg.DSN` before calling
`NewPostgresClient`. Returns `ERR_SYSTEM_CONFIG_INVALID` if the env var is unset or empty.

---

## DSN Normalization

PostgreSQL passwords often contain `@`, e.g. `postgres://user:pa@ss@host/db`.
`pgxpool.ParseConfig` cannot parse the raw form. `normalizePostgresDSN` percent-encodes the
password (and only the password) so the DSN becomes valid before parsing. The function is
idempotent — passing an already-encoded DSN returns it unchanged.

---

## Stored Procedures

### Interface

```go
type StoredProcedurer[T any] interface {
    QueryRow(ctx context.Context, query string, args ...any) (T, error)
    Query(ctx context.Context, query string, args ...any) ([]T, error)
    QueryRowJSON(ctx context.Context, query string, jsonArg any, args ...any) (T, error)
    QueryJSON(ctx context.Context, query string, jsonArg any, args ...any) ([]T, error)
}
```

`T` may be a struct (mapped via `pgx.RowToStructByNameLax[T]`, lax = extra columns ignored,
missing columns get zero values) or any scalar pgx supports (mapped via `pgx.RowTo[T]`).

The `Pool` field on the concrete `StoredProcedure[T]` is a `Querier`, so `*pgxpool.Pool`,
`pgx.Tx`, or any test fake satisfying `Querier` works equally well.

### Stored Procedure Constructors

| Constructor | Tracing |
|---|---|
| `NewStoredProcedure[T](pool)` | Auto-traced via the global OTel tracer (compatibility) |
| `WrapWithTracing[T](sp)` | Same — explicit wrap |
| `WrapWithInstrumenter[T](sp, instrumenter)` | Wraps with an explicit `*telemetry.Instrumenter`; pass `nil` to disable tracing entirely |

### Method Reference

- **`QueryRow`** — runs `query` with the given positional `args`, returns the single row scanned
  into `T`. Pgx-level errors are wrapped with `ERR_DB_POSTGRES_ERROR`.

- **`Query`** — runs `query` with the given positional `args`, returns all rows scanned into
  `[]T`.

- **`QueryRowJSON`** — JSON-marshals `jsonArg` and appends the resulting string as the **last**
  positional parameter; positional `args` go in `$1…$N`, the JSON value goes in `$N+1`.

- **`QueryJSON`** — same JSON appending rule, but returns multiple rows.

There is no special handling of `[]string` etc. — pass values pgx already understands and they
are forwarded as-is.

---

## Tracing

### Stored-procedure span wrapper

`NewStoredProcedure[T]`, `WrapWithTracing[T]`, and `WrapWithInstrumenter[T]` install a wrapper
that opens one span per call. The span name is derived from the SQL string by extracting the
function name in `SELECT * FROM <fn>(...)`:

```
SELECT * FROM get_item($1)             → span: "postgres.get_item"
SELECT id  FROM insert_items($1::jsonb) → span: "postgres.insert_items"
```

If the regular expression does not match the SQL, the span name falls back to `"postgres.query"`.

Pass `nil` to `WrapWithInstrumenter` to skip the wrapper and use the bare `StoredProcedure[T]`
directly.

### Adding pgx tracing

The package does **not** install `otelpgx` (or any other driver-level tracer) on the pool.
Callers that want pgx-level spans (one per `Query`/`Exec`) configure it themselves:

```go
import (
    "github.com/exaring/otelpgx"
    "github.com/jackc/pgx/v5/pgxpool"
)

cfg, err := pgxpool.ParseConfig(dsn)
if err != nil { /* ... */ }
cfg.ConnConfig.Tracer = otelpgx.NewTracer()

pool, err := pgxpool.NewWithConfig(ctx, cfg)
```

`otelpgx` is intentionally not pulled in by the SDK so consumers without telemetry needs do not
inherit the dependency.

---

## Real-World Usage

```go
// Wire pool + repository
pool, err := database.NewPostgresClient(database.Config{
    DSN:             cfg.Postgres.DSN,
    MaxConnLifetime: cfg.Postgres.MaxConnLifetime,
    MaxConnIdleTime: cfg.Postgres.MaxConnIdleTime,
    MaxConns:        cfg.Postgres.MaxConns,
    MinConns:        cfg.Postgres.MinConns,
})
if err != nil {
    return err
}
defer pool.Close()

sp := database.NewStoredProcedure[entities.Item](pool)
repo := repository.NewItem(sp)
```

```go
// Repository methods
type ItemRepo struct {
    sp database.StoredProcedurer[entities.Item]
}

func (r *ItemRepo) Get(ctx context.Context, id int64) (*entities.Item, error) {
    item, err := r.sp.QueryRow(ctx, "SELECT * FROM get_item($1)", id)
    if err != nil {
        return nil, err
    }
    return &item, nil
}

func (r *ItemRepo) ListByStatus(ctx context.Context, status string) ([]entities.Item, error) {
    return r.sp.Query(ctx, "SELECT * FROM list_items_by_status($1)", status)
}

func (r *ItemRepo) InsertBatch(ctx context.Context, items []entities.Item) ([]entities.Item, error) {
    return r.sp.QueryJSON(ctx, "SELECT * FROM insert_items($1::jsonb)", items)
}
```

---

## Lifecycle

`*pgxpool.Pool` is safe for concurrent use. Create one pool per process, share it across
goroutines, and call `pool.Close()` exactly once during shutdown to drain idle connections.

`StoredProcedure[T]` and the tracing wrappers are stateless after construction — share them
freely across goroutines.

---

## Dependencies

| Package | Role |
|---|---|
| `github.com/jackc/pgx/v5` | PostgreSQL driver and row scanning utilities |
| `github.com/jackc/pgx/v5/pgxpool` | Connection pool |
| `github.com/trypanic/go-sdk/errorkit` | Structured error wrapping |
| `github.com/trypanic/go-sdk/telemetry` | Optional tracing wrapper for stored-procedure calls |

The package has **no dependency on `otelpgx`** or any other pgx tracer. Callers that want
driver-level tracing wire it up themselves.
