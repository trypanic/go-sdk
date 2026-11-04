# v2 API migration

The audit intentionally removes compatibility aliases. The module is now
`github.com/trypanic/go-sdk/v2`. Update the requirement with
`go get github.com/trypanic/go-sdk/v2@latest` after the v2 release and rewrite
all SDK imports, including test doubles and `logger/otlp`.

| v1 import (historical) | v2 import |
| --- | --- |
| `github.com/trypanic/go-sdk/postgres` | `github.com/trypanic/go-sdk/v2/postgres` |
| `github.com/trypanic/go-sdk/slices` | `github.com/trypanic/go-sdk/v2/batch` |

The repository URL remains `https://github.com/trypanic/go-sdk`; `/v2` belongs
to the Go module path. The SDK remains a single module; only the packages a
binary imports are linked. The approved API names below are final.

| Previous API | Replacement |
| --- | --- |
| package `database` at `/postgres` | package `postgres` |
| package `mongodb` at `/mongo` | package `mongo` (alias the upstream driver when both are imported) |
| `/slices`, `slices.Chunk`, `slices.MergeUniqueStrings` | `/batch`, `batch.Chunk`, `batch.MergeUniqueStrings` |
| `httprequest.HTTPRequest`, `HTTPRequester` | `httprequest.Request`, `Requester` |
| `httprequest.NewHTTPRequestWithRetry(client, rc)` | `NewWithOptions(client, WithRetryConfig(rc), WithInstrumenter(instr))` |
| `httpserver.HTTPServer`, `HTTPContext` | `httpserver.Server`, `Context` |
| `llmclient.LLMProvider`, `LLMRequest`, `LLMRequestConfig`, `LLMResponse` | `llmclient.Provider`, `Request`, `RequestConfig`, `Response` |
| `postgres.NewPostgresClient`, `NewPostgresClientFromEnv` | `postgres.New`, `NewFromEnv` |
| `postgres.NewPostgresClientContext`, `NewPostgresClientFromEnvContext` | `postgres.NewContext`, `NewFromEnvContext` |
| `postgres.PostgresEnvVar` | `postgres.DSNEnvVar` |
| `storage.KVStorager`, `AppendLogStorager` | `storage.KVStore`, `AppendLog` |
| `messaging.Messager` | `messaging.Broker` |
| `messagingtest.Messager` | `messagingtest.Broker` |
| `envs.NewLoader(&cfg)` | `envs.LoadInto(&cfg)`; `Load[T]()` returns a typed value |
| logger OTLP exports | Import `logger/otlp` explicitly |
| `otlp.OTLPWriter`, `NewOTLPWriter` | `otlp.Writer`, `NewWriter` |
| `otlp.LogProviderHandle`, `LogProviderConfig` | `otlp.Provider`, `Config` |
| `otlp.NewLogProvider`, `InitLogProvider`, `SetupOTLP` | `otlp.NewProvider`, `InitProvider`, `Setup` |
| exported ANSI colour constants in `errorkit` and `logger` | Removed; use `errorkit.EnableColors` / `DisableColors` to control formatting |

HTTP, LLM, Mongo, Hertz and messaging constructors now expose concrete adapters.
Use interfaces at consumer boundaries; use `Unwrap`, `Engine`, or `Database`
for documented driver escape hatches. A Mongo client supplied to `NewFromClient`
is borrowed: its owner disconnects it. Router contracts include PATCH, HEAD,
OPTIONS and Handle directly.

`logger.Init` and `logger.Config.OTLPWriter` accept `io.Writer`; importing the
base logger no longer imports gRPC. Strict OTLP setup uses `otlp.NewProvider`
with TLS configuration. `InitProvider` and `Setup` retain their documented
never-failing compatibility behavior.

`algorithms.NewExponentialBackoff` returns the SDK-owned `algorithms.Backoff`.
Transport retry loops use bounded full jitter. Validation rejections retain
metadata and identity but omit stacks; configuration failures remain stackful.

Span selectors also change: HTTP uses `external.http.<host>.<METHOD>`, LLM
`external.llm.execute`, PostgreSQL `postgres.<routine>.query`, and Mongo client
lifecycle `mongo.client.<operation>`. Models and HTTP paths are attributes;
resource IDs do not enter operation names. Failed client spans record sanitized
catalogue descriptions, classification and retryability.

### Local integration infrastructure

Test-only infrastructure binds published ports to `127.0.0.1`, including Ryuk.
The pinned Testcontainers v0.44.0 helper uses its exported `ReaperProvider`
hook with one localized deprecation exception because there is no Ryuk
HostIP option. It preserves upstream session reuse and crash cleanup; this
is not a public SDK API. See [local integration tests](../CONTRIBUTING.md#local-infrastructure-integration-tests).
