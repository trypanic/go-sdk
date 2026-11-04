# `envs` — Environment Variable Loader

Thin wrapper around [`caarlos0/env`](https://github.com/caarlos0/env) that parses environment variables into a typed Go struct and surfaces failures as a structured `errorkit` error rather than a raw `error`.

---

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `Load[T]` | `func[T any]() (T, error)` | Parse environment variables into a new `T` and return it; returns `ERR_SYSTEM_CONFIG_INVALID` on failure |
| `LoadInto` | `func(c any) error` | Parse environment variables into an existing config struct; returns `ERR_SYSTEM_CONFIG_INVALID` on failure |
| `LoadWithOptions` | `func(c any, opts env.Options) error` | Like `LoadInto`, with `caarlos0/env` options (prefix, custom parsers, injected environment map) |

---

## Quick Start

```go
import "github.com/trypanic/go-sdk/envs"

type Config struct {
    Host string `env:"SERVER_HOST" envDefault:"0.0.0.0"`
    Port int    `env:"SERVER_PORT,required"`
}

cfg, err := envs.Load[Config]()
if err != nil {
    return err // *errorkit.AppError, ERR_SYSTEM_CONFIG_INVALID
}

// Equivalent, filling an existing struct:
var c Config
if err := envs.LoadInto(&c); err != nil {
    return err
}
```

---

## Configuration / Settings

`LoadInto` accepts any pointer to a struct whose fields are annotated with struct tags understood by `caarlos0/env/v11`.

### Struct Tag Reference

| Tag | Type | Default | Description |
|---|---|---|---|
| `env:"VAR_NAME"` | `string` | — | Maps the field to the named environment variable |
| `env:"VAR_NAME,required"` | `string` | — | Marks the variable as mandatory; loading fails if it is not set (a set but empty value passes; add `notEmpty` to reject it) |
| `envDefault:"value"` | `string` | — | Value used when the environment variable is not set |

### Supported Field Types

The underlying `caarlos0/env` library supports `string`, `bool`, `int*`, `uint*`, `float*`, `time.Duration`, `[]T` (comma-separated), and any type implementing `encoding.TextUnmarshaler`. Nested structs are flattened — struct fields are parsed recursively without a prefix unless you add `envPrefix`.

### Example annotated struct

```go
type ServerConfig struct {
    Host         string        `env:"SERVER_HOST"         envDefault:"0.0.0.0"`
    Port         int           `env:"SERVER_PORT"`
    ReadTimeout  time.Duration `env:"READ_TIMEOUT"        envDefault:"30s"`
    WriteTimeout time.Duration `env:"WRITE_TIMEOUT"       envDefault:"30s"`
}

type RedisConfig struct {
    Host     string `env:"REDIS_HOST"     envDefault:"localhost"`
    Port     int    `env:"REDIS_PORT"     envDefault:"6379"`
    Password string `env:"REDIS_PASSWORD" envDefault:""`
    DB       int    `env:"REDIS_DB"       envDefault:"0"`
}

// Top-level config composes sub-configs; LoadInto parses all fields recursively.
type Config struct {
    Server ServerConfig
    Redis  RedisConfig
}
```

---

## API Reference

### `Load`

```go
func Load[T any]() (T, error)
```

Allocates a `T`, parses the environment into it (same rules and errors as
`LoadInto`) and returns it. On failure it returns the zero `T` (never a
partially loaded value) and the `ERR_SYSTEM_CONFIG_INVALID` error. `T` must be
a struct type, not a pointer; any other type fails with
`ERR_SYSTEM_CONFIG_INVALID`.

---

### `LoadWithOptions`

```go
func LoadWithOptions(c any, opts env.Options) error
```

Same as `LoadInto`, but calls `env.ParseWithOptions(c, opts)` from
`caarlos0/env/v11`, so callers can set a `Prefix`, custom parsers (`FuncMap`)
or an `Environment` map that replaces the process environment. The last one
lets tests of consumer code avoid mutating the process environment:

```go
import "github.com/caarlos0/env/v11"

var cfg Config
err := envs.LoadWithOptions(&cfg, env.Options{
    Environment: map[string]string{"SERVER_PORT": "8080"},
})
```

---

### `LoadInto`

```go
func LoadInto(c any) error
```

**Parameters**

| Parameter | Description |
|---|---|
| `c` | Pointer to the config struct to populate. Must be a non-nil pointer. |

**Behaviour**

1. Calls `env.Parse(c)` from `caarlos0/env/v11`, which reads environment variables and sets fields on `c` according to their `env` and `envDefault` struct tags.
2. If parsing succeeds, returns `nil`.
3. If parsing fails for any reason (missing required variable, type conversion error, etc.), returns a structured `*errorkit.AppError` with:
   - Code: `errorkit.ERR_SYSTEM_CONFIG_INVALID` (HTTP 500, non-retriable)
   - Reason: `"failed to parse environment variables: <details>"`, where the details name the
     variables, fields and types involved but **never the raw values** (a secret assigned to
     an `int` field does not end up in logs)
   - Wrapped: a value-free error with the same details whose chain still contains the
     original `caarlos0/env` error, so `errors.As(err, &env.ParseError{})` and similar
     checks keep working; `Pretty()`, `PrettyJSON()` and the logger print only the
     value-free text

**Error conditions**

| Condition | errorkit Code | Notes |
|---|---|---|
| Required variable not set (or empty with `notEmpty`) | `ERR_SYSTEM_CONFIG_INVALID` | The reason names the missing variable |
| Value cannot be converted to field type | `ERR_SYSTEM_CONFIG_INVALID` | e.g. `"abc"` for an `int` field |
| `c` is `nil` or not a pointer | `ERR_SYSTEM_CONFIG_INVALID` | Propagated from `env.Parse` |

**Inspecting the error**

```go
import (
    "errors"
    "github.com/trypanic/go-sdk/errorkit"
    "github.com/trypanic/go-sdk/envs"
)

cfg := &Config{}
if err := envs.LoadInto(cfg); err != nil {
    if appErr, ok := errors.AsType[*errorkit.AppError](err); ok {
        // appErr.ErrCode  == errorkit.ERR_SYSTEM_CONFIG_INVALID
        // appErr.Reason   == "failed to parse environment variables: ..." (no values)
        // errors.As(err, &env.ParseError{}) reaches the original env.Parse error
        log.Fatalf("config error: %s", appErr.Pretty())
    }
}
```

---

## Real-World Usage

The pattern below shows loading both shared and caller-specific configuration. `envs.LoadInto` is the intended standardised entry point for caller-specific config structs.

### Caller-specific config alongside shared config

```go
package config

type Auth struct {
    Username   string `env:"AUTH_USERNAME,required"`
    Password   string `env:"AUTH_PASSWORD,required"`
    AppKey     string `env:"AUTH_APP_KEY,required"`
    URLService string `env:"AUTH_URL_SERVICE,required"`
}
```

```go
package main

import (
    "github.com/trypanic/go-sdk/envs"
    "github.com/trypanic/go-sdk/logger"
)

func main() {
    // Load caller-specific env vars with structured error on failure.
    authConfig := &Auth{}
    if err := envs.LoadInto(authConfig); err != nil {
        logger.Panic(err)
    }

    // ... use authConfig
}
```

### Generic helper pattern

When a caller defines multiple independent config sections, call `LoadInto` once per top-level struct or use a single aggregate struct:

```go
type Config struct {
    LLM      LLMConfig
    RabbitMQ RabbitMQConfig
    App      AppConfig
}

cfg := &Config{}
if err := envs.LoadInto(cfg); err != nil {
    return err
}
```

---

## Lifecycle / Concurrency Notes

- `Load`, `LoadInto` and `LoadWithOptions` have no internal state and are safe to call from multiple goroutines simultaneously, though in practice they are called once at process startup before any concurrent code runs.
- The functions read from the process environment at call time (`LoadWithOptions` reads `opts.Environment` instead when it is set). Environment variables must be set before loading (e.g. via a `.env` file loaded by `godotenv`, or injected by the container runtime).
- There is no `Close` or teardown step — the populated struct is a plain Go value.

---

## Dependencies

| Package | Role |
|---|---|
| `github.com/caarlos0/env/v11` | Parses environment variables into annotated Go structs |
| `github.com/trypanic/go-sdk/errorkit` | Wraps parse failures as structured `AppError` with code `ERR_SYSTEM_CONFIG_INVALID` |
