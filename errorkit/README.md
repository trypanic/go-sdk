# errorkit

A structured, type-safe error handling library for Go. Every error carries a unique ID, stack trace, metadata, HTTP status code, and retryability flag — all automatically captured at creation time.

## Table of Contents

- [errorkit](#errorkit)
  - [Table of Contents](#table-of-contents)
  - [Installation](#installation)
  - [Quick Start](#quick-start)
  - [Core Concepts](#core-concepts)
    - [AppError](#apperror)
    - [ErrorCode](#errorcode)
    - [Metadata](#metadata)
  - [Creating Errors](#creating-errors)
    - [Basic](#basic)
    - [With Options](#with-options)
    - [Wrapping Standard Errors](#wrapping-standard-errors)
  - [Functional Options](#functional-options)
  - [Formatting \& Output](#formatting--output)
    - [Example JSON output](#example-json-output)
    - [Color control](#color-control)
  - [Registry API](#registry-api)
  - [Configuration](#configuration)
    - [Stack depth](#stack-depth)
    - [Custom ID generator](#custom-id-generator)
    - [ULID utilities](#ulid-utilities)
  - [Registering New Error Codes](#registering-new-error-codes)
    - [Step 1 — Declare the constant in `codes.go`](#step-1--declare-the-constant-in-codesgo)
    - [Step 2 — Register metadata in `registry.go`](#step-2--register-metadata-in-registrygo)
    - [Runtime registration (without modifying source files)](#runtime-registration-without-modifying-source-files)
  - [Adding New Groups and Types](#adding-new-groups-and-types)
    - [New ErrorGroup](#new-errorgroup)
    - [New ErrorType](#new-errortype)
    - [Choosing `Type` and `Retriable`](#choosing-type-and-retriable)
  - [Built-in Error Codes Reference](#built-in-error-codes-reference)

---

## Installation

```bash
go get github.com/your-org/errorkit
```

---

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/your-org/errorkit"
)

func main() {
    // Create a structured error
    err := errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
        With(
            errorkit.WithReason("field 'email' is required"),
            errorkit.WithPayload(map[string]string{"field": "email"}),
        )

    // Use as a standard error
    fmt.Println(err) // [ERR_VALIDATION_MISSING_FIELD] field 'email' is required

    // Pretty-print for debugging (auto-detects TTY colors)
    fmt.Println(err.Pretty())

    // JSON output for HTTP responses / structured logging
    fmt.Println(err.PrettyJSON())
}
```

---

## Core Concepts

### AppError

`AppError` is the central struct. It implements the standard `error` interface and is automatically populated on creation.

```go
type AppError struct {
    ErrCode   ErrorCode      // e.g. "ERR_VALIDATION_MISSING_FIELD"
    Reason    string         // human-readable reason (optional)
    Payload   any            // arbitrary context data (optional)
    Wrapped   error          // wrapped underlying error (optional)
    Metadata  Metadata       // classification, HTTP status, retryability
    Trace     []TraceContext // automatic stack trace
    ID        string         // unique ULID per error instance
    TraceID   string         // distributed tracing ID (optional)
    Timestamp string         // ISO 8601 UTC timestamp
}
```

### ErrorCode

`ErrorCode` is a typed string that identifies the error. Constants are defined in `codes.go` following the naming convention:

```
ERR_<GROUP>_<SPECIFIC>
```

```go
type ErrorCode string

const ERR_VALIDATION_MISSING_FIELD ErrorCode = "ERR_VALIDATION_MISSING_FIELD"
```

### Metadata

`Metadata` describes the error's classification and behavior. It is loaded automatically from the registry when you call `NewError`.

```go
type Metadata struct {
    Code        ErrorCode  // same as AppError.ErrCode
    Type        ErrorType  // "internal" or "external"
    Group       ErrorGroup // e.g. "auth", "database", "cache"
    Category    string     // sub-group label (e.g. "validation")
    Description string     // short human-readable description
    HTTPStatus  int        // suggested HTTP response status
    Retriable   bool       // whether the caller should retry
}
```

---

## Creating Errors

### Basic

```go
err := errorkit.NewError(errorkit.ERR_AUTH_UNAUTHENTICATED)
```

This automatically:
- Looks up metadata from the registry
- Captures the current stack trace
- Generates a unique ULID as the error ID
- Records an ISO 8601 timestamp

### With Options

```go
err := errorkit.NewError(errorkit.ERR_DB_MONGO_TIMEOUT).
    With(
        errorkit.WithReason("timed out after 5s querying users collection"),
        errorkit.WithPayload(map[string]any{
            "collection": "users",
            "query":      bson.M{"status": "active"},
        }),
        errorkit.WithWrapped(originalMongoErr),
        errorkit.WithTraceID(ctx.Value("trace_id").(string)),
    )
```

### Wrapping Standard Errors

```go
_, dbErr := collection.FindOne(ctx, filter)
if dbErr != nil {
    return errorkit.NewError(errorkit.ERR_DB_MONGO_NOT_FOUND).
        With(errorkit.WithWrapped(dbErr))
}
```

The wrapped error is accessible via `errors.Unwrap(err)` and is included in JSON output.

---

## Functional Options

| Option                        | Description                                       |
| ----------------------------- | ------------------------------------------------- |
| `WithReason(format, args...)` | Printf-style human-readable reason string         |
| `WithPayload(any)`            | Attach arbitrary structured data to the error     |
| `WithWrapped(error)`          | Wrap an existing error (preserves original stack) |
| `WithTraceID(string)`         | Set a distributed tracing ID                      |

All options are applied via `.With(...)` and return the same `*AppError` for chaining.

---

## Formatting & Output

None of the formatting methods print to the console — they all return strings.

```go
// Auto-detects TTY: colored if terminal, plain otherwise
output := err.Pretty()

// Always plain text, no ANSI codes
output := errorkit.NewStackFormatter().FormatPlain(err)

// Always colored ANSI output
output := errorkit.NewStackFormatter().FormatColored(err)

// Structured JSON — ideal for HTTP responses and log aggregation
output := err.PrettyJSON()

// Write directly to any io.Writer
err.WriteTo(os.Stderr)
err.WriteTo(logFile)
```

### Example JSON output

```json
{
  "code": "ERR_AUTH_TOKEN_EXPIRED",
  "reason": "JWT expired at 2024-01-15T10:00:00Z",
  "metadata": {
    "code": "ERR_AUTH_TOKEN_EXPIRED",
    "type": "internal",
    "group": "auth",
    "category": "auth",
    "description": "Authentication token expired",
    "http_status": 401,
    "retriable": false
  },
  "stack_trace": [
    {
      "file": "/app/service/auth.go",
      "line": 42,
      "package": "auth",
      "function": "ValidateToken"
    }
  ],
  "id": "01HV8F3KJ2XQDT8JQMZR4Y5WN"
}
```

### Color control

Global compatibility helpers:

```go
errorkit.DisableColors() // disable ANSI globally
errorkit.EnableColors()  // re-enable ANSI globally
```

SDK-safe formatter config:

```go
formatter := errorkit.NewStackFormatterWithConfig(errorkit.FormatterConfig{
    ColorsEnabled: false,
})

output := formatter.Format(err)
```

---

## Registry API

The package exposes global registry helpers. They are safe for concurrent use, but they mutate process-global state.

```go
// Look up metadata for a code
meta, ok := errorkit.GetMetadata(errorkit.ERR_CACHE_TIMEOUT)

// Register one or more codes at runtime (validates + detects conflicts)
if err := errorkit.RegisterMany(meta1, meta2); err != nil {
    // ErrInvalidMetadata or ErrMetadataConflict
}

// Same, but panic on failure (intended for package init())
errorkit.MustRegister(meta1, meta2)

// Replace metadata for an existing code (testing / advanced overrides)
errorkit.OverrideMetadata(errorkit.ERR_INTERNAL, func(m *errorkit.Metadata) {
    m.Description = "Custom internal error description"
})

// List all registered codes
codes := errorkit.GetAllCodes()

// Filter by group
dbCodes := errorkit.GetCodesByGroup(errorkit.GroupDatabase)

// Filter by type
externalCodes := errorkit.GetCodesByType(errorkit.ErrorTypeExternal)
```

`RegisterMany` and `MustRegister` validate required fields and reject duplicate codes whose metadata differs from what is already registered. Identical duplicate metadata is a no-op.

SDK consumers that need isolation should use an explicit registry:

```go
registry := errorkit.NewDefaultRegistry()
if err := registry.RegisterMany(errorkit.Metadata{
    Code:        "ERR_SDK_CUSTOM",
    Type:        errorkit.ErrorTypeInternal,
    Group:       errorkit.GroupUnknown,
    Category:    "custom",
    Description: "SDK-local custom error",
    HTTPStatus:  400,
    Retriable:   false,
}); err != nil {
    panic(err)
}

err := registry.NewError("ERR_SDK_CUSTOM")
```

`NewRegistry(...)` creates an empty isolated registry seeded only with the metadata you pass. `NewDefaultRegistry()` creates an isolated copy of the built-in registry. `Registry.Merge(other)` copies metadata from another registry with the same conflict semantics.

---

## Configuration

### Stack depth

Global compatibility helpers:

```go
// At startup (default is 32, max is 100)
errorkit.SetMaxStackDepth(16)

// Via environment variable
// ERROR_KIT_MAX_STACK=16 ./your-binary

// Or call InitFromEnv() to load from environment
errorkit.InitFromEnv()
```

SDK-safe factory config:

```go
factory := errorkit.NewFactory(errorkit.Config{
    Registry:      errorkit.NewDefaultRegistry(),
    MaxStackDepth: 16,
})

err := factory.NewError(errorkit.ERR_INTERNAL)
```

The factory uses its own registry and stack-depth setting without mutating global `ErrorRegistry`, `MaxStackDepth`, or color settings.

### Custom ID generator

```go
// Replace ULID with UUID v4
errorkit.SetIDGenerator(func() string {
    return uuid.New().String()
})

// Reset to default ULID generator
errorkit.SetIDGenerator(nil)
```

### ULID utilities

```go
// Parse the timestamp embedded in a ULID
t := errorkit.ParseULID("01HV8F3KJ2XQDT8JQMZR4Y5WN")

// Validate a ULID string
ok := errorkit.ValidateULID(someID)

// Range queries — get min/max ULID for a timestamp
min := errorkit.MinULID(startTime)
max := errorkit.MaxULID(endTime)
```

---

## Registering New Error Codes

Adding new error codes is a two-step process: declare the constant in `codes.go` and register its metadata in `registry.go`.

### Step 1 — Declare the constant in `codes.go`

Follow the naming convention `ERR_<GROUP>_<SPECIFIC>`. Add your code to an existing section or create a new one.

```go
// codes.go

// ==============================
// Payment Errors
// ==============================

const (
    // ERR_PAYMENT_DECLINED indicates the payment was declined by the processor
    // Context: Insufficient funds, card expired, fraud flag, CVV mismatch
    ERR_PAYMENT_DECLINED ErrorCode = "ERR_PAYMENT_DECLINED"

    // ERR_PAYMENT_PROVIDER_UNAVAILABLE indicates the payment gateway is down
    // Context: Stripe/PayPal outage, gateway timeout, network partition
    ERR_PAYMENT_PROVIDER_UNAVAILABLE ErrorCode = "ERR_PAYMENT_PROVIDER_UNAVAILABLE"

    // ERR_PAYMENT_INVALID_AMOUNT indicates the amount is invalid
    // Context: Negative amount, zero amount, exceeds maximum allowed
    ERR_PAYMENT_INVALID_AMOUNT ErrorCode = "ERR_PAYMENT_INVALID_AMOUNT"
)
```

### Step 2 — Register metadata in `registry.go`

Add entries to the `ErrorRegistry` map inside the `var` block. Match the group to an existing `ErrorGroup` constant (or define a new one — see next section).

```go
// registry.go — inside ErrorRegistry = map[ErrorCode]Metadata{ ... }

// ==============================
// Payment Errors
// ==============================
ERR_PAYMENT_DECLINED: {
    Code:        ERR_PAYMENT_DECLINED,
    Type:        ErrorTypeExternal,   // fault lies with the payment provider
    Group:       GroupPayment,        // new group — see below
    Category:    "payment",
    Description: "Payment was declined by the processor",
    HTTPStatus:  402,
    Retriable:   false,
},
ERR_PAYMENT_PROVIDER_UNAVAILABLE: {
    Code:        ERR_PAYMENT_PROVIDER_UNAVAILABLE,
    Type:        ErrorTypeExternal,
    Group:       GroupPayment,
    Category:    "payment",
    Description: "Payment provider is unavailable",
    HTTPStatus:  503,
    Retriable:   true,
},
ERR_PAYMENT_INVALID_AMOUNT: {
    Code:        ERR_PAYMENT_INVALID_AMOUNT,
    Type:        ErrorTypeInternal,   // our code sent a bad amount
    Group:       GroupPayment,
    Category:    "payment",
    Description: "Invalid payment amount",
    HTTPStatus:  400,
    Retriable:   false,
},
```

### Runtime registration (without modifying source files)

You can also register codes at runtime from your application, e.g. in an `init()` function:

```go
func init() {
    errorkit.MustRegister(errorkit.Metadata{
        Code:        "ERR_SUBSCRIPTION_EXPIRED",
        Type:        errorkit.ErrorTypeInternal,
        Group:       errorkit.GroupData,
        Category:    "subscription",
        Description: "User subscription has expired",
        HTTPStatus:  402,
        Retriable:   false,
    })
}
```

Then use it like any other code:

```go
err := errorkit.NewError("ERR_SUBSCRIPTION_EXPIRED").
    With(errorkit.WithReason("subscription ended on 2024-01-01"))
```

---

## Adding New Groups and Types

### New ErrorGroup

Groups are declared in `types.go`. Add a new constant to the existing `const` block:

```go
// types.go

const (
    GroupUnknown      ErrorGroup = "unknown"
    GroupData         ErrorGroup = "data"
    // ... existing groups ...
    GroupPayment      ErrorGroup = "payment"   // ← add here
    GroupNotification ErrorGroup = "notification"
)
```

### New ErrorType

`ErrorType` distinguishes whether the fault is internal (our application) or external (a third-party service). The two built-in types cover most cases:

```go
const (
    ErrorTypeInternal ErrorType = "internal"
    ErrorTypeExternal ErrorType = "external"
)
```

If you need additional types (e.g. for auditing or routing), extend the block:

```go
const (
    ErrorTypeInternal ErrorType = "internal"
    ErrorTypeExternal ErrorType = "external"
    ErrorTypeUser     ErrorType = "user"     // ← fault lies with the end-user
)
```

### Choosing `Type` and `Retriable`

| Scenario                     | Type                | Retriable |
| ---------------------------- | ------------------- | --------- |
| Bug in our code              | `ErrorTypeInternal` | `false`   |
| Bad input from client        | `ErrorTypeInternal` | `false`   |
| Third-party API returned 5xx | `ErrorTypeExternal` | `true`    |
| Third-party API returned 4xx | `ErrorTypeExternal` | `false`   |
| Transient timeout            | either              | `true`    |
| Auth/credential failure      | either              | `false`   |

---

## Built-in Error Codes Reference

| Code                                | HTTP | Retriable | Description                            |
| ----------------------------------- | ---- | --------- | -------------------------------------- |
| `ERR_UNKNOWN`                       | 500  | ✗         | Uncategorized error                    |
| `ERR_INTERNAL`                      | 500  | ✗         | Generic internal server error          |
| `ERR_VALIDATION`                    | 400  | ✗         | Generic validation failure             |
| `ERR_VALIDATION_INVALID_FORMAT`     | 400  | ✗         | Field has invalid format               |
| `ERR_VALIDATION_MISSING_FIELD`      | 400  | ✗         | Required field is missing              |
| `ERR_VALIDATION_INCONSISTENT`       | 400  | ✗         | Fields have conflicting values         |
| `ERR_VALIDATION_BUSINESS_RULE`      | 422  | ✗         | Business rule violation                |
| `ERR_VALIDATION_DUPLICATE`          | 409  | ✗         | Duplicate resource                     |
| `ERR_CLIENT_BAD_REQUEST`            | 400  | ✗         | Malformed client request               |
| `ERR_CLIENT_NOT_FOUND`              | 404  | ✗         | Resource not found                     |
| `ERR_CLIENT_RATE_LIMIT`             | 429  | ✓         | Rate limit exceeded                    |
| `ERR_AUTH_UNAUTHENTICATED`          | 401  | ✗         | Authentication required                |
| `ERR_AUTH_UNAUTHORIZED`             | 403  | ✗         | Insufficient permissions               |
| `ERR_AUTH_INVALID_TOKEN`            | 401  | ✗         | Invalid authentication token           |
| `ERR_AUTH_TOKEN_EXPIRED`            | 401  | ✗         | Authentication token expired           |
| `ERR_AUTH_INVALID_CREDENTIALS`      | 401  | ✗         | Wrong credentials                      |
| `ERR_SYSTEM_UNEXPECTED`             | 500  | ✗         | Panic or impossible state              |
| `ERR_SYSTEM_CONFIG_INVALID`         | 500  | ✗         | Invalid system configuration           |
| `ERR_SYSTEM_TIMEOUT_INTERNAL`       | 500  | ✓         | Internal operation timeout             |
| `ERR_SYSTEM_CONCURRENCY`            | 409  | ✓         | Concurrency / optimistic lock conflict |
| `ERR_OAUTH_CONFIG_INVALID`          | 500  | ✗         | Invalid OAuth configuration            |
| `ERR_OAUTH_URL_MALFORMED`           | 500  | ✗         | Failed to build OAuth URL              |
| `ERR_OAUTH_PARSE_FAILED`            | 500  | ✗         | Failed to parse OAuth response         |
| `ERR_OAUTH_STATE_INVALID`           | 400  | ✗         | OAuth state mismatch (CSRF)            |
| `ERR_OAUTH_PROVIDER_ERROR`          | 503  | ✓         | OAuth provider returned error          |
| `ERR_OAUTH_ACCESS_DENIED`           | 403  | ✗         | OAuth access denied                    |
| `ERR_OAUTH_INVALID_GRANT`           | 400  | ✗         | OAuth grant invalid/expired            |
| `ERR_OAUTH_INVALID_REQUEST`         | 400  | ✗         | Malformed OAuth request                |
| `ERR_DB_QUERY_INVALID`              | 500  | ✗         | Invalid database query                 |
| `ERR_DB_SCHEMA_MISMATCH`            | 500  | ✗         | Data doesn't match schema              |
| `ERR_DB_CONSTRAINT_VIOLATION`       | 409  | ✗         | DB constraint violated                 |
| `ERR_DB_MONGO_UNAVAILABLE`          | 503  | ✓         | MongoDB unavailable                    |
| `ERR_DB_MONGO_TIMEOUT`              | 504  | ✓         | MongoDB operation timeout              |
| `ERR_DB_MONGO_AUTH_FAILED`          | 502  | ✗         | MongoDB auth failed                    |
| `ERR_DB_MONGO_ERROR`                | 503  | ✓         | General MongoDB error                  |
| `ERR_DB_MONGO_NOT_FOUND`            | 404  | ✗         | MongoDB document not found             |
| `ERR_DB_MONGO_DECODE_FAILED`        | 500  | ✗         | MongoDB document decode failed         |
| `ERR_DB_POSTGRES_UNAVAILABLE`       | 503  | ✓         | PostgreSQL unavailable                 |
| `ERR_DB_POSTGRES_TIMEOUT`           | 504  | ✓         | PostgreSQL operation timeout           |
| `ERR_DB_POSTGRES_AUTH_FAILED`       | 502  | ✗         | PostgreSQL auth failed                 |
| `ERR_DB_POSTGRES_CONNECTION_FAILED` | 503  | ✓         | PostgreSQL connection failed           |
| `ERR_DB_POSTGRES_DEADLOCK`          | 409  | ✓         | PostgreSQL deadlock detected           |
| `ERR_DB_POSTGRES_ERROR`             | 503  | ✓         | General PostgreSQL error               |
| `ERR_CACHE_KEY_INVALID`             | 500  | ✗         | Invalid cache key                      |
| `ERR_CACHE_SERIALIZATION_FAILED`    | 500  | ✗         | Cache serialization failed             |
| `ERR_CACHE_UNAVAILABLE`             | 503  | ✓         | Cache (Redis) unavailable              |
| `ERR_CACHE_TIMEOUT`                 | 504  | ✓         | Cache operation timeout                |
| `ERR_CACHE_ERROR`                   | 503  | ✓         | General cache error                    |
| `ERR_STORAGE_UNAVAILABLE`           | 503  | ✓         | Storage (S3) unavailable               |
| `ERR_STORAGE_TIMEOUT`               | 504  | ✓         | Storage operation timeout              |
| `ERR_STORAGE_ACCESS_DENIED`         | 502  | ✗         | Storage access denied                  |
| `ERR_STORAGE_INVALID_CREDENTIALS`   | 502  | ✗         | Invalid storage credentials            |
| `ERR_STORAGE_ERROR`                 | 503  | ✓         | General storage error                  |
| `ERR_NETWORK_ERROR`                 | 503  | ✓         | Network communication error            |
| `ERR_NETWORK_TIMEOUT`               | 504  | ✓         | Network operation timeout              |
| `ERR_EXTERNAL_SERVICE_UNAVAILABLE`  | 503  | ✓         | External service down                  |
| `ERR_EXTERNAL_SERVICE_TIMEOUT`      | 504  | ✓         | External service timeout               |
| `ERR_EXTERNAL_SERVICE_ERROR`        | 503  | ✓         | External service 5xx error             |
| `ERR_EXTERNAL_INVALID_RESPONSE`     | 502  | ✗         | Invalid response from external service |
| `ERR_MQ_UNAVAILABLE`                | 503  | ✓         | RabbitMQ unavailable                   |
| `ERR_MQ_TIMEOUT`                    | 504  | ✓         | Message queue timeout                  |
| `ERR_MQ_AUTH_FAILED`                | 502  | ✗         | Message queue auth failed              |
| `ERR_MQ_CONNECTION_FAILED`          | 503  | ✓         | Message queue connection failed        |
| `ERR_MQ_CHANNEL_ERROR`              | 503  | ✓         | Message queue channel error            |
| `ERR_MQ_PUBLISH_FAILED`             | 503  | ✓         | Message publish failed                 |
| `ERR_MQ_CONSUME_FAILED`             | 503  | ✓         | Message consume failed                 |
| `ERR_MQ_QUEUE_ERROR`                | 503  | ✓         | Queue operation error                  |
| `ERR_MQ_EXCHANGE_ERROR`             | 503  | ✓         | Exchange operation error               |
| `ERR_MQ_ERROR`                      | 503  | ✓         | General message queue error            |
