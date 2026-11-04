# go-sdk — Remediation Plan (P3)

- Input: `AUDIT_REPORT.md` (170 unique findings, P2.Z approved) and the
  findings files under `findings/` (full evidence and recommendations).
- Output of P4: one branch per cluster, `audit/2026-11-<cluster>`, merged in
  P4.M; `go.mod`/`go.sum` changes only in **R-DEPS** (last).
- Every CRITICAL/HIGH finding has an item (§8 coverage check). Nothing here
  is implemented until gate **G1** is `DONE`.

## How to read this plan

**Class**

- `auto` — non-breaking, no architectural decision, no new dependency or
  tooling, no consumer-visible behavior change beyond fixing a defect
  against its documented contract. Implementable once G1 is `DONE`.
- `approval` — breaking change, new dependency or tooling, architecture
  change, or behavior change visible to consumers (including any change to
  what a README/AGENTS.md documents). Needs an explicit per-item decision:
  the human writes `APPROVED` or `REJECTED` in the status table (§9).

When a finding has an approval-free part and a part that needs approval,
it is split into two items (marked "auto part" / "approval part").

**Clusters** (one P4 worker each): `C1` errorkit/logger/telemetry · `C2`
httpclient/httprequest/httpserver(/hertz)/llmclient · `C3` grpc · `C4`
postgres/mongo/storage · `C5` messaging · `C6` utility packages · `X`
cross-cutting (docs, tooling, CI, multi-package changes) · `R-DEPS`
(`go.mod`/`go.sum`, runs alone after P4.XP).

**Cross-cluster rule.** `X` items that edit package code (marked
"post-merge") run in a separate step **P4.XP** after the cluster branches
are merged in P4.M and before R-DEPS, on top of `audit/2026-11`, so two
workers never edit the same file. `X` items without "post-merge" touch
only root docs, tooling or CI and run in P4.X in parallel with the
clusters.

**Shared documents.** All `AGENTS.md` edits are applied by **R-153**
(X, post-merge). Items whose Files say `AGENTS.md → R-153` only record the
required wording in their commit body; R-153 applies it once, so no two
workers edit `AGENTS.md`.

**Rejected dependencies.** When an item depends on an `approval` item that
is `REJECTED`, it proceeds with the "If rejected" line it carries; an item
without such a line and with a rejected dependency is re-planned by the
orchestrator before P4 starts.

**Fields.** Findings · Solution · Files · Tests · Regression risk ·
Compatibility · Effort (S ≤ ½ day, M ≤ 2 days, L > 2 days) · Depends on ·
Cluster · Class.

---

## 1. Security and correctness

> HIGH X3-001 (reachable stdlib vulnerabilities) is a security item whose
> fix is a toolchain policy (R-145, §7) plus a `go.mod`/workflow change
> (R-DEPS, §8); it sits there because only R-DEPS may touch `go.mod`.

### R-001 — Client-safe error serialization in `errorkit`
- Findings: C1-002 (HIGH)
- Solution: add `AppError.PublicJSON()` (code, public message =
  `Metadata.Description` or reason for client classes, id, trace_id); keep
  `PrettyJSON` for logs; change AGENTS.md §4, errorkit README and the
  `error.go` comment to recommend `PublicJSON` for responses. (The
  hard-coded `market-ecosystem/` path root is removed by R-131.)
- Files: errorkit/fmt.go, errorkit/error.go, errorkit/README.md,
  AGENTS.md → R-153
- Tests: `PublicJSON` never contains `stack_trace`, `payload`, wrapped text
- Regression risk: low (additive) · Compatibility: additive API; changes a
  documented recommendation (AGENTS §4) · Effort: M · Depends on: —
- Cluster: C1 · Class: **approval**

### R-002 — HTTP replies must not carry internal error text
- Findings: C2-017 (HIGH)
- Solution: `WithErrorOpt` puts code + stable public message for
  `*AppError`, a generic message otherwise; full error logged server-side
  with trace id; Hertz context returns a generic 500 message on marshal
  failure.
- Files: httpserver/response.go, httpserver/hertz/context.go, READMEs
- Tests: reply JSON for AppError / plain error never contains `err.Error()`
- Regression risk: medium (clients parsing `error` text) · Compatibility:
  consumer-visible reply content change · Effort: S · Depends on: R-001
- Cluster: C2 · Class: **approval**

### R-003 — gRPC recovery returns a fixed message
- Findings: C3-001 (HIGH)
- Solution: keep logging the panic value and stack; return
  `status.New(codes.Internal, "internal error")` with `ErrorInfo{Reason:
  ERR_SYSTEM_UNEXPECTED}` and the errorkit ID.
- Files: grpc/interceptors.go · Tests: panic text absent from
  `status.Convert(err).Message()` (unary + stream)
- Regression risk: low · Compatibility: fixes a leak; message text changes
  for panics only (not documented) · Effort: S · Depends on: —
- Cluster: C3 · Class: auto

### R-004 — `ToStatus` hides internal reasons
- Findings: C3-002 (HIGH)
- Solution: for codes mapping to `Internal`/`Unknown`/`DataLoss` send
  `Metadata.Description` or "internal error"; keep code and ID in
  `ErrorInfo`; keep the reason for client-error classes; optional
  `ToStatusWithPolicy` keeping today's behavior.
- Files: grpc/status.go, grpc/README.md · Tests: over-the-wire message per
  class
- Regression risk: medium · Compatibility: README promises "preserving the
  message" · Effort: M · Depends on: R-019
- Cluster: C3 · Class: **approval**

### R-005 — Postgres pool errors: wrap the cause, never the raw driver error
- Findings: C4-001 (HIGH), C4-003
- Solution: replace `WithPayload(err)` with `WithWrapped(err)` on the three
  constructor paths and set the payload to the driver's redacted
  `err.Error()` string, which keeps the README promise ("carry the
  underlying error in the payload").
- Files: postgres/postgres.go, postgres/README.md
- Tests: password absent from `PrettyJSON()` and from a captured log line;
  `errors.As(err, *pgconn.ParseConfigError)` works
- Regression risk: low · Compatibility: fixes a leak; payload content changes
  · Effort: S · Depends on: —
- Cluster: C4 · Class: auto

### R-006 — Sanitize URLs and provider messages in `httprequest`
- Findings: C2-006 (HIGH)
- Solution: `u.Redacted()` + drop/mask query (or allowlist keys) before any
  payload, log or span; put `provider_error_message` behind
  `captureBodies` + redactor; keep `provider_error_code`/`type`.
- Files: httprequest/httperror.go, httprequest/audit.go, httprequest/tracing.go
- Tests: userinfo, `?key=` and API-key text absent from payload, log, span
- Regression risk: low · Compatibility: error payload fields change
  (documented in README) · Effort: S · Depends on: —
- Cluster: C2 · Class: **approval**

### R-007 — Wider log redaction in `logger`
- Findings: C1-004
- Solution: extend the key list (token, secret, authorization, cookie,
  api_key, DSN/URL userinfo), mask URL userinfo inside string values, add
  `Config.Redactor`, document the policy in logger/README.md.
- Files: logger/config.go, logger/README.md · Tests: table test per key
- Regression risk: low · Compatibility: more fields masked; the README
  documents no policy today, so this documents it for the first time ·
  Effort: S · Depends on: R-030 (same file, do after)
- Cluster: C1 · Class: auto

### R-008 — Single, wider sensitive-header list
- Findings: C2-021
- Solution: one shared list in an internal package, pattern rules (token,
  secret, key, auth), mask values instead of dropping keys.
- Files: httprequest/audit.go, httpserver/hertz/server.go, new
  `internal/redact` · Tests: table test · Regression risk: low ·
  Compatibility: audit lines now show masked header keys that were dropped
  before (values never exposed) · Effort: S · Depends on: —
- Cluster: C2 · Class: auto

### R-009 — `urlkit.BuildURL` treats `path` as a path
- Findings: C6-001 (HIGH)
- Solution: parse `base` once; reject `path` with `?`/`#` or add the leading
  slash; join with `baseURL.JoinPath`; keep base query; document new rules.
- Files: urlkit/url.go, urlkit/README.md · Tests: `@evil.com/x`, `?a=1`,
  base with query
- Regression risk: medium (callers relying on concatenation) ·
  Compatibility: README documents raw concatenation · Effort: S ·
  Depends on: —
- Cluster: C6 · Class: **approval**

### R-010 — `urlkit.JoinPath` dot segments and scheme
- Findings: C6-002
- Solution: clean/reject `.`/`..`; add a validating variant requiring
  http(s) + host (additive) instead of changing the current function.
- Files: urlkit/url.go · Tests: dot segments, scheme-less base · Risk: low ·
  Compat: behavior change for `..` · Effort: S · Depends on: R-009
- Cluster: C6 · Class: **approval**

### R-011 — gRPC transport credentials (auto part)
- Findings: C3-005 (HIGH, auto part)
- Solution: add `ServerConfig.Creds` wired through `grpc.Creds`; add
  explicit `WithInsecure()` client/server options; README warning that the
  nil default is plaintext (meant for meshes terminating mTLS).
- Files: grpc/config.go, grpc/server.go, grpc/client.go, grpc/options.go,
  grpc/README.md · Tests: TLS round-trip over bufconn with test certs
- Regression risk: low (additive) · Compatibility: additive · Effort: M ·
  Depends on: —
- Cluster: C3 · Class: auto

### R-012 — gRPC insecure default (approval part)
- Findings: C3-005 (HIGH, approval part)
- Solution: make nil credentials an error unless `WithInsecure()` is passed.
- Files: grpc/client.go, grpc/server.go · Tests: nil creds → error
- Regression risk: high · Compatibility: **breaking** default (v2
  candidate) · Effort: S · Depends on: R-011
- Cluster: C3 · Class: **approval**

### R-013 — Inbound audit middleware: implement or remove
- Findings: C2-005 (HIGH)
- Solution: restore redacted logging through `logger.CtxOrGlobal(ctx)` with
  trace correlation honoring `CaptureBodies`, **or** remove the copies and
  document that inbound audit is not implemented (decision required).
- Files: httpserver/hertz/server.go, httpserver/README.md · Tests: log line
  and redaction asserted
- Regression risk: medium (log volume) · Compatibility: default-on behavior
  change · Effort: S · Depends on: R-008
- Cluster: C2 · Class: **approval**

### R-014 — `MaxRetries < 1` must still send one request
- Findings: C2-003 (HIGH)
- Solution: clamp in `normalizeRetryConfig`; make the trailing
  `return lastErr` unreachable or guarded.
- Files: httprequest/httprequest.go · Tests: 0, -1, 1 attempts (counting
  handler)
- Regression risk: low · Compatibility: fixes a defect · Effort: S ·
  Depends on: —
- Cluster: C2 · Class: auto

### R-015 — Response decoding without panics
- Findings: C2-004 (HIGH)
- Solution: `Elem().SetString/SetBool/SetBytes`; `IsNil()` → error.
- Files: httprequest/decode.go · Tests: named types per kind, typed nil
- Regression risk: low · Compatibility: none · Effort: S · Depends on: —
- Cluster: C2 · Class: auto

### R-016 — Idempotency-aware retries
- Findings: C2-002 (HIGH)
- Solution: retry by default only idempotent methods or requests with
  `Idempotency-Key`; retry non-idempotent only when the request provably
  was not sent; opt-in `RetryConfig.RetryNonIdempotent`; document LLM usage.
- Files: httprequest/httprequest.go, httprequest/README.md, llmclient/README.md
- Tests: POST on 500 → 1 attempt; POST on dial error → retried
- Regression risk: medium · Compatibility: README says retry depends only on
  `Retriable` · Effort: M · Depends on: R-014
- Cluster: C2 · Class: **approval**

### R-017 — Hertz `Run()` returns bind errors
- Findings: C2-001 (HIGH)
- Solution: call `engine.Run()` instead of `Spin()`; pre-bind with
  `net.Listen` + `server.WithListener` so bind errors become `*AppError`;
  signal handling left to the caller.
- Files: httpserver/hertz/wrapper.go, httpserver/README.md · Tests: busy
  port → `Run()` returns error, process alive
- Regression risk: medium (consumers relying on Spin's SIGTERM handling) ·
  Compatibility: behavior change · Effort: M · Depends on: —
- Cluster: C2 · Class: **approval**

### R-018 — Postgres error classifier
- Findings: C4-002 (HIGH)
- Solution: one `classifyPgError(err, op)`: `ErrNoRows` → non-retriable 404,
  class 23 → validation codes, 40001/40P01 → DEADLOCK, 28xxx → AUTH_FAILED,
  deadline/`pgconn.Timeout` → TIMEOUT, connection → UNAVAILABLE, cancel →
  non-retriable; wrap the cause.
- Files: postgres/stored_procedure.go, postgres/postgres.go,
  postgres/errors.go, postgres/README.md · Tests: error matrix table (fake
  Querier)
- Regression risk: medium · Compatibility: codes/retriability visible to
  consumers · Effort: M · Depends on: R-005
- Cluster: C4 · Class: **approval**

### R-019 — gRPC code fallback from errorkit metadata
- Findings: C3-003 (HIGH)
- Solution: keep the explicit switch for the 15 core codes; for any other
  code map `Metadata.HTTPStatus` (400/401/403/404/409/429/503/504/5xx) with
  `Retriable` as tie-breaker; fix the README wildcard.
- Files: grpc/status.go, grpc/README.md · Tests: table over registered codes
  of every package
- Regression risk: medium (client retry policies keyed on gRPC codes) ·
  Compatibility: gRPC codes observed by clients change from `Internal` for
  every package-registered code · Effort: S · Depends on: —
- Cluster: C3 · Class: **approval**

### R-020 — Transport status helpers driven by metadata
- Findings: X1-007 (merged into C3-003)
- Solution: `httpserver.StatusFor(err)`/`ReplyError(c, err)` and
  `grpc.FromStatus(err) error`.
- Files: httpserver/response.go, grpc/status.go · Tests: tables
- Regression risk: low · Compatibility: additive API, new documented
  contract · Effort: M · Depends on: R-019, R-002 · Cluster: X
  (post-merge) · Class: **approval**

### R-021 — `ToStatus` maps context errors
- Findings: C3-004
- Solution: `status.FromContextError` for `context.Canceled`/`DeadlineExceeded`.
- Files: grpc/status.go, grpc/README.md · Tests: both errors
- Regression risk: low · Compatibility: README documents `Unknown` ·
  Effort: S · Depends on: R-019 · Cluster: C3 · Class: **approval**

### R-022 — Reliable publishing
- Findings: C5-002 (HIGH)
- Solution: confirm mode + wait for ack (nack → `ERR_MQ_PUBLISH_FAILED`)
  bounded by ctx; `mandatory=true` and `basic.return` → error; optional
  `WithDeclareTopology()`; document the delivery guarantee.
- Files: messaging/amqp.go, messaging/messaging.go, messaging/README.md
- Tests: fake channel returning nack / return
- Regression risk: medium (latency, unroutable now errors) · Compatibility:
  behavior change · Effort: M · Depends on: R-064
- Cluster: C5 · Class: **approval**

### R-023 — Shutdown requeues in-flight messages
- Findings: C5-001 (HIGH)
- Solution: `backoff.WithContext(bo, ctx)`; on `ctx.Err()` → `Nack(false,
  true)`. Handlers keep receiving the subscription ctx (a detached handler
  ctx would change what handlers observe and is not proposed).
- Files: messaging/messaging.go · Tests: fake Acknowledger, cancelled ctx →
  requeue=true, no backoff wait
- Regression risk: low · Compatibility: fixes a defect · Effort: S ·
  Depends on: —
- Cluster: C5 · Class: auto

### R-024 — Recover handler panics
- Findings: C5-004
- Solution: recover in `run`, record stack on the span, non-retried error,
  `Nack` without requeue (to DLX); document.
- Files: messaging/messaging.go, README · Tests: panicking handler → nack
- Regression risk: low · Compatibility: fixes crash loop · Effort: S ·
  Depends on: R-023 · Cluster: C5 · Class: auto

### R-025 — Messaging error codes preserved
- Findings: C5-006
- Solution: return SDK AppErrors unchanged instead of re-wrapping them in
  `ERR_INTERNAL` (brings `Subscribe`/`Consume` in line with AGENTS §2.1).
- Files: messaging/messaging.go · Tests: inner code and retriability
  preserved
- Regression risk: low · Compatibility: fixes a contract violation (callers
  see the original code) · Effort: S · Depends on: — · Cluster: C5 ·
  Class: auto

### R-025b — Specific AMQP error codes and reconnect stop
- Findings: C5-006 (approval part)
- Solution: 403 → `ERR_MQ_AUTH_FAILED` (non-retriable); declare/bind/consume
  → `ERR_MQ_QUEUE_ERROR`/`_EXCHANGE_ERROR`/`_CONSUME_FAILED`; topology
  lookups → validation code; stop the reconnect loop on non-retriable
  errors.
- Files: messaging/messaging.go, messaging/amqp.go, messaging/README.md ·
  Tests: code per path; auth failure stops reconnect
- Regression risk: medium · Compatibility: error codes change; README
  documents unconditional automatic reconnect · Effort: S ·
  Depends on: R-025 · Cluster: C5 · Class: **approval**

### R-026 — Topology validation and backoff guards
- Findings: C5-010, C6-005
- Solution: `Topology.Validate()` (unique names, declared exchanges/DLX, one
  binding per queue, `MaxAttempts >= 1`, delays ≥ 0, multiplier ≥ 1),
  `DisallowUnknownFields`, range check before `uint64`. (The `algorithms`
  guards are in R-035, cluster C6; this item edits only `messaging`.)
- Files: messaging/topology.go, messaging/messaging.go
- Tests: typo, negative attempts, duplicate binding
- Regression risk: medium (previously accepted topologies now fail) ·
  Compatibility: stricter loading · Effort: M · Depends on: — ·
  Cluster: C5 · Class: **approval**

### R-027 — Mongo decode errors per README (auto part)
- Findings: C4-010 (auto part)
- Solution: `Decode` returns `WrapDecodeError` for decode failures (checks
  `result.Err()` first), `All` classifies decode errors as decode.
- Files: mongo/adapter.go · Tests: decode vs. operation error
- Regression risk: low · Compatibility: aligns with README table · Effort: S
  · Depends on: — · Cluster: C4 · Class: auto

### R-028 — Mongo error classification (approval part)
- Findings: C4-010 (approval part)
- Solution: `context.Canceled` non-retriable; `mongo.IsTimeout` → TIMEOUT;
  `IsNetworkError` → UNAVAILABLE; option errors → `ERR_SYSTEM_CONFIG_INVALID`.
- Files: mongo/errors.go, mongo/client.go · Tests: table
- Regression risk: medium · Compatibility: codes visible to consumers ·
  Effort: S · Depends on: R-027 · Cluster: C4 · Class: **approval**

### R-029 — `FileStorage.Clear` deletes only owned files
- Findings: C4-014
- Solution: remove only names accepted by `keyFromFileName`; update README.
- Files: storage/file_storage.go, storage/README.md · Tests: foreign file
  survives
- Regression risk: low · Compatibility: README documents current behavior ·
  Effort: S · Depends on: — · Cluster: C4 · Class: **approval**

### R-030 — `logger` handles wrapped and nil errors; `RecordError(nil)`
- Findings: C1-008, C1-009, C1-021
- Solution: `errors.AsType[*errorkit.AppError]` in `logger.Error`/
  `Logger.Error`; nil guard in `WithErrorKit`; `telemetry.RecordError`
  returns early on nil.
- Files: logger/wrapper.go, logger/instance.go, logger/errorkit.go,
  telemetry/helpers.go · Tests: `%w`-wrapped AppError keeps its code; nil
  does not panic; nil error leaves span status unset
- Regression risk: low · Compatibility: fixes defects · Effort: S ·
  Depends on: R-050 · Cluster: C1 · Class: auto

### R-031 — `FileStorage.Append(nil)` returns an error
- Findings: C4-015
- Solution: nil guard in `FileStorage.Append` with the same error as
  `MemoryStorage`.
- Files: storage/file_storage.go · Tests: shared conformance test for both
  stores
- Regression risk: low · Compatibility: panic becomes the documented error ·
  Effort: S · Depends on: R-054 · Cluster: C4 · Class: auto

### R-031b — Specific storage error codes
- Findings: C4-018
- Solution: validation codes for nil input and over-long keys (validate key
  length up front), `ERR_STORAGE_ACCESS_DENIED` for `fs.ErrPermission`,
  non-retriable code for corruption.
- Files: storage/*.go, storage/README.md · Tests: code per path
- Regression risk: medium · Compatibility: codes/retriability change ·
  Effort: S · Depends on: R-031 · Cluster: C4 · Class: **approval**

### R-032 — `httprequest` error classification
- Findings: C2-013, C2-023
- Solution: preserve `CheckRedirect` AppErrors; TLS/x509, invalid URL and
  unsupported scheme non-retriable; validate URL once before the loop;
  `context.Canceled` preserved/non-5xx; empty `choices` →
  `ERR_EXTERNAL_INVALID_RESPONSE`.
- Files: httprequest/httperror.go, httprequest/httprequest.go,
  llmclient/client.go · Tests: tables + attempt counts
- Regression risk: medium · Compatibility: codes/retriability visible ·
  Effort: M · Depends on: R-014 · Cluster: C2 · Class: **approval**

### R-033 — `RemoveMarkdownCodeBlock` correctness
- Findings: C6-003, C6-013
- Solution: line-based parse (single outer fence, no inner fence lines);
  widen the info string; move the doc comment.
- Files: stringutils/normalize.go · Tests: multi-block, info strings
- Regression risk: low · Compatibility: matches README · Effort: S ·
  Depends on: — · Cluster: C6 · Class: auto

### R-034 — `NewExponentialBackoff` resets before use
- Findings: C6-004
- Solution: call `Reset()` after setting fields.
- Files: algorithms/backoff.go · Tests: first delay within
  `InitialInterval ± RandomizationFactor`
- Regression risk: low · Compatibility: fixes defect · Effort: S ·
  Depends on: — · Cluster: C6 · Class: auto

### R-035 — `algorithms` zero config and hidden `MaxElapsedTime`
- Findings: C6-012
- Solution: substitute defaults for non-positive intervals and multiplier
  < 1 and cap `MaxRetries` (moved here from R-026, so only C6 edits
  `algorithms/backoff.go`); set or expose `MaxElapsedTime`; document.
- Files: algorithms/backoff.go, algorithms/README.md · Tests: zero config,
  negative values
- Regression risk: medium · Compatibility: changes behavior for long
  schedules · Effort: S · Depends on: R-034 · Cluster: C6 ·
  Class: **approval**

### R-036 — `slices.Chunk` clips capacity
- Findings: C6-006
- Solution: `items[i:end:end]`.
- Files: slices/chunks.go · Tests: append to chunk does not overwrite next
- Regression risk: low · Compatibility: compatible with README · Effort: S ·
  Depends on: — · Cluster: C6 · Class: auto

### R-037 — OTLP timestamp precision
- Findings: C1-011
- Solution: per-logger `TimeFormatUnixMicro`/RFC3339Nano, parse accordingly
  in `OTLPWriter`, `UseNumber` for integers.
- Files: logger/config.go, logger/otlp.go · Tests: sub-second timestamp and
  large int preserved
- Regression risk: medium (log timestamp format) · Compatibility: stdout
  timestamp field changes unit · Effort: S · Depends on: R-050 ·
  Cluster: C1 · Class: **approval**

### R-038 — `DetectEnv` defaults and matching
- Findings: C1-012
- Solution: case-insensitive match, fix doc comment, read once; default to
  Prod (or warn on stderr) when unset.
- Files: logger/config.go, logger/README.md · Tests: table
- Regression risk: medium · Compatibility: README documents unset → Dev ·
  Effort: S · Depends on: — · Cluster: C1 · Class: **approval**

### R-039 — OTLP log provider configuration and errors
- Findings: C1-010
- Solution: `NewLogProvider(ctx, LogProviderConfig{Endpoint, Insecure,
  TLSConfig, Headers, Timeout}) (*LogProviderHandle, error)` and a
  `Close(ctx) error`; `InitLogProvider` stays as wrapper; fix README
  `WithBlock` claim.
- Files: logger/logprovider.go, logger/README.md · Tests: config applied,
  errors returned
- Regression risk: low · Compatibility: additive · Effort: M ·
  Depends on: R-100 · Cluster: C1 · Class: auto

### R-040 — `errorkit` low-severity correctness
- Findings: C1-016, C1-017, C1-018, C1-019
- Solution: validate registry batches before writing, sorted listings;
  JSON fallback via struct marshal; colors from the target writer; ULID
  clock read inside the lock, case-insensitive decoder. Printing the payload
  in `FormatPlain` (C1-018) is **not** done: it could expose payload
  secrets (compare C4-001); revisit after R-001.
- Files: errorkit/register.go, errorkit/fmt.go, errorkit/idgen.go
- Tests: per fix · Regression risk: low · Compatibility: listing order
  becomes sorted; batch registration becomes all-or-nothing · Effort: M ·
  Depends on: — · Cluster: C1 · Class: auto

### R-041 — Postgres DSN normalization
- Findings: C4-007
- Solution: userinfo ends at the last `@` before the first following `/`;
  fuzz target; README wording.
- Files: postgres/postgres.go, README · Tests: fuzz + round-trip
- Risk: low · Compat: none · Effort: S · Depends on: — · Cluster: C4 ·
  Class: auto

### R-042 — `WrapWithInstrumenter(nil)` disables tracing
- Findings: C4-005
- Solution: `if instrumenter == nil { return inner }` as in mongo.
- Files: postgres/tracing.go · Tests: nil → inner returned
- Risk: low · Compat: matches README · Effort: S · Depends on: — ·
  Cluster: C4 · Class: auto

### R-043 — `Reply` nil data with `omitempty`
- Findings: C2-024
- Solution: store values directly (`r.Data = data`).
- Files: httpserver/response.go · Tests: nil omitted
- Risk: low · Compat: JSON output changes (`null` → omitted) · Effort: S ·
  Depends on: R-002 · Cluster: C2 · Class: **approval**

### R-044 — Configurable `/health` path
- Findings: C2-022
- Solution: document the reserved path; option for the path and a readiness
  hook (additive).
- Files: httpserver/hertz/server.go, READMEs · Tests: custom path, no panic
  on consumer `/health` when disabled
- Risk: low · Compat: additive · Effort: S · Depends on: — · Cluster: C2 ·
  Class: auto

### R-045 — Utility low-severity correctness
- Findings: C6-011, C6-014, C6-016
- Solution: value-free env parse message; document `NormalizeText`
  collisions/empty result; `SaveJSON` encodes to a buffer then writes 0600.
- Files: envs/loader.go, stringutils/README.md, ioutils/save.go
- Tests: per fix · Risk: low · Compat: none · Effort: S · Depends on: — ·
  Cluster: C6 · Class: auto

### R-046 — `validators` consistency
- Findings: C6-015
- Solution: rename shadowing params, simplify condition (QF1001), consistent
  messages and codes.
- Files: validators/validators.go · Tests: R-128
- Risk: low · Compat: message/code changes · Effort: S · Depends on: — ·
  Cluster: C6 · Class: **approval**

### R-047 — LLM API key masked when printed
- Findings: C2-016 (auto part)
- Solution: `Config.String()`/`GoString()` mask `APIKey`.
- Files: llmclient/client.go · Tests: `%v`/`%+v`/`%#v` masked
- Regression risk: low · Compatibility: printed form of `Config` changes
  (secret no longer shown) · Effort: S · Depends on: — · Cluster: C2 ·
  Class: auto

### R-047b — LLM endpoint and key validation
- Findings: C2-016 (approval part)
- Solution: reject an empty `APIKey` and non-https endpoints unless an
  explicit opt-in is set (`AllowInsecureEndpoint`). The constructors return
  no error today, so the check runs at `Execute` and returns
  `ERR_SYSTEM_CONFIG_INVALID` (no signature change).
- Files: llmclient/client.go, llmclient/README.md · Tests: empty key /
  http endpoint → config error unless opted in
- Regression risk: medium (local http model servers need the opt-in) ·
  Compatibility: behavior change · Effort: S · Depends on: R-047 ·
  Cluster: C2 · Class: **approval**

### R-048 — `llmclient` `*FromEnv` constructor
- Findings: C2-015
- Solution: `ConfigFromEnv()` reading `LLM_API_KEY` (+ optional
  `LLM_ENDPOINT`), `ERR_SYSTEM_CONFIG_INVALID` when missing; README.
- Files: llmclient/client.go, llmclient/README.md · Tests: `t.Setenv`
- Risk: low · Compat: additive, matches AGENTS §2.4 · Effort: S ·
  Depends on: — · Cluster: C2 · Class: auto

### R-049 — Response size limits
- Findings: C2-007
- Solution: `io.LimitReader` (64 KiB error bodies; configurable
  `MaxResponseBytes` with a generous default); bounded drain; specific code
  when exceeded.
- Files: httprequest/httprequest.go, httprequest/decode.go · Tests: oversized
  body → error, connection reused
- Regression risk: medium (large legitimate responses) · Compatibility: new
  limit · Effort: S · Depends on: — · Cluster: C2 · Class: **approval**

## 2. Concurrency and stability

### R-050 — `logger.New` stops mutating zerolog globals
- Findings: C1-001 (HIGH)
- Solution: instance level `zerolog.New(w).Level(level)`; time format only
  in `Init` or via a per-logger hook.
- Files: logger/config.go, logger/instance.go · Tests: extend
  `TestNewDoesNotMutateGlobal` (level + `TimeFieldFormat`), `-race` test
- Regression risk: low · Compatibility: matches README/AGENTS §2.3 ·
  Effort: S · Depends on: — · Cluster: C1 · Class: auto

### R-051 — `errorkit` globals behind atomics
- Findings: C1-005
- Solution: unexported atomics behind the setters; mark `MaxStackDepth`,
  `ColorsEnabled`, `GenerateID`, `ErrorRegistry` `Deprecated:`.
- Files: errorkit/config.go, errorkit/colors.go, errorkit/register.go ·
  Tests: `-race` tests for setters
- Regression risk: low · Compatibility: deprecates exported v1 API ·
  Effort: M · Depends on: — · Cluster: C1 · Class: **approval**

### R-052 — `OverrideMetadata` without lock re-entry
- Findings: C1-006
- Solution: copy under lock, run updater unlocked, validate (reject changed
  `Code`), store if unchanged (or retry); document.
- Files: errorkit/register.go · Tests: re-entrant updater does not deadlock
  (isolated with `-run` and `timeout`)
- Risk: low · Compat: none · Effort: S · Depends on: — · Cluster: C1 ·
  Class: auto

### R-053 — `InitTracer` synchronization
- Findings: C1-013
- Solution: `atomic.Pointer[Instrumenter]` set by `InitTracer`.
- Files: telemetry/trace.go · Tests: `-race`
- Risk: low · Compat: none · Effort: S · Depends on: — · Cluster: C1 ·
  Class: auto

### R-054 — Safe `FileStorage` writes
- Findings: C4-012 (HIGH), C4-013, C4-016
- Solution: `os.CreateTemp(dir, ".tmp-*")` (O_EXCL, 0600) → write →
  `Sync` → `Close` → `Rename` → directory `Sync`; zero-length file is
  corruption; document "one `FileStorage` per directory per host".
- Files: storage/file_storage.go, storage/README.md · Tests: two instances,
  200 concurrent appends → valid JSON; symlink at temp path not followed
- Lock: no `flock` (Unix-only); unique temp files remove the corruption and
  the README documents "one `FileStorage` per directory per host" for
  read-modify-write consistency
- Regression risk: low · Compatibility: fixes corruption · Effort: M ·
  Depends on: — · Cluster: C4 · Class: auto

### R-055 — Storage file permissions
- Findings: C4-022
- Solution: default `0o700`/`0o600`, optional mode option.
- Files: storage/file_storage.go, storage/README.md · Tests: modes ·
  Regression risk: medium (other users/groups lose read access) ·
  Compatibility: default file modes change · Effort: S ·
  Depends on: R-054 · Cluster: C4 · Class: **approval**

### R-056 — Hertz `Shutdown(ctx)` honors the context
- Findings: C2-010
- Solution: large `WithExitWaitTime`, caller ctx as the bound; timeout →
  `ERR_SYSTEM_TIMEOUT_INTERNAL`.
- Files: httpserver/hertz/wrapper.go · Tests: in-flight request beyond 5 s
- Regression risk: low · Compatibility: `Shutdown` now returns a timeout
  error instead of `nil` · Effort: S · Depends on: R-017 (ordering only) ·
  If R-017 is rejected: apply the same `WithExitWaitTime` fix to the
  `Spin`-based server · Cluster: C2 · Class: auto

### R-057 — `EnableRecovery` controls recovery
- Findings: C2-011
- Solution: build with `server.New`; SDK recovery outermost when enabled,
  with stack in the structured log.
- Files: httpserver/hertz/server.go · Tests: disabled → panic propagates
  per contract; enabled → single structured log
- Regression risk: medium · Compatibility: behavior change when false ·
  Effort: S · Depends on: R-017 · Cluster: C2 · Class: **approval**

### R-058 — End-to-end bound on LLM calls
- Findings: C2-012
- Solution: non-zero default timeout for `llmclient`; `RetryConfig.MaxElapsed`
  or document ctx deadline; lower default `DialTimeout`.
- Files: httpclient/httpclient.go, llmclient/client.go,
  httprequest/httprequest.go · Tests: total time bounded
- Regression risk: medium (long generations) · Compatibility: new defaults ·
  Effort: S · Depends on: R-014 · Cluster: C2 · Class: **approval**

### R-059 — gRPC `MaxConcurrentStreams` option
- Findings: C3-006 (auto part)
- Solution: add `ServerConfig.MaxConcurrentStreams` (0 = gRPC default),
  document and recommend a value; default unchanged.
- Files: grpc/config.go, grpc/server.go, README · Tests: limit applied
- Risk: low · Compat: additive · Effort: S · Depends on: — · Cluster: C3 ·
  Class: auto (changing the default is not proposed)

### R-060 — Recovery outermost with raw options
- Findings: C3-007
- Solution: document that raw `grpc.UnaryInterceptor` runs outside
  recovery; end-to-end test that recovery wraps SDK-managed interceptors.
- Tests: end-to-end test that recovery wraps SDK-managed unary and stream interceptors
- Files: grpc/options.go, grpc/README.md, grpc tests · Risk: low ·
  Compat: none · Effort: S · Depends on: — · Cluster: C3 · Class: auto

### R-061 — gRPC client default timeout
- Findings: C3-008
- Solution: `WithClientDefaultTimeout(d)` interceptors applying a timeout
  only when ctx has no deadline; document in the migration table.
- Files: grpc/client.go, grpc/options.go, README · Tests: deadline applied /
  not overridden
- Risk: low · Compat: additive · Effort: S · Depends on: — · Cluster: C3 ·
  Class: auto

### R-062 — Safe default prefetch
- Findings: C5-003
- Solution: `Prefetch <= 0` → small default (e.g. 1) or validation error;
  document.
- Files: messaging/amqp.go, messaging/topology.go, README · Tests: Qos value
- Regression risk: medium (throughput) · Compatibility: default changes ·
  Effort: S · Depends on: R-026 · Cluster: C5 · Class: **approval**

### R-063 — Ack/Nack failures visible
- Findings: C5-005
- Solution: record on `recvSpan`, log through injected logger; document
  at-least-once and idempotency.
- Files: messaging/messaging.go, README · Tests: failing Acknowledger
- Risk: low · Compat: none · Effort: S · Depends on: R-023 · Cluster: C5 ·
  Class: auto

### R-064 — `Publish` honors ctx; no dial under the mutex
- Findings: C5-007
- Solution: `PublishWithContext`, `backoff.WithContext`, ctx check before
  reconnect, single-flight reconnect outside the critical section; the
  documented "reconnect once more and retry" behavior is kept.
- Files: messaging/amqp.go, messaging/messaging.go · Tests: cancelled ctx →
  no dial (local listener)
- Risk: medium · Compat: fixes defect · Effort: M · Depends on: — ·
  Cluster: C5 · Class: auto

### R-065 — Terminal `Close`
- Findings: C5-008
- Solution: `closed` flag; `Publish` after `Close` → error; `PubSub.Close`
  stops tracked subscribers (or document that it does not).
- Files: messaging/amqp.go, messaging/messaging.go · Tests: publish after
  close
- Regression risk: medium · Compatibility: behavior change · Effort: S ·
  Depends on: R-064 · Cluster: C5 · Class: **approval**

### R-066 — Per-key publish retry, single retry layer
- Findings: C5-009
- Solution: retry per routing key tracking completed keys; remove one of
  the two retry layers.
- Files: messaging/messaging.go, messaging/amqp.go, messaging/README.md ·
  Tests: attempt counts
- Regression risk: medium · Compatibility: README documents that the
  publisher "reconnects once more and retries"; attempt counts change ·
  Effort: S · Depends on: R-064 · Cluster: C5 · Class: **approval**

### R-067 — Cancellable network constructors
- Findings: C4-004, X1-009 (merged)
- Solution: `NewPostgresClientContext`/`NewPostgresClientFromEnvContext`
  (5 s default only without a deadline), `Config.ConnectTimeout`;
  `messaging.NewPubSubContext` or `WithDialTimeout`; document the context
  rule for network constructors.
- Files: postgres/postgres.go, messaging/messaging.go, READMEs · Tests:
  cancelled ctx returns promptly
- Risk: low · Compat: additive; new documented contract · Effort: M ·
  Depends on: R-005, R-064 · Cluster: X (post-merge) · Class: **approval**

### R-068 — Mongo option precedence
- Findings: C4-011
- Solution: apply `Set*` only for non-zero `Config` fields; SDK defaults
  only when neither Config nor URI sets the option; clamp `MinPoolSize`.
- Files: mongo/client.go, README · Tests: URI options survive;
  `MaxPoolSize < 10` starts
- Regression risk: medium · Compatibility: effective options change ·
  Effort: S · Depends on: — · Cluster: C4 · Class: **approval**

### R-069 — Mongo cursor spans and ping budget
- Findings: C4-019
- Solution: traced cursor adapter; ping timeout from
  `max(5s, ServerSelectionTimeout)`; `WithoutCancel` for clean-up disconnect.
- Files: mongo/tracing.go, mongo/client.go · Tests: span covers iteration
- Regression risk: low · Compatibility: start-up ping waits up to
  `ServerSelectionTimeout` when it is larger than 5 s (it was cut at 5 s) ·
  Effort: S · Depends on: — · Cluster: C4 · Class: auto

### R-070 — AMQP connection options
- Findings: C5-014
- Solution: additive `WithAMQPConfig`/`WithTLSConfig`, `WithHeartbeat`,
  `WithDialTimeout`; correct README.
- Files: messaging/amqp.go, messaging/messaging.go (options),
  messaging/README.md · Tests: options applied
- Risk: low · Compat: additive · Effort: S · Depends on: R-064 ·
  Cluster: C5 · Class: auto

### R-071 — Checked `Close` errors in messaging
- Findings: C5-013
- Solution: `errors.Join` into returned errors or log; explicit `_ =` with a
  comment where intended.
- Files: messaging/amqp.go, messaging/messaging.go · Tests: — (errcheck
  clean) · Risk: low · Compat: none · Effort: S · Depends on: R-064 ·
  Cluster: C5 · Class: auto

### R-072 — Jittered backoff and `Retry-After`
- Findings: C2-014
- Solution: full jitter; honor `Retry-After` up to `MaxDelay` and the ctx
  budget.
- Files: httprequest/httprequest.go, httprequest/httperror.go,
  httprequest/README.md · Tests: synctest timing
- Regression risk: low · Compatibility: README documents the doubling
  schedule (`100 ms → 5 s`, "doubles the delay … capped at `MaxDelay`") ·
  Effort: S · Depends on: R-014 · Cluster: C2 · Class: **approval**

## 3. Architecture and design

### R-080 — `httprequest`/`llmclient` stop importing `logger` (step 1)
- Findings: X1-001 (HIGH, non-breaking step), X3-006 (doc note)
- Solution: move the helpers `httprequest` needs (`logger.CtxOrGlobal`,
  `logger.WithTrace`, used at httprequest/audit.go:50-51 and
  httperror.go) into an internal package that `logger` also uses, with
  identical behavior (context logger, else global); drop the `logger`
  import from `httprequest` (llmclient then stops linking gRPC through it);
  document in logger/grpc READMEs that `logger` links `x/net/trace` and
  services must not expose `http.DefaultServeMux`.
- Files: httprequest/audit.go, httprequest/httperror.go, logger/*.go, new
  `internal/logctx`, logger/README.md, grpc/README.md
- Tests: `go list -deps ./httprequest` has no `google.golang.org/grpc`
  (test asserting it); existing tests green
- Regression risk: low · Compatibility: none (internal) · Effort: M ·
  Depends on: R-085 decision · If R-085 is rejected: keep today's logging
  calls, routed through the internal helper with unchanged semantics; if
  approved: the calls are removed and only `audit.go` needs the helper ·
  Cluster: X (post-merge of C1, C2, C3) · Class: auto

### R-081 — OTLP exporter in its own package (step 2)
- Findings: X1-001 (approval part), X3-005
- Solution: new `logger/otlp` package; `logger.SetupOTLP`/`InitLogProvider`/
  `NewOTLPWriter` become `Deprecated:` forwarders (still link gRPC until v2).
- Files: logger/*.go, new logger/otlp · Tests: forwarders equivalent
- Deferred: removing the forwarders (so `logger` stops linking gRPC) is a
  v2 change and is not part of this plan
- Regression risk: low · Compatibility: additive + deprecations;
  architecture change · Effort: M · Depends on: R-039, R-080 ·
  Cluster: X (post-merge) · Class: **approval**

### R-082 — Module strategy (single module vs. nested modules)
- Findings: X3-004
- Solution: decide and document; option: nested modules for hertz, grpc,
  mongo, postgres, messaging and logger/otlp, root keeps the stdlib core.
- Files: go.mod (+ new go.mod files), docs · Tests: consumer module builds
- Regression risk: high · Compatibility: release-sequencing hazard;
  effectively a v2-scale change · Effort: L · Depends on: R-081 ·
  Cluster: R-DEPS · Class: **approval** (recommendation: document the
  single-module choice now; revisit with v2)

### R-083 — One tracing injection story
- Findings: C3-016, X1-002 (merged)
- Solution: `(*Instrumenter).TracerProvider()`; optional
  `grpc.WithServerInstrumenter/WithClientInstrumenter`,
  `messaging.WithInstrumenter`; AGENTS §2.2 lists grpc and hertz as
  exceptions and states hertz uses the global provider.
- Files: telemetry/trace.go, grpc/options.go, messaging/messaging.go
  (options), AGENTS.md → R-153
- Tests: provider passed through · Risk: low · Compat: additive; contract
  text change · Effort: M · Depends on: — · Cluster: X (post-merge) ·
  Class: **approval**

### R-084 — Messaging publisher uses the injected logger
- Findings: C5-011
- Solution: pass the resolved logger into `Publisher`; Warn level with
  structured fields; ctx-aware helpers.
- Files: messaging/amqp.go, messaging/messaging.go · Tests: captured log
- Regression risk: low · Compatibility: publisher logs move from the global
  logger to the `WithLogger` one and from Info to Warn (fixes the
  documented `WithLogger` contract) · Effort: S ·
  Depends on: R-064 · Cluster: C5 · Class: auto

### R-085 — No log-and-return in library code
- Findings: C2-009, X1-008
- Solution: shared rule (option > `logger.CtxOrGlobal(ctx)` > global; do not
  log errors returned to the caller); remove the per-attempt non-2xx log in
  `httprequest` (or make it opt-in, once, after the final attempt).
- Files: httprequest/httperror.go, AGENTS.md §2.3 → R-153 · Tests: no log
  line on non-2xx by default
- Regression risk: medium (operators relying on those lines) ·
  Compatibility: log output change · Effort: S · Depends on: — ·
  Cluster: C2 (policy text in X) · Class: **approval**

### R-086 — Extensible `httpserver` routing
- Findings: X1-005
- Solution: optional interfaces (`PatchRouter`, `HeadRouter`,
  `OptionsRouter`) via comma-ok, generic `Handle(method, path, h)` on the
  concrete adapter.
- Files: httpserver/contracts.go, httpserver/hertz/*.go · Tests: PATCH route
- Risk: low · Compat: additive; v1 surface design decision · Effort: M ·
  Depends on: R-087 · Cluster: X (post-merge) · Class: **approval**

### R-087 — Adapter escape hatches
- Findings: X1-006
- Solution: `hertz.NewEngine` with `Engine()`; `mongodb.NewFromClient`,
  `WithClientOptions`; `Unwrap()` on tracing decorators.
- Files: httpserver/hertz, mongo, httprequest/llmclient/postgres tracing ·
  Tests: per hatch · Risk: low · Compat: additive · Effort: M ·
  Depends on: — · Cluster: X (post-merge) · Class: **approval**

### R-088 — Hertz functional options
- Findings: X1-010
- Solution: `hertz.NewServer(cfg, opts...)` with default-on behavior
  (`WithoutTracing`, `WithLogger`, …); fix the server.go comment.
- Files: httpserver/hertz/server.go, httpserver/README.md · Tests: zero
  options = all defaults on
- Regression risk: low · Compatibility: additive (new function) · Effort: M
  · Depends on: R-057 (ordering) · If R-057 is rejected: the recovery
  option keeps today's semantics · Cluster: X (post-merge) · Class: auto

### R-089 — SDK-owned `Backoff` interface
- Findings: X1-011, X3-008 (merged)
- Solution: AGENTS.md policy on upstream types in the API; `algorithms.Backoff`
  interface + constructor; deprecate the v4-typed function; v4 removal in v2.
- Files: algorithms/backoff.go, AGENTS.md → R-153 · Tests: new constructor
- Risk: low · Compat: additive + deprecation · Effort: M ·
  Depends on: R-034 · Cluster: X (post-merge) · Class: **approval**

### R-090 — One internal backoff implementation
- Findings: X1-012
- Solution: internal backoff with full jitter used by `httprequest` and
  `messaging`; public config types unchanged, semantics documented.
- Files: new internal/backoff, httprequest, messaging, both READMEs ·
  Tests: shared table tests
- Regression risk: medium · Compatibility: replaces the documented
  schedules (messaging reconnect 5 s doubling to 60 s; httprequest
  doubling) with jittered ones · Effort: M · Depends on: R-072, R-089 ·
  If R-089 is rejected: the internal backoff does not use the public
  `algorithms.Backoff` type · Cluster: X (post-merge) · Class: **approval**

### R-091 — Published test doubles
- Findings: X1-016
- Solution: `httpserver/httpservertest`, `mongo/mongotest`,
  `messaging/messagingtest`; move private fakes there.
- Files: new packages · Tests: in `<pkg>_test` · Risk: low · Compat: new
  public API surface to maintain · Effort: M · Depends on: — ·
  Cluster: X (post-merge) · Class: **approval**

### R-092 — `ERR_EXTERNAL_*` codes owned by `errorkit`
- Findings: X1-018
- Solution: define in `errorkit`, keep `httpclient.ERR_EXTERNAL_*` as equal
  constants; no double registration.
- Files: errorkit/codes.go, httpclient/errors_codes.go · Tests: registry
  lookups unchanged
- Risk: low · Compat: none · Effort: S · Depends on: — · Cluster: X
  (post-merge) · Class: auto

### R-093 — Package identifiers (v2)
- Findings: X1-014
- Solution: v1: state the import alias in package docs (done in R-155);
  v2: rename `database`/`mongodb`, rename or drop `slices`.
- Files: postgres/*.go, mongo/*.go, slices/*.go (v2 only) · Tests: n/a
  (deferred) · Depends on: v2 decision
- Regression risk: high · Compatibility: **breaking** · Effort: L ·
  Cluster: X · Class: **approval** (recommendation: defer to v2)

### R-094 — Naming cleanup (v2)
- Findings: X1-017
- Solution: v1: no new stuttering names, document canonical constructors,
  mark colour vars non-stable; v2: batch renames.
- Files: httprequest, errorkit/colors.go, envs (docs only in v1) · Tests:
  n/a (docs; renames deferred) · Regression risk: none in v1 ·
  Depends on: v2 decision
- Compatibility: renames are **breaking** · Effort: M · Cluster: X ·
  Class: **approval** (recommendation: defer renames to v2)

### R-095 — `RequestLoggerMiddleware` installable and request-ID aware
- Findings: C2-025
- Solution: `httpserver.MiddlewareFunc` variant or `ServerOptions` switch;
  honor/echo `X-Request-ID`.
- Files: httpserver/hertz/request_logger.go · Tests: header echo
- Regression risk: low · Compatibility: additive; `remote_addr` stays logged
  by default (making it opt-in is not done, it would remove a logged field)
  · Effort: S · Depends on: — · Cluster: C2 · Class: auto

### R-096 — gRPC `ListenAndServe`
- Findings: C3-009 (auto part)
- Solution: `(*Server).ListenAndServe()` using `cfg.Address`; document
  `Address` otherwise informational. Relaxing the requirement is not
  proposed.
- Files: grpc/server.go, README · Tests: loopback listen
- Risk: low · Compat: additive · Effort: S · Depends on: — · Cluster: C3 ·
  Class: auto

### R-097 — Mongo result API
- Findings: C4-021
- Solution: `Next/Decode/Err` on `Cursor`; getters on `BulkWriteResult`.
- Files: mongo/contracts.go, mongo/adapter.go · Tests: adapter tests
- Risk: low · Compat: **interface change** for implementers (breaking for
  consumers implementing `Cursor`) · Effort: M · Depends on: R-087 ·
  Cluster: C4 · Class: **approval**

## 4. Idiomatic Go and modernization

### R-100 — Replace deprecated `otel/log/global` (gate is red)
- Findings: C1-024, X3-007 (merged)
- Solution: `otel.SetLoggerProvider`/`otel.GetLoggerProvider`; test that
  `NewOTLPWriter` resolves the provider set by `InitLogProvider`.
- Files: logger/logprovider.go, logger/otlp.go · Tests: provider resolution
- Regression risk: low · Compatibility: none · Effort: S · Depends on: — ·
  Cluster: C1 · Class: auto (**do first**: unblocks `task staticanalysis`)

### R-101 — Error-chain helpers
- Findings: C1-020
- Solution: `errorkit.As(err)` and `errorkit.HasCode(err, code)` on
  `errors.AsType`; AGENTS §4 and README examples use them.
- Files: errorkit/error.go, errorkit/README.md, AGENTS.md → R-153 ·
  Tests: wrapped chains
- Regression risk: low · Compatibility: additive API; changes the
  recommended idiom in AGENTS §4 · Effort: S · Depends on: — ·
  Cluster: C1 · Class: **approval**

### R-102 — Modernization sweeps (Go 1.26 only, after fixes)
- Findings: C1-025, C2-026, C3-014, C4-024, C5-018, C6-023, C6-018
- Solution: per cluster, one `refactor(<pkg>)` commit per category after the
  correctness items (JetBrains IDs listed in each finding); factor the
  shared JSON encoder in `marshal` (C6-018). Do not raise the `go` directive.
- Files: per finding · Tests: existing + `-race` · Risk: low · Compat: none
  · Effort: S per cluster · Depends on: all other items of the cluster ·
  Cluster: C1, C2, C3, C4, C5, C6 (each worker does its own part) ·
  Class: auto

### R-103 — `slices` package details
- Findings: C6-020
- Solution: preallocate, use `min`, document the size contract; note the
  stdlib name collision in AGENTS §0; `Chunk(size<=0)` behavior documented.
- Files: slices/*.go, slices/README.md, AGENTS.md → R-153 · Tests:
  preallocation benchmark, `size<=0` documented case · Risk: low ·
  Compat: none (no rename) ·
  Effort: S · Depends on: R-036 · Cluster: C6 · Class: **approval**
  (finding marks AGENTS/contract text)

### R-104 — `envs` API
- Findings: C6-019
- Solution: either fix AGENTS (no `Load[T]`) or add `Load[T any]() (T,
  error)` and `LoadWithOptions` (additive).
- Files: envs/loader.go, AGENTS.md → R-153 · Tests: `t.Setenv`
- Risk: low · Compat: additive or doc change · Effort: S · Depends on: — ·
  Cluster: C6 · Class: **approval**

### R-105 — Implicit `RABBITMQ_TOPOLOGY_FILE` read
- Findings: C5-015
- Solution: add `NewPubSubFromEnv` and deprecate the fallback, or amend
  AGENTS §2.4 with this documented exception.
- Files: messaging/messaging.go, AGENTS.md → R-153 · Tests: `t.Setenv`
  for `NewPubSubFromEnv` · Risk: low · Compat: deprecation
  · Effort: S · Depends on: R-067 · Cluster: C5 · Class: **approval**

### R-106 — Messaging span conventions
- Findings: C5-017
- Solution: align names with `telemetry` (or document the exemption);
  current semconv keys (optionally both for one release).
- Files: messaging/messaging.go · Tests: tracetest names/attributes
- Risk: medium (dashboards) · Compat: span names/attributes change ·
  Effort: S · Depends on: R-113 decision · Cluster: C5 · Class: **approval**

### R-112 — Tracing wrappers record failures (auto part)
- Findings: C2-018, X1-003 (merged), C4-006
- Solution: shared `telemetry.End(span, err)` (additive) recording error +
  status and errorkit code/retriable attributes; client span kind;
  `db.system` attribute on postgres spans. Span **names** are not changed
  here (postgres regex → R-113).
- Files: telemetry/helpers.go, httprequest/tracing.go, llmclient/tracing.go,
  mongo/tracing.go, postgres/tracing.go · Tests: tracetest status on error
- Risk: low · Compat: span status/attributes added · Effort: M ·
  Depends on: R-030, R-042 · Cluster: X (post-merge) · Class: auto

### R-113 — Bounded span names (approval part)
- Findings: C2-018 / X1-004 (merged), C4-006 (span-name part)
- Solution: route names through `telemetry.External/DB` (or new
  `HTTP`/`LLM` helpers); HTTP name = method + host or caller route template;
  path as attribute; postgres name regex for schema-qualified and `CALL`
  statements (C4-006, documented in postgres/README.md and
  postgres/tracing.go); announce in release notes.
- Files: same as R-112 · Tests: names bounded
- Regression risk: medium (dashboards/alerts) · Compatibility: span-name
  contract change · Effort: S · Depends on: R-112 · Cluster: X
  (post-merge) · Class: **approval**

### R-114 — gRPC public-endpoint tracing option
- Findings: C3-011
- Solution: `WithServerPublicEndpoint()` and optional
  `WithServerOTelOptions(...)` forwarding to otelgrpc.
- Files: grpc/options.go, grpc/server.go, README · Tests: incoming
  traceparent not parented when set
- Risk: low · Compat: additive · Effort: S · Depends on: — · Cluster: C3 ·
  Class: auto

### R-115 — gRPC recovery `ErrorInfo` and domain option
- Findings: C3-015
- Solution: build the recovery status through `ToStatus`; domain option;
  `id`/`retriable` in `ErrorInfo.Metadata`.
- Files: grpc/interceptors.go, grpc/status.go · Tests: details present
- Regression risk: low · Compatibility: additive details · Effort: S ·
  Depends on: R-003, R-004 · If R-004 is rejected: add the `ErrorInfo`
  details and domain option on top of today's `ToStatus` message policy ·
  Cluster: C3 · Class: auto

## 5. Testing

### R-120 — `errorkit`/`logger`/`telemetry` tests
- Findings: C1-014
- Solution: `-race` tests for setters, `New` global-state test, `OTLPWriter`
  against a fake `log.Logger`, redaction and `DetectEnv` tables, ULID
  round-trips, `b.Loop()` benchmarks with `ReportAllocs`.
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: errorkit, logger, telemetry `*_test.go` · Regression risk: none
  (tests only) · Compatibility: none · Effort: M · Depends on: R-050, R-007
  · Cluster: C1 · Class: auto

### R-121 — `httprequest` tests
- Findings: C2-019
- Solution: status→code→retriable and kind→decoder tables; attempt counts
  with `httptest`; synctest backoff/cancellation; audit-log capture; fuzz
  targets for parsers.
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: httprequest/*_test.go · Regression risk: none (tests only) ·
  Compatibility: none · Effort: M · Depends on: R-014, R-015 ·
  If R-016/R-032/R-072 are rejected: tests assert today's documented
  behavior for those paths · Cluster: C2 · Class: auto

### R-122 — Hertz adapter and `llmclient` tests
- Findings: C2-020
- Solution: `ut.PerformRequest` tests (routing, groups, middleware order,
  NoRoute/NoMethod, panic → 500, audit line, Reply JSON, BindJSON); one
  loopback `Run`/`Shutdown` test; llmclient tracing constructors.
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: httpserver/hertz/*_test.go, llmclient/*_test.go · Regression
  risk: none (tests only) · Compatibility: none · Effort: M ·
  Depends on: R-017 (ordering) · If R-017 is rejected: the lifecycle test
  covers `Run`/`Shutdown` without the bind-error case · Cluster: C2 ·
  Class: auto

### R-123 — gRPC tests and hygiene
- Findings: C3-012 (auto part), C3-013
- Solution: tracetest span assertions per call mode, recovery-wraps-
  interceptor test, client interceptor test, over-the-wire `ToStatus` test,
  replace private-field test; per-test deadlines, `wg.Go`, `range n`,
  `t.Cleanup`, `t.Parallel()`. `goleak` is **not** added (new dependency).
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: grpc/*_test.go · Regression risk: none (tests only) ·
  Compatibility: none · Effort: M · Depends on: R-003, R-004 (ordering) ·
  If R-004 is rejected: the over-the-wire test asserts today's message
  policy · Cluster: C3 · Class: auto

### R-124 — Postgres, Mongo and storage unit tests
- Findings: C4-009 (auto part), C4-020 (auto part), C4-023
- Solution: error-matrix tables, `rows.closed` assertions, argument order;
  `WrapOperationError`/`WrapDecodeError` tables and adapter tests on driver
  in-memory constructors; storage conformance suite and exact-count
  concurrency assertions.
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: postgres, mongo, storage `*_test.go` · Regression risk: none
  (tests only) · Compatibility: none · Effort: M · Depends on: R-018,
  R-027, R-054 (ordering) · If R-018/R-028/R-031b are rejected: the tables
  assert today's codes · Cluster: C4 · Class: auto

### R-125 — Integration suites with testcontainers
- Findings: C4-009 (approval part), C4-020 (approval part), C5-012
  (approval part)
- Solution: `//go:build integration` suites for Postgres, Mongo and
  RabbitMQ via testcontainers, separate Task target.
- Files: new *_integration_test.go, Taskfile.yml, go.mod, go.sum ·
  Tests: the suites themselves · Regression risk: none for consumers
  (test-only requirement still enters go.mod) · Compatibility: new
  dependency + tooling · Effort: L · Depends on: P4.XP ·
  Cluster: R-DEPS (runs with R-DEPS, after P4.XP) · Class: **approval**

### R-126 — Messaging unit tests
- Findings: C5-012 (auto part)
- Solution: `buildQueueArgs` and carrier tables; `handleDelivery` with fake
  Acknowledger (success, retries, shutdown, panic); synctest for
  `reconnectWithBackoff`.
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: messaging/*_test.go · Regression risk: none (tests only) ·
  Compatibility: none · Effort: M · Depends on: R-023, R-024 · Cluster: C5
  · Class: auto

### R-127 — Utility package tests
- Findings: C6-008, C6-010, C6-021, C6-017 (auto part)
- Solution: validators tables asserting `.Code()` + fuzz; envs `t.Setenv`
  table; commit the clean fuzz targets and `Example` functions; a test that
  fails if a non-test package imports `ioutils`; `SaveJSON` asserts content.
- Tests: this item is the test work itself; each new test must fail on the pre-fix code where it covers a fixed defect
- Files: validators, envs, urlkit, slices, stringutils, marshal, ioutils
  `*_test.go` · Regression risk: none (tests only) · Compatibility: none ·
  Effort: M · Depends on: R-046 (ordering) · If R-046 is rejected: the
  validator tables assert today's messages and codes · Cluster: C6 ·
  Class: auto

## 6. Performance

### R-130 — Cheap redaction path
- Findings: C1-003
- Solution: `bytes.Contains` pre-filter on the key list; only parse strings
  starting with `{`/`[`; cache the fallback logger; benchmarks committed.
- Files: logger/config.go · Tests: benchmarks + redaction tables unchanged
- Risk: low · Compat: none · Effort: M · Depends on: R-007 · Cluster: C1 ·
  Class: auto

### R-131 — Stack capture with `runtime.Callers`
- Findings: C1-007, C1-015
- Solution: one `runtime.Callers` into a fixed array + `CallersFrames`
  (lazy resolve optional); skip errorkit frames by function prefix; remove
  the hard-coded `market-ecosystem/` path root (module-relative trimming).
- Files: errorkit/stack.go · Tests: benchmark; first frame is the caller
- Regression risk: low · Compatibility: file paths in traces lose the
  foreign `market-ecosystem/` root (undocumented) · Effort: S ·
  Depends on: — · Cluster: C1 · Class: auto

### R-132 — Stack-less errors for validation paths
- Findings: C6-009
- Solution: errorkit option/constructor without stack capture for
  validation codes, used by `validators`; or document the cost.
- Files: errorkit/error.go, validators/validators.go · Tests: benchmark
- Risk: low · Compat: changes "stack trace on every error" contract ·
  Effort: S · Depends on: R-131 · Cluster: X (post-merge) ·
  Class: **approval**

### R-133 — No double buffering of decoded responses
- Findings: C2-008
- Solution: tee only when audit is on and the redactor needs content.
- Files: httprequest/httprequest.go · Tests: alloc benchmark
- Regression risk: low · Compatibility: none · Effort: S · Depends on: R-049
  (ordering only; independent if R-049 is rejected) · Cluster: C2 ·
  Class: auto

### R-134 — `FileStorage` append cost
- Findings: C4-017
- Solution: JSON Lines with `O_APPEND` (reader for the old array format), or
  at least drop `MarshalIndent` and the extra `Stat`; benchmark with
  benchstat.
- Files: storage/file_storage.go · Tests: benchmark, old-format read
- Risk: medium · Compat: on-disk format change · Effort: M ·
  Depends on: R-054 · Cluster: C4 · Class: **approval**

### R-135 — `NormalizeText` without per-call regexp
- Findings: C6-007
- Solution: single-pass loop (or package-level regexp); fix README sentence.
- Files: stringutils/normalize.go · Tests: benchmark + existing table
- Risk: low · Compat: none · Effort: S · Depends on: R-033 · Cluster: C6 ·
  Class: auto

## 7. Documentation and maintainability

### R-140 — CI on push and pull request
- Findings: X2-001, X3-003 (merged)
- Solution: `ci.yml` (push to main + PR, read-only permissions, SHA-pinned
  actions): build+vet+`go test -race -shuffle=on -short`; staticcheck/
  semgrep; pinned govulncheck with the patched toolchain (also scheduled);
  `go mod tidy -diff` + `go mod verify`; informational gorelease on PRs.
- Files: .github/workflows/ci.yml · Risk: none for consumers · Compat: new
  tooling · Effort: M · Depends on: R-141, R-142, R-100 · Cluster: X ·
  Class: **approval**
- Tests: workflow runs green on a PR (actionlint locally) · Regression risk: none for consumers · Compatibility: new CI tooling

### R-141 — Static-analysis gate runs from a fresh clone
- Findings: X2-002
- Solution: (a) pin the skill source in `.skillshare/config.yaml` and make
  `tools:install` install it, or (b) commit the runner under `scripts/`
  and point Taskfile, pre-commit and SECURITY.md at it.
- Files: .skillshare/config.yaml or scripts/, Taskfile.yml,
  .pre-commit-config.yaml, SECURITY.md · Effort: S · Depends on: — ·
  Cluster: X · Class: **approval**
- Tests: fresh clone → `task tools:install && task staticanalysis` runs · Regression risk: low · Compatibility: none for consumers (tooling)

### R-142 — Runner fails when its tools fail
- Findings: X2-003
- Solution: non-zero exit on tool crash or missing tool unless
  `QG_SKIP_*`; report which semgrep path ran. Fix upstream in
  `trypanic/skills` and pin the new ref (or in the committed runner, R-141b).
- Files: staticanalysis.sh (upstream or scripts/) · Effort: S ·
  Depends on: R-141 · Cluster: X · Class: **approval**
- Tests: fake tool shims (crash, missing) → non-zero exit · Regression risk: low (gate may turn red where it silently passed) · Compatibility: none for consumers (tooling)

### R-143 — Release verification before publishing
- Findings: X2-004
- Solution: rely on R-140 being required before merging the release PR;
  keep the post-release job as smoke; optional `workflow_dispatch`.
- Files: .github/workflows/release.yml · Effort: S · Depends on: R-140 ·
  Cluster: X · Class: **approval**
- Tests: n/a (workflow config) · Regression risk: low · Compatibility: none for consumers (release process)

### R-144 — Hook coverage and semgrep test scope
- Findings: X2-005, X2-006
- Solution: hook honors `QG_FULL=1` under `--all-files` (or a manual-stage
  full hook), `--diff-filter=ACMR`; explicit decision on scanning tests
  (`.semgrepignore` or documented exclusion) and fix/suppress the
  `postgres/stored_procedure_test.go:47` hit.
- Files: .pre-commit-config.yaml, Taskfile.yml, .semgrepignore, SECURITY.md
- Effort: S · Depends on: R-141 · Cluster: X · Class: **approval**
- Tests: `pre-commit run --all-files` exercises the full scan · Regression risk: low · Compatibility: none for consumers (tooling)

### R-145 — Toolchain policy (docs + CI part)
- Findings: X2-011, X3-002 (merged), X3-001 (HIGH, docs/CI part)
- Solution: document supported toolchains (latest 1.26.x ≥ 1.26.10 or
  1.27.x) in README/SECURITY; CI matrix {latest 1.26.x, latest 1.27.x};
  optional `task tools:check`; the `go.mod` part is R-DEPS.
- Files: README.md, SECURITY.md, ci.yml, Taskfile.yml · Effort: S ·
  Depends on: R-140 · Cluster: X · Class: **approval**
- Tests: CI matrix green on both toolchains · Regression risk: low · Compatibility: documents a supported-toolchain policy (new consumer-facing statement)

### R-146 — Reproducible gate tools
- Findings: X2-012
- Solution: pin goimports, pre-commit, semgrep image digest and rule packs,
  proto plugins by tag/commit; `go run …@vX.Y.Z` (no `tool` directives, to
  keep consumers' go.mod clean).
- Files: Taskfile.yml, .prototools, .pre-commit-config.yaml, runner ·
  Effort: M · Depends on: R-141 · Cluster: X · Class: **approval**
- Tests: two runs on different machines give the same findings · Regression risk: low · Compatibility: none for consumers (tooling)

### R-147 — Dependabot
- Findings: X2-013
- Solution: `dependabot.yml` for gomod and github-actions (weekly, grouped
  minor/patch).
- Files: .github/dependabot.yml · Effort: S · Depends on: — · Cluster: X ·
  Class: **approval**
- Tests: n/a (config) · Regression risk: low (PR volume) · Compatibility: none for consumers

### R-148 — Dead tooling configuration
- Findings: X2-014, C6-017 (sentrux rule part)
- Solution: wire sentrux into `task check`/CI with updated rules (incl. an
  `ioutils` boundary) or drop it; fix or remove `.env.shared`; remove
  "(was moon …)" leftovers.
- Files: .prototools, .sentrux/rules.toml, Taskfile.yml · Effort: S ·
  Depends on: — · Cluster: X · Class: **approval**
- Tests: `task check` runs sentrux or it is gone from `.prototools` · Regression risk: low · Compatibility: none for consumers (tooling)

### R-149 — Ignore `.moon/`
- Findings: X2-015
- Solution: add `.moon/` to `.gitignore`; precondition message shows
  `git status --porcelain`. (Local cache already deleted on 2026-11-04.)
- Files: .gitignore, Taskfile.yml · Effort: S · Depends on: — · Cluster: X ·
  Class: auto
- Tests: n/a (config) · Regression risk: none · Compatibility: none

### R-150 — Local gates match CI
- Findings: X2-016
- Solution: `test:race`, `vuln`, `tidy:check` tasks (pinned); `task check`
  calls the CI commands; golangci-lint stays informational.
- Files: Taskfile.yml · Effort: S · Depends on: R-140, R-146 · Cluster: X ·
  Class: **approval**
- Tests: `task check` runs the CI commands · Regression risk: low · Compatibility: none for consumers (tooling)

### R-151 — SECURITY.md accuracy and reporting policy
- Findings: X2-007, X2-008, X3-010 (triage note)
- Solution: describe what exists (after R-140/R-141 decisions); add
  "Reporting a vulnerability" and "Supported versions"; record the
  GO-2026-5932 "not affected" triage; move runner docs to CONTRIBUTING.md
  if preferred.
- Files: SECURITY.md, README.md, llms.txt, AGENTS.md → R-153 · Tests: n/a
  (docs) · Regression risk: none · Compatibility: adds a supported-versions
  policy and a reporting channel (owner's choice; GitHub private reporting
  proposed) · Effort: S · Depends on: R-141 · Cluster: X ·
  Class: **approval**

### R-152 — LICENSE
- Findings: X2-009
- Solution: add the licence chosen by the owner; README line; fix AGENTS.md:14.
- Files: LICENSE, README.md, AGENTS.md → R-153 · Effort: S · Depends on: — ·
  Cluster: X · Class: **approval** (owner must choose the licence)
- Tests: n/a (legal file) · Regression risk: none · Compatibility: licence terms become explicit

### R-153 — Root documentation corrections
- Findings: X2-010, X1-015
- Solution: correct README/AGENTS/llms.txt statements the code contradicts;
  restate the "always `AppError`" guarantee with its exceptions; keep
  intended-behavior questions with their C/X items (e.g. R-048, R-104).
- Files: README.md, AGENTS.md (sole editor, see "Shared documents"),
  llms.txt · Effort: M · Depends on: decisions on R-048, R-104, R-105 and
  every item listing `AGENTS.md → R-153` · If R-153 is rejected: apply
  only the AGENTS.md wording of the approved items that list
  `AGENTS.md → R-153`, and none of the root README/llms.txt corrections ·
  Cluster: X (post-merge) · Class: **approval**
- Tests: n/a (docs) · Regression risk: none · Compatibility: documented contracts change to match the code and the approved items; this item also applies every `AGENTS.md → R-153` edit listed by other items

### R-154 — Package documentation per cluster
- Findings: C1-022, C1-023, C2-027, C3-010, C4-008, C5-016, C6-022
- Solution: per cluster: `doc.go`/package comments, exported-symbol docs,
  README fixes and corrected samples (gRPC loops and `Shutdown` ctx,
  Quick Start clients, wrapper examples), removal of other-project
  references.
- Files: per finding · Effort: S per cluster · Depends on: the cluster's
  code items · Cluster: C1, C2, C3, C4, C5, C6 · Class: auto
- Tests: `go vet` + doc examples compile · Regression risk: none · Compatibility: none (docs)

### R-155 — GoDoc coverage and examples (cross-cutting)
- Findings: X1-013
- Solution: `doc.go` for every remaining package (grpc style), import-alias
  note for `database`/`mongodb`, `example_test.go` for main entry points.
- Files: all packages · Effort: M · Depends on: R-154 · Cluster: X
  (post-merge) · Class: auto
- Tests: `go test` runs the examples · Regression risk: none · Compatibility: none (docs)

### R-156 — Release notes wording and consumer README
- Findings: X2-017, X2-019
- Solution: "release notes generated by release-please" wording; README
  Install / Quick start / Compatibility sections, identifier column,
  CONTRIBUTING.md. `CHANGELOG.md` itself is not edited (§3).
- Files: README.md, llms.txt, CONTRIBUTING.md, AGENTS.md → R-153 · Tests:
  n/a (docs) · Regression risk: none · Compatibility: new README
  Compatibility section restates the R-145 policy · Effort: S ·
  Depends on: R-145 · Cluster: X · Class: **approval**

### R-157 — Release provenance and tidy checks
- Findings: X2-018
- Solution: `go mod verify` + `go mod tidy -diff` in the release verify
  step; consider attestations for `checksums.txt`.
- Files: .github/workflows/release.yml · Effort: S · Depends on: R-140 ·
  Cluster: X · Class: **approval**
- Tests: release workflow dry run · Regression risk: low · Compatibility: none for consumers (release process)

### R-158 — `deps:outdated` sees all requirements
- Findings: X3-011
- Solution: `-test` in per-package `go list -deps` plus a whole-module
  `go list -m -u … all` pass labelling linked vs. graph-only modules.
- Files: Taskfile.yml · Effort: S · Depends on: — · Cluster: X ·
  Class: auto (fixes an existing task; no new tool)
- Tests: `task deps:outdated` reports the known outdated test/graph-only modules · Regression risk: none · Compatibility: none (tooling)

### R-159 — Hertz OTel contrib module (deferred)
- Findings: X3-009
- Solution: explicit deferral — monitor; if it breaks or gets an advisory,
  replace with an in-repo middleware on the OTel API.
- Cluster: X · Class: **approval** (only if replacement is triggered)
- Files: httpserver/hertz/server.go (only if triggered) · Tests: n/a (deferred) · Regression risk: n/a (deferred) · Compatibility: n/a (deferred) · Effort: M if triggered · Depends on: an upstream break or advisory

## 8. R-DEPS (runs alone after P4.XP)

### R-DEPS — `go.mod` changes (incl. the toolchain part of HIGH X3-001)
- Findings: X3-001 (HIGH, toolchain part), X2-011/X3-002 (toolchain line),
  X3-010 (x/crypto bump when a tagged fix exists), plus any approved new
  requirement (R-125 testcontainers; R-082 if a split is approved)
- Solution: add `toolchain go1.27.3` (or latest patched 1.26.x) and/or set
  `go-version` explicitly in `release.yml`; keep `go 1.26.1`; bump
  `golang.org/x/crypto` only to a tagged release containing the fix; add
  approved test-only requirements.
- Files: go.mod, go.sum, .github/workflows/release.yml · Tests: §6 V2–V8
- Regression risk: low · Compatibility: `toolchain` does not affect
  consumers · Effort: S · Depends on: P4.XP, R-145 · Cluster: R-DEPS ·
  Class: **approval**

## 9. Status table

The human approves each `approval` item by replacing `PENDING` with
`APPROVED` or `REJECTED` (§5 P3 of `audit.md`). `auto` items become
eligible once G1 is `DONE`. Workers fill `Status` and `Commit` in P4.

| Item | Class | Cluster | Title | Approval | Status | Commit |
| --- | --- | --- | --- | --- | --- | --- |
| R-001 | approval | C1 | Client-safe error serialization in `errorkit` | PENDING | TODO | |
| R-002 | approval | C2 | HTTP replies must not carry internal error text | PENDING | TODO | |
| R-003 | auto | C3 | gRPC recovery returns a fixed message | — | TODO | |
| R-004 | approval | C3 | `ToStatus` hides internal reasons | PENDING | TODO | |
| R-005 | auto | C4 | Postgres pool errors: wrap the cause, never the raw driver error | — | TODO | |
| R-006 | approval | C2 | Sanitize URLs and provider messages in `httprequest` | PENDING | TODO | |
| R-007 | auto | C1 | Wider log redaction in `logger` | — | TODO | |
| R-008 | auto | C2 | Single, wider sensitive-header list | — | TODO | |
| R-009 | approval | C6 | `urlkit.BuildURL` treats `path` as a path | PENDING | TODO | |
| R-010 | approval | C6 | `urlkit.JoinPath` dot segments and scheme | PENDING | TODO | |
| R-011 | auto | C3 | gRPC transport credentials (auto part) | — | TODO | |
| R-012 | approval | C3 | gRPC insecure default (approval part) | PENDING | TODO | |
| R-013 | approval | C2 | Inbound audit middleware: implement or remove | PENDING | TODO | |
| R-014 | auto | C2 | `MaxRetries < 1` must still send one request | — | TODO | |
| R-015 | auto | C2 | Response decoding without panics | — | TODO | |
| R-016 | approval | C2 | Idempotency-aware retries | PENDING | TODO | |
| R-017 | approval | C2 | Hertz `Run()` returns bind errors | PENDING | TODO | |
| R-018 | approval | C4 | Postgres error classifier | PENDING | TODO | |
| R-019 | approval | C3 | gRPC code fallback from errorkit metadata | PENDING | TODO | |
| R-020 | approval | X (post-merge) | Transport status helpers driven by metadata | PENDING | TODO | |
| R-021 | approval | C3 | `ToStatus` maps context errors | PENDING | TODO | |
| R-022 | approval | C5 | Reliable publishing | PENDING | TODO | |
| R-023 | auto | C5 | Shutdown requeues in-flight messages | — | TODO | |
| R-024 | auto | C5 | Recover handler panics | — | TODO | |
| R-025 | auto | C5 | Messaging error codes preserved | — | TODO | |
| R-025b | approval | C5 | Specific AMQP error codes and reconnect stop | PENDING | TODO | |
| R-026 | approval | C5 | Topology validation and backoff guards | PENDING | TODO | |
| R-027 | auto | C4 | Mongo decode errors per README (auto part) | — | TODO | |
| R-028 | approval | C4 | Mongo error classification (approval part) | PENDING | TODO | |
| R-029 | approval | C4 | `FileStorage.Clear` deletes only owned files | PENDING | TODO | |
| R-030 | auto | C1 | `logger` handles wrapped and nil errors; `RecordError(nil)` | — | TODO | |
| R-031 | auto | C4 | `FileStorage.Append(nil)` returns an error | — | TODO | |
| R-031b | approval | C4 | Specific storage error codes | PENDING | TODO | |
| R-032 | approval | C2 | `httprequest` error classification | PENDING | TODO | |
| R-033 | auto | C6 | `RemoveMarkdownCodeBlock` correctness | — | TODO | |
| R-034 | auto | C6 | `NewExponentialBackoff` resets before use | — | TODO | |
| R-035 | approval | C6 | `algorithms` zero config and hidden `MaxElapsedTime` | PENDING | TODO | |
| R-036 | auto | C6 | `slices.Chunk` clips capacity | — | TODO | |
| R-037 | approval | C1 | OTLP timestamp precision | PENDING | TODO | |
| R-038 | approval | C1 | `DetectEnv` defaults and matching | PENDING | TODO | |
| R-039 | auto | C1 | OTLP log provider configuration and errors | — | TODO | |
| R-040 | auto | C1 | `errorkit` low-severity correctness | — | TODO | |
| R-041 | auto | C4 | Postgres DSN normalization | — | TODO | |
| R-042 | auto | C4 | `WrapWithInstrumenter(nil)` disables tracing | — | TODO | |
| R-043 | approval | C2 | `Reply` nil data with `omitempty` | PENDING | TODO | |
| R-044 | auto | C2 | Configurable `/health` path | — | TODO | |
| R-045 | auto | C6 | Utility low-severity correctness | — | TODO | |
| R-046 | approval | C6 | `validators` consistency | PENDING | TODO | |
| R-047 | auto | C2 | LLM API key masked when printed | — | TODO | |
| R-047b | approval | C2 | LLM endpoint and key validation | PENDING | TODO | |
| R-048 | auto | C2 | `llmclient` `*FromEnv` constructor | — | TODO | |
| R-049 | approval | C2 | Response size limits | PENDING | TODO | |
| R-050 | auto | C1 | `logger.New` stops mutating zerolog globals | — | TODO | |
| R-051 | approval | C1 | `errorkit` globals behind atomics | PENDING | TODO | |
| R-052 | auto | C1 | `OverrideMetadata` without lock re-entry | — | TODO | |
| R-053 | auto | C1 | `InitTracer` synchronization | — | TODO | |
| R-054 | auto | C4 | Safe `FileStorage` writes | — | TODO | |
| R-055 | approval | C4 | Storage file permissions | PENDING | TODO | |
| R-056 | auto | C2 | Hertz `Shutdown(ctx)` honors the context | — | TODO | |
| R-057 | approval | C2 | `EnableRecovery` controls recovery | PENDING | TODO | |
| R-058 | approval | C2 | End-to-end bound on LLM calls | PENDING | TODO | |
| R-059 | auto | C3 | gRPC `MaxConcurrentStreams` option | — | TODO | |
| R-060 | auto | C3 | Recovery outermost with raw options | — | TODO | |
| R-061 | auto | C3 | gRPC client default timeout | — | TODO | |
| R-062 | approval | C5 | Safe default prefetch | PENDING | TODO | |
| R-063 | auto | C5 | Ack/Nack failures visible | — | TODO | |
| R-064 | auto | C5 | `Publish` honors ctx; no dial under the mutex | — | TODO | |
| R-065 | approval | C5 | Terminal `Close` | PENDING | TODO | |
| R-066 | approval | C5 | Per-key publish retry, single retry layer | PENDING | TODO | |
| R-067 | approval | X (post-merge) | Cancellable network constructors | PENDING | TODO | |
| R-068 | approval | C4 | Mongo option precedence | PENDING | TODO | |
| R-069 | auto | C4 | Mongo cursor spans and ping budget | — | TODO | |
| R-070 | auto | C5 | AMQP connection options | — | TODO | |
| R-071 | auto | C5 | Checked `Close` errors in messaging | — | TODO | |
| R-072 | approval | C2 | Jittered backoff and `Retry-After` | PENDING | TODO | |
| R-080 | auto | X (post-merge of C1, C2, C3) | `httprequest`/`llmclient` stop importing `logger` (step 1) | — | TODO | |
| R-081 | approval | X (post-merge) | OTLP exporter in its own package (step 2) | PENDING | TODO | |
| R-082 | approval | R-DEPS | Module strategy (single module vs. nested modules) | PENDING | TODO | |
| R-083 | approval | X (post-merge) | One tracing injection story | PENDING | TODO | |
| R-084 | auto | C5 | Messaging publisher uses the injected logger | — | TODO | |
| R-085 | approval | C2 (policy text in X) | No log-and-return in library code | PENDING | TODO | |
| R-086 | approval | X (post-merge) | Extensible `httpserver` routing | PENDING | TODO | |
| R-087 | approval | X (post-merge) | Adapter escape hatches | PENDING | TODO | |
| R-088 | auto | X (post-merge) | Hertz functional options | — | TODO | |
| R-089 | approval | X (post-merge) | SDK-owned `Backoff` interface | PENDING | TODO | |
| R-090 | approval | X (post-merge) | One internal backoff implementation | PENDING | TODO | |
| R-091 | approval | X (post-merge) | Published test doubles | PENDING | TODO | |
| R-092 | auto | X (post-merge) | `ERR_EXTERNAL_*` codes owned by `errorkit` | — | TODO | |
| R-093 | approval | X | Package identifiers (v2) | PENDING | TODO | |
| R-094 | approval | X | Naming cleanup (v2) | PENDING | TODO | |
| R-095 | auto | C2 | `RequestLoggerMiddleware` installable and request-ID aware | — | TODO | |
| R-096 | auto | C3 | gRPC `ListenAndServe` | — | TODO | |
| R-097 | approval | C4 | Mongo result API | PENDING | TODO | |
| R-100 | auto | C1 | Replace deprecated `otel/log/global` (gate is red) | — | TODO | |
| R-101 | approval | C1 | Error-chain helpers | PENDING | TODO | |
| R-102 | auto | C1, C2, C3, C4, C5, C6 (each worker does its own part) | Modernization sweeps (Go 1.26 only, after fixes) | — | TODO | |
| R-103 | approval | C6 | `slices` package details | PENDING | TODO | |
| R-104 | approval | C6 | `envs` API | PENDING | TODO | |
| R-105 | approval | C5 | Implicit `RABBITMQ_TOPOLOGY_FILE` read | PENDING | TODO | |
| R-106 | approval | C5 | Messaging span conventions | PENDING | TODO | |
| R-112 | auto | X (post-merge) | Tracing wrappers record failures (auto part) | — | TODO | |
| R-113 | approval | X (post-merge) | Bounded span names (approval part) | PENDING | TODO | |
| R-114 | auto | C3 | gRPC public-endpoint tracing option | — | TODO | |
| R-115 | auto | C3 | gRPC recovery `ErrorInfo` and domain option | — | TODO | |
| R-120 | auto | C1 | `errorkit`/`logger`/`telemetry` tests | — | TODO | |
| R-121 | auto | C2 | `httprequest` tests | — | TODO | |
| R-122 | auto | C2 | Hertz adapter and `llmclient` tests | — | TODO | |
| R-123 | auto | C3 | gRPC tests and hygiene | — | TODO | |
| R-124 | auto | C4 | Postgres, Mongo and storage unit tests | — | TODO | |
| R-125 | approval | R-DEPS (runs with R-DEPS, after P4.XP) | Integration suites with testcontainers | PENDING | TODO | |
| R-126 | auto | C5 | Messaging unit tests | — | TODO | |
| R-127 | auto | C6 | Utility package tests | — | TODO | |
| R-130 | auto | C1 | Cheap redaction path | — | TODO | |
| R-131 | auto | C1 | Stack capture with `runtime.Callers` | — | TODO | |
| R-132 | approval | X (post-merge) | Stack-less errors for validation paths | PENDING | TODO | |
| R-133 | auto | C2 | No double buffering of decoded responses | — | TODO | |
| R-134 | approval | C4 | `FileStorage` append cost | PENDING | TODO | |
| R-135 | auto | C6 | `NormalizeText` without per-call regexp | — | TODO | |
| R-140 | approval | X | CI on push and pull request | PENDING | TODO | |
| R-141 | approval | X | Static-analysis gate runs from a fresh clone | PENDING | TODO | |
| R-142 | approval | X | Runner fails when its tools fail | PENDING | TODO | |
| R-143 | approval | X | Release verification before publishing | PENDING | TODO | |
| R-144 | approval | X | Hook coverage and semgrep test scope | PENDING | TODO | |
| R-145 | approval | X | Toolchain policy (docs + CI part) | PENDING | TODO | |
| R-146 | approval | X | Reproducible gate tools | PENDING | TODO | |
| R-147 | approval | X | Dependabot | PENDING | TODO | |
| R-148 | approval | X | Dead tooling configuration | PENDING | TODO | |
| R-149 | auto | X | Ignore `.moon/` | — | TODO | |
| R-150 | approval | X | Local gates match CI | PENDING | TODO | |
| R-151 | approval | X | SECURITY.md accuracy and reporting policy | PENDING | TODO | |
| R-152 | approval | X | LICENSE | PENDING | TODO | |
| R-153 | approval | X (post-merge) | Root documentation corrections | PENDING | TODO | |
| R-154 | auto | C1, C2, C3, C4, C5, C6 | Package documentation per cluster | — | TODO | |
| R-155 | auto | X (post-merge) | GoDoc coverage and examples (cross-cutting) | — | TODO | |
| R-156 | approval | X | Release notes wording and consumer README | PENDING | TODO | |
| R-157 | approval | X | Release provenance and tidy checks | PENDING | TODO | |
| R-158 | auto | X | `deps:outdated` sees all requirements | — | TODO | |
| R-159 | approval | X | Hertz OTel contrib module (deferred) | PENDING | TODO | |
| R-DEPS | approval | R-DEPS | `go.mod` changes (incl. the toolchain part of HIGH X3-001) | PENDING | TODO | |

Totals: 139 items — 63 `auto`, 76 `approval`.

## 10. Coverage of findings

All 181 raw finding IDs (170 unique + 11 merged duplicates) map to at least one item; every HIGH finding has an item. Generated mapping:

| Finding | Item(s) |
| --- | --- |
| C1-001 | R-050 |
| C1-002 | R-001 |
| C1-003 | R-130 |
| C1-004 | R-007 |
| C1-005 | R-051 |
| C1-006 | R-052 |
| C1-007 | R-131 |
| C1-008 | R-030 |
| C1-009 | R-030 |
| C1-010 | R-039 |
| C1-011 | R-037 |
| C1-012 | R-038 |
| C1-013 | R-053 |
| C1-014 | R-120 |
| C1-015 | R-131 |
| C1-016 | R-040 |
| C1-017 | R-040 |
| C1-018 | R-040 (payload-in-`FormatPlain` part deferred; P5 reports it) |
| C1-019 | R-040 |
| C1-020 | R-101 |
| C1-021 | R-030 |
| C1-022 | R-154 |
| C1-023 | R-154 |
| C1-024 | R-100 |
| C1-025 | R-102 |
| C2-001 | R-017 |
| C2-002 | R-016 |
| C2-003 | R-014 |
| C2-004 | R-015 |
| C2-005 | R-013 |
| C2-006 | R-006 |
| C2-007 | R-049 |
| C2-008 | R-133 |
| C2-009 | R-085 |
| C2-010 | R-056 |
| C2-011 | R-057 |
| C2-012 | R-058 |
| C2-013 | R-032 |
| C2-014 | R-072 |
| C2-015 | R-048 |
| C2-016 | R-047, R-047b |
| C2-017 | R-002 |
| C2-018 | R-112, R-113 |
| C2-019 | R-121 |
| C2-020 | R-122 |
| C2-021 | R-008 |
| C2-022 | R-044 |
| C2-023 | R-032 |
| C2-024 | R-043 |
| C2-025 | R-095 |
| C2-026 | R-102 |
| C2-027 | R-154 |
| C3-001 | R-003 |
| C3-002 | R-004 |
| C3-003 | R-019, R-020 |
| C3-004 | R-021 |
| C3-005 | R-011, R-012 |
| C3-006 | R-059 |
| C3-007 | R-060 |
| C3-008 | R-061 |
| C3-009 | R-096 |
| C3-010 | R-154 |
| C3-011 | R-114 |
| C3-012 | R-123 |
| C3-013 | R-123 |
| C3-014 | R-102 |
| C3-015 | R-115 |
| C3-016 | R-083 |
| C4-001 | R-005 |
| C4-002 | R-018 |
| C4-003 | R-005 |
| C4-004 | R-067 |
| C4-005 | R-042 |
| C4-006 | R-112, R-113 |
| C4-007 | R-041 |
| C4-008 | R-154 |
| C4-009 | R-124, R-125 |
| C4-010 | R-027, R-028 |
| C4-011 | R-068 |
| C4-012 | R-054 |
| C4-013 | R-054 |
| C4-014 | R-029 |
| C4-015 | R-031 |
| C4-016 | R-054 |
| C4-017 | R-134 |
| C4-018 | R-031b |
| C4-019 | R-069 |
| C4-020 | R-124, R-125 |
| C4-021 | R-097 |
| C4-022 | R-055 |
| C4-023 | R-124 |
| C4-024 | R-102 |
| C5-001 | R-023 |
| C5-002 | R-022 |
| C5-003 | R-062 |
| C5-004 | R-024 |
| C5-005 | R-063 |
| C5-006 | R-025, R-025b |
| C5-007 | R-064 |
| C5-008 | R-065 |
| C5-009 | R-066 |
| C5-010 | R-026 |
| C5-011 | R-084 |
| C5-012 | R-125, R-126 |
| C5-013 | R-071 |
| C5-014 | R-070 |
| C5-015 | R-105 |
| C5-016 | R-154 |
| C5-017 | R-106 |
| C5-018 | R-102 |
| C6-001 | R-009 |
| C6-002 | R-010 |
| C6-003 | R-033 |
| C6-004 | R-034 |
| C6-005 | R-026 |
| C6-006 | R-036 |
| C6-007 | R-135 |
| C6-008 | R-127 |
| C6-009 | R-132 |
| C6-010 | R-127 |
| C6-011 | R-045 |
| C6-012 | R-035 |
| C6-013 | R-033 |
| C6-014 | R-045 |
| C6-015 | R-046 |
| C6-016 | R-045 |
| C6-017 | R-127, R-148 |
| C6-018 | R-102 |
| C6-019 | R-104 |
| C6-020 | R-103 |
| C6-021 | R-127 |
| C6-022 | R-154 |
| C6-023 | R-102 |
| X1-001 | R-080, R-081 |
| X1-002 | R-083 |
| X1-003 | R-112 |
| X1-004 | R-113 |
| X1-005 | R-086 |
| X1-006 | R-087 |
| X1-007 | R-020 |
| X1-008 | R-085 |
| X1-009 | R-067 |
| X1-010 | R-088 |
| X1-011 | R-089 |
| X1-012 | R-090 |
| X1-013 | R-155 |
| X1-014 | R-093 |
| X1-015 | R-153 |
| X1-016 | R-091 |
| X1-017 | R-094 |
| X1-018 | R-092 |
| X2-001 | R-140 |
| X2-002 | R-141 |
| X2-003 | R-142 |
| X2-004 | R-143 |
| X2-005 | R-144 |
| X2-006 | R-144 |
| X2-007 | R-151 |
| X2-008 | R-151 |
| X2-009 | R-152 |
| X2-010 | R-153 |
| X2-011 | R-145, R-DEPS |
| X2-012 | R-146 |
| X2-013 | R-147 |
| X2-014 | R-148 |
| X2-015 | R-149 |
| X2-016 | R-150 |
| X2-017 | R-156 |
| X2-018 | R-157 |
| X2-019 | R-156 |
| X3-001 | R-145, R-DEPS |
| X3-002 | R-145, R-DEPS |
| X3-003 | R-140 |
| X3-004 | R-082 |
| X3-005 | R-081 |
| X3-006 | R-080 |
| X3-007 | R-100 |
| X3-008 | R-089 |
| X3-009 | R-159 |
| X3-010 | R-151, R-DEPS |
| X3-011 | R-158 |
## 11. Recommended order inside P4

1. **C1 first item: R-100** (static-analysis gate green again), then R-050.
2. HIGH `auto` items in every cluster: R-003, R-005, R-011, R-014, R-015,
   R-023, R-050, R-054; R-080 in P4.XP.
3. HIGH `approval` items as approved: R-001, R-002, R-004, R-006, R-009,
   R-012, R-013, R-016, R-017, R-018, R-019, R-022, R-081, R-DEPS.
4. Phases: P4.C1–C6 and P4.X (root docs/tooling) in parallel → P4.M →
   **P4.XP** (X post-merge items, sequential) → R-DEPS (with R-125) → P5.
5. Remaining items by section order (1 → 7), modernization (R-102) and
   documentation (R-154, R-155) last in each cluster.
