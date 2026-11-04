# v2 API migration

The audit intentionally removes compatibility aliases. The module `/v2` suffix
is applied in the separate dependency phase; the names below are already final.

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
