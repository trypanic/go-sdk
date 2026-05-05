# `envs` — Environment Variable Loader

Thin wrapper around [`caarlos0/env`](https://github.com/caarlos0/env) that parses environment variables into a typed Go struct and surfaces failures as a structured `errorkit` error rather than a raw `error`.

---

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `NewLoader` | `func(c any) error` | Parse environment variables into a config struct; returns `ERR_SYSTEM_CONFIG_INVALID` on failure |

---

## Quick Start

```go
import "github.com/trypanic/go-sdk/envs"

type Config struct {
    Host string `env:"SERVER_HOST" envDefault:"0.0.0.0"`
    Port int    `env:"SERVER_PORT,required"`
}

func Load() (*Config, error) {
    cfg := &Config{}
    if err := envs.NewLoader(cfg); err != nil {
        return nil, err
    }
    return cfg, nil
}
```

---

## Configuration / Settings

`NewLoader` accepts any pointer to a struct whose fields are annotated with struct tags understood by `caarlos0/env/v11`.

### Struct Tag Reference

| Tag | Type | Default | Description |
|---|---|---|---|
| `env:"VAR_NAME"` | `string` | — | Maps the field to the named environment variable |
| `env:"VAR_NAME,required"` | `string` | — | Marks the variable as mandatory; `NewLoader` returns an error if it is absent or empty |
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

// Top-level config composes sub-configs; NewLoader parses all fields recursively.
type Config struct {
    Server ServerConfig
    Redis  RedisConfig
}
```

---

## API Reference

### `NewLoader`

```go
func NewLoader(c any) error
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
   - Reason: `"failed to parse environment variables"`
   - Wrapped: the original `error` from `env.Parse`

**Error conditions**

| Condition | errorkit Code | Notes |
|---|---|---|
| Required variable absent or empty | `ERR_SYSTEM_CONFIG_INVALID` | The wrapped error names the missing variable |
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
if err := envs.NewLoader(cfg); err != nil {
    var appErr *errorkit.AppError
    if errors.As(err, &appErr) {
        // appErr.ErrCode  == errorkit.ERR_SYSTEM_CONFIG_INVALID
        // appErr.Reason   == "failed to parse environment variables"
        // appErr.Wrapped  == original env.Parse error
        log.Fatalf("config error: %s", appErr.Pretty())
    }
}
```

---

## Real-World Usage

The pattern below shows loading both shared and caller-specific configuration. `envs.NewLoader` is the intended standardised entry point for caller-specific config structs.

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
    if err := envs.NewLoader(authConfig); err != nil {
        logger.Panic(err)
    }

    // ... use authConfig
}
```

### Generic helper pattern

When a caller defines multiple independent config sections, call `NewLoader` once per top-level struct or use a single aggregate struct:

```go
type Config struct {
    LLM      LLMConfig
    RabbitMQ RabbitMQConfig
    App      AppConfig
}

cfg := &Config{}
if err := envs.NewLoader(cfg); err != nil {
    return err
}
```

---

## Lifecycle / Concurrency Notes

- `NewLoader` has no internal state and is safe to call from multiple goroutines simultaneously, though in practice it is called once at process startup before any concurrent code runs.
- The function reads from the process environment at call time. Environment variables must be set before `NewLoader` is invoked (e.g. via a `.env` file loaded by `godotenv`, or injected by the container runtime).
- There is no `Close` or teardown step — the populated struct is a plain Go value.

---

## Dependencies

| Package | Role |
|---|---|
| `github.com/caarlos0/env/v11` | Parses environment variables into annotated Go structs |
| `github.com/trypanic/go-sdk/errorkit` | Wraps parse failures as structured `AppError` with code `ERR_SYSTEM_CONFIG_INVALID` |
