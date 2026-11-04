# go-sdk — Audit Report (P2.Z)

- Scope: `github.com/trypanic/go-sdk`, branch `audit/2026-11` @ `bc82052`
  (code identical to `origin/main` @ `f85f5cf`; `.go` files unchanged since
  `cfd8b82`). Released API baseline: `v1.0.0`.
- Inputs: `discovery.md` (P1), `baseline.md` (P0.2) and the nine findings
  files under `findings/` (P2.C1–C6, P2.X1–X3), each reviewed by the
  orchestrator (see the ledger in `audit.md` §9).
- Raw evidence (code excerpts, reproduction output) lives in the findings
  files; this report consolidates, deduplicates and ranks. Every finding ID
  below links back to `findings/P2.<task>.md`.

## 1. Executive summary

The SDK is a well-layered, single-module toolkit (22 packages, ~9.7k lines
of production code) with a dependency-free core (`errorkit`), consistent
error-code registration, decorator-based tracing and no API break since
`v1.0.0` (gorelease: compatible, suggests `v1.1.0`). Tests are fast and
race-clean, but total coverage is 47.7% and the riskiest paths (messaging
consume/publish, Hertz adapter, retry loop, OTLP) are largely untested.

The audit found **no CRITICAL** issues and **21 HIGH** issues out of **170
unique findings** (181 raw, 11 merged duplicates). The HIGH issues cluster
in four themes:

1. **Information disclosure to clients and logs** — panic values, error
   reasons and stack traces cross the gRPC/HTTP boundary (C1-002, C2-017,
   C3-001, C3-002); credentials land in logs and error payloads (C4-001:
   Postgres DSN password; C2-006: URL userinfo, query secrets, API-key
   error text).
2. **Silent failure / wrong success** — `Publish` reports success for
   dropped messages (C5-002); `httprequest.Do` returns `nil` without sending
   anything for negative `MaxRetries` (C2-003); the Hertz audit middleware
   logs nothing while copying every body (C2-005); a Hertz bind failure
   kills the process because `Run()` cannot return an error (C2-001).
3. **Unsafe retry and shutdown semantics** — non-idempotent POSTs (incl.
   billable LLM calls) are retried (C2-002); consumer shutdown dead-letters
   in-flight messages (C5-001); concurrent `FileStorage` instances corrupt
   data (C4-012); `logger.New` mutates zerolog globals with a demonstrated
   data race (C1-001).
4. **Error-model erosion and defaults** — errorkit metadata is ignored by the
   transports, so most codes become gRPC `Internal` (C3-003) and every
   Postgres error is a retriable 503 (C4-002); gRPC transport defaults to
   plaintext (C3-005); `urlkit.BuildURL` lets a path change the host
   (C6-001); a decode panic on named types (C2-004).

Two structural issues complete the HIGH list: `logger` embeds the OTLP gRPC
exporter, so any consumer of `httprequest` links gRPC (X1-001, 17 MB vs
6 MB); and builds with Go ≤ 1.26.9 — including the SDK's own release
pipeline, which uses `go.mod`'s 1.26.1 — reach 19 standard-library
vulnerabilities (X3-001).

On the engineering-process side, there is no CI for tests, lint or
vulnerabilities (X2-001), and the static-analysis gate cannot run from a
fresh clone and reports "OK" when its tools crash (X2-002, X2-003); the
untouched tree already fails `task staticanalysis` (SA1019, C1-024).

Many fixes change behavior that a README or AGENTS.md documents; those are
marked `Needs approval: yes` and must go through gate G1 (§3 of the work
order). A v2 is only needed for the items marked breaking (package renames,
interface reshaping, removal of third-party types from the API).

## 2. Overall state

| Dimension | State | Evidence |
| --- | --- | --- |
| Build / vet / format | Green | baseline V1–V3 |
| Unit tests / race | Green, fast (~0.8 s) | baseline V4, V5 |
| Coverage | 47.7% total; messaging 10.5%, validators 9.7%, hertz 19.4% | baseline V6 |
| Static analysis | **Red** on untouched tree (4× SA1019) | baseline V7, C1-024 |
| Vulnerabilities | **Red** with go1.26.2 (19 stdlib), green with go1.27.3 | baseline V8, X3-001 |
| Lint (informational) | 25 golangci-lint issues | baseline V9 |
| API compatibility | Compatible with v1.0.0 | baseline V10, X1 rerun |
| CI | Release workflow only | X2-001 |

## 3. Architecture

- **Layering is sound.** The import graph is acyclic (30 intra-module
  edges): `errorkit` at the bottom with no SDK imports; `telemetry`,
  `logger`, `algorithms` above it; transport and storage adapters on top
  (`discovery.md` §3).
- **One module, one version.** All 22 packages share `go.mod`, so every
  consumer inherits the full requirement list in version selection
  (X3-004) even when importing only stdlib-only packages.
- **Weight leaks through `logger`.** Because `logger` contains the OTLP gRPC
  exporter, `httprequest`, `llmclient`, `messaging`, `grpc` and `hertz` all
  link gRPC; importing `logger` also registers `/debug/requests` and
  `/debug/events` on the default mux (X1-001, X3-006).
- **Inconsistent cross-cutting seams.** Tracing injection has four shapes
  (`*Instrumenter`, `TracerProvider`, `trace.Tracer`, global only), logger
  injection differs per package and library code logs-and-returns through
  the global logger (C3-016, X1-008, C2-009, C5-011); constructors that do
  network I/O treat `context` differently (C4-004).
- **Contracts drift from code.** Several README/AGENTS promises are not
  implemented (`LLM_API_KEY` `*FromEnv`, "no package mutates the global
  logger", `WrapWithInstrumenter(nil)` disables tracing, audit middleware
  logs redacted bodies) — X1-015, C2-015, C1-001, C4-005, C2-005, X2-010.

## 4. Strengths (keep these)

- Acyclic, layered graph with a stdlib-only core; framework isolation in
  `httpserver` (no Hertz import in the contracts package).
- Consistent error-code registration via `errorkit.MustRegister` in `init()`
  across six packages; error IDs from `crypto/rand`; `WithWrapped` keeps
  `errors.Is/As` working in most constructors.
- `grpc` is the model package: default-on functional options, otelgrpc stats
  handler covering all four call modes, correct `GracefulStop`/`Stop`
  shutdown, `grpc.NewClient`, full godoc, bufconn tests of the real
  `New`/`Dial` path.
- `httprequest` is immutable after construction, rebuilds the body per
  attempt, honors ctx between attempts, propagates W3C trace context, keeps
  connections reused (verified), and redacts bodies by default.
- Postgres helpers never build SQL (positional parameters only, including
  the JSON argument); `rows.Close`/`rows.Err` handled via `pgx.Collect*`.
- Messaging uses explicit acks, ctx-aware reconnect, exact routing keys and
  end-to-end trace propagation over AMQP headers.
- `storage` hex-encodes keys, preventing path traversal; `ioutils` really is
  dev-only (no production importer).
- Release pipeline hygiene: actions pinned by SHA, least-privilege
  permissions, Conventional Commits enforced; `go mod tidy -diff` and
  `go mod verify` clean; every linked dependency at its latest version; no
  reachable third-party vulnerability.

## 5. HIGH findings (verified by the orchestrator)

Each was re-checked against the code during the P2 reviews (ledger notes in
`audit.md` §9); reproduction output is in the cited findings file.

| ID | Area | Finding | Evidence (verified) | Needs approval |
| --- | --- | --- | --- | --- |
| C1-001 | C | `logger.New` mutates zerolog global level and `TimeFieldFormat`; race demonstrated with `-race` | `logger/config.go:303-310` | no |
| C1-002 | D | `PrettyJSON`, recommended by AGENTS §4 for HTTP responses, exposes stack traces, paths, payload and wrapped error text | `errorkit/error.go:38,192`; `errorkit/fmt.go:202-245` | yes |
| C2-001 | C | Hertz `Run()` calls `Spin()` and returns `nil`; a bind failure panics the process | `httpserver/hertz/wrapper.go:58-61` | yes |
| C2-002 | B | Retry loop ignores method idempotency; POST re-sent 5× | `httprequest/httprequest.go:187-223` | yes |
| C2-003 | B | `MaxRetries < 0` skips the loop and returns `nil` (no request sent) | `httprequest/httprequest.go:117-128,187,222` | no |
| C2-004 | B | `out.(*string)` / bool assertions panic for named types and typed-nil pointers | `httprequest/decode.go:15-57` | no |
| C2-006 | D | URL credentials, query-string secrets and upstream error messages (e.g. `Incorrect API key provided: sk-live-…`) are copied into error payloads, the global error log, audit logs and span events on every non-2xx | `httprequest/httperror.go:30-31,156`; reproduction log line in P2.C2 | yes |
| C2-005 | D | Inbound audit middleware is on by default, logs nothing (logging commented out), still copies bodies and headers | `httpserver/hertz/server.go:152-205` | yes |
| C2-017 | D | `httpserver.WithErrorOpt` / Hertz context reply put `err.Error()` into `Reply.Error` sent to HTTP clients (HTTP counterpart of C3-002) | `httpserver/response.go:28-40`; `httpserver/hertz/context.go:85-96` | yes |
| C3-001 | D | Recovery puts the panic value and method name in the client-visible status | `grpc/interceptors.go:43-53` | no |
| C3-002 | D | `ToStatus` sends `appErr.Error()` (Reason) for every code, including `Internal` | `grpc/status.go:53-62` | yes |
| C3-003 | G | `codeFor` maps every package-registered code to `codes.Internal`, ignoring metadata (absorbs X1-007, same defect in `httpserver`) | `grpc/status.go:14-38` | yes (X1-007 part) |
| C3-005 | D | Client defaults to insecure credentials; `ServerConfig` has no credentials field | `grpc/client.go:31-36`, `grpc/config.go:78-80` | yes |
| C4-001 | D | pgx parse error (with unredacted `ConnString`) stored via `WithPayload` and logged | `postgres/postgres.go:37-42,65-79` | no |
| C4-002 | B | Only `ERR_DB_POSTGRES_ERROR` (retriable, 503) is ever constructed; TIMEOUT/DEADLOCK/AUTH/UNAVAILABLE unused | `postgres/stored_procedure.go:32-70`; grep of `NewError(` | yes |
| C4-012 | C | `FileStorage` uses a fixed `.tmp` name and an in-process mutex only: two instances corrupt the file | `storage/file_storage.go:18-21,200-222` | no |
| C5-001 | C | Consumer retry uses `backoff.Retry` without ctx, then `Nack(requeue=false)`: shutdown dead-letters in-flight messages | `messaging/messaging.go:328-331,371-382` | no |
| C5-002 | B | `Publish` uses `mandatory=false`, no publisher confirms, queues declared only on `Subscribe` | `messaging/amqp.go:108,123,159,167` | yes |
| C6-001 | D | `BuildURL` parses `base + path`; `@evil.com/x` changes the host | `urlkit/url.go:41-56` | yes |
| X1-001 | A | `logger` imports `otlploggrpc`; `httprequest` imports `logger` → links gRPC (absorbs X3-005) | `go list -deps ./httprequest` | yes |
| X3-001 | D | 19 reachable stdlib vulns with Go ≤ 1.26.9; release workflow builds with go.mod's 1.26.1 | baseline V8; `.github/workflows/release.yml:57-59` | yes |

## 6. Findings by area and severity

Full detail (problem, evidence, impact, source, recommendation, breaking)
is in the findings files under the same ID. "Absorbs" lists duplicates
merged into the kept ID; the kept finding takes the union of their scope
(locations appended as `+ <ID>: …`), the highest severity and the strongest
`Needs approval` value. `partial` means the finding's recommendation has an
approval-free part and a part that needs approval (see the findings file).

Unique findings after deduplication: **170** (from 181 raw; 11 duplicates merged). CRITICAL 0 · HIGH 21 · MEDIUM 87 · LOW 62. `Needs approval`: yes 67, partial 5, no 98.

| Area | CRITICAL | HIGH | MEDIUM | LOW | Total |
| --- | --- | --- | --- | --- | --- |
| A. Architecture & design | 0 | 1 | 2 | 2 | 5 |
| B. Idiomatic & modern Go / correctness | 0 | 5 | 19 | 21 | 45 |
| C. Concurrency & lifecycle | 0 | 4 | 11 | 0 | 15 |
| D. Security & supply chain | 0 | 10 | 10 | 9 | 29 |
| E. Testing | 0 | 0 | 9 | 3 | 12 |
| F. Performance | 0 | 0 | 8 | 1 | 9 |
| G. Public API, observability & DX | 0 | 1 | 17 | 18 | 36 |
| H. Maintainability & DevEx | 0 | 0 | 11 | 8 | 19 |
| **Total** | 0 | 21 | 87 | 62 | 170 |

### A. Architecture & design

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| X1-001 | HIGH | M | `logger` mixes the zerolog API with the OTLP gRPC exporter, so every package that logs pulls gRPC and OTLP into consumer binaries | logger/logprovider.go:8,62 (imports and calls `otlploggrpc`); httprequest/httperror.go:14,156 and httprequest/audit.go:10,50-51 (only uses of `logger` in `httprequest`); grpc/options.go:8,59; messaging/messaging.go:14,75, messaging/amqp.go:10; httpserver/hertz/server.go:17, request_logger.go:11; + X3-005: logger/logprovider.go:8 (`otlploggrpc`); httprequest/audit.go:10, httprequest/httperror.go:14 (import `logger`); messaging/messaging.go (public `WithLogger(*logger.Logger)`) | yes | X3-005 |
| X1-008 | MEDIUM | M | Logger injection is inconsistent; library code logs returned errors through the global logger | httprequest/httperror.go:156 (`logger.Error(appErr)` on every non-2xx); messaging/amqp.go:98,105,116,143,160 (global `logger.Info` despite `WithLogger`, messaging.go:75); httpserver/hertz/server.go:103-109 (`RecoveryHandler` → global `logger.ErrorCtx`, no option); grpc/options.go:59 (`WithServerLogger`, the one complete case); AGENTS.md:120-121 | no |  |
| X1-012 | MEDIUM | M | Retry/backoff is implemented three times with divergent semantics, and `httprequest` does not use the SDK's own `algorithms` | httprequest/httprequest.go:40-44 (`RetryConfig{MaxRetries int, InitialDelay, MaxDelay time.Duration}`), 117-128, 224-232 (own doubling loop, no jitter); algorithms/backoff.go:9-42 (`MaxRetries uint64`, cenkalti with its default randomisation); messaging/types.go:30-35 (`RetryConfig{MaxAttempts int, InitialDelay/MaxDelay int64 ms, BackoffMult}`) → `algorithms`; messaging/messaging.go:388-409 (a third reconnect loop, 5 s doubling to 60 s) | no |  |
| C6-017 | LOW | S | `ioutils` "dev-only" is enforced by convention only; its tests are weak | ioutils/ioutils_test.go:38-46 (`SaveJSON` test never reads the file back); no tests for `PrintJSON`, `SaveProductsToJSON`, the close-error branch (coverage 68.8%); .sentrux/rules.toml lists `ioutils/*` in the same layer as production packages | partial |  |
| X1-018 | LOW | S | Generic "external service" error codes are owned by `httpclient`, coupling `llmclient` and `httprequest` to the transport factory | httpclient/errors_codes.go (`ERR_EXTERNAL_SERVICE_UNAVAILABLE/TIMEOUT/ERROR/INVALID_RESPONSE`); httpclient/httpclient.go:271 (its only use there); httprequest/httperror.go:13,109-139; llmclient/client.go:10,138 | no |  |

### B. Idiomatic & modern Go / correctness

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C2-002 | HIGH | M | The retry loop ignores HTTP method idempotency: non-idempotent POSTs, including billable LLM calls, are re-sent up to 5 times | `httprequest/httprequest.go:181-223` (decision at 211), `309-317`; `httprequest/errors_codes.go` and `httpclient/errors_codes.go` (`Retriable: true` for 5xx, 429, network error, network timeout); `llmclient/client.go:124-133` | yes |  |
| C2-003 | HIGH | S | A negative `MaxRetries` makes `Do` return `nil` without sending any request | `httprequest/httprequest.go:117-128` (only `== 0` is normalized), `187` (`for attempt := 0; attempt < rc.MaxRetries; …`), `222` (`return lastErr`, which is `nil`) | no |  |
| C2-004 | HIGH | S | Response decoding panics for named string or bool types and for typed-nil `out` pointers | `httprequest/decode.go:15-49` (dispatch on `Kind()`), `57` (`out.(*string)`), `124` (`out.(*bool)`), `23` (`outValue.Elem().Type()` on a nil pointer) | no |  |
| C4-002 | HIGH | M | Every PostgreSQL failure is `ERR_DB_POSTGRES_ERROR` (retriable, HTTP 503), including not-found, constraint violations and caller cancellation | postgres/stored_procedure.go:32-48, 57-70; postgres/postgres.go:37-80; postgres/errors.go:88-96 | yes |  |
| C5-002 | HIGH | M | `Publish` reports success for messages the broker drops (no publisher confirms, `mandatory=false`, queues declared lazily) | messaging/amqp.go:108, 123, 159, 167 (`channel.Publish(…, false, false, …)`); | yes |  |
| C1-008 | MEDIUM | S | `logger.Error` / `Logger.Error` use a type assertion: an `*AppError` wrapped with `%w` is logged as a new generic `ERR_INTERNAL` | logger/wrapper.go:27-43 (assertion 33, re-wrap 39); logger/instance.go:77-91 (assertion 82, re-wrap 87) | no |  |
| C1-009 | MEDIUM | S | `WithErrorKit` (and every `*Ctx` / `*WithTrace` logging helper) panics on a nil `*AppError` | logger/errorkit.go:21-31 (`appErr.ID` at 25); callers logger/wrapper.go:73-91, logger/otel.go:47-63, logger/instance.go:99-108 | no |  |
| C1-011 | MEDIUM | S | OTLP records get second-precision timestamps and lose large integer precision | logger/config.go:310 (`TimeFieldFormat = TimeFormatUnix`); logger/otlp.go:50, 64-65 (`time.Unix(int64(ts), 0)`), 150-170 (`anyToLogValue`); logger/otlp.go:109 (map allocated per write) | no |  |
| C1-024 | MEDIUM | S | `logger` uses the deprecated `go.opentelemetry.io/otel/log/global` package (pre-existing, baseline V7) | logger/logprovider.go:9, 88; logger/otlp.go:10, 41; + X3-007: logger/logprovider.go:9,88; logger/otlp.go:10,41 | no | X3-007 |
| C2-009 | MEDIUM | S | `httprequest` logs every non-2xx response through the global logger and also returns the error (log-and-return, once per attempt) | `httprequest/httperror.go:156` (DN-09); its return value is also unchecked (baseline V9 errcheck) | yes |  |
| C2-013 | MEDIUM | S | Non-transient failures are classified as retriable network errors (redirect-limit violations, invalid URL or scheme) | `httprequest/httperror.go:68-74` (every non-timeout transport error becomes `ERR_NETWORK_ERROR`, `Retriable: true`); `httprequest/httprequest.go:309-317` (`errors.As` finds the outer error first); `httpclient/httpclient.go:269-276` (redirect-limit error is `ERR_EXTERNAL_SERVICE_ERROR`, `Retriable: true`) | yes |  |
| C3-004 | MEDIUM | S | `ToStatus` turns context cancellation and deadline errors into `codes.Unknown` | grpc/status.go:65 | yes |  |
| C4-003 | MEDIUM | S | Pool-construction errors carry the cause as `Payload`, not `Wrapped`: no `errors.Is/As`, and connect/ping failures serialize as "marshal_failed" | postgres/postgres.go:39-42, 67-70, 76-79; errorkit/fmt.go:236-240 | no |  |
| C4-010 | MEDIUM | S | Mongo error classification contradicts its README and marks cancellations and config errors as retriable | mongo/adapter.go:32-37, 47-52; mongo/errors.go:42-72; mongo/client.go:62-65, 133-140; mongo/README.md:221-226 | partial |  |
| C4-011 | MEDIUM | S | Defaults silently override URI options and make `MaxPoolSize < 10` fail at start-up | mongo/client.go:51-60, 108-131; mongo/README.md:96-107 | yes |  |
| C4-014 | MEDIUM | S | `Clear()` deletes every `*.json` in `baseDir`, including files the store did not create | storage/file_storage.go:91-116 (vs 131-139); storage/README.md:109 | yes |  |
| C4-015 | MEDIUM | S | `FileStorage.Append(key, nil)` panics; `MemoryStorage` returns an error for the same input | storage/file_storage.go:37-46 (`*item` at 46); storage/memory_storage.go:53-58 | no |  |
| C4-018 | MEDIUM | S | All storage failures are `ERR_STORAGE_ERROR` (retriable, 503), including programmer errors and the undocumented key-length limit | storage/errors_codes.go:64-72; storage/memory_storage.go:28-32, 54-58; storage/file_storage.go:150-158, 180-195 | yes |  |
| C5-005 | MEDIUM | S | Ack/Nack errors are discarded; at-least-once redelivery is invisible and undocumented | messaging/messaging.go:331 (`_ = msg.Nack(false, false)`), 335 (`_ = msg.Ack(false)`) | no |  |
| C5-006 | MEDIUM | S | SDK errors are re-encoded: `Subscribe` turns a retriable `ERR_MQ_CONNECTION_FAILED` into a non-retriable `ERR_INTERNAL`; several MQ codes are never used | - messaging/messaging.go:249-252 and 258-261: `NewSubscriber` / `Consume` | no |  |
| C5-009 | MEDIUM | S | Retry of a multi-routing-key publish re-sends keys that already succeeded; nested retries multiply attempts | messaging/messaging.go:195-215; messaging/amqp.go:159-173 | no |  |
| C6-003 | MEDIUM | S | `stringutils.RemoveMarkdownCodeBlock` mangles content that has several fenced blocks and breaks its documented "whole content" rule | stringutils/normalize.go:30-32 (lazy `(.*?)` with `(?s)` and `$` anchor), README stringutils/README.md:75, 79 | no |  |
| C6-004 | MEDIUM | S | `algorithms.NewExponentialBackoff` returns a `BackOff` that has not been `Reset`, so its first delay ignores `InitialInterval` | algorithms/backoff.go:33-41 | no |  |
| C6-006 | MEDIUM | S | `slices.Chunk` returns sub-slices with spare capacity, so `append` on one chunk overwrites the next one | slices/chunks.go:16 (README slices/README.md:68-69 documents shared elements only) | no |  |
| C1-015 | LOW | S | Stack-depth budget is spent on errorkit's own frames; file paths keep a hard-coded root from another repository | errorkit/stack.go:10 (`skipNewErrorFrames = 2`), 12-15, 47-64, 107-113 (`const projectRoot = "market-ecosystem/"`) | no |  |
| C1-016 | LOW | S | Registry batch operations commit partially on error; `NewRegistry` skips validation; listings are in random order | errorkit/register.go:52-69, 96-117, 143-159 (`Merge`); errorkit/registry.go:170-178 (`NewRegistry`), 217-306 (`GetAllCodes`, `GetCodesBy*`) | no |  |
| C1-017 | LOW | S | `JSONFormatter` fallback builds JSON with `Sprintf` and can emit invalid JSON | errorkit/fmt.go:238-242 | no |  |
| C1-018 | LOW | S | Formatter details: `WriteTo`/`Pretty` pick colors from stdout's TTY state, `FormatPlain` omits the payload, the builder pool does nothing | errorkit/fmt.go:40-51 (TTY check on `os.Stdout`, one `Stat` syscall per call), 87-92 vs 138-185 (payload only in colored output), 251-264 (`builderPool`); errorkit/error.go:201-210 | no |  |
| C1-019 | LOW | S | ULID generator: monotonicity not guaranteed under contention, decoder not case-insensitive as documented, dead fallback RNG | errorkit/idgen.go:18-19 (doc), 44-77 (`now` read at 46 before `Lock` at 48; `time.Sleep` under the lock at 58), 143-152 (LCG fallback), 174-216 (`ParseULID` accepts upper case only), 219-257 | no |  |
| C1-021 | LOW | S | `telemetry.RecordError` marks a span as failed even when `err` is nil | telemetry/helpers.go:31-34 | no |  |
| C1-025 | LOW | S | Modernization opportunities within Go 1.26 (JetBrains guidelines) | (see findings file) | no |  |
| C2-023 | LOW | S | Imprecise error codes: cancellation becomes "unexpected / 500", and an empty LLM reply is retriable | `httprequest/httperror.go:48-55` (`context.Canceled` → `ERR_SYSTEM_UNEXPECTED`, HTTP 500); `llmclient/client.go:137-141` (no choices → `ERR_EXTERNAL_SERVICE_ERROR`, `Retriable: true`) | yes |  |
| C2-024 | LOW | S | `WithDataOpt` and `WithMetadataOpt` store `*any`, so a nil value renders as `null` despite `omitempty` | `httpserver/response.go:43-50` | yes |  |
| C2-026 | LOW | S | Go 1.26 modernization opportunities | - `httprequest/httprequest.go:310-311`, `httprequest/httperror.go:58-59`, `httpserver/response.go:35-36` (`errors.As` with a target variable) | no |  |
| C3-014 | LOW | S | `errors.As` with a declared target where Go 1.26 `errors.AsType` applies | grpc/status.go:53-54 | no |  |
| C4-007 | LOW | S | DSN normalization handles only `@` in the password; README says the password is percent-encoded | postgres/postgres.go:98-136; postgres/README.md:152-157 | no |  |
| C4-024 | LOW | S | Modernization opportunities (Go ≤ 1.26) flagged by use-modern-go | postgres/stored_procedure.go:102; postgres/postgres.go:99-109; mongo/contracts.go:58; mongo/client.go:108-130; storage/memory_storage.go:62, 75; tests: postgres/*_test.go, mongo/*_test.go, storage/*_test.go | no |  |
| C5-013 | LOW | S | `Close` errors are unchecked on error and teardown paths | messaging/amqp.go:56, 70, 85, 259, 320; messaging/messaging.go:254 | no |  |
| C5-018 | LOW | S | Modernization (Go 1.26 guidelines) | messaging/messaging.go:400-403; messaging/otel_carrier.go:30-36; | no |  |
| C6-013 | LOW | S | `RemoveMarkdownCodeBlock`: narrow fence grammar and its doc comment is attached to the wrong declaration | stringutils/normalize.go:24-43 | no |  |
| C6-014 | LOW | S | `NormalizeText` is lossy and not injective; the "non-ASCII is removed" statement has an exception | stringutils/normalize.go:10-22; README stringutils/README.md:52-55 | no |  |
| C6-015 | LOW | S | `validators`: inconsistent error codes/messages and API details (QF1001, shadowed `min`/`max`, `int`-only, no `min > max` check) | validators/validators.go:14-19, 46-55, 61-76, 96-100 | yes |  |
| C6-018 | LOW | S | `marshal` repeats the same encoder block twice, and `ioutils.WriteJSON` a third time | marshal/json.go:10-40; ioutils/printer.go:18-28 | no |  |
| C6-020 | LOW | S | `slices`: no preallocation, `min` not used, name shadows the standard library, `Chunk(size<=0)` drops all items silently | slices/chunks.go:6-18, slices/string.go:5-24, package name | yes |  |
| C6-023 | LOW | S | JetBrains `use-modern-go` results for the scope are small | `list` output was identical (48 guidelines, Go 1.26) for envs/loader.go, urlkit/url.go, validators/validators.go, marshal/json.go, slices/chunks.go, slices/string.go, stringutils/normalize.go, | no |  |

### C. Concurrency & lifecycle

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-001 | HIGH | S | `logger.New` mutates process-wide zerolog state (global level and time format) | logger/instance.go:42,59 → logger/config.go:301-313 (303-308 `zerolog.SetGlobalLevel`, 310 `zerolog.TimeFieldFormat`) | no |  |
| C2-001 | HIGH | M | A Hertz `Run()` bind failure panics the whole process; `Run()` can never return an error | `httpserver/hertz/wrapper.go:58-61`; Hertz `pkg/app/server/hertz.go:56-79,90-113`, `pkg/network/netpoll/transport.go:100` | yes |  |
| C4-012 | HIGH | M | `FileStorage` corrupts data when two instances or processes share a directory (shared fixed temp-file name, in-process lock only) | storage/file_storage.go:18-21, 37-51, 200-222; storage/README.md:58-102 | no |  |
| C5-001 | HIGH | M | Shutdown (ctx cancel) dead-letters or drops the in-flight message; the retry loop ignores ctx | messaging/messaging.go:286-336, 328-331, 371-382 | no |  |
| C1-005 | MEDIUM | M | Unsynchronized mutable globals in `errorkit` (`MaxStackDepth`, `ColorsEnabled`, `GenerateID`, exported `ErrorRegistry` map) | errorkit/config.go:15, 23-25, 44-52; errorkit/colors.go:18, 21-36; errorkit/idgen.go:35, 160-166; errorkit/registry.go:5-8; errorkit/error.go:65, 72 | yes |  |
| C1-006 | MEDIUM | S | `OverrideMetadata` runs the updater while holding the registry write lock: re-entry deadlocks the whole registry; results are not validated | errorkit/register.go:171-183 (global, updater call at 180), 187-201 (`Registry.OverrideMetadata`) | no |  |
| C1-013 | MEDIUM | S | `telemetry.InitTracer` writes the package tracer without synchronization | telemetry/trace.go:53, 115-117, 308-310 (and 139, 147 via the compatibility helpers) | no |  |
| C2-010 | MEDIUM | S | Hertz `Shutdown(ctx)` is silently capped at 5 s and returns `nil` while requests are still in flight | `httpserver/hertz/wrapper.go:65-67`; Hertz `pkg/route/engine.go:307-352` (`context.WithTimeout(ctx, opt.ExitWaitTimeout)`), default `ExitWaitTimeout = 5s` (`pkg/common/config/option.go:41`); `httpserver.ServerConfig` has no field to change it | no |  |
| C2-012 | MEDIUM | S | No end-to-end bound on the LLM path, and per-attempt budgets multiply on the default path | `httpclient/httpclient.go:91-107` (`SetupForLLM`: `Timeout: 0`, `ResponseHeaderTimeout: 4m`), `:62` (`DialTimeout: 60s`), `:263`; `llmclient/client.go:103-107` (`Timeout` is optional, and 0 means no deadline); `httprequest/httprequest.go:187-221` | yes |  |
| C3-007 | MEDIUM | S | Recovery is not outermost when an interceptor is passed through `WithRawServerOptions` | grpc/server.go:51-66, 81; grpc/options.go:75-79 | no |  |
| C4-004 | MEDIUM | S | Network constructors handle `context` inconsistently: `NewPostgresClient` cannot be cancelled (fixed 5 s budget), `NewPubSub` takes no ctx, mongo differs (merged with X1-009) | postgres/postgres.go:30, 61-63, 88; + X1-009: mongo/client.go:38-73 (`New(ctx, cfg)`); postgres/postgres.go:30, 60-65 (`NewPostgresClient(cfg)` with `context.WithTimeout(context.Background(), 5*time.Second)`); messaging/messaging.go:88, amqp.go:29,43-46 (`NewPubSub(url, ...)` dials with no ctx and no timeout option) | yes | X1-009 |
| C4-016 | MEDIUM | S | No durability: writes are never `fsync`ed and a zero-length file is read as an empty list | storage/file_storage.go:185-187, 200-222; storage/README.md:98-102 | no |  |
| C5-004 | MEDIUM | S | Handler panics are not recovered: a poison message crash-loops the service | messaging/messaging.go:312-326 (handler call at 320), 264-283 | no |  |
| C5-007 | MEDIUM | M | `Publish` ignores its ctx; the publisher mutex is held across dial and network I/O | messaging/messaging.go:143, 195-215, 371-382; messaging/amqp.go:93-136, 138-176 | no |  |
| C5-008 | MEDIUM | S | `Close` is not terminal: `Publish` after `Close` silently re-dials and leaks a connection; `PubSub.Close` does not stop subscribers | messaging/amqp.go:97-106, 142-150 (auto-reconnect on `IsClosed()`), 180-202; | yes |  |

### D. Security & supply chain

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-002 | HIGH | M | `PrettyJSON`, recommended for HTTP responses, sends stack traces, absolute paths and internal error text to clients | errorkit/fmt.go:202-245 (217 `stack_trace`, 215 `payload`, 232-236 `wrapped.message`); errorkit/error.go:173-174, 189-197; errorkit/stack.go:107-113; AGENTS.md:256-257; errorkit/README.md:205-206 | yes |  |
| C2-005 | HIGH | S | The inbound audit middleware is on by default but logs nothing, and it still copies every request body and both header sets | `httpserver/hertz/server.go:152-205`, installed at `57-59`; `httpserver/options.go:31-49` (`EnableAudit: true` by default); `httpserver/README.md` §"Body redaction" ("The inbound audit middleware passes every captured request/response body through `Audit.BodyRedactor` before logging"); AGENTS.md §2.5 | yes |  |
| C2-006 | HIGH | S | URL credentials, query-string secrets and upstream error messages bypass redaction into error payloads, the global error log, audit logs and span events | `httprequest/httperror.go:31` (`url` in network-error payload), `:30-36` (`original_error` = `url.Error` text), `:80,84` (`res.Request.URL.String()`), `:91-99` (`provider_error_message` copied from the response body without redaction), `:156` (`logger.Error(appErr)`); `httprequest/audit.go:54`; `httprequest/httprequest.go:198` (span event `http.url`) | yes |  |
| C2-017 | HIGH | S | `WithErrorOpt` and `PrettyJSON` send `err.Error()` text to HTTP clients | `httpserver/response.go:28-40` (`s := err.Error(); r.Error = &s`); `httpserver/hertz/context.go:85-96` (marshal error text placed in the response) | yes |  |
| C3-001 | HIGH | S | Panic recovery sends the panic value and method name to the remote caller | grpc/interceptors.go:43-53 | no |  |
| C3-002 | HIGH | M | `ToStatus` puts the errorkit `Reason` on the wire for every code, including `Internal` | grpc/status.go:53-62 (with errorkit/error.go:90-101) | yes |  |
| C3-005 | HIGH | M | Transport security silently defaults to plaintext on both client and server | grpc/client.go:31-36, grpc/config.go:78-80, grpc/server.go:24-84 (no credentials option), grpc/README.md:93 | yes |  |
| C4-001 | HIGH | S | DSN password leaked in plaintext through the error payload (logs and `PrettyJSON`) | postgres/postgres.go:37-42 (also 65-70, 74-79); logger/errorkit.go:43-44; errorkit/fmt.go:215-224 | no |  |
| C6-001 | HIGH | S | `urlkit.BuildURL` concatenates `base + path` and re-parses it, so `path` can change the host, query or base query | urlkit/url.go:41-56 (also README "Edge cases" urlkit/README.md:63-67) | yes |  |
| X3-001 | HIGH | S | Core packages reach 19 standard-library vulnerabilities when built with Go ≤ 1.26.9, and the SDK's own release pipeline builds with Go 1.26.1 | go.mod:3; .github/workflows/release.yml:57-65,70; httprequest/httprequest.go:245,253; messaging/amqp.go:247; postgres/postgres.go:65,75; mongo/client.go:54,62,94,102; httpserver/hertz/wrapper.go:59 | yes |  |
| C1-004 | MEDIUM | S | Log redaction only covers `password`/`email`-like keys; tokens, secrets, credentials in DSNs and free text pass through | logger/config.go:257-287 (`shouldRedactEmailField`, `shouldRedactPasswordField`, `maskEmailDomain`); logger/errorkit.go:43-50 (payload and wrapped error logged verbatim) | no |  |
| C1-010 | MEDIUM | M | OTLP log provider: plaintext transport forced, setup and shutdown errors swallowed, README promises `WithBlock` | logger/logprovider.go:23-27 (`Shutdown`), 61-91 (64 `WithInsecure()`, 66-69 error → stderr + no-op provider, 71-78 resource, 88 global registration); logger/README.md:176 | no |  |
| C1-012 | MEDIUM | S | `DetectEnv`: insecure default (Dev, Debug level), case-sensitive match, wrong doc comment, env re-read on every Dev log line, implicit read inside `New` | logger/config.go:31-39 (doc says `APP_ENV`, reads `ENVIRONMENT`), 44-62 (`os.Getenv("ENVIRONMENT")` inside `FormatExtra` at 57), logger/instance.go:44-47, logger/logger.go:45 | yes |  |
| C2-007 | MEDIUM | S | Response bodies are read without a size limit (error bodies, decoders, transparent gzip) | `httprequest/httprequest.go:253` (`io.ReadAll(res.Body)` on non-2xx), `:262-263`; `httprequest/decode.go:53,63,80,97,114,130,140` (`io.ReadAll` / unbounded `json.Decoder`); `httpclient/httpclient.go:246-252` (`DisableCompression: false`, so gzip is decompressed transparently) | yes |  |
| C2-011 | MEDIUM | S | `EnableRecovery=false` cannot disable recovery; with `true`, two recovery layers run and Hertz logs panics through its own logger | `httpserver/hertz/server.go:48,51` (`server.Default` always adds `recovery.Recovery()`; Hertz `pkg/app/server/hertz.go:48-53`), `:54-56`, `:103-109` (DN-16) | yes |  |
| C3-006 | MEDIUM | S | The server accepts an unbounded number of concurrent streams per connection by default | grpc/config.go:45-56 (`ServerConfig`), grpc/server.go:36-49 | partial |  |
| C4-013 | MEDIUM | S | Fixed, predictable temp-file name follows symlinks: an attacker with write access to `baseDir` can overwrite arbitrary files | storage/file_storage.go:208-209 | no |  |
| C5-010 | MEDIUM | M | Topology is not validated: typos are ignored, negative `maxAttempts` wraps to effectively unbounded retries, duplicate bindings are dropped silently | messaging/topology.go:69-78 (bare `json.Unmarshal`), 49-56 (`GetBindingByQueue`; + C6-005: messaging/messaging.go:375-380 → algorithms/backoff.go:28-31; messaging/types.go:30-35. Owner of the fix: P2.C5 (cross-reference); the guard belongs in `algorithms` | yes | C6-005 |
| C6-002 | MEDIUM | S | `urlkit.JoinPath` keeps `..` segments and accepts scheme-less or non-HTTP bases | urlkit/url.go:110-134 (`BuildURL` has the same lack of scheme check at 28-38) | yes |  |
| X3-004 | MEDIUM | L | The single module puts the SDK's whole requirement list into every consumer's version selection, even for stdlib-only packages | go.mod:5-68 (single module for 22 packages) | yes |  |
| C2-016 | LOW | S | LLM credential hygiene: the API key prints through `%v`, plain-HTTP endpoints are accepted, and an empty key is sent | `llmclient/client.go:61-65` (`Config.APIKey string`), `:69-77`, `:129-131` | no |  |
| C2-021 | LOW | S | The sensitive-header deny-list is incomplete and duplicated | `httprequest/audit.go:14-33`; `httpserver/hertz/server.go:207-220` | no |  |
| C4-022 | LOW | S | Data written world-readable: directory `0o755`, files `0o644` | storage/file_storage.go:26, 209 | no |  |
| C6-011 | LOW | S | `envs.NewLoader` copies the raw env value into the error reason | envs/loader.go:12-14 | no |  |
| C6-016 | LOW | S | `ioutils.SaveJSON` truncates the target before encoding and creates files with default permissions | ioutils/save.go:15-33 | no |  |
| X3-006 | LOW | S | Importing `logger` (and anything that depends on it) registers `/debug/requests` and `/debug/events` on `http.DefaultServeMux` | logger/logprovider.go:8 → `google.golang.org/grpc` → `golang.org/x/net/trace` (`trace.go:120-131` in x/net v0.62.0) | no |  |
| X3-009 | LOW | M | Low-maintenance modules in the Hertz adapter's closure: `hertz-contrib/obs-opentelemetry/tracing` (last release 2024-05) and `twitchyliquid64/golang-asm` (2021) | httpserver/hertz/server.go:13; go.mod:10, go.mod:51, go.mod:57 | yes |  |
| X3-010 | LOW | S | Module-level advisory GO-2026-5932 (`golang.org/x/crypto/openpgp`) on the required x/crypto v0.58.0; not imported, and fixed only in an untagged pseudo-version | go.mod:62 (`golang.org/x/crypto v0.58.0 // indirect`) | yes |  |
| X3-011 | LOW | S | `task deps:outdated` cannot see test-only and graph-only requirements, so it reports "up to date" while `go.mod` lists outdated modules | Taskfile.yml:138-147 (`go list -deps … ./<pkg>/...` without `-test`) | no |  |

### E. Testing

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-014 | MEDIUM | M | Tests miss the risky paths: no concurrency tests, no OTLP tests, contract test checks the wrong global, no benchmarks | logger/instance_test.go:13-28; logger/*_test.go; errorkit/*_test.go; telemetry/trace_test.go; coverage in docs/audit/coverage.out | no |  |
| C2-019 | MEDIUM | M | The retry loop, decoders, audit log and status mapping of `httprequest` are untested | `httprequest/*_test.go` (16 tests); `docs/audit/coverage.out` (package coverage 44.9%) | no |  |
| C2-020 | MEDIUM | M | The Hertz adapter is essentially untested (19.4%), and `llmclient` tracing constructors are untested | `httpserver/hertz/wrapper_test.go` (2 tests); `llmclient/*_test.go`; `docs/audit/coverage.out` | no |  |
| C3-012 | MEDIUM | M | Tests do not check the package's main promises: tracing, user interceptors inside recovery, and the client options | grpc/grpc_test.go:47-64, grpc/roundtrip_test.go, grpc/recovery_test.go, grpc/internal/testsvc/testsvc.go | partial |  |
| C4-009 | MEDIUM | M | postgres tests skip every error path and never assert that rows are closed; no integration tests | postgres/stored_procedure_test.go:17-105, 120-155; postgres/postgres_test.go; postgres/tracing_test.go | no |  |
| C4-020 | MEDIUM | S | mongo adapter layer is untested although the driver ships in-memory result constructors | mongo/adapter.go (whole file); mongo/errors_test.go:27-57; mongo/client_test.go:67-68 | no |  |
| C5-012 | MEDIUM | M | Test coverage 10.5%: the core consume/publish logic is untested, even where it needs no broker | messaging/*_test.go (16 tests: options, topology loaders, error | no |  |
| C6-008 | MEDIUM | S | `validators` is 9.7% covered; three of four validators have no test | validators/validators_test.go (one test, `ValidateDateRange("bad", ...)`) | no |  |
| C6-010 | MEDIUM | S | `envs` has one test, mutates the process environment without restoring it, and never exercises the success path | envs/loader_test.go:12-45 (`os.Unsetenv` at line 14); coverage 66.7% | no |  |
| C3-013 | LOW | S | Test hygiene: no deadlines, `context.Background()`, sleep pacing, pre-1.22 loops and unchecked `Close` | grpc/roundtrip_test.go:46, 134, 144, 173, 177, 196, 203-212, 244, 248, 265, 277, 292, 295, 313, 317, 324, 326, 382, 385, 395; grpc/recovery_test.go:26, 37, 50, 62 | no |  |
| C4-023 | LOW | S | storage tests: weak concurrency assertion, no `FileStorage` concurrency or `Clear` tests, zero-value panic untested | storage/storage_test.go:143-169, 171-184; storage/file_storage_phase11_test.go; storage/memory_storage.go:12-24 | no |  |
| C6-021 | LOW | M | No fuzz tests and no `Example*` functions in any of the eight packages | all `*_test.go` in scope (DN-36) | no |  |

### F. Performance

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-003 | MEDIUM | M | The redacting writer re-parses every log line (and every string field) as JSON: ~60× slower than zerolog | logger/config.go:312, 323-344 (`redactingWriter.Write`), 204-255 (`compactAndRedactJSON`, `redactStructuredValue`, 248-250); logger/otlp.go:47-50 (second decode on the OTLP path); logger/logger.go:14-21 | no |  |
| C1-007 | MEDIUM | S | Stack capture on every `NewError` costs ~31 µs and 67 allocations; the per-frame `runtime.Caller` loop is ~5× slower than `runtime.Callers` | errorkit/stack.go:12-44 (loop at 21-41, `runtime.Caller(i)` at 22, `runtime.FuncForPC` at 27), 47-64 (`filterStack` builds `fullName` per frame); errorkit/error.go:68-81 | no |  |
| C2-008 | MEDIUM | S | Every successful decoded response is buffered a second time, even when the audit log is off | `httprequest/httprequest.go:260-268` | no |  |
| C2-014 | MEDIUM | S | Backoff has no jitter and ignores `Retry-After` on 429 and 503 | `httprequest/httprequest.go:215-232`, `321-328`; `httprequest/httperror.go:119-125` | no |  |
| C4-017 | MEDIUM | M | `FileStorage.Append` rewrites and re-indents the whole file on every call (O(n) per append, O(n²) per log) | storage/file_storage.go:37-51, 174-222 (`MarshalIndent` at 201); storage/file_storage.go:60-69 (extra `Stat` before `ReadFile` in `List`) | yes |  |
| C5-003 | MEDIUM | S | Default prefetch is unlimited; the client buffers the whole backlog in memory | messaging/amqp.go:303 (`ch.Qos(s.queue.Prefetch, 0, false)`); messaging/types.go:135 | yes |  |
| C6-007 | MEDIUM | S | `stringutils.NormalizeText` compiles its regexp on every call | stringutils/normalize.go:18 | no |  |
| C6-009 | MEDIUM | M | A failing validation costs ~600x a passing one because every error captures a stack trace | validators/validators.go:35-54 and every `errorkit.NewError` call (errorkit/error.go:52-77, `MaxStackDepth = 32`) | yes |  |
| C4-021 | LOW | M | Mongo result API forces whole-result buffering and exposes an empty `BulkWriteResult` | mongo/contracts.go:46-58; mongo/adapter.go:28-41, 54-56 | yes |  |

### G. Public API, observability & DX

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C3-003 | HIGH | M | `ToStatus` maps every package-registered code to `codes.Internal` and ignores errorkit metadata | grpc/status.go:14-38; + X1-007: grpc/status.go:14-38 (`codeFor`), 48-66; httpserver/response.go:28-40 (`WithErrorOpt`); AGENTS.md §2.1 and §4 ("use `appErr.Metadata.HTTPStatus`") | yes | X1-007 |
| C2-015 | MEDIUM | S | `llmclient` has no `*FromEnv` constructor, although the root README and AGENTS promise one for `LLM_API_KEY` | `llmclient/client.go:61-93`; `README.md:65-67`; `AGENTS.md` §2.4 (line 130); `llmclient/README.md:11` (example uses `os.Getenv`) (DN-22) | no |  |
| C2-018 | MEDIUM | S | SDK tracing wrappers (httprequest, llmclient, mongo, postgres, messaging) record no error status and use ad-hoc, high-cardinality span names (merged with X1-003, X1-004) | `httprequest/tracing.go:57-105`; `llmclient/tracing.go:38-49` (DN-10; overlaps P2.X1); + X1-003: httprequest/tracing.go:57-61; llmclient/tracing.go:38-42; mongo/tracing.go:66-122; postgres/tracing.go:46-72; + X1-004: httprequest/tracing.go:63-105; llmclient/tracing.go:44-49; mongo/tracing.go:66-68; postgres/tracing.go:70-79; messaging/messaging.go:170-172, 290-291, 314-315; AGENTS.md:113-115 | yes | X1-003, X1-004 |
| C3-008 | MEDIUM | S | There is no client-side deadline facility, and Kitex `RPCTimeout`/`ConnectTimeout` were dropped with no replacement | grpc/client.go:19-77, grpc/config.go:76-86, grpc/README.md:136-164, 179 | no |  |
| C3-009 | MEDIUM | S | `ServerConfig.Address` is required but never used | grpc/server.go:18, 25-29, 83, 94; grpc/config.go:47-48 | partial |  |
| C3-010 | MEDIUM | S | README quick start: receive loops spin forever on errors, and `Shutdown` gets a context that never expires | grpc/README.md:128, 130, 150-163 | no |  |
| C3-011 | MEDIUM | S | No way to treat incoming trace context as untrusted (public endpoints) | grpc/server.go:68-79, grpc/options.go:45-62 | no |  |
| C3-016 | MEDIUM | S | Tracing injection takes four shapes (`*Instrumenter`, `TracerProvider`, `trace.Tracer`, global); `grpc` does not use `*Instrumenter` and AGENTS §2.2 omits it (merged with X1-002) | grpc/options.go:33-62, 99-114; AGENTS.md §2.2; + X1-002: telemetry/trace.go:56-58 (`Instrumenter{tracer}`, no accessor), 143-147 (nil receiver falls back to the global tracer); grpc/options.go:47,106 (`trace.TracerProvider`); messaging/messaging.go:48-51,125-130 (`trace.Tracer`, default `otel.Tracer("messaging")`); httpserver/hertz/server.go:45-49 (`hertztracing.NewServerTracer()` with no options); postgres/tracing.go:34-39; AGENTS.md:106, 226, 339; README.md:57-60 | yes | X1-002 |
| C4-005 | MEDIUM | S | `WrapWithInstrumenter(sp, nil)` does not disable tracing, contrary to the README | postgres/tracing.go:34-39; telemetry/trace.go:143-146; postgres/README.md:186, 221 | no |  |
| C5-011 | MEDIUM | S | `WithLogger` is ignored by the publisher; failures are logged at Info with printf-style messages | messaging/amqp.go:98, 105, 116, 143, 160 (global `logger.Info`); | no |  |
| X1-005 | MEDIUM | M | Exported interfaces that consumers are invited to implement freeze the v1 surface; `httpserver` cannot route PATCH/HEAD/OPTIONS | httpserver/contracts.go:12-67 (`HTTPContext`, 15 methods), 119-139 (`RouterGroup`), 147-177 (`HTTPServer`); mongo/contracts.go (`Collection`, 9 methods; `ClientPort`); httpserver/README.md:10; mongo/README.md:257 | yes |  |
| X1-006 | MEDIUM | M | Adapters return interfaces, hide their concrete types and offer no escape hatch (unlike `grpc`) | httpserver/hertz/server.go:23,30 and wrapper.go:13-17 (unexported `wrapperHertzServer`, no engine accessor); httpserver/hertz/request_logger.go:16; mongo/client.go:38-80 (returns `ClientPort`), 83-85 (`(*Client).Database`); httprequest/httprequest.go:142-170; llmclient/client.go:80-95; messaging/messaging.go:88; mongo/README.md:206-210; httprequest/README.md:125,135 | yes |  |
| X1-010 | MEDIUM | S | `httpserver.ServerOptions` is an options struct whose zero value silently disables tracing, recovery, audit and health | httpserver/options.go:41-65 ("The zero value is not meaningful"); httpserver/hertz/server.go:30-31, 45-69, 78-99 ("keeps NewWithOptions safe when callers construct ServerOptions partially") | no |  |
| X1-011 | MEDIUM | L | Third-party types, including a superseded major and a pre-1.0 module, are part of the v1 API | algorithms/backoff.go:28 (returns `backoff.BackOff` from `cenkalti/backoff/v4`); httpserver/hertz (`app.HandlerFunc`, `*app.RequestContext` from `cloudwego/hertz v0.10.6`, in `RecoveryHandler` and `RequestLoggerMiddleware`); logger/logger.go:44 (`*zerolog.Logger`), logger methods returning `*zerolog.Event`; postgres (`*pgxpool.Pool`, `pgx.Row(s)`, `pgx.Tx`, `pgconn.CommandTag` in `Querier`/`Execer`/`ClientPort`); messaging/amqp.go:224-230 (`amqp.Table`); mongo (`options.Lister[...]`, `mongo.WriteModel`, `bson.ObjectID`); + X3-008: algorithms/backoff.go:6,28; go.mod:7 (v4), go.mod:32 (v5, indirect via `otlploggrpc`) | yes | X3-008 |
| X1-013 | MEDIUM | M | GoDoc is the declared symbol reference but 15 of 21 importable packages have no package comment, 59 exported identifiers have none, and there are no examples | all packages, see evidence; AGENTS.md:356 ("godoc — symbol-level reference, generated from source comments"); telemetry/trace.go:28-48, 90, 219-293 and httprequest/tracing.go:19-37 (stale text) | no |  |
| X1-014 | MEDIUM | L | Package identifiers differ from their directories or collide with stdlib and upstream packages | postgres/*.go (`package database`), mongo/*.go (`package mongodb`), slices/chunks.go:1 (`package slices`, same name as stdlib `slices`), grpc/doc.go:19-24 (collides with `google.golang.org/grpc`); AGENTS.md §0 | yes |  |
| X1-015 | MEDIUM | S | Cross-cutting contracts promise APIs and guarantees the code does not provide | README.md:65-67 and AGENTS.md:129-131 (`LLM_API_KEY` … "a sibling `*FromEnv` constructor reads it explicitly"); llmclient/client.go:75-95 (no `*FromEnv`); AGENTS.md:85-86 and README.md:63-64 ("Every public function that can fail returns `*errorkit.AppError`"); httpserver/hertz/README.md "Shutdown" (Hertz error returned "verbatim"); httpserver/hertz/wrapper.go:58-67 | yes |  |
| X1-016 | MEDIUM | M | Consumer testability depends on hand-written fakes of large interfaces; the SDK ships no test doubles | httpserver/contracts.go:12-67 (15-method `HTTPContext`); mongo/contracts.go (`Collection` 9 methods + 7 result interfaces); private fakes duplicated in package tests: mongo `stubClientPort`/`stubCollection`, postgres `fakeQuerier`/`fakeRows`/`fakeStoredProcedurer`, llmclient stub requester (discovery §7); AGENTS.md §6 | yes |  |
| C1-020 | LOW | S | No helper to match an error chain by code; documented examples use a direct type assertion | errorkit/error.go (no `Is` method or code helper); AGENTS.md §4 (`err.(*errorkit.AppError)`); logger/README.md "Logging Errors With Trace Context"; project skill go-design-principles appendix | no |  |
| C1-022 | LOW | S | `telemetry` has no package documentation; exported helpers are undocumented; comments reference other projects | telemetry/trace.go:10-44 (comment block placed after the imports, so it is not a package doc), 48, 90 (`go-pkgs/...`), 35-44 (`taobao`/`signoz` examples); telemetry/helpers.go:11-26 (`WithString`, `WithInt`, `WithBool`, `WithSpanKind` without doc comments) | no |  |
| C1-023 | LOW | S | logger documentation disagrees with the code | logger/README.md:122 (claims `caller` on every line; no `.Caller()` is configured in config.go:312), 126-131 ("The logger has no `Config` struct", DN-03; `Config` exists at instance.go:14-33), 176 (`WithBlock`, see C1-010); logger/config.go:31 (doc names `APP_ENV`); logger/otlp.go:17-18 ("ships logs over the same gRPC connection already used for traces"; `InitLogProvider` creates its own exporter connection); logger/logger.go:26-31 and logprovider.go:35-60 (SigNoz/`main.go`/`provider.NewOpenTelemetryProvider` references from another codebase) | no |  |
| C2-022 | LOW | S | The built-in `/health` route makes a consumer's own `GET /health` panic at registration | `httpserver/hertz/server.go:67-69`, `222-226` | no |  |
| C2-025 | LOW | S | `RequestLoggerMiddleware` cannot be installed through the SDK interface, ignores inbound request IDs and logs client IPs | `httpserver/hertz/request_logger.go:16-45`; `httpserver/hertz/wrapper.go:13-28` (DN-17; overlaps P2.X1) | no |  |
| C2-027 | LOW | S | Package documentation is inaccurate in places that change consumer behavior | `httpclient/httpclient.go:82-90`; `httprequest/README.md:44-45,93-99,125,135,193,201-218`; `httprequest/httprequest.go:300-302`; `httpserver/README.md:19-47` (overlaps P2.X2; DN-06, DN-07, DN-08, DN-33) | no |  |
| C3-015 | LOW | S | Recovery status omits `ErrorInfo`; `ErrorInfo.Domain` is hard-coded to `"go-sdk"` | grpc/interceptors.go:53, grpc/status.go:56-59 | no |  |
| C4-006 | LOW | S | Stored-procedure spans: generic name for schema-qualified or `CALL` statements, no `db.*` attributes, failures not recorded | postgres/tracing.go:11, 46-79 | no |  |
| C4-008 | LOW | S | Package docs: misleading double-wrap example and missing godoc on exported symbols | postgres/tracing.go:24-28; postgres/contracts.go:10-24; postgres/stored_procedure.go:12-25; postgres/postgres.go:14-20; mongo/client.go:15-34; mongo/contracts.go:10-67 | no |  |
| C4-019 | LOW | S | Cursor-returning `Find` spans close before iteration; `Ping`/`Close` untraced; start-up ping ignores `ServerSelectionTimeout` | mongo/tracing.go:52-58, 82-86; mongo/client.go:67-72 | no |  |
| C5-014 | LOW | S | Connection options are hard-wired: no TLS config for `amqps://`, no dial timeout or properties; heartbeat detection is slower than documented | messaging/amqp.go:44-46, 247-249 (`amqp.Config{Heartbeat: DefaultHeartbeat}`); | no |  |
| C5-015 | LOW | S | `NewPubSub` silently falls back to reading `RABBITMQ_TOPOLOGY_FILE` | messaging/messaging.go:114-123; messaging/topology.go:101-109 | yes |  |
| C5-016 | LOW | S | Package-level documentation gaps | messaging/otel_carrier.go:1; messaging/amqp.go:138, 208-220; messaging/README.md:57, 118-126 | no |  |
| C5-017 | LOW | S | Span naming and attributes bypass `telemetry` conventions and use deprecated OTel semconv keys | messaging/messaging.go:170-181, 290-299, 314-317 | yes |  |
| C6-012 | LOW | S | `algorithms`: zero-valued config hot-loops, the hidden 15-minute `MaxElapsedTime` is undocumented, and the README misdescribes `MaxRetries = 0` | algorithms/backoff.go:29-41; README algorithms/README.md:58-59, 69-72, 131-133 (DN-26) | yes |  |
| C6-019 | LOW | M | `envs` API: `NewLoader` is not a constructor, offers no options, and the contract mentions `envs.Load[T]()`, which does not exist | envs/loader.go:10; AGENTS.md §5 (DN-32) | yes |  |
| C6-022 | LOW | S | README/AGENTS statements about these packages that the code contradicts | (see findings file) | no |  |
| X1-017 | LOW | S | Naming: stuttering identifiers, redundant constructors and exported presentation constants | httprequest (`HTTPRequest`, `HTTPRequester`, `NewHTTPRequestWithRetry`, httprequest.go:142-170); httpserver (`HTTPServer`, `HTTPContext`); llmclient (`LLMProvider`, `LLMRequest`, `LLMRequestConfig`, `LLMResponse`); postgres (`NewPostgresClient`, `NewPostgresClientFromEnv`, `PostgresEnvVar`); storage (`KVStorager`, `AppendLogStorager`); messaging (`Messager`); envs/loader.go:10 (`NewLoader(c any) error` loads instead of constructing; AGENTS.md:268 refers to a non-existent `envs.Load[T]`); errorkit/colors.go:4-18 (nine exported mutable ANSI `var`s and `ColorsEnabled`); logger/config.go:17-18 (exported `Blue`, `Reset`) | yes |  |

### H. Maintainability & DevEx

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| X2-001 | MEDIUM | M | No CI runs on push or pull request; the repo's own gates are already red and nothing notices | `.github/workflows/release.yml:15-17` (the only workflow; trigger `push: branches: [main]`); + X3-003: Taskfile.yml (no `vuln` task); .pre-commit-config.yaml; .github/workflows/ (only release.yml); SECURITY.md (no mention of govulncheck) | yes | X3-003 |
| X2-002 | MEDIUM | S | The static-analysis gate cannot run from a fresh clone: its script is gitignored and `task skills:install` installs nothing | `Taskfile.yml:15,44-52,70-79`; `.pre-commit-config.yaml:43-50`; `.gitignore` (pattern `.*/skills/*`); `.skillshare/config.yaml:1-8` | yes |  |
| X2-003 | MEDIUM | S | `staticanalysis.sh` reports "OK" (exit 0) when staticcheck or semgrep crash, or when staticcheck is missing | `.skillshare/skills/go-code-quality-check/scripts/staticanalysis.sh:275-278, 284-304, 330-359, 365-368, 399-409` (external, gitignored skill; executed by Taskfile and pre-commit) | yes |  |
| X2-004 | MEDIUM | S | Release verification (build/vet/test) runs only after the tag and GitHub Release are published; nothing gates the merge to `main` | `.github/workflows/release.yml:26-38` (job `release-please`), `:40-65` (job `assets`, `if: release_created == 'true'`, step "Verify tagged tree") | yes |  |
| X2-005 | MEDIUM | S | `task precommit:run` and `task check` do not exercise the staticanalysis hook; staged mode also skips renamed files | `.pre-commit-config.yaml:43-50`; `Taskfile.yml:63-69, 94-99`; script lines 64-83 (staged mode), 71 | yes |  |
| X2-007 | MEDIUM | S | SECURITY.md (and README/AGENTS/llms.txt references to it) describe a script location and CI that do not exist | `SECURITY.md:5-6, 14, 102, 110, 121, 127-141, 154, 170-171`; `README.md:102-103, 132-134`; `AGENTS.md:315-316`; `llms.txt:11` (SECURITY entry) | no |  |
| X2-008 | MEDIUM | S | SECURITY.md has no vulnerability reporting or supported-versions policy | `SECURITY.md` (whole file) | no |  |
| X2-009 | MEDIUM | S | No LICENSE file; AGENTS.md points to the README for the licence and the README has none | repo root (`git ls-files \| grep -i licen` → nothing); `AGENTS.md:14`; `README.md` | yes |  |
| X2-010 | MEDIUM | M | Root and per-package documentation states things the code does not do (items owned by X2 in the discovery notes, re-verified) | see table | yes |  |
| X2-011 | MEDIUM | S | Go toolchain pins disagree and nothing enforces them: `.prototools` 1.27.3 vs `go.mod` 1.26.1 vs CI 1.26.1 vs PATH 1.26.2 | `.prototools:1`; `go.mod:3`; `.github/workflows/release.yml:57-59`; + X3-002: go.mod:3; .prototools:1; .github/workflows/release.yml:57-59 | yes | X3-002 |
| X2-012 | MEDIUM | M | Gate results are not reproducible: tools and rule packs float (`@latest`, Docker `latest`, remote packs, plugins on `main`) | `Taskfile.yml:39` (`goimports@latest`), `:42` (`pre-commit` unpinned; `semgrep==1.95.0`, `git-cliff==2.14.2` pinned), `:120` (`gorelease@latest`); `.pre-commit-config.yaml:30-34` (system `goimports`); runner script `:333-337` (`semgrep/semgrep` image without tag, `p/golang`, `p/gosec`, `p/secrets` fetched at run time); `.prototools:15-18` (plugins from `raw.githubusercontent.com/trypanic/proto-plugins/main/…`) | yes |  |
| X2-006 | LOW | S | Semgrep scans no `_test.go` file in full mode but does scan them in staged mode; `audit.md` §1 "does not reproduce" needs a qualifier | `.skillshare/skills/.../staticanalysis.sh:64-68, 82, 333-357`; no `.semgrepignore` | yes |  |
| X2-013 | LOW | S | Pinned GitHub Actions have no automated updater; no `.github/dependabot.yml` | `.github/` (no `dependabot.yml`); `release.yml` (all `uses:` pinned by SHA) | yes |  |
| X2-014 | LOW | S | Dead or broken tooling configuration: sentrux never runs, `.env.shared` missing, "(was moon …)" leftovers | `.prototools:5,9-11`; `.sentrux/rules.toml`; `Taskfile.yml:45,56,64,72`; `.gitignore` | yes |  |
| X2-015 | LOW | S | `.moon/` is an untracked, non-ignored local cache; it blocks `task release:check` and dirties `git status` | `.moon/cache/` (untracked); `Taskfile.yml:116-117`; `.gitignore` | no |  |
| X2-016 | LOW | S | Local gates omit race, vulnerability, lint and coverage; `task test` differs from what the hook and CI run | `Taskfile.yml:81-92, 94-99, 168-181`; `.pre-commit-config.yaml:37-41` | yes |  |
| X2-017 | LOW | S | `CHANGELOG.md` is empty while README, AGENTS.md and llms.txt call it "the full per-phase change log" | `CHANGELOG.md` (0 bytes); `README.md:165`; `AGENTS.md:357`; `llms.txt:11` | no |  |
| X2-018 | LOW | M | Release assets lack provenance/signature and the pipeline does not check `go mod tidy`/`verify` | `.github/workflows/release.yml:67-80` | yes |  |
| X2-019 | LOW | S | README has no consumer section (install, minimum Go, usage example, pkg.go.dev link, compatibility, contributing) | `README.md` (package map, status, developer setup, releases only) | no |  |

## 7. Deduplication and severity adjustments

### 7.1 Duplicates merged (first ID kept)

| Duplicate | Kept as | Duplicate's raw severity | Duplicate's Needs approval |
| --- | --- | --- | --- |
| C6-005 | C5-010 | MEDIUM | no |
| X1-002 | C3-016 | MEDIUM | yes |
| X1-003 | C2-018 | MEDIUM | no |
| X1-004 | C2-018 | MEDIUM | yes |
| X1-007 | C3-003 | MEDIUM | yes |
| X1-009 | C4-004 | MEDIUM | no |
| X3-002 | X2-011 | MEDIUM | yes |
| X3-003 | X2-001 | MEDIUM | yes |
| X3-005 | X1-001 | MEDIUM | yes |
| X3-007 | C1-024 | MEDIUM | no |
| X3-008 | X1-011 | LOW | yes |

Related but **not** merged (different defect or package-specific angle,
cross-referenced in the plan): C1-007 ↔ C6-009 (stack-capture algorithm vs.
attacker-driven amplification needing a stack-less constructor);
C1-002 ↔ C2-017 ↔ C3-002 (errorkit serializer vs. HTTP and gRPC transport
replies); C2-009 ↔ X1-008 ↔ C5-011 (log-and-return /
logger injection); C2-015 ↔ X1-015 (X1-015 also covers the "always
`AppError`" guarantee); C1-004 ↔ C2-021 ↔ C4-001 (different redaction
mechanisms); C2-014 ↔ X1-012 (jitter vs. three retry implementations);
X1-013 ↔ C1-022 ↔ C5-016 ↔ C4-008 (umbrella vs. per-package godoc);
C2-011 ↔ X1-010; C2-012 ↔ C3-008; C4-006 ↔ C5-017 ↔ C4-019 (package span
details vs. C2-018); X2-007 ↔ X2-010 ↔ C6-022 ↔ C2-027 ↔ C1-023 (docs).

### 7.2 Severity adjustments

| ID | Raw severity | Consolidated | Reason |
| --- | --- | --- | --- |
| C1-024 | LOW | MEDIUM | absorbs X3-007: the deprecation makes the static-analysis gate fail on an untouched tree |
| C3-016 | LOW | MEDIUM | absorbs X1-002: four tracing-injection shapes across the SDK, not only grpc |
| C2-006 | MEDIUM | HIGH | demonstrated credential leak (URL userinfo, API-key error text) into logs and payloads on a default path; consistent with C4-001 |
| C2-017 | MEDIUM | HIGH | HTTP counterpart of C3-002 (internal error text sent to clients by default); kept separate from C1-002 |
| X2-001 | HIGH | MEDIUM | tooling/CI gap: a maintainability problem by nature (§5 scale), not a security or correctness defect in the SDK; ranked first among MEDIUM |
| X2-002 | HIGH | MEDIUM | same as X2-001 |
| X2-003 | HIGH | MEDIUM | same as X2-001 |

No other severity was changed. The three X2 items remain the top-ranked
MEDIUM findings because they keep every other gate from being enforced.

Criteria applied for consistency within a defect class:

- **Concurrency:** HIGH when the race/deadlock is triggered through a
  default or documented SDK path (C1-001: two `logger.New` calls; C4-012:
  two `FileStorage` instances on one directory; C5-001: ordinary shutdown).
  MEDIUM when it requires a consumer to mutate exported globals or call a
  compatibility shim concurrently at runtime (C1-005, C1-013) or to re-enter
  the registry from its own updater callback (C1-006). The reproductions are
  real; the trigger is non-default usage.
- **Error classification:** HIGH for C4-002 because routine, deterministic
  outcomes on the hot path (`pgx.ErrNoRows`, unique violations,
  cancellation) become retriable 503s for every caller. MEDIUM for C4-010,
  C4-018 and C2-013, whose misclassified cases are rarer (decode failures,
  programmer errors, redirect limits, invalid URLs).
- **Disclosure:** HIGH whenever internal error text, panic values, stack
  traces or credentials reach a client or a log on a default path
  (C1-002, C2-006, C2-017, C3-001, C3-002, C4-001).

### 7.3 State changes since the audit ran

- X2-015 (`.moon/` local cache): the directory was deleted from the
  working tree on 2026-11-04 at the user's request; `origin/main` no longer
  tracks moon files. Only the `.gitignore` recommendation remains.
- `audit.md` §1 corrected after P1 and X2-006 (semgrep test-file scanning
  depends on the runner mode).

## 8. Open questions (consolidated, no proof yet)

Each findings file ends with its own list; the cross-cutting ones are:

1. Whether external consumers implement `httpserver`/`mongo` interfaces —
   decides the cost of X1-005/X1-016 (X1).
2. Whether any consumer relies on `Retriable` for retries — sizes the
   impact of C4-002/C4-018/C2-013 (C4, C2).
3. Repository settings not visible from the tree: branch protection,
   Dependabot, private vulnerability reporting (X2, X3).
4. Behavior against real infrastructure (RabbitMQ channel closes, Hertz
   slow-client limits, OTLP flush on `logger.Panic`) — needs integration
   tests (C5, C2, C1).
5. Whether `actions/setup-go` honors a `toolchain` line (X3-002 → X2-011).
6. Which semgrep path produced the baseline "0 findings"; the local semgrep
   CLI now crashes under Python 3.15 (protobuf TypeError) (X2, C4).

## 9. Sources used

| Source | Revision | Used for |
| --- | --- | --- |
| Project contracts: `AGENTS.md`, `README.md`, 21 package READMEs, `SECURITY.md`, `llms.txt` | `audit/2026-11` | precedence 1 |
| Project skills `.skillshare/skills/{go-code-quality-check,go-design-principles,go-testing-strategy}` | installed copy | precedence 2 |
| eduardo-sl/go-agent-skills | `e25b517bbd0c11db35f75a83a14f18848c1abdb0` | all areas (per §2.3 map; each auditor lists the SKILL.md and references read) |
| JetBrains/go-modern-guidelines `use-modern-go` CLI v0.1.1 | `155dc7ca10da5e1f6c841503086957b1b37f5815` | area B, Go 1.26 guidelines only |
| Tools | go 1.26.2 / 1.27.3, staticcheck 2026.1/2026.2.2, semgrep 1.95.0, govulncheck v1.9.0, golangci-lint v2.15.0, gorelease @latest | baseline and evidence |
| Library source in module cache | grpc-go v1.85.0, otelgrpc v0.72.0, Hertz v0.10.6, amqp091-go v1.16.0, backoff v4.3.0, pgx | behavior confirmation |

Known process deviations: P2.X2 caused pre-commit to fetch its remote hook
repositories in a scratch clone, and gorelease reruns need the default Go
module proxy; both are network access the work order forbids for tests.

## 10. Residual risks

- **Untested infrastructure paths.** Connection, reconnect and broker/DB
  failure paths of postgres, mongo and messaging have no tests, so several
  findings (C5-005, C5-014, C4-016) are argued from library source, not
  reproduced.
- **Contract-bound fixes.** Many security fixes (C1-002, C3-002, C6-001,
  C2-005) change documented behavior; until G1 decides, consumers following
  the current docs keep the exposure.
- **Toolchain-dependent exposure.** Consumers building with an outdated Go
  1.26.x stay exposed to X3-001 regardless of SDK changes.
- **Gate blind spots.** Until X2-001..003 are fixed, regressions in vet,
  staticcheck, semgrep, race or vulnerabilities can land unnoticed.
- **Single-module weight.** Without a module split (approval, likely v2),
  every consumer keeps inheriting the full dependency graph (X3-004).
