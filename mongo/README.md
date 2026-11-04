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
  - [Cursor](#cursor)
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

    "github.com/trypanic/go-sdk/mongo" // package identifier: mongodb
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

Each option is resolved in this order:

1. the `Config` field, when it is non-zero;
2. otherwise the option in the URI (`connectTimeoutMS`,
   `serverSelectionTimeoutMS`, `maxPoolSize`, `minPoolSize`, `maxIdleTimeMS`,
   `retryWrites`);
3. otherwise the SDK default:

```
ConnectTimeout         → 10s
ServerSelectionTimeout → 5s
MaxPoolSize            → 100
MinPoolSize            → 10, clamped to the effective MaxPoolSize
MaxConnIdleTime        → 30s
RetryWrites            → true
```

A zero `Config` field means "not set", so a `MinPoolSize` of 0 (the driver
default: no idle connections kept) is expressed in the URI with
`minPoolSize=0`. The default `MinPoolSize` never exceeds the effective
`MaxPoolSize`, so `MaxPoolSize: 5` starts with a pool of at most 5
connections. An explicit `MinPoolSize` larger than `MaxPoolSize` is a
configuration error (`ERR_SYSTEM_CONFIG_INVALID`).

The `URI` is also normalized: leading `=`, surrounding quotes, and a missing
`mongodb://` scheme are corrected before being passed to the driver.

### RetryWrites

`RetryWrites` is a pointer so callers can express three states unambiguously:

- `nil` — use `retryWrites` from the URI, else the SDK default (`true`).
- pointer to `true` — explicit opt-in (same effect as the default).
- pointer to `false` — explicitly disable driver retry-writes.

The previous `bool` field made `false` ambiguous between "I want it off" and
"I never set it", because the SDK default overrode `false` with `true`.

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
| `WrapWithInstrumenter(client, instrumenter)` | Wrap an existing `ClientPort` (from `NewWithoutTracing`; `New` already wraps) |

Startup behavior is identical across constructors:

1. Options are resolved: `Config` field, then URI, then SDK default (see [Defaults](#defaults)).
2. `mongo.Connect` builds the driver client (no I/O yet).
3. The primary is pinged with a timeout of `max(5s, ServerSelectionTimeout)` (the
   effective value from `Config` or the URI); an earlier deadline on `ctx` still applies.
   On failure the driver client is disconnected (bounded to 5 s, even if `ctx` is already
   cancelled) before the error is returned.
4. Invalid URIs or options return `ERR_SYSTEM_CONFIG_INVALID`; a failed ping returns
   `ERR_DB_MONGO_UNAVAILABLE`.

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
`GetModifiedCount`, `GetDeletedCount`, `GetInsertedIDs`). `BulkWriteResult`
exposes `GetInsertedCount`, `GetMatchedCount`, `GetModifiedCount`,
`GetDeletedCount`, `GetUpsertedCount` and `GetUpsertedIDs`. `SingleResult.Decode`
decodes the document returned by `FindOne`.

### Cursor

```go
type Cursor interface {
    Next(ctx) bool
    Decode(v any) error
    Err() error
    All(ctx, results any) error
    Close(ctx) error
}
```

`All` buffers every remaining document. For large results, stream them:

```go
cursor, err := client.Collection("products").Find(ctx, bson.D{})
if err != nil {
    return err
}
defer cursor.Close(ctx)

for cursor.Next(ctx) {
    var p Product
    if err := cursor.Decode(&p); err != nil {
        return err // ERR_DB_MONGO_DECODE_FAILED
    }
    process(p)
}
return cursor.Err() // nil, or the iteration error classified by WrapOperationError
```

Fakes and other implementations of `Cursor` and `BulkWriteResult` must
implement every method listed above.

---

## Tracing

`New` and `WrapWithTracing` install a wrapper that opens one span per
collection operation, named `mongo.<collection>.<operation>` (e.g.
`mongo.products.insertOne`). The cursor returned by `Find` opens a second span,
`mongo.<collection>.find.all`, around `Cursor.All`, because fetching further
batches and decoding happen after `Find` returns. `ClientPort.Ping` and
`ClientPort.Close` open `mongo.ping` and `mongo.close`.

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

`mongodb` produces `errorkit.AppError` values for adapter-level failures. The
driver error stays wrapped, so `errors.Is`/`errors.As` still reach it.
Invalid URIs or client options are configuration errors; a failed start-up
ping is `ERR_DB_MONGO_UNAVAILABLE`; per-operation errors (including
`Client.Ping` and `Client.Close`) go through `WrapOperationError`:

| Driver condition | errorkit Code | Retriable |
|---|---|---|
| Invalid URI or options in `New*` | `ERR_SYSTEM_CONFIG_INVALID` | no |
| Start-up ping failure | `ERR_DB_MONGO_UNAVAILABLE` | yes |
| `mongo.ErrNoDocuments` | `ERR_DB_MONGO_NOT_FOUND` | no |
| `context.Canceled` | `ERR_DB_MONGO_CANCELED` (HTTP 499) | no |
| `mongo.IsTimeout` (deadline, network timeout, `MaxTimeMSExpired`) | `ERR_DB_MONGO_TIMEOUT` | yes |
| `mongo.IsDuplicateKeyError` | `ERR_VALIDATION_DUPLICATE` | no |
| `mongo.InvalidArgumentError` (e.g. `mongo.ErrNilDocument`) | `ERR_VALIDATION` | no |
| `mongo.ErrClientDisconnected` (client already closed) | `ERR_SYSTEM_UNEXPECTED` | no |
| `mongo.IsNetworkError` | `ERR_DB_MONGO_UNAVAILABLE` | yes |
| Any other operation failure | `ERR_DB_MONGO_ERROR` | yes |
| Decode failure (cursor/SingleResult) | `ERR_DB_MONGO_DECODE_FAILED` | no |

`SingleResult.Decode` reports the error of the `FindOne` operation itself
(including `mongo.ErrNoDocuments`) through `WrapOperationError`, and only a
failure to decode the returned document as `ERR_DB_MONGO_DECODE_FAILED`.
`Cursor.All` does the same: an error while fetching a batch (for example a
failed `getMore`) goes through `WrapOperationError`; a document that does not
decode into the target type is `ERR_DB_MONGO_DECODE_FAILED`.

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
