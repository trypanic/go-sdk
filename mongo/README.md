# `mongodb` — MongoDB Client with Adapter-Based API

Package `mongodb` exposes a small, framework-agnostic interface (`ClientPort`,
`Collection`) over the official MongoDB Go driver. Connection setup applies safe
defaults, performs a fail-fast ping, and normalizes errors to `errorkit`. Tracing
is **opt-in** via a wrapper that emits one span per collection operation; the
package itself imports no telemetry package by default.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
  - [Config](#config)
  - [Defaults](#defaults)
  - [RetryWrites](#retrywrites)
- [Constructors](#constructors)
- [API Reference](#api-reference)
  - [ClientPort](#clientport)
  - [Collection](#collection)
- [Tracing](#tracing)
- [Error Handling](#error-handling)
- [Real-World Usage](#real-world-usage)
- [Lifecycle and Concurrency](#lifecycle-and-concurrency)
- [Dependencies](#dependencies)

---

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `Config` | struct | URI, database, pool, timeouts, `*bool RetryWrites` |
| `ClientPort` | interface | `Collection(name) Collection`, `Ping(ctx)`, `Close(ctx)` |
| `Collection` | interface | Adapter over `*mongo.Collection` returning SDK result interfaces |
| `New` | func | Builds a `ClientPort` with auto-tracing via the global tracer |
| `NewWithoutTracing` | func | Same as `New` but no tracing wrapper |
| `NewWithInstrumenter` | func | Builds with an explicit `*telemetry.Instrumenter` (`nil` = no tracing) |
| `WrapWithInstrumenter` | func | Wraps an existing `ClientPort` with an explicit instrumenter |
| `WrapOperationError` | func | Adapter-level error normalization to `errorkit` |
| `InsertOneResult`, `InsertManyResult`, `Cursor`, `SingleResult`, `BulkWriteResult`, `UpdateResult`, `DeleteResult` | interfaces | Driver-agnostic result types |

---

## Quick Start

```go
import (
    "context"
    "log"

    "github.com/trypanic/go-sdk/mongodb"
)

client, err := mongodb.New(context.Background(), mongodb.Config{
    URI:      "mongodb://root:secret@localhost:27017",
    Database: "my_app",
})
if err != nil {
    log.Fatal(err)
}
defer client.Close(context.Background())

type Product struct {
    Name  string `bson:"name"`
    Price int    `bson:"price"`
}

result, err := client.Collection("products").
    InsertOne(context.Background(), Product{Name: "Widget", Price: 99})
if err != nil {
    log.Fatal(err)
}
log.Printf("inserted: %v", result.GetInsertedID())
```

---

## Configuration

### Config

```go
type Config struct {
    URI                    string
    Database               string
    ConnectTimeout         time.Duration
    ServerSelectionTimeout time.Duration
    MaxPoolSize            uint64
    MinPoolSize            uint64
    MaxConnIdleTime        time.Duration
    RetryWrites            *bool
}
```

### Defaults

`New` calls `applyDefaults` which fills zero values:

```
ConnectTimeout         → 10s
ServerSelectionTimeout → 5s
MaxPoolSize            → 100
MinPoolSize            → 10
MaxConnIdleTime        → 30s
RetryWrites            → true (only when nil)
```

The `URI` is also normalized: leading `=`, surrounding quotes, and a missing
`mongodb://` scheme are corrected before being passed to the driver.

### RetryWrites

`RetryWrites` is a pointer so callers can express three states unambiguously:

- `nil` — use the SDK default (`true`).
- pointer to `true` — explicit opt-in (same effect as the default).
- pointer to `false` — explicitly disable driver retry-writes.

The previous `bool` field made `false` ambiguous between "I want it off" and
"I never set it" because `applyDefaults` overrode `false` with `true`.

```go
disable := false
client, err := mongodb.New(ctx, mongodb.Config{
    URI:         os.Getenv("MONGO_URI"),
    Database:    "my_app",
    RetryWrites: &disable, // explicitly off
})
```

The package itself reads no environment variables — callers wire env loading.

---

## Constructors

| Constructor | Tracing |
|---|---|
| `New(ctx, cfg)` | Auto-traced via the global OTel tracer (compatibility) |
| `NewWithoutTracing(ctx, cfg)` | No tracing wrapper |
| `NewWithInstrumenter(ctx, cfg, instrumenter)` | Explicit instrumenter; pass `nil` to disable |
| `WrapWithInstrumenter(client, instrumenter)` | Wrap an existing `ClientPort` |

Startup behavior is identical across constructors:

1. `applyDefaults` fills zero-valued fields.
2. `mongo.Connect` builds the driver client (no I/O yet).
3. A 5-second context pings the primary. On failure the driver client is disconnected
   before the error is returned.
4. Errors are wrapped with `ERR_DB_MONGO_UNAVAILABLE`.

---

## API Reference

### ClientPort

```go
type ClientPort interface {
    Collection(name string) Collection
    Ping(ctx context.Context) error
    Close(ctx context.Context) error
}
```

`Collection` returns a thin adapter over `*mongo.Collection` that maps driver
errors to `errorkit` codes (see [Error Handling](#error-handling)) and returns
SDK result interfaces.

### Collection

```go
type Collection interface {
    InsertOne(ctx, doc, opts...) (InsertOneResult, error)
    InsertMany(ctx, docs, opts...) (InsertManyResult, error)
    Find(ctx, filter, opts...) (Cursor, error)
    FindOne(ctx, filter, opts...) SingleResult
    BulkWrite(ctx, models, opts...) (BulkWriteResult, error)
    ReplaceOne(ctx, filter, replacement, opts...) (UpdateResult, error)
    CountDocuments(ctx, filter, opts...) (int64, error)
    UpdateOne(ctx, filter, update, opts...) (UpdateResult, error)
    DeleteOne(ctx, filter, opts...) (DeleteResult, error)
}
```

Result interfaces (`InsertOneResult`, `UpdateResult`, etc.) expose only the
fields callers typically need (`GetInsertedID`, `GetMatchedCount`,
`GetModifiedCount`, `GetDeletedCount`, `GetInsertedIDs`). The full
`*mongo.SingleResult` is returned via `SingleResult.Decode`.

---

## Tracing

`New` and `WrapWithTracing` install a wrapper that opens one span per
collection operation, named `mongo.<collection>.<operation>` (e.g.
`mongo.products.insertOne`).

The wrapper holds an explicit `*telemetry.Instrumenter`, so consumers can:

- Use the global tracer (the default `New` path).
- Pass `WrapWithInstrumenter(plain, telemetry.NewInstrumenter(...))` to use an
  explicit `TracerProvider`.
- Pass `nil` to skip the wrapper entirely.

Driver-level command monitoring (e.g. `otelmongo`) is **not** installed by this
package. SDK consumers that want one span per MongoDB command can attach
`otelmongo.NewMonitor()` themselves before calling the constructor by passing a
custom `*options.ClientOptions` — at present this requires direct driver use,
which is acceptable since the wrapper-level span already covers the SDK API.

---

## Error Handling

`mongodb` produces `errorkit.AppError` values for adapter-level failures.
Connection and ping errors use `ERR_DB_MONGO_UNAVAILABLE`; per-operation errors
go through `WrapOperationError` which classifies common driver errors:

| Driver condition | errorkit Code |
|---|---|
| Connect / ping failure | `ERR_DB_MONGO_UNAVAILABLE` |
| Generic operation failure | `ERR_DB_MONGO_ERROR` |
| `mongo.ErrNoDocuments` | `ERR_DB_MONGO_NOT_FOUND` |
| Decode failure (cursor/SingleResult) | `ERR_DB_MONGO_DECODE_FAILED` |

Repositories should log/return the wrapped error directly — re-wrapping is not
needed.

---

## Real-World Usage

```go
type ProductRepo struct {
    client mongodb.ClientPort
}

func NewProductRepo(c mongodb.ClientPort) *ProductRepo {
    return &ProductRepo{client: c}
}

func (r *ProductRepo) Save(ctx context.Context, p Product) (any, error) {
    res, err := r.client.Collection("products").InsertOne(ctx, p)
    if err != nil {
        return nil, err
    }
    return res.GetInsertedID(), nil
}

func (r *ProductRepo) Count(ctx context.Context, filter any) (int64, error) {
    return r.client.Collection("products").CountDocuments(ctx, filter)
}
```

Tests can substitute a fake `ClientPort` directly — no driver dependency
needed because the interface is not bound to `*mongo.Database`.

---

## Lifecycle and Concurrency

- `ClientPort` is safe for concurrent use after `New` returns.
- Create one instance per process. The driver manages pool locking internally.
- Call `Close(ctx)` exactly once during shutdown. The context controls how long
  to wait for in-flight operations.

---

## Dependencies

| Package | Role |
|---|---|
| `go.mongodb.org/mongo-driver/v2/mongo` | Official MongoDB Go driver |
| `go.mongodb.org/mongo-driver/v2/mongo/options` | Driver option builders |
| `go.mongodb.org/mongo-driver/v2/mongo/readpref` | Read-preference constants for the startup ping |
| `github.com/trypanic/go-sdk/errorkit` | Structured error wrapping |
| `github.com/trypanic/go-sdk/telemetry` | Optional tracing wrapper for collection operations |

The package has **no `otelmongo` import**. Tracing is wrapper-level and opt-in.
