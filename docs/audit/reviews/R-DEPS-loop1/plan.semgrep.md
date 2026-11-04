# semgrep plan (full)

- Generated: 2026-11-04T05:51:11Z
- Runner: docker semgrep/semgrep:1.95.0@sha256:30e6afa99ebd8e7b4115d4904898108eb4bf77025819e9263f09cf14e6f6e549 (.semgrep.yml p/golang p/gosec p/secrets)
- Findings: **11** (errors=0, warnings=11, info=0)
- Report: `.staticanalysis/report.semgrep.json`

## Fixes

### httpclient/httpclient_test.go
- **[WARNING]** L68 — `go.lang.security.audit.crypto.missing-ssl-minversion.missing-ssl-minversion` — `MinVersion` is missing from this TLS configuration.  By default, as of Go 1.22, TLS 1.2 is currently used as the minimum. General purpose web applications should default to TLS 1.3 with all other protocols disabled.  Only where it is known that a web server must support legacy clients with unsupported an insecure browsers (such as Internet Explorer 10), it may be necessary to enable TLS 1.0 to provide support. Add `MinVersion: tls.VersionTLS13' to the TLS configuration to bump the minimum version to TLS 1.3.

### internal/backoff/backoff.go
- **[WARNING]** L5 — `go.lang.security.audit.crypto.math_random.math-random-used` — Do not use `math/rand`. Use `crypto/rand` instead.

### messaging/amqp.go
- **[WARNING]** L72 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L266 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L271 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L503 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L561 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.

### messaging/messaging.go
- **[WARNING]** L148 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L166 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L182 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
- **[WARNING]** L236 — `no-context-background-in-request-path` — Do not create context.Background() in request handlers. Propagate the caller's ctx.
