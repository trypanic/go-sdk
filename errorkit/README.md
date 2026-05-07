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
  - [Factory API](#factory-api)
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
  - [Built-in Error Groups](#built-in-error-groups)
  - [Built-in Error Codes Reference](#built-in-error-codes-reference)

---

## Installation

```bash
go get github.com/trypanic/go-sdk/errorkit
```

---

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/trypanic/go-sdk/errorkit"
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
    Group       ErrorGroup // e.g. "data", "system", "client"
    Category    string     // sub-group label (e.g. "validation")
    Description string     // short human-readable description
    HTTPStatus  int        // suggested HTTP response status
    Retriable   bool       // whether the caller should retry
}
```

When `NewError` is called with a code that has no registered metadata, an `unknownMetadata` placeholder is used (Group=`unknown`, Type=`internal`, HTTP=500, Retriable=false).

---

## Creating Errors

### Basic

```go
err := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
```

This automatically:
- Looks up metadata from the registry
- Captures the current stack trace
- Generates a unique ULID as the error ID
- Records an ISO 8601 timestamp

### With Options

```go
err := errorkit.NewError(errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL).
    With(
        errorkit.WithReason("timed out after 5s querying users"),
        errorkit.WithPayload(map[string]any{
            "collection": "users",
            "filter":     map[string]string{"status": "active"},
        }),
        errorkit.WithWrapped(originalErr),
        errorkit.WithTraceID(traceID),
    )
```

### Wrapping Standard Errors

```go
result, dbErr := repo.FindOne(ctx, filter)
if dbErr != nil {
    return errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND).
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
  "code": "ERR_VALIDATION_MISSING_FIELD",
  "reason": "field 'email' is required",
  "payload": {"field": "email"},
  "metadata": {
    "code": "ERR_VALIDATION_MISSING_FIELD",
    "type": "internal",
    "group": "data",
    "category": "validation",
    "description": "Required field is missing",
    "http_status": 400,
    "retriable": false
  },
  "stack_trace": [
    {
      "file": "/app/handler/users.go",
      "line": 42,
      "package": "handler",
      "function": "CreateUser"
    }
  ],
  "id": "01HV8F3KJ2XQDT8JQMZR4Y5WN"
}
```

`reason`, `payload`, `trace_id`, and `wrapped` are omitted when empty. `Timestamp` lives on the `AppError` struct but is not serialized by `PrettyJSON`.

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
meta, ok := errorkit.GetMetadata(errorkit.ERR_CLIENT_RATE_LIMIT)

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
dataCodes := errorkit.GetCodesByGroup(errorkit.GroupData)

// Filter by type
externalCodes := errorkit.GetCodesByType(errorkit.ErrorTypeExternal)
```

`RegisterMany` and `MustRegister` validate required fields (`Code`, `Type`, `Group`, `Category`, `Description`, `HTTPStatus` in [100, 599]) and reject duplicate codes whose metadata differs from what is already registered. Identical duplicate metadata is a no-op.

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

`NewRegistry(...)` creates an empty isolated registry seeded only with the metadata you pass. `NewDefaultRegistry()` creates an isolated copy of the built-in registry. `Registry.Merge(other)` copies metadata from another registry with the same conflict semantics. `Registry` exposes `GetMetadata`, `GetAllCodes`, `GetCodesByGroup`, `GetCodesByType`, `RegisterMany`, `MustRegister`, `OverrideMetadata`, `Merge`, and `NewError` — mirroring the global API but scoped to the instance.

---

## Factory API

Use a `Factory` when you need explicit, mutation-free configuration (for example, in SDK code that should not touch the package globals).

```go
factory := errorkit.NewFactory(errorkit.Config{
    Registry:       errorkit.NewDefaultRegistry(),
    MaxStackDepth:  16,
    StrictMetadata: true,
})

err := factory.NewError(errorkit.ERR_INTERNAL)
```

`Config` fields:

| Field            | Description                                                                                          |
| ---------------- | ---------------------------------------------------------------------------------------------------- |
| `Registry`       | Registry to look up metadata in. Defaults to `NewDefaultRegistry()` when nil.                        |
| `MaxStackDepth`  | Stack depth for this factory only. Clamped to [1, 100].                                              |
| `StrictMetadata` | When true, errors created from unregistered codes get `Reason = StrictMetadataReason` automatically. |

The factory does not mutate `ErrorRegistry`, `MaxStackDepth`, or color globals.

---

## Configuration

### Stack depth

Global compatibility helpers:

```go
// At startup (default is 32, clamped to [1, 100])
errorkit.SetMaxStackDepth(16)

// Via environment variable
// ERROR_KIT_MAX_STACK=16 ./your-binary

// Or call InitFromEnv() to load from environment
errorkit.InitFromEnv()
```

For per-instance control without globals, use `Factory` (see above).

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
minID := errorkit.MinULID(startTime)
maxID := errorkit.MaxULID(endTime)
```

`ValidateULID` rejects timestamps before 2000 and more than 1 year in the future.

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
)
```

### Step 2 — Register metadata in `registry.go`

Add entries to the `ErrorRegistry` map inside the `var` block. Match the group to an existing `ErrorGroup` constant (or define a new one — see next section).

```go
// registry.go — inside ErrorRegistry = map[ErrorCode]Metadata{ ... }

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
    GroupSystem       ErrorGroup = "system"
    // ... existing groups ...
    GroupPayment      ErrorGroup = "payment"   // ← add here
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

## Built-in Error Groups

Declared in `types.go`. Several groups are reserved for downstream code that registers its own codes — only `GroupUnknown`, `GroupData`, `GroupSystem`, and `GroupClient` are populated by built-in registry entries today.

| Constant            | Value             |
| ------------------- | ----------------- |
| `GroupUnknown`      | `unknown`         |
| `GroupData`         | `data`            |
| `GroupSystem`       | `system`          |
| `GroupClient`       | `client`          |
| `GroupAuth`         | `auth`            |
| `GroupStorage`      | `storage`         |
| `GroupDatabase`     | `database`        |
| `GroupMessageQueue` | `message_queue`   |

---

## Built-in Error Codes Reference

The package ships with a small core registry. Domain-specific codes (auth, database, cache, storage, message queue, etc.) are expected to be registered by the consuming application via `MustRegister` or by editing `registry.go`.

| Code                            | Group  | HTTP | Retriable | Description                            |
| ------------------------------- | ------ | ---- | --------- | -------------------------------------- |
| `ERR_UNKNOWN`                   | unknown | 500 | ✗         | Unknown error occurred                 |
| `ERR_INTERNAL`                  | system  | 500 | ✗         | Internal server error                  |
| `ERR_VALIDATION`                | data    | 400 | ✗         | Validation failed                      |
| `ERR_VALIDATION_INVALID_FORMAT` | data    | 400 | ✗         | Field has invalid format               |
| `ERR_VALIDATION_MISSING_FIELD`  | data    | 400 | ✗         | Required field is missing              |
| `ERR_VALIDATION_INCONSISTENT`   | data    | 400 | ✗         | Fields are inconsistent                |
| `ERR_VALIDATION_BUSINESS_RULE`  | data    | 422 | ✗         | Business rule violation                |
| `ERR_VALIDATION_DUPLICATE`      | data    | 409 | ✗         | Duplicate resource                     |
| `ERR_CLIENT_BAD_REQUEST`        | client  | 400 | ✗         | Invalid client request                 |
| `ERR_CLIENT_NOT_FOUND`          | client  | 404 | ✗         | Resource not found                     |
| `ERR_CLIENT_RATE_LIMIT`         | client  | 429 | ✓         | Rate limit exceeded                    |
| `ERR_SYSTEM_UNEXPECTED`         | system  | 500 | ✗         | Unexpected error occurred              |
| `ERR_SYSTEM_CONFIG_INVALID`     | system  | 500 | ✗         | Invalid system configuration           |
| `ERR_SYSTEM_TIMEOUT_INTERNAL`   | system  | 500 | ✓         | Internal operation timeout             |
| `ERR_SYSTEM_CONCURRENCY`        | system  | 409 | ✓         | Concurrency conflict detected          |
