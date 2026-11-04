# `algorithms` — Exponential Backoff Factory

Package `algorithms` provides a thin, configuration-driven factory for building exponential
backoff policies on top of `github.com/cenkalti/backoff/v4`.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [API Reference](#api-reference)
  - [ExponentialBackoffConfig](#exponentialbackoffconfig)
  - [DefaultExponentialBackoffConfig](#defaultexponentialbackoffconfig)
  - [NewExponentialBackoff](#newexponentialbackoff)
- [Real-World Usage](#real-world-usage)
  - [Custom retry policy](#custom-retry-policy)
- [Lifecycle / Concurrency Notes](#lifecycle--concurrency-notes)
- [Dependencies](#dependencies)

---

## Overview

| Symbol | Purpose |
|---|---|
| `ExponentialBackoffConfig` | Holds the tuning parameters for an exponential backoff policy |
| `MaxRetriesLimit` | Upper bound (1000) applied to `MaxRetries` |
| `DefaultExponentialBackoffConfig` | Returns a ready-to-use default configuration (1 s → 30 s, ×2, 5 total attempts) |
| `NewExponentialBackoff` | Constructs a `algorithms.Backoff` value ready to pass to `backoff.Retry` |

---

## Quick Start

```go
import (
    "github.com/trypanic/go-sdk/v2/algorithms"
    "github.com/cenkalti/backoff/v4"
)

bo := algorithms.NewExponentialBackoff(algorithms.DefaultExponentialBackoffConfig())
err := backoff.Retry(func() error {
    return callExternalAPI()
}, bo)
```

---

## Configuration

`ExponentialBackoffConfig` is the only configuration surface exposed by the package.
There are no environment variables.

```go
type ExponentialBackoffConfig struct {
    InitialInterval time.Duration // wait before the first retry
    MaxInterval     time.Duration // cap on inter-retry delay
    Multiplier      float64       // factor applied to the interval after each attempt
    MaxRetries      uint64        // total number of attempts (initial call + retries)
    MaxElapsedTime  time.Duration // optional overall time budget; 0 = no limit
}
```

| Field | Type | Default | Controls |
|---|---|---|---|
| `InitialInterval` | `time.Duration` | `1s` | Wait before the first retry |
| `MaxInterval` | `time.Duration` | `30s` | Upper bound on the inter-retry delay |
| `Multiplier` | `float64` | `2.0` | Multiplicative growth factor between attempts |
| `MaxRetries` | `uint64` | `5` | Total number of attempts, including the initial call |
| `MaxElapsedTime` | `time.Duration` | `0` (no limit) | Stops the schedule once elapsed time plus the next delay exceeds it |

**Invalid values are replaced, never passed through** (so a zero or partial config can
not turn into a retry loop without waiting):

| Field | Invalid value | Replaced by |
|---|---|---|
| `InitialInterval` | `<= 0` | `1s` |
| `MaxInterval` | `<= 0` | `30s`, or `InitialInterval` when that is larger |
| `Multiplier` | `< 1` or NaN | `2.0` (`1` is valid: constant delay) |
| `MaxRetries` | `0` | `1` (one attempt, no retry) |
| `MaxRetries` | `> MaxRetriesLimit` (1000) | `MaxRetriesLimit` (e.g. an `int` `-1` converted to `uint64`) |
| `MaxElapsedTime` | `< 0` | `0` (no limit) |

> **`MaxRetries` semantics.** The value is passed to `backoff.WithMaxRetries` as
> `MaxRetries - 1` so that the policy executes exactly `MaxRetries` attempts in total
> (1 initial attempt + `MaxRetries - 1` retries). With the default of `5`, five attempts
> are made before `backoff.Retry` gives up and returns the last error.
>
> **`MaxElapsedTime` decision.** `cenkalti/backoff` defaults to a 15-minute
> `MaxElapsedTime`, which used to stop long schedules (large `MaxRetries`, long
> intervals) before `MaxRetries` attempts. The factory now sets it from the config, and
> the zero value means **no elapsed-time limit**, so `MaxRetries` alone bounds the
> schedule (at most `MaxRetriesLimit` attempts, each delay at most
> `MaxInterval × 1.5`). Set `MaxElapsedTime` to restore an overall time budget.

---

## API Reference

### ExponentialBackoffConfig

```go
type ExponentialBackoffConfig struct {
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
    MaxRetries      uint64
    MaxElapsedTime  time.Duration
}
```

Value type. A zero-initialized struct is valid: every field gets the defaults from the
table above (one attempt, no retry). Prefer `DefaultExponentialBackoffConfig` and override
individual fields.

---

### DefaultExponentialBackoffConfig

```go
func DefaultExponentialBackoffConfig() ExponentialBackoffConfig
```

Returns:

```go
ExponentialBackoffConfig{
    InitialInterval: 1 * time.Second,
    MaxInterval:     30 * time.Second,
    Multiplier:      2.0,
    MaxRetries:      5,
}
```

Use this as a starting point when the caller does not need to override individual fields.

---

### NewExponentialBackoff

```go
func NewExponentialBackoff(config ExponentialBackoffConfig) Backoff
```

Constructs a `algorithms.Backoff` by:

1. Replacing invalid config values (see [Configuration](#configuration)) and creating a
   `*backoff.ExponentialBackOff` with `InitialInterval`, `MaxInterval`, `Multiplier` and
   `MaxElapsedTime` applied from the config.
2. Calling `Reset()` so the first `NextBackOff()` already uses the configured
   `InitialInterval` (relevant for custom loops that call `NextBackOff()` directly;
   `backoff.Retry` resets on its own).
3. Wrapping it with `backoff.WithMaxRetries(expBackoff, config.MaxRetries-1)` to bound
   the total number of attempts.

The returned value satisfies the `algorithms.Backoff` interface and is intended to be passed
directly to `backoff.Retry`.

**Edge cases.** `MaxRetries` of `0` is treated as `1`: the initial call is the only
attempt and no retry occurs (there is no `uint64` underflow). Values above
`MaxRetriesLimit` are capped.

**Error conditions.** The function never returns an error. Invalid values are replaced as
documented above; an explicit `InitialInterval > MaxInterval` is clamped to `MaxInterval`
by the upstream `cenkalti/backoff` library.

---

## Real-World Usage

### Custom retry policy

Use `NewExponentialBackoff` in a caller-owned loop and compare each delay with
`algorithms.Stop`. Its historical half-to-one-and-a-half jitter distribution
remains unchanged. SDK transports use their separate shared full-jitter schedule
in `internal/backoff`, with their own attempt and context budgets.

---

## Lifecycle / Concurrency Notes

- `NewExponentialBackoff` allocates a new `*backoff.ExponentialBackOff` on every call.
  The returned `algorithms.Backoff` is **not safe for concurrent use**; the cenkalti/backoff
  library maintains mutable internal state (current interval, attempt count) that is
  updated on each call to `NextBackOff`.
- Create a new `algorithms.Backoff` for each independent retry sequence. Do not share an
  instance across goroutines or reuse one after a retry loop completes.
- The package itself has no global state and requires no shutdown procedure.

---

## Dependencies

| Package | Role |
|---|---|
| `github.com/cenkalti/backoff/v4` | Provides `ExponentialBackOff`, `WithMaxRetries`, and the `BackOff` interface consumed by `backoff.Retry` |

The public schedule contract is SDK-owned: `Backoff` exposes `NextBackOff` and
`Reset`; compare the returned duration with `algorithms.Stop` to detect exhaustion.
The upstream implementation is an internal detail; structural compatibility still
allows passing the result to retry loops expecting those two methods.
