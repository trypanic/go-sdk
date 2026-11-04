# Baseline — checks before any change (P0.2)

- Date: 2026-11-04T00:16Z
- Tree: branch `audit/2026-11` at `75882a7` (code identical to
  `chore/migrate-moon-to-taskfile` @ `d966f55`; only `docs/audit/audit.md`
  and `.claude/agents/` added). Untracked `.moon/` present in the main tree.
- Toolchain on `PATH`: `go1.26.2 linux/amd64`
  (`/home/trypanic/.proto/tools/go/1.26.2`). Note: `.prototools` pins
  `go = "1.27.3"` (installed, used by `proto run go`), and `go.mod` says
  `go 1.26.1`. All checks below ran with the PATH toolchain (1.26.2) unless
  stated.
- Other tools: task 3.54.0, staticcheck 2026.1 on PATH (proto pin 2026.2.2;
  the runner script resolved its own), semgrep 1.95.0, pre-commit 4.7.0,
  gcc 15.3.0, govulncheck v1.9.0, golangci-lint v2.15.0 (both via `go run`),
  gorelease `@latest` (via `task release:check`).

## Summary

| # | Check | Exit | Result |
| --- | --- | --- | --- |
| V1 | `gofmt -l .` | 0 | PASS — no output |
| V2 | `go build ./...` | 0 | PASS |
| V3 | `go vet ./...` | 0 | PASS |
| V4 | `task test` | 0 | PASS — 21 packages ok (most results `(cached)`), `grpc/internal/testsvc` has no tests |
| V5 | `go test -race -short ./...` | 0 | PASS — 21 packages ok, no races reported |
| V6 | coverage | 0 | total **47.7%** of statements (table below) |
| V7 | `task staticanalysis` | 201 (task) / 1 (script) | **FAIL** — staticcheck 4 errors (SA1019), vet 0, semgrep 0 |
| V8 | govulncheck (go1.26.2) | 3 (go run → 1) | **FAIL** — 19 reachable stdlib vulns fixed in later 1.26.x |
| V8b | govulncheck (go1.27.3, informational) | 0 | PASS — 0 reachable; 1 unreachable module vuln |
| V9 | golangci-lint (default linters) | 1 | informational — 25 issues |
| V10 | `task release:check` (clean worktree) | 0 | PASS — base `v1.0.0`, suggested `v1.1.0`, no incompatible changes |
| V11 | `task precommit:run` | 0 | PASS — all hooks passed; the `staticanalysis` hook skips itself ("no staged .go files"), so V11 does **not** exercise staticcheck |

## Details

### V6 — coverage per package

| Package | Coverage |
| --- | --- |
| algorithms | 100.0% |
| envs | 66.7% |
| errorkit | 60.0% |
| grpc | 77.2% |
| grpc/internal/testsvc | 0.0% (no tests) |
| httpclient | 57.1% |
| httprequest | 44.9% |
| httpserver | 75.8% |
| httpserver/hertz | 19.4% |
| ioutils | 68.8% |
| llmclient | 60.0% |
| logger | 52.7% |
| marshal | 87.5% |
| messaging | **10.5%** |
| mongo | 25.3% |
| postgres | 52.7% |
| slices | 100.0% |
| storage | 79.8% |
| stringutils | 100.0% |
| telemetry | 35.3% |
| urlkit | 70.3% |
| validators | **9.7%** |

### V7 — static analysis

Runner: `.skillshare/skills/go-code-quality-check/scripts/staticanalysis.sh`
(`QG_FULL=1`). Its banner printed `pkgs=1 files=1` even in full mode — to be
checked in P2.X2 whether full mode really scans the whole tree.

staticcheck (4 errors, all SA1019 deprecations):

- `logger/logprovider.go:9` — `go.opentelemetry.io/otel/log/global` deprecated
- `logger/logprovider.go:88` — `otelglobal.SetLoggerProvider` deprecated, use `otel.SetLoggerProvider`
- `logger/otlp.go:10` — `go.opentelemetry.io/otel/log/global` deprecated
- `logger/otlp.go:41` — `otelglobal.GetLoggerProvider` deprecated, use `otel.GetLoggerProvider`

semgrep: 0 findings. The `defer-rows-close` warnings at
`postgres/stored_procedure_test.go:47` listed as known pre-existing in §1
were **not** reproduced.

### V8 — govulncheck

With go1.26.2: "Your code is affected by 19 vulnerabilities from the Go
standard library" (+14 in imported packages and 3 in required modules not
called). All 19 are stdlib, found in `@go1.26.2`:

| Stdlib package | Count |
| --- | --- |
| net/http | 9 |
| crypto/tls | 3 |
| crypto/x509 | 2 |
| net/textproto | 2 |
| encoding/asn1 | 1 |
| net | 1 |
| net/url | 1 |

IDs: GO-2026-6809, -6617, -6613, -6612, -6611, -6610, -6608, -6607, -6605,
-6603, -6218, -6090, -5972, -5856, -5039, -5037, -5026, -4971, -4918.
Example trace: GO-2026-4918 via `httprequest/httprequest.go:245`
(`http.Client.Do`), fixed in `net/http@go1.26.3`.

With go1.27.3 (the `.prototools` pin): 0 reachable vulnerabilities; 1
vulnerability in a required module not called by the code.

Conclusion: these depend on the toolchain, not the module. Consumers
building with an outdated 1.26.x toolchain are affected; whether the SDK
should raise the `go`/`toolchain` directive is a P2.X3 question.

### V9 — golangci-lint (informational, default linters)

25 issues: errcheck 15, staticcheck 7, govet 2, ineffassign 1.

- errcheck (non-test): `httprequest/httperror.go:156`,
  `httprequest/httprequest.go:249`, `logger/wrapper.go:46`, `logger/wrapper.go:51`,
  `messaging/amqp.go:56`, `:70`, `:85`, `:259`, `:320`, `messaging/messaging.go:254`
- errcheck (tests): `grpc/roundtrip_test.go:292`, `:324`, `:382`,
  `httpclient/httpclient_test.go:115`, `:144`
- govet inline (`reflect.Ptr` → `reflect.Pointer`): `httprequest/decode.go:18`,
  `postgres/stored_procedure_test.go:172`
- ineffassign: `httpserver/hertz/server.go:155` (`redact = httpserver.DefaultBodyRedactor`)
- staticcheck: ST1023 at `httpserver/hertz/wrapper_test.go:34`, `logger/instance.go:54`;
  SA1019 ×4 (same as V7); QF1001 at `validators/validators.go:47`

### V10 — gorelease

Ran from a detached clean worktree (`../go-sdk-wt/baseline`, removed after).
`go run golang.org/x/exp/cmd/gorelease@latest -base=v1.0.0` → "Suggested
version: v1.1.0" (compatible additions since v1.0.0, no incompatible
changes).

### V11 — pre-commit

All hooks passed on all files, and no file was modified by the fixers
(`git status` was unchanged afterwards).

## Pre-existing issues carried into the audit

1. staticcheck SA1019 in `logger` (V7 fails on an untouched tree).
2. Reachable stdlib vulnerabilities when built with go ≤ 1.26.2 (V8).
3. 25 golangci-lint issues (V9), notably unchecked `Close` errors in
   `messaging` and `httprequest`, and an ineffectual assignment in
   `httpserver/hertz/server.go:155`.
4. Low coverage: `messaging` 10.5%, `validators` 9.7%, `httpserver/hertz` 19.4%.
5. The pre-commit `staticanalysis` hook is a no-op under `--all-files` with
   nothing staged.
