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

The audit found **no CRITICAL** issues and **19 HIGH** issues out of **168
unique findings** (181 raw, 13 merged duplicates). The HIGH issues cluster
in four themes:

1. **Information disclosure to clients and logs** — panic values, error
   reasons and stack traces cross the gRPC/HTTP boundary (C1-002, C3-001,
   C3-002); the Postgres DSN password lands in logs (C4-001).
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
| C1-002 | D | `PrettyJSON`, recommended by AGENTS §4 for HTTP responses, exposes stack traces, paths, payload and wrapped error text (absorbs C2-017: `httpserver.WithErrorOpt` sends `err.Error()`) | `errorkit/error.go:38,192`; `errorkit/fmt.go:202-245`; `httpserver/response.go:28-40` | yes |
| C2-001 | C | Hertz `Run()` calls `Spin()` and returns `nil`; a bind failure panics the process | `httpserver/hertz/wrapper.go:58-61` | yes |
| C2-002 | B | Retry loop ignores method idempotency; POST re-sent 5× | `httprequest/httprequest.go:187-223` | yes |
| C2-003 | B | `MaxRetries < 0` skips the loop and returns `nil` (no request sent) | `httprequest/httprequest.go:117-128,187,231` | no |
| C2-004 | B | `out.(*string)` / bool assertions panic for named types and typed-nil pointers | `httprequest/decode.go:15-57` | no |
| C2-005 | D | Inbound audit middleware is on by default, logs nothing (logging commented out), still copies bodies and headers | `httpserver/hertz/server.go:152-205` | yes |
| C3-001 | D | Recovery puts the panic value and method name in the client-visible status | `grpc/interceptors.go:43-53` | no |
| C3-002 | D | `ToStatus` sends `appErr.Error()` (Reason) for every code, including `Internal` | `grpc/status.go:53-62` | yes |
| C3-003 | G | `codeFor` maps every package-registered code to `codes.Internal`, ignoring metadata (absorbs X1-007, same defect in `httpserver`) | `grpc/status.go:14-38` | no (X1-007 part: yes) |
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
and the highest severity.

Unique findings after deduplication: **168** (from 181 raw; 13 duplicates merged). CRITICAL 0 · HIGH 19 · MEDIUM 87 · LOW 62.

| Area | CRITICAL | HIGH | MEDIUM | LOW | Total |
| --- | --- | --- | --- | --- | --- |
| A. Architecture & design | 0 | 1 | 2 | 2 | 5 |
| B. Idiomatic & modern Go / correctness | 0 | 5 | 19 | 21 | 45 |
| C. Concurrency & lifecycle | 0 | 4 | 11 | 0 | 15 |
| D. Security & supply chain | 0 | 8 | 11 | 9 | 28 |
| E. Testing | 0 | 0 | 9 | 3 | 12 |
| F. Performance | 0 | 0 | 7 | 1 | 8 |
| G. Public API, observability & DX | 0 | 1 | 17 | 18 | 36 |
| H. Maintainability & DevEx | 0 | 0 | 11 | 8 | 19 |
| **Total** | 0 | 19 | 87 | 62 | 168 |

### A. Architecture & design

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| X1-001 | HIGH | M | `logger` mixes the zerolog API with the OTLP gRPC exporter, so every package that logs pulls gRPC and OTLP int | logger/logprovider.go:8,62 (imports and calls `otlploggrpc`); httprequ | yes | X3-005 |
| X1-008 | MEDIUM | M | Logger injection is inconsistent; library code logs returned errors through the global logger | httprequest/httperror.go:156 (`logger.Error(appErr)` on every non-2xx) | no |  |
| X1-012 | MEDIUM | M | Retry/backoff is implemented three times with divergent semantics, and `httprequest` does not use the SDK's ow | httprequest/httprequest.go:40-44 (`RetryConfig{MaxRetries int, Initial | no |  |
| C6-017 | LOW | S | `ioutils` "dev-only" is enforced by convention only; its tests are weak | ioutils/ioutils_test.go:38-46 (`SaveJSON` test never reads the file ba | no |  |
| X1-018 | LOW | S | Generic "external service" error codes are owned by `httpclient`, coupling `llmclient` and `httprequest` to th | httpclient/errors_codes.go (`ERR_EXTERNAL_SERVICE_UNAVAILABLE/TIMEOUT/ | no |  |

### B. Idiomatic & modern Go / correctness

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C2-002 | HIGH | M | The retry loop ignores HTTP method idempotency: non-idempotent POSTs, including billable LLM calls, are re-sen | `httprequest/httprequest.go:181-223` (decision at 211), `309-317`; `ht | yes |  |
| C2-003 | HIGH | S | A negative `MaxRetries` makes `Do` return `nil` without sending any request | `httprequest/httprequest.go:117-128` (only `== 0` is normalized), `187 | no |  |
| C2-004 | HIGH | S | Response decoding panics for named string or bool types and for typed-nil `out` pointers | `httprequest/decode.go:15-49` (dispatch on `Kind()`), `57` (`out.(*str | no |  |
| C4-002 | HIGH | M | Every PostgreSQL failure is `ERR_DB_POSTGRES_ERROR` (retriable, HTTP 503), including not-found, constraint vio | postgres/stored_procedure.go:32-48, 57-70; postgres/postgres.go:37-80; | yes |  |
| C5-002 | HIGH | M | `Publish` reports success for messages the broker drops (no publisher confirms, `mandatory=false`, queues decl | messaging/amqp.go:108, 123, 159, 167 (`channel.Publish(…, false, false | yes |  |
| C1-008 | MEDIUM | S | `logger.Error` / `Logger.Error` use a type assertion: an `*AppError` wrapped with `%w` is logged as a new gene | logger/wrapper.go:27-43 (assertion 33, re-wrap 39); logger/instance.go | no |  |
| C1-009 | MEDIUM | S | `WithErrorKit` (and every `*Ctx` / `*WithTrace` logging helper) panics on a nil `*AppError` | logger/errorkit.go:21-31 (`appErr.ID` at 25); callers logger/wrapper.g | no |  |
| C1-011 | MEDIUM | S | OTLP records get second-precision timestamps and lose large integer precision | logger/config.go:310 (`TimeFieldFormat = TimeFormatUnix`); logger/otlp | no |  |
| C1-024 | MEDIUM | S | `logger` uses the deprecated `go.opentelemetry.io/otel/log/global` package (pre-existing, baseline V7) | logger/logprovider.go:9, 88; logger/otlp.go:10, 41 | no | X3-007 |
| C2-009 | MEDIUM | S | `httprequest` logs every non-2xx response through the global logger and also returns the error (log-and-return | `httprequest/httperror.go:156` (DN-09); its return value is also unche | yes |  |
| C2-013 | MEDIUM | S | Non-transient failures are classified as retriable network errors (redirect-limit violations, invalid URL or s | `httprequest/httperror.go:68-74` (every non-timeout transport error be | yes |  |
| C3-004 | MEDIUM | S | `ToStatus` turns context cancellation and deadline errors into `codes.Unknown` | grpc/status.go:65 | yes |  |
| C4-003 | MEDIUM | S | Pool-construction errors carry the cause as `Payload`, not `Wrapped`: no `errors.Is/As`, and connect/ping fail | postgres/postgres.go:39-42, 67-70, 76-79; errorkit/fmt.go:236-240 | no |  |
| C4-010 | MEDIUM | S | Mongo error classification contradicts its README and marks cancellations and config errors as retriable | mongo/adapter.go:32-37, 47-52; mongo/errors.go:42-72; mongo/client.go: | no |  |
| C4-011 | MEDIUM | S | Defaults silently override URI options and make `MaxPoolSize < 10` fail at start-up | mongo/client.go:51-60, 108-131; mongo/README.md:96-107 | yes |  |
| C4-014 | MEDIUM | S | `Clear()` deletes every `*.json` in `baseDir`, including files the store did not create | storage/file_storage.go:91-116 (vs 131-139); storage/README.md:109 | yes |  |
| C4-015 | MEDIUM | S | `FileStorage.Append(key, nil)` panics; `MemoryStorage` returns an error for the same input | storage/file_storage.go:37-46 (`*item` at 46); storage/memory_storage. | no |  |
| C4-018 | MEDIUM | S | All storage failures are `ERR_STORAGE_ERROR` (retriable, 503), including programmer errors and the undocumente | storage/errors_codes.go:64-72; storage/memory_storage.go:28-32, 54-58; | yes |  |
| C5-005 | MEDIUM | S | Ack/Nack errors are discarded; at-least-once redelivery is invisible and undocumented | messaging/messaging.go:331 (`_ = msg.Nack(false, false)`), 335 (`_ = m | no |  |
| C5-006 | MEDIUM | S | SDK errors are re-encoded: `Subscribe` turns a retriable `ERR_MQ_CONNECTION_FAILED` into a non-retriable `ERR_ | - messaging/messaging.go:249-252 and 258-261: `NewSubscriber` / `Consu | no |  |
| C5-009 | MEDIUM | S | Retry of a multi-routing-key publish re-sends keys that already succeeded; nested retries multiply attempts | messaging/messaging.go:195-215; messaging/amqp.go:159-173 | no |  |
| C6-003 | MEDIUM | S | `stringutils.RemoveMarkdownCodeBlock` mangles content that has several fenced blocks and breaks its documented | stringutils/normalize.go:30-32 (lazy `(.*?)` with `(?s)` and `$` ancho | no |  |
| C6-004 | MEDIUM | S | `algorithms.NewExponentialBackoff` returns a `BackOff` that has not been `Reset`, so its first delay ignores ` | algorithms/backoff.go:33-41 | no |  |
| C6-006 | MEDIUM | S | `slices.Chunk` returns sub-slices with spare capacity, so `append` on one chunk overwrites the next one | slices/chunks.go:16 (README slices/README.md:68-69 documents shared el | no |  |
| C1-015 | LOW | S | Stack-depth budget is spent on errorkit's own frames; file paths keep a hard-coded root from another repositor | errorkit/stack.go:10 (`skipNewErrorFrames = 2`), 12-15, 47-64, 107-113 | no |  |
| C1-016 | LOW | S | Registry batch operations commit partially on error; `NewRegistry` skips validation; listings are in random or | errorkit/register.go:52-69, 96-117, 143-159 (`Merge`); errorkit/regist | no |  |
| C1-017 | LOW | S | `JSONFormatter` fallback builds JSON with `Sprintf` and can emit invalid JSON | errorkit/fmt.go:238-242 | no |  |
| C1-018 | LOW | S | Formatter details: `WriteTo`/`Pretty` pick colors from stdout's TTY state, `FormatPlain` omits the payload, th | errorkit/fmt.go:40-51 (TTY check on `os.Stdout`, one `Stat` syscall pe | no |  |
| C1-019 | LOW | S | ULID generator: monotonicity not guaranteed under contention, decoder not case-insensitive as documented, dead | errorkit/idgen.go:18-19 (doc), 44-77 (`now` read at 46 before `Lock` a | no |  |
| C1-021 | LOW | S | `telemetry.RecordError` marks a span as failed even when `err` is nil | telemetry/helpers.go:31-34 | no |  |
| C1-025 | LOW | S | Modernization opportunities within Go 1.26 (JetBrains guidelines) |  | no |  |
| C2-023 | LOW | S | Imprecise error codes: cancellation becomes "unexpected / 500", and an empty LLM reply is retriable | `httprequest/httperror.go:48-55` (`context.Canceled` → `ERR_SYSTEM_UNE | yes |  |
| C2-024 | LOW | S | `WithDataOpt` and `WithMetadataOpt` store `*any`, so a nil value renders as `null` despite `omitempty` | `httpserver/response.go:43-50` | yes |  |
| C2-026 | LOW | S | Go 1.26 modernization opportunities | - `httprequest/httprequest.go:310-311`, `httprequest/httperror.go:58-5 | no |  |
| C3-014 | LOW | S | `errors.As` with a declared target where Go 1.26 `errors.AsType` applies | grpc/status.go:53-54 | no |  |
| C4-007 | LOW | S | DSN normalization handles only `@` in the password; README says the password is percent-encoded | postgres/postgres.go:98-136; postgres/README.md:152-157 | no |  |
| C4-024 | LOW | S | Modernization opportunities (Go ≤ 1.26) flagged by use-modern-go | postgres/stored_procedure.go:102; postgres/postgres.go:99-109; mongo/c | no |  |
| C5-013 | LOW | S | `Close` errors are unchecked on error and teardown paths | messaging/amqp.go:56, 70, 85, 259, 320; messaging/messaging.go:254 | no |  |
| C5-018 | LOW | S | Modernization (Go 1.26 guidelines) | messaging/messaging.go:400-403; messaging/otel_carrier.go:30-36; | no |  |
| C6-013 | LOW | S | `RemoveMarkdownCodeBlock`: narrow fence grammar and its doc comment is attached to the wrong declaration | stringutils/normalize.go:24-43 | no |  |
| C6-014 | LOW | S | `NormalizeText` is lossy and not injective; the "non-ASCII is removed" statement has an exception | stringutils/normalize.go:10-22; README stringutils/README.md:52-55 | no |  |
| C6-015 | LOW | S | `validators`: inconsistent error codes/messages and API details (QF1001, shadowed `min`/`max`, `int`-only, no  | validators/validators.go:14-19, 46-55, 61-76, 96-100 | yes |  |
| C6-018 | LOW | S | `marshal` repeats the same encoder block twice, and `ioutils.WriteJSON` a third time | marshal/json.go:10-40; ioutils/printer.go:18-28 | no |  |
| C6-020 | LOW | S | `slices`: no preallocation, `min` not used, name shadows the standard library, `Chunk(size<=0)` drops all item | slices/chunks.go:6-18, slices/string.go:5-24, package name | yes |  |
| C6-023 | LOW | S | JetBrains `use-modern-go` results for the scope are small | `list` output was identical (48 guidelines, Go 1.26) for envs/loader.g | no |  |

### C. Concurrency & lifecycle

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-001 | HIGH | S | `logger.New` mutates process-wide zerolog state (global level and time format) | logger/instance.go:42,59 → logger/config.go:301-313 (303-308 `zerolog. | no |  |
| C2-001 | HIGH | M | A Hertz `Run()` bind failure panics the whole process; `Run()` can never return an error | `httpserver/hertz/wrapper.go:58-61`; Hertz `pkg/app/server/hertz.go:56 | yes |  |
| C4-012 | HIGH | M | `FileStorage` corrupts data when two instances or processes share a directory (shared fixed temp-file name, in | storage/file_storage.go:18-21, 37-51, 200-222; storage/README.md:58-10 | no |  |
| C5-001 | HIGH | M | Shutdown (ctx cancel) dead-letters or drops the in-flight message; the retry loop ignores ctx | messaging/messaging.go:286-336, 328-331, 371-382 | no |  |
| C1-005 | MEDIUM | M | Unsynchronized mutable globals in `errorkit` (`MaxStackDepth`, `ColorsEnabled`, `GenerateID`, exported `ErrorR | errorkit/config.go:15, 23-25, 44-52; errorkit/colors.go:18, 21-36; err | no |  |
| C1-006 | MEDIUM | S | `OverrideMetadata` runs the updater while holding the registry write lock: re-entry deadlocks the whole regist | errorkit/register.go:171-183 (global, updater call at 180), 187-201 (` | no |  |
| C1-013 | MEDIUM | S | `telemetry.InitTracer` writes the package tracer without synchronization | telemetry/trace.go:53, 115-117, 308-310 (and 139, 147 via the compatib | no |  |
| C2-010 | MEDIUM | S | Hertz `Shutdown(ctx)` is silently capped at 5 s and returns `nil` while requests are still in flight | `httpserver/hertz/wrapper.go:65-67`; Hertz `pkg/route/engine.go:307-35 | no |  |
| C2-012 | MEDIUM | S | No end-to-end bound on the LLM path, and per-attempt budgets multiply on the default path | `httpclient/httpclient.go:91-107` (`SetupForLLM`: `Timeout: 0`, `Respo | yes |  |
| C3-007 | MEDIUM | S | Recovery is not outermost when an interceptor is passed through `WithRawServerOptions` | grpc/server.go:51-66, 81; grpc/options.go:75-79 | no |  |
| C4-004 | MEDIUM | S | `NewPostgresClient` cannot be cancelled: no `context.Context`, fixed 5 s `context.Background()` budget | postgres/postgres.go:30, 61-63, 88 | yes | X1-009 |
| C4-016 | MEDIUM | S | No durability: writes are never `fsync`ed and a zero-length file is read as an empty list | storage/file_storage.go:185-187, 200-222; storage/README.md:98-102 | no |  |
| C5-004 | MEDIUM | S | Handler panics are not recovered: a poison message crash-loops the service | messaging/messaging.go:312-326 (handler call at 320), 264-283 | no |  |
| C5-007 | MEDIUM | M | `Publish` ignores its ctx; the publisher mutex is held across dial and network I/O | messaging/messaging.go:143, 195-215, 371-382; messaging/amqp.go:93-136 | no |  |
| C5-008 | MEDIUM | S | `Close` is not terminal: `Publish` after `Close` silently re-dials and leaks a connection; `PubSub.Close` does | messaging/amqp.go:97-106, 142-150 (auto-reconnect on `IsClosed()`), 18 | yes |  |

### D. Security & supply chain

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-002 | HIGH | M | `PrettyJSON`, recommended for HTTP responses, sends stack traces, absolute paths and internal error text to cl | errorkit/fmt.go:202-245 (217 `stack_trace`, 215 `payload`, 232-236 `wr | yes | C2-017 |
| C2-005 | HIGH | S | The inbound audit middleware is on by default but logs nothing, and it still copies every request body and bot | `httpserver/hertz/server.go:152-205`, installed at `57-59`; `httpserve | yes |  |
| C3-001 | HIGH | S | Panic recovery sends the panic value and method name to the remote caller | grpc/interceptors.go:43-53 | no |  |
| C3-002 | HIGH | M | `ToStatus` puts the errorkit `Reason` on the wire for every code, including `Internal` | grpc/status.go:53-62 (with errorkit/error.go:90-101) | yes |  |
| C3-005 | HIGH | M | Transport security silently defaults to plaintext on both client and server | grpc/client.go:31-36, grpc/config.go:78-80, grpc/server.go:24-84 (no c | yes |  |
| C4-001 | HIGH | S | DSN password leaked in plaintext through the error payload (logs and `PrettyJSON`) | postgres/postgres.go:37-42 (also 65-70, 74-79); logger/errorkit.go:43- | no |  |
| C6-001 | HIGH | S | `urlkit.BuildURL` concatenates `base + path` and re-parses it, so `path` can change the host, query or base qu | urlkit/url.go:41-56 (also README "Edge cases" urlkit/README.md:63-67) | yes |  |
| X3-001 | HIGH | S | Core packages reach 19 standard-library vulnerabilities when built with Go ≤ 1.26.9, and the SDK's own release | go.mod:3; .github/workflows/release.yml:57-65,70; httprequest/httprequ | yes |  |
| C1-004 | MEDIUM | S | Log redaction only covers `password`/`email`-like keys; tokens, secrets, credentials in DSNs and free text pas | logger/config.go:257-287 (`shouldRedactEmailField`, `shouldRedactPassw | no |  |
| C1-010 | MEDIUM | M | OTLP log provider: plaintext transport forced, setup and shutdown errors swallowed, README promises `WithBlock | logger/logprovider.go:23-27 (`Shutdown`), 61-91 (64 `WithInsecure()`,  | no |  |
| C1-012 | MEDIUM | S | `DetectEnv`: insecure default (Dev, Debug level), case-sensitive match, wrong doc comment, env re-read on ever | logger/config.go:31-39 (doc says `APP_ENV`, reads `ENVIRONMENT`), 44-6 | no |  |
| C2-006 | MEDIUM | S | URL credentials, query-string secrets and upstream error messages bypass redaction into error payloads, the gl | `httprequest/httperror.go:31` (`url` in network-error payload), `:30-3 | yes |  |
| C2-007 | MEDIUM | S | Response bodies are read without a size limit (error bodies, decoders, transparent gzip) | `httprequest/httprequest.go:253` (`io.ReadAll(res.Body)` on non-2xx),  | yes |  |
| C2-011 | MEDIUM | S | `EnableRecovery=false` cannot disable recovery; with `true`, two recovery layers run and Hertz logs panics thr | `httpserver/hertz/server.go:48,51` (`server.Default` always adds `reco | yes |  |
| C3-006 | MEDIUM | S | The server accepts an unbounded number of concurrent streams per connection by default | grpc/config.go:45-56 (`ServerConfig`), grpc/server.go:36-49 | no |  |
| C4-013 | MEDIUM | S | Fixed, predictable temp-file name follows symlinks: an attacker with write access to `baseDir` can overwrite a | storage/file_storage.go:208-209 | no |  |
| C5-010 | MEDIUM | M | Topology is not validated: typos are ignored, negative `maxAttempts` wraps to effectively unbounded retries, d | messaging/topology.go:69-78 (bare `json.Unmarshal`), 49-56 (`GetBindin | no | C6-005 |
| C6-002 | MEDIUM | S | `urlkit.JoinPath` keeps `..` segments and accepts scheme-less or non-HTTP bases | urlkit/url.go:110-134 (`BuildURL` has the same lack of scheme check at | yes |  |
| X3-004 | MEDIUM | L | The single module puts the SDK's whole requirement list into every consumer's version selection, even for stdl | go.mod:5-68 (single module for 22 packages) | yes |  |
| C2-016 | LOW | S | LLM credential hygiene: the API key prints through `%v`, plain-HTTP endpoints are accepted, and an empty key i | `llmclient/client.go:61-65` (`Config.APIKey string`), `:69-77`, `:129- | no |  |
| C2-021 | LOW | S | The sensitive-header deny-list is incomplete and duplicated | `httprequest/audit.go:14-33`; `httpserver/hertz/server.go:207-220` | no |  |
| C4-022 | LOW | S | Data written world-readable: directory `0o755`, files `0o644` | storage/file_storage.go:26, 209 | no |  |
| C6-011 | LOW | S | `envs.NewLoader` copies the raw env value into the error reason | envs/loader.go:12-14 | no |  |
| C6-016 | LOW | S | `ioutils.SaveJSON` truncates the target before encoding and creates files with default permissions | ioutils/save.go:15-33 | no |  |
| X3-006 | LOW | S | Importing `logger` (and anything that depends on it) registers `/debug/requests` and `/debug/events` on `http. | logger/logprovider.go:8 → `google.golang.org/grpc` → `golang.org/x/net | no |  |
| X3-009 | LOW | M | Low-maintenance modules in the Hertz adapter's closure: `hertz-contrib/obs-opentelemetry/tracing` (last releas | httpserver/hertz/server.go:13; go.mod:10, go.mod:51, go.mod:57 | yes |  |
| X3-010 | LOW | S | Module-level advisory GO-2026-5932 (`golang.org/x/crypto/openpgp`) on the required x/crypto v0.58.0; not impor | go.mod:62 (`golang.org/x/crypto v0.58.0 // indirect`) | yes |  |
| X3-011 | LOW | S | `task deps:outdated` cannot see test-only and graph-only requirements, so it reports "up to date" while `go.mo | Taskfile.yml:138-147 (`go list -deps … ./<pkg>/...` without `-test`) | no |  |

### E. Testing

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-014 | MEDIUM | M | Tests miss the risky paths: no concurrency tests, no OTLP tests, contract test checks the wrong global, no ben | logger/instance_test.go:13-28; logger/*_test.go; errorkit/*_test.go; t | no |  |
| C2-019 | MEDIUM | M | The retry loop, decoders, audit log and status mapping of `httprequest` are untested | `httprequest/*_test.go` (16 tests); `docs/audit/coverage.out` (package | no |  |
| C2-020 | MEDIUM | M | The Hertz adapter is essentially untested (19.4%), and `llmclient` tracing constructors are untested | `httpserver/hertz/wrapper_test.go` (2 tests); `llmclient/*_test.go`; ` | no |  |
| C3-012 | MEDIUM | M | Tests do not check the package's main promises: tracing, user interceptors inside recovery, and the client opt | grpc/grpc_test.go:47-64, grpc/roundtrip_test.go, grpc/recovery_test.go | no |  |
| C4-009 | MEDIUM | M | postgres tests skip every error path and never assert that rows are closed; no integration tests | postgres/stored_procedure_test.go:17-105, 120-155; postgres/postgres_t | no |  |
| C4-020 | MEDIUM | S | mongo adapter layer is untested although the driver ships in-memory result constructors | mongo/adapter.go (whole file); mongo/errors_test.go:27-57; mongo/clien | no |  |
| C5-012 | MEDIUM | M | Test coverage 10.5%: the core consume/publish logic is untested, even where it needs no broker | messaging/*_test.go (16 tests: options, topology loaders, error | no |  |
| C6-008 | MEDIUM | S | `validators` is 9.7% covered; three of four validators have no test | validators/validators_test.go (one test, `ValidateDateRange("bad", ... | no |  |
| C6-010 | MEDIUM | S | `envs` has one test, mutates the process environment without restoring it, and never exercises the success pat | envs/loader_test.go:12-45 (`os.Unsetenv` at line 14); coverage 66.7% | no |  |
| C3-013 | LOW | S | Test hygiene: no deadlines, `context.Background()`, sleep pacing, pre-1.22 loops and unchecked `Close` | grpc/roundtrip_test.go:46, 134, 144, 173, 177, 196, 203-212, 244, 248, | no |  |
| C4-023 | LOW | S | storage tests: weak concurrency assertion, no `FileStorage` concurrency or `Clear` tests, zero-value panic unt | storage/storage_test.go:143-169, 171-184; storage/file_storage_phase11 | no |  |
| C6-021 | LOW | M | No fuzz tests and no `Example*` functions in any of the eight packages | all `*_test.go` in scope (DN-36) | no |  |

### F. Performance

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C1-003 | MEDIUM | M | The redacting writer re-parses every log line (and every string field) as JSON: ~60× slower than zerolog | logger/config.go:312, 323-344 (`redactingWriter.Write`), 204-255 (`com | no |  |
| C1-007 | MEDIUM | S | Stack capture on every `NewError` costs ~31 µs and 67 allocations; the per-frame `runtime.Caller` loop is ~5×  | errorkit/stack.go:12-44 (loop at 21-41, `runtime.Caller(i)` at 22, `ru | no | C6-009 |
| C2-008 | MEDIUM | S | Every successful decoded response is buffered a second time, even when the audit log is off | `httprequest/httprequest.go:260-268` | no |  |
| C2-014 | MEDIUM | S | Backoff has no jitter and ignores `Retry-After` on 429 and 503 | `httprequest/httprequest.go:215-232`, `321-328`; `httprequest/httperro | no |  |
| C4-017 | MEDIUM | M | `FileStorage.Append` rewrites and re-indents the whole file on every call (O(n) per append, O(n²) per log) | storage/file_storage.go:37-51, 174-222 (`MarshalIndent` at 201); stora | yes |  |
| C5-003 | MEDIUM | S | Default prefetch is unlimited; the client buffers the whole backlog in memory | messaging/amqp.go:303 (`ch.Qos(s.queue.Prefetch, 0, false)`); messagin | yes |  |
| C6-007 | MEDIUM | S | `stringutils.NormalizeText` compiles its regexp on every call | stringutils/normalize.go:18 | no |  |
| C4-021 | LOW | M | Mongo result API forces whole-result buffering and exposes an empty `BulkWriteResult` | mongo/contracts.go:46-58; mongo/adapter.go:28-41, 54-56 | yes |  |

### G. Public API, observability & DX

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| C3-003 | HIGH | M | `ToStatus` maps every package-registered code to `codes.Internal` and ignores errorkit metadata | grpc/status.go:14-38 | no | X1-007 |
| C2-015 | MEDIUM | S | `llmclient` has no `*FromEnv` constructor, although the root README and AGENTS promise one for `LLM_API_KEY` | `llmclient/client.go:61-93`; `README.md:65-67`; `AGENTS.md` §2.4 (line | no |  |
| C2-018 | MEDIUM | S | Tracing wrappers record no errors, status or HTTP attributes and produce high-cardinality, ad-hoc span names | `httprequest/tracing.go:57-105`; `llmclient/tracing.go:38-49` (DN-10;  | yes | X1-003, X1-004 |
| C3-008 | MEDIUM | S | There is no client-side deadline facility, and Kitex `RPCTimeout`/`ConnectTimeout` were dropped with no replac | grpc/client.go:19-77, grpc/config.go:76-86, grpc/README.md:136-164, 17 | no |  |
| C3-009 | MEDIUM | S | `ServerConfig.Address` is required but never used | grpc/server.go:18, 25-29, 83, 94; grpc/config.go:47-48 | no |  |
| C3-010 | MEDIUM | S | README quick start: receive loops spin forever on errors, and `Shutdown` gets a context that never expires | grpc/README.md:128, 130, 150-163 | no |  |
| C3-011 | MEDIUM | S | No way to treat incoming trace context as untrusted (public endpoints) | grpc/server.go:68-79, grpc/options.go:45-62 | no |  |
| C3-016 | MEDIUM | S | `grpc` tracing wiring is not described in AGENTS.md §2.2 and does not use `*telemetry.Instrumenter` | grpc/options.go:33-62, 99-114; AGENTS.md §2.2 | no | X1-002 |
| C4-005 | MEDIUM | S | `WrapWithInstrumenter(sp, nil)` does not disable tracing, contrary to the README | postgres/tracing.go:34-39; telemetry/trace.go:143-146; postgres/README | no |  |
| C5-011 | MEDIUM | S | `WithLogger` is ignored by the publisher; failures are logged at Info with printf-style messages | messaging/amqp.go:98, 105, 116, 143, 160 (global `logger.Info`); | no |  |
| X1-005 | MEDIUM | M | Exported interfaces that consumers are invited to implement freeze the v1 surface; `httpserver` cannot route P | httpserver/contracts.go:12-67 (`HTTPContext`, 15 methods), 119-139 (`R | yes |  |
| X1-006 | MEDIUM | M | Adapters return interfaces, hide their concrete types and offer no escape hatch (unlike `grpc`) | httpserver/hertz/server.go:23,30 and wrapper.go:13-17 (unexported `wra | yes |  |
| X1-010 | MEDIUM | S | `httpserver.ServerOptions` is an options struct whose zero value silently disables tracing, recovery, audit an | httpserver/options.go:41-65 ("The zero value is not meaningful"); http | no |  |
| X1-011 | MEDIUM | L | Third-party types, including a superseded major and a pre-1.0 module, are part of the v1 API | algorithms/backoff.go:28 (returns `backoff.BackOff` from `cenkalti/bac | yes | X3-008 |
| X1-013 | MEDIUM | M | GoDoc is the declared symbol reference but 15 of 21 importable packages have no package comment, 59 exported i | all packages, see evidence; AGENTS.md:356 ("godoc — symbol-level refer | no |  |
| X1-014 | MEDIUM | L | Package identifiers differ from their directories or collide with stdlib and upstream packages | postgres/*.go (`package database`), mongo/*.go (`package mongodb`), sl | yes |  |
| X1-015 | MEDIUM | S | Cross-cutting contracts promise APIs and guarantees the code does not provide | README.md:65-67 and AGENTS.md:129-131 (`LLM_API_KEY` … "a sibling `*Fr | yes |  |
| X1-016 | MEDIUM | M | Consumer testability depends on hand-written fakes of large interfaces; the SDK ships no test doubles | httpserver/contracts.go:12-67 (15-method `HTTPContext`); mongo/contrac | yes |  |
| C1-020 | LOW | S | No helper to match an error chain by code; documented examples use a direct type assertion | errorkit/error.go (no `Is` method or code helper); AGENTS.md §4 (`err. | no |  |
| C1-022 | LOW | S | `telemetry` has no package documentation; exported helpers are undocumented; comments reference other projects | telemetry/trace.go:10-44 (comment block placed after the imports, so i | no |  |
| C1-023 | LOW | S | logger documentation disagrees with the code | logger/README.md:122 (claims `caller` on every line; no `.Caller()` is | no |  |
| C2-022 | LOW | S | The built-in `/health` route makes a consumer's own `GET /health` panic at registration | `httpserver/hertz/server.go:67-69`, `222-226` | no |  |
| C2-025 | LOW | S | `RequestLoggerMiddleware` cannot be installed through the SDK interface, ignores inbound request IDs and logs  | `httpserver/hertz/request_logger.go:16-45`; `httpserver/hertz/wrapper. | no |  |
| C2-027 | LOW | S | Package documentation is inaccurate in places that change consumer behavior | `httpclient/httpclient.go:82-90`; `httprequest/README.md:44-45,93-99,1 | no |  |
| C3-015 | LOW | S | Recovery status omits `ErrorInfo`; `ErrorInfo.Domain` is hard-coded to `"go-sdk"` | grpc/interceptors.go:53, grpc/status.go:56-59 | no |  |
| C4-006 | LOW | S | Stored-procedure spans: generic name for schema-qualified or `CALL` statements, no `db.*` attributes, failures | postgres/tracing.go:11, 46-79 | no |  |
| C4-008 | LOW | S | Package docs: misleading double-wrap example and missing godoc on exported symbols | postgres/tracing.go:24-28; postgres/contracts.go:10-24; postgres/store | no |  |
| C4-019 | LOW | S | Cursor-returning `Find` spans close before iteration; `Ping`/`Close` untraced; start-up ping ignores `ServerSe | mongo/tracing.go:52-58, 82-86; mongo/client.go:67-72 | no |  |
| C5-014 | LOW | S | Connection options are hard-wired: no TLS config for `amqps://`, no dial timeout or properties; heartbeat dete | messaging/amqp.go:44-46, 247-249 (`amqp.Config{Heartbeat: DefaultHeart | no |  |
| C5-015 | LOW | S | `NewPubSub` silently falls back to reading `RABBITMQ_TOPOLOGY_FILE` | messaging/messaging.go:114-123; messaging/topology.go:101-109 | yes |  |
| C5-016 | LOW | S | Package-level documentation gaps | messaging/otel_carrier.go:1; messaging/amqp.go:138, 208-220; messaging | no |  |
| C5-017 | LOW | S | Span naming and attributes bypass `telemetry` conventions and use deprecated OTel semconv keys | messaging/messaging.go:170-181, 290-299, 314-317 | yes |  |
| C6-012 | LOW | S | `algorithms`: zero-valued config hot-loops, the hidden 15-minute `MaxElapsedTime` is undocumented, and the REA | algorithms/backoff.go:29-41; README algorithms/README.md:58-59, 69-72, | yes |  |
| C6-019 | LOW | M | `envs` API: `NewLoader` is not a constructor, offers no options, and the contract mentions `envs.Load[T]()`, w | envs/loader.go:10; AGENTS.md §5 (DN-32) | yes |  |
| C6-022 | LOW | S | README/AGENTS statements about these packages that the code contradicts |  | no |  |
| X1-017 | LOW | S | Naming: stuttering identifiers, redundant constructors and exported presentation constants | httprequest (`HTTPRequest`, `HTTPRequester`, `NewHTTPRequestWithRetry` | yes |  |

### H. Maintainability & DevEx

| ID | Sev | Effort | Title | Location | Needs approval | Absorbs |
| --- | --- | --- | --- | --- | --- | --- |
| X2-001 | MEDIUM | M | No CI runs on push or pull request; the repo's own gates are already red and nothing notices | `.github/workflows/release.yml:15-17` (the only workflow; trigger `pus | yes | X3-003 |
| X2-002 | MEDIUM | S | The static-analysis gate cannot run from a fresh clone: its script is gitignored and `task skills:install` ins | `Taskfile.yml:15,44-52,70-79`; `.pre-commit-config.yaml:43-50`; `.giti | yes |  |
| X2-003 | MEDIUM | S | `staticanalysis.sh` reports "OK" (exit 0) when staticcheck or semgrep crash, or when staticcheck is missing | `.skillshare/skills/go-code-quality-check/scripts/staticanalysis.sh:27 | yes |  |
| X2-004 | MEDIUM | S | Release verification (build/vet/test) runs only after the tag and GitHub Release are published; nothing gates  | `.github/workflows/release.yml:26-38` (job `release-please`), `:40-65` | yes |  |
| X2-005 | MEDIUM | S | `task precommit:run` and `task check` do not exercise the staticanalysis hook; staged mode also skips renamed  | `.pre-commit-config.yaml:43-50`; `Taskfile.yml:63-69, 94-99`; script l | yes |  |
| X2-007 | MEDIUM | S | SECURITY.md (and README/AGENTS/llms.txt references to it) describe a script location and CI that do not exist | `SECURITY.md:5-6, 14, 102, 110, 121, 127-141, 154, 170-171`; `README.m | no |  |
| X2-008 | MEDIUM | S | SECURITY.md has no vulnerability reporting or supported-versions policy | `SECURITY.md` (whole file) | no |  |
| X2-009 | MEDIUM | S | No LICENSE file; AGENTS.md points to the README for the licence and the README has none | repo root (`git ls-files \| grep -i licen` → nothing); `AGENTS.md:14`;  | yes |  |
| X2-010 | MEDIUM | M | Root and per-package documentation states things the code does not do (items owned by X2 in the discovery note | see table | yes |  |
| X2-011 | MEDIUM | S | Go toolchain pins disagree and nothing enforces them: `.prototools` 1.27.3 vs `go.mod` 1.26.1 vs CI 1.26.1 vs  | `.prototools:1`; `go.mod:3`; `.github/workflows/release.yml:57-59` | yes | X3-002 |
| X2-012 | MEDIUM | M | Gate results are not reproducible: tools and rule packs float (`@latest`, Docker `latest`, remote packs, plugi | `Taskfile.yml:39` (`goimports@latest`), `:42` (`pre-commit` unpinned;  | yes |  |
| X2-006 | LOW | S | Semgrep scans no `_test.go` file in full mode but does scan them in staged mode; `audit.md` §1 "does not repro | `.skillshare/skills/.../staticanalysis.sh:64-68, 82, 333-357`; no `.se | yes |  |
| X2-013 | LOW | S | Pinned GitHub Actions have no automated updater; no `.github/dependabot.yml` | `.github/` (no `dependabot.yml`); `release.yml` (all `uses:` pinned by | yes |  |
| X2-014 | LOW | S | Dead or broken tooling configuration: sentrux never runs, `.env.shared` missing, "(was moon …)" leftovers | `.prototools:5,9-11`; `.sentrux/rules.toml`; `Taskfile.yml:45,56,64,72 | yes |  |
| X2-015 | LOW | S | `.moon/` is an untracked, non-ignored local cache; it blocks `task release:check` and dirties `git status` | `.moon/cache/` (untracked); `Taskfile.yml:116-117`; `.gitignore` | no |  |
| X2-016 | LOW | S | Local gates omit race, vulnerability, lint and coverage; `task test` differs from what the hook and CI run | `Taskfile.yml:81-92, 94-99, 168-181`; `.pre-commit-config.yaml:37-41` | yes |  |
| X2-017 | LOW | S | `CHANGELOG.md` is empty while README, AGENTS.md and llms.txt call it "the full per-phase change log" | `CHANGELOG.md` (0 bytes); `README.md:165`; `AGENTS.md:357`; `llms.txt: | no |  |
| X2-018 | LOW | M | Release assets lack provenance/signature and the pipeline does not check `go mod tidy`/`verify` | `.github/workflows/release.yml:67-80` | yes |  |
| X2-019 | LOW | S | README has no consumer section (install, minimum Go, usage example, pkg.go.dev link, compatibility, contributi | `README.md` (package map, status, developer setup, releases only) | no |  |

## 7. Deduplication and severity adjustments

### 7.1 Duplicates merged (first ID kept)

| Duplicate | Kept as | Original severity of duplicate |
| --- | --- | --- |
| C2-017 | C1-002 | MEDIUM |
| C6-005 | C5-010 | MEDIUM |
| C6-009 | C1-007 | MEDIUM |
| X1-002 | C3-016 | MEDIUM |
| X1-003 | C2-018 | MEDIUM |
| X1-004 | C2-018 | MEDIUM |
| X1-007 | C3-003 | MEDIUM |
| X1-009 | C4-004 | MEDIUM |
| X3-002 | X2-011 | MEDIUM |
| X3-003 | X2-001 | MEDIUM |
| X3-005 | X1-001 | MEDIUM |
| X3-007 | C1-024 | MEDIUM |
| X3-008 | X1-011 | LOW |

Related but **not** merged (different defect or package-specific angle,
cross-referenced in the plan): C2-009 ↔ X1-008 ↔ C5-011 (log-and-return /
logger injection); C2-015 ↔ X1-015 (X1-015 also covers the "always
`AppError`" guarantee); C1-004 ↔ C2-021 ↔ C4-001 (different redaction
mechanisms); C2-014 ↔ X1-012 (jitter vs. three retry implementations);
X1-013 ↔ C1-022 ↔ C5-016 ↔ C4-008 (umbrella vs. per-package godoc);
C2-011 ↔ X1-010; C2-012 ↔ C3-008; C4-006 ↔ C5-017 ↔ C4-019 (package span
details vs. C2-018); X2-007 ↔ X2-010 ↔ C6-022 ↔ C2-027 ↔ C1-023 (docs).

### 7.2 Severity adjustments

| ID | Raw severity | Consolidated | Reason |
| --- | --- | --- | --- |
| C1-024 | LOW | MEDIUM | absorbs X3-007: the deprecation makes the static-analysis gate fail |
| C3-016 | LOW | MEDIUM | absorbs X1-002: four tracing-injection shapes across the SDK, not only grpc |
| X2-001 | HIGH | MEDIUM | area H is MEDIUM on the §5 scale; ranked first among MEDIUM |
| X2-002 | HIGH | MEDIUM | area H is MEDIUM on the §5 scale; ranked first among MEDIUM |
| X2-003 | HIGH | MEDIUM | area H is MEDIUM on the §5 scale; ranked first among MEDIUM |

No other severity was changed. The three X2 items remain the top-ranked
MEDIUM findings because they keep every other gate from being enforced.

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
