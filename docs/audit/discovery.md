# Discovery — github.com/trypanic/go-sdk (task P1)

- Date: 2026-11-03/04 (same session window as `baseline.md`).
- Tree: branch `audit/2026-11` at `3b38064` (code identical to `d966f55`,
  `chore/migrate-moon-to-taskfile`); untracked `.moon/` present.
- Toolchain used for every command below: `go1.26.2 linux/amd64` (PATH).
- Read-only task: no file other than this one was written in the repository.
  Temporary artifacts (`go doc` dumps, a semgrep JSON) went to the session
  scratchpad outside the repo.

## 0. Sources and method

Project sources read in full: `AGENTS.md` (on-disk version; it already
references `task`, not `moon`), `README.md`, `SECURITY.md`, `llms.txt`,
`Taskfile.yml`, `.prototools`, `.pre-commit-config.yaml`, `.semgrep.yml`,
`.sentrux/rules.toml`, `.skillshare/config.yaml`, `cliff.toml`,
`release-please-config.json`, `.release-please-manifest.json`,
`.github/workflows/release.yml`, `docs/audit/audit.md` (§1–§3, §5),
`docs/audit/baseline.md`, and the `README.md` of all 21 package directories
(`algorithms envs errorkit grpc httpclient httprequest httpserver
httpserver/hertz ioutils llmclient logger marshal messaging mongo postgres
slices storage stringutils telemetry urlkit validators`).

External orientation sources (go-agent-skills @ `e25b517bbd0c11db35f75a83a14f18848c1abdb0`,
verified with `git rev-parse HEAD`):

- `~/.cache/go-audit-sources/go-agent-skills/skills/(architecture)/go-project-layout/SKILL.md`
- `~/.cache/go-audit-sources/go-agent-skills/skills/(architecture)/go-architecture-review/SKILL.md`
- `~/.cache/go-audit-sources/go-agent-skills/skills/(architecture)/go-architecture-review/references/layout-and-wiring.md`

They were used only as orientation (library layout, dependency direction,
`init()` side effects); P1 records facts, not findings.

Commands run (all read-only): `go list ./...`,
`go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./...`,
`go list -f '... {{join .TestImports " "}}' ./...`,
`go list -deps -f '{{.Module.Path}}' <pkg>` per package,
`go doc -all <pkg>` for all 22 packages, `wc -l` per package, `grep` for
build tags / `testing.Short` / `t.Skip` / env reads / fakes,
`semgrep scan --config=.semgrep.yml postgres/` (output to scratchpad),
`git tag`, `git status --porcelain --ignored`, `git branch --contains`.

Mismatches between documentation and implementation are recorded as
**discovery notes `DN-NN`** (§10). They are not findings; each names the P2
task that should confirm or reject it.

---

## 1. Purpose of the SDK

An opinionated, single-module Go SDK (`github.com/trypanic/go-sdk`, released
`v1.0.0`) that gives backend services a uniform set of building blocks:

- classified, structured errors with stack traces and retry metadata
  (`errorkit`) used as the error type of every other package;
- structured logging on zerolog with OpenTelemetry trace correlation and an
  OTLP log exporter (`logger`), and a span factory with naming conventions
  (`telemetry`);
- outbound HTTP (`httpclient` transport factory, `httprequest` retrying
  requester), inbound HTTP contracts with a Hertz adapter (`httpserver`,
  `httpserver/hertz`) and gRPC server/client factories (`grpc`);
- data and messaging adapters: PostgreSQL pool + generic stored-procedure
  runner (`postgres`, package `database`), MongoDB adapter (`mongo`, package
  `mongodb`), RabbitMQ pub/sub driven by a JSON topology (`messaging`),
  in-memory/file storage (`storage`), an OpenAI-compatible chat client
  (`llmclient`);
- small utilities (`envs`, `urlkit`, `validators`, `marshal`, `slices`,
  `stringutils`, `algorithms`) and a dev-only JSON dumper (`ioutils`).

Its stated cross-cutting contract (AGENTS.md §2, README "Cross-cutting
concern policy"): errors are `*errorkit.AppError`; tracing constructor
triplet (`New` / `NewWithoutTracing` / `NewWithInstrumenter`); no package
mutates the global logger except `logger.Init`; no silent env reads; HTTP
bodies redacted by default; constructed values are safe for concurrent use;
lifecycle belongs to the caller.

README.md:3-5 also promises "metrics"; there is no metrics package. The only
metrics emitted are by `otelgrpc` to the global `MeterProvider`
(grpc/README.md "Observability note"). See DN-28.

---

## 2. Package roles

22 packages in one module (20 top-level directories + `httpserver/hertz` +
`grpc/internal/testsvc`). 21 are importable by consumers.

| Package (import path suffix) | Pkg name | Role | Status (README/AGENTS) |
| --- | --- | --- | --- |
| `errorkit` | errorkit | `AppError` (ID, stack, metadata, HTTP status, retriable), global + instance registries, factory, formatters, ULID utilities, 15 built-in codes | Stable; foundation of every other package |
| `logger` | logger | zerolog global logger (`Init`) and instance API (`New`), trace-aware helpers, console redaction, OTLP log provider + writer | Stable |
| `telemetry` | telemetry | `Instrumenter` span factory, naming helpers (`Job`, `Batch`, `External`, `Messaging`, `DB`), compatibility globals (`InitTracer`, `Start`…), span attribute helpers | Stable |
| `httpclient` | httpclient | `*http.Client` factory (pool, timeouts, redirect policy, TLS ≥ 1.3, transport-wrapper seam); owns `ERR_EXTERNAL_*` codes | Stable |
| `httprequest` | httprequest | Retrying requester (`HTTPRequester.Do`), status→errorkit mapping, body redaction, audit log, response decoding, tracing wrapper; owns `ERR_NETWORK_*`, `ERR_AUTH_*` | Stable |
| `httpserver` | httpserver | Framework-agnostic contracts (`HTTPServer`, `RouterGroup`, `HTTPContext`, handler/middleware types), `ServerOptions`, `Reply` envelope, redactors, ctx helper shims | Contracts stable |
| `httpserver/hertz` | hertz | Hertz implementation of the contracts: tracing, recovery, audit, health, NoRoute/NoMethod, request logger | Adapter |
| `grpc` | grpc | `google.golang.org/grpc` server (`New`/`Serve`/`Shutdown`/`Stop`) and client (`Dial`) with otelgrpc stats handler, panic recovery, keepalive; `ToStatus` errorkit→gRPC mapping | Stable |
| `grpc/internal/testsvc` | testsvc | Hand-written `grpc.ServiceDesc` (Echo, 4 call modes) used only by `grpc` tests | Internal, test support |
| `postgres` | database | pgx/v5 pool factory (`NewPostgresClient[FromEnv]`), DSN normalization, generic `StoredProcedure[T]`, tracing wrapper; owns `ERR_DB_POSTGRES_*` | Stable |
| `mongo` | mongodb | mongo-driver/v2 adapter (`ClientPort`, `Collection`, result interfaces), tracing wrapper, error classification; owns `ERR_DB_MONGO_*` | Stable |
| `messaging` | messaging | RabbitMQ `PubSub` (topology-driven publish/subscribe, retry, DLX, reconnect), lower-level `Publisher`/`Subscriber`, topology loaders; owns `ERR_MQ_*` | Stable |
| `storage` | storage | `KVStorager[T]` / `AppendLogStorager[T]`; `MemoryStorage[T]` and JSON-file `FileStorage[T]`; owns `ERR_STORAGE_*` | Stable |
| `llmclient` | llmclient | Non-streaming OpenAI-compatible chat client on top of `httprequest` | Stable |
| `envs` | envs | `caarlos0/env` wrapper returning `ERR_SYSTEM_CONFIG_INVALID` | Stable |
| `urlkit` | urlkit | URL build/join helpers | Stable |
| `validators` | validators | `ozzo-validation`-style field validators (date, alphanumeric, range, date range) | Stable |
| `marshal` | marshal | JSON encode without HTML escaping | Stable |
| `slices` | slices | `Chunk[T]`, `MergeUniqueStrings` | Stable |
| `stringutils` | stringutils | `NormalizeText`, `RemoveMarkdownCodeBlock` | Stable |
| `algorithms` | algorithms | `cenkalti/backoff/v4` exponential-backoff factory | Stable |
| `ioutils` | ioutils | `WriteJSON`, `SaveJSON`, `PrintJSON` debug dumpers | **Dev-only**; no production package imports it (verified with `go list`) |

---

## 3. Internal import graph

Source: `go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./...`
(production imports; test-only edges listed separately). 30 intra-module
production edges, no cycles (the build succeeds; V2 in baseline).

### 3.1 Adjacency list (production imports, module prefix stripped)

| Package | Imports (intra-module) |
| --- | --- |
| `algorithms` | — |
| `errorkit` | — |
| `slices` | — |
| `stringutils` | — |
| `telemetry` | — |
| `grpc/internal/testsvc` | — |
| `envs` | errorkit |
| `httpclient` | errorkit |
| `httpserver` | errorkit |
| `ioutils` | errorkit |
| `logger` | errorkit |
| `marshal` | errorkit |
| `storage` | errorkit |
| `urlkit` | errorkit |
| `validators` | errorkit |
| `mongo` | errorkit, telemetry |
| `postgres` | errorkit, telemetry |
| `grpc` | errorkit, logger |
| `messaging` | algorithms, errorkit, logger |
| `httpserver/hertz` | errorkit, httpserver, logger |
| `httprequest` | errorkit, httpclient, logger, telemetry |
| `llmclient` | errorkit, httpclient, httprequest, marshal, telemetry |

Test-only intra-module edges (`TestImports`): `grpc → grpc/internal/testsvc`;
`postgres → telemetry` (already a prod edge); `llmclient → httprequest,
telemetry`; `httprequest → telemetry`; `httpserver/hertz → httpserver`;
`mongo → telemetry`. No external test packages (`XTestImports` empty for
every package): all tests are white-box, in-package.

### 3.2 Layering as observed

```text
level 0 (no intra deps): errorkit, telemetry, algorithms, slices, stringutils
level 1: logger, httpclient, httpserver, envs, ioutils, marshal, storage, urlkit,
         validators, mongo(+telemetry), postgres(+telemetry)
level 2: httprequest(httpclient, logger, telemetry), grpc(logger),
         messaging(algorithms, logger), httpserver/hertz(httpserver, logger)
level 3: llmclient(httprequest, httpclient, marshal, telemetry)
```

`.sentrux/rules.toml` declares layers (`clients` > `composite` > `primitives`
> `foundation`) and boundaries (`logger` must not import `telemetry`,
`httpclient`/`httpserver` isolated, `mongo`/`postgres` isolated). The
observed graph respects all declared boundaries, but the rules file has no
entry for `grpc`, `httpserver/hertz` or `grpc/internal/testsvc`, and no Task
or CI job runs sentrux (DN-38).

### 3.3 Heavy third-party dependencies per package

Distinct non-main modules in each package's transitive closure
(`go list -deps -f '{{.Module.Path}}' ./<pkg>`); the whole module pulls 52.

| Package | Modules | Notable direct third-party deps |
| --- | --- | --- |
| `httpserver/hertz` | 37 | `cloudwego/hertz` (+ `bytedance/sonic`, `cloudwego/netpoll`), `hertz-contrib/obs-opentelemetry/tracing`, `rs/zerolog` |
| `messaging` | 27 | `rabbitmq/amqp091-go`, `cenkalti/backoff/v4`, `google/uuid`, OTel API; plus everything `logger` pulls |
| `grpc` | 26 | `google.golang.org/grpc`, `otelgrpc`, `genproto/googleapis/rpc/errdetails` |
| `httprequest` | 25 | OTel API; via `logger`: zerolog, OTel log SDK, `otlploggrpc` (→ `google.golang.org/grpc`, protobuf) |
| `llmclient` | 25 | same closure as `httprequest` |
| `logger` | 25 | `rs/zerolog`, `go.opentelemetry.io/otel/{log,sdk/log,sdk/resource}`, `otlploggrpc` |
| `mongo` | 16 | `go.mongodb.org/mongo-driver/v2` |
| `postgres` | 14 | `jackc/pgx/v5` (+ puddle, pgservicefile) |
| `telemetry` | 8 | `go.opentelemetry.io/otel`, `otel/trace` |
| `grpc/internal/testsvc` | 6 | `google.golang.org/grpc`, protobuf `wrapperspb` |
| `algorithms` | 1 | `cenkalti/backoff/v4` |
| `envs` | 1 | `caarlos0/env/v11` |
| `errorkit`, `httpclient`, `httpserver`, `ioutils`, `marshal`, `slices`, `storage`, `stringutils`, `urlkit`, `validators` | 0 | stdlib only |

Observation for P2.X1/P2.X3: any consumer of `httprequest` or `llmclient`
links the OTLP gRPC log exporter and `google.golang.org/grpc` because
`httprequest/audit.go` and `httprequest/httperror.go` import `logger`.
`stretchr/testify` is a direct `go.mod` requirement but is used only by
`urlkit/url_test.go` (TestImports listing).

---

## 4. Exported API inventory

Derived from `go doc -all` for every package (22/22). Line counts of the
dumps: algorithms 25, envs 8, errorkit 563, grpc 218, testsvc 46,
httpclient 199, httprequest 262, httpserver 309, hertz 28, ioutils 32,
llmclient 96, logger 336, marshal 15, messaging 399, mongo 250,
postgres 206, slices 14, storage 162, stringutils 10, telemetry 229,
urlkit 47, validators 22.

Package doc comments: present for `envs`, `grpc`, `testsvc`, `httpserver`,
`hertz`, `ioutils`; **missing** for the other 15; `messaging`'s package doc
is the stray file-path comment `messaging/otel_carrier.go` (DN-35). No
package has `Example*` functions.

Registration pattern shared by `httpclient`, `httprequest`, `postgres`,
`mongo`, `messaging`, `storage`: an exported `var ErrorMetadata
[]errorkit.Metadata`, `func RegisterErrors(registry *errorkit.Registry)
error`, and an `init()` that calls `errorkit.MustRegister(ErrorMetadata...)`
(httpclient/errors_codes.go:85, httprequest/errors_codes.go:142,
postgres/errors.go:109, mongo/errors_codes.go:107,
messaging/errors_codes.go:145, storage/errors_codes.go:85).

### errorkit
- Types: `AppError` (fields `ErrCode, Reason, Payload, Wrapped, Metadata, Trace, ID, TraceID, Timestamp`), `ErrorCode`, `ErrorGroup`, `ErrorType`, `ErrorOption`, `Metadata`, `TraceContext`, `Registry`, `Factory`, `Config{Registry, MaxStackDepth, StrictMetadata}`, `FormatterConfig`, `StackFormatter`, `JSONFormatter`.
- Constructors: `NewError(code) *AppError`, `NewFactory(Config) *Factory`, `NewRegistry(...Metadata)`, `NewDefaultRegistry()`, `NewStackFormatter()`, `NewStackFormatterWithConfig(FormatterConfig)`, `NewJSONFormatter()`.
- `AppError` methods: `Code`, `Error`, `Pretty`, `PrettyJSON`, `Unwrap`, `With(...ErrorOption)`, `WriteTo(io.Writer)`.
- Options: `WithPayload`, `WithReason(format, args...)`, `WithTraceID`, `WithWrapped`.
- Global registry funcs: `GetMetadata`, `GetAllCodes`, `GetCodesByGroup`, `GetCodesByType`, `RegisterMany`, `MustRegister`, `OverrideMetadata`, `ValidateMetadata`.
- `Registry` methods: `GetMetadata`, `GetAllCodes`, `GetCodesByGroup`, `GetCodesByType`, `RegisterMany`, `MustRegister`, `OverrideMetadata`, `Merge`, `NewError`. `Factory.NewError`. `StackFormatter.{Format,FormatColored,FormatPlain}`, `JSONFormatter.Format`.
- Config/utility funcs: `SetMaxStackDepth`, `InitFromEnv` (reads `ERROR_KIT_MAX_STACK`), `DisableColors`, `EnableColors`, `SetIDGenerator`, `ParseULID`, `ValidateULID`, `MinULID`, `MaxULID`.
- Vars: `ErrorRegistry` (exported map), `MaxStackDepth = 32`, `ColorsEnabled = true`, `GenerateID`, `ErrInvalidMetadata`, `ErrMetadataConflict`, ANSI color vars (`Reset, Red, Green, Yellow, Blue, Purple, Cyan, Gray, Bold`).
- Consts: 15 codes `ERR_UNKNOWN, ERR_INTERNAL, ERR_VALIDATION, ERR_VALIDATION_{INVALID_FORMAT,MISSING_FIELD,INCONSISTENT,BUSINESS_RULE,DUPLICATE}, ERR_CLIENT_{BAD_REQUEST,NOT_FOUND,RATE_LIMIT}, ERR_SYSTEM_{UNEXPECTED,CONFIG_INVALID,TIMEOUT_INTERNAL,CONCURRENCY}`; groups `GroupUnknown, GroupData, GroupSystem, GroupClient, GroupAuth, GroupStorage, GroupDatabase, GroupMessageQueue`; types `ErrorTypeInternal, ErrorTypeExternal`.

### logger
- Types: `Logger`, `Config{AppName, Version, Env, Writer, OTLPWriter}`, `Environment` (`Dev`, `Prod`), `OTLPWriter`, `LogProviderHandle`.
- Constructors: `New(Config) *Logger`, `Init(appName, version, ...*OTLPWriter) (*zerolog.Logger, io.Closer)`, `InitLogProvider(ctx, serviceName, endpoint) *LogProviderHandle`, `NewOTLPWriter(serviceName)`, `SetupOTLP(ctx, serviceName, endpoint) (*LogProviderHandle, *OTLPWriter)`, `DetectEnv() Environment`.
- Global funcs: `Error`, `LogAndReturn`, `Panic` (`os.Exit(1)`), `Warn`, `Info`, `ErrorCtx`, `WarnCtx`, `LogInfoWithTrace`, `LogErrorWithTrace`, `LogWarnWithTrace`, `WithTrace`, `WithErrorKit`, `TraceIDFromCtx`, `Ctx`, `CtxOrGlobal`, `WithLogger`.
- Methods: `Logger.{Error, ErrorCtx, Info, InfoCtx, IntoContext, Warn, WarnCtx, WithFields, Zerolog}`; `LogProviderHandle.Shutdown(ctx)` (no return value); `OTLPWriter.Write`.
- Consts: `Blue`, `Reset` (ANSI strings exported from the package).

### telemetry
- Types: `Instrumenter`, `InstrumenterConfig{ScopeName, TracerProvider}`, `Span = trace.Span` (alias).
- Constructor: `NewInstrumenter(InstrumenterConfig) *Instrumenter`.
- Methods: `Instrumenter.{Start, Job, Batch, External, Messaging, DB}`.
- Compatibility globals: `InitTracer(serviceName)`, `Start`, `Job`, `Batch`, `External`, `Messaging`, `DB`, `SpanFromContext`.
- Helpers: `RecordError`, `SetAttrString`, `SetAttrInt`, `SetAttrInt64`, `SetAttrStringSlice`; span-start options `WithString`, `WithInt`, `WithBool`, `WithSpanKind`.

### httpclient
- Types: `ClientConfig` (timeouts, pool sizes, redirects, TLS, `TransportWrapper`), `ClientOption`.
- Constructors: `NewClient(*ClientConfig) *http.Client`, `NewClientWithOptions(...ClientOption)`, `NewDefaultClient()`; presets `DefaultConfig()`, `SetupForLLM()`.
- Options: `WithTimeout, WithDialTimeout, WithKeepAlive, WithMaxIdleConns, WithMaxIdleConnsPerHost, WithMaxConnsPerHost, WithIdleConnTimeout, WithTLSHandshakeTimeout, WithResponseHeaderTimeout, WithMaxRedirects, WithDisableCompression, WithDisableKeepAlives, WithInsecureSkipVerify, WithTLSConfig, WithTransportWrapper`.
- Registration: `ErrorMetadata`, `RegisterErrors`.
- Consts: `ERR_EXTERNAL_SERVICE_UNAVAILABLE, ERR_EXTERNAL_SERVICE_TIMEOUT, ERR_EXTERNAL_SERVICE_ERROR, ERR_EXTERNAL_INVALID_RESPONSE`; defaults `DefaultTimeout, DefaultKeepAlive, DefaultMaxIdleConns, DefaultMaxIdleConnsPerHost, DefaultMaxConnsPerHost, DefaultIdleConnTimeout, DefaultTLSHandshakeTimeout, DefaultExpectContinueTimeout, DefaultResponseHeaderTimeout, DefaultMaxRedirects`.

### httprequest
- Types: `HTTPRequester` (interface `Do(ctx, RequestConfig, out any) error`), `HTTPRequest` (unexported fields), `RequestConfig{Method, URL, Body, ContentType, Headers}`, `RetryConfig{MaxRetries, InitialDelay, MaxDelay}`, `Option`, `BodyRedactor`.
- Constructors (all return `HTTPRequester`): `New`, `NewHTTPRequestWithRetry`, `NewWithoutTracing`, `NewWithInstrumenter`, `NewWithOptions`; wrappers `WrapWithTracing`, `WrapWithInstrumenter`.
- Methods: `HTTPRequest.Do`.
- Options: `WithRetryConfig, WithAuditLog, WithBodyCapture, WithBodyRedactor, WithRawBodies`; func `DefaultBodyRedactor`.
- Registration: `ErrorMetadata`, `RegisterErrors`.
- Consts: `ERR_NETWORK_ERROR, ERR_NETWORK_TIMEOUT, ERR_AUTH_{UNAUTHENTICATED,UNAUTHORIZED,INVALID_TOKEN,TOKEN_EXPIRED,INVALID_CREDENTIALS,INTEGRATION_NOT_FOUND}`; `DefaultMaxRetries = 5`, `DefaultInitialDelay = 100ms`, `DefaultMaxDelay = 5s`.

### httpserver
- Interfaces: `HTTPServer` (`GET, POST, PUT, DELETE, Group, Use, Run, Shutdown`), `RouterGroup`, `HTTPContext` (`Param, Query, BindJSON, GetBody, GetHeader, JSON, String, Status, SetHeader, Redirect, Reply, WithMessage, WithError, WithMetadata, WithData`).
- Func types: `HandlerFunc`, `MiddlewareFunc`, `OptionReply`, `BodyRedactor`.
- Structs: `ServerConfig{Host, Port, ReadTimeout, WriteTimeout}` (+ `Address()`), `ServerOptions` (6 `Enable*` bools, NoRoute/NoMethod status+handler, `Audit`, `Reply`), `AuditOptions{CaptureBodies, BodyRedactor}`, `ReplyOptions{Layout, Clock}`, `Reply`.
- Funcs: `DefaultServerOptions`, `BuildReply`, `WithMessageOpt`, `WithErrorOpt`, `WithMetadataOpt`, `WithDataOpt`, `DefaultBodyRedactor`, `RawBodyRedactor`; ctx shims `WithTimeout`, `WithCancel`, `WithValue`, `GetValue`, `Deadline`, `Done`.
- Const: `DateTimeLayout`.

### httpserver/hertz
- Constructors: `New(httpserver.ServerConfig) httpserver.HTTPServer`, `NewWithOptions(cfg, httpserver.ServerOptions) httpserver.HTTPServer`.
- Funcs: `RecoveryHandler(ctx, *app.RequestContext, err any, stack []byte)`, `RequestLoggerMiddleware(*logger.Logger) app.HandlerFunc` (Hertz type, not an `httpserver.MiddlewareFunc`, see DN-17).

### grpc
- Types: `Server`, `ServerConfig{Address, Keepalive, MaxRecvMsgSize, MaxSendMsgSize}`, `ServerKeepalive{Time, Timeout, MinTime, PermitWithoutStream, MaxConnectionIdle, MaxConnectionAge}`, `ClientConfig{Target, Keepalive, Creds, MaxRecvMsgSize, MaxSendMsgSize}`, `ClientKeepalive{Time, Timeout, PermitWithoutStream}`, `ServerOption`, `ClientOption`.
- Constructors: `New(ServerConfig, ...ServerOption) (*Server, error)`, `Dial(ClientConfig, ...ClientOption) (*grpc.ClientConn, error)`.
- Methods: `Server.{Registrar, Serve, Shutdown, Stop}`.
- Server options: `WithServerTracing, WithServerRecovery, WithServerTracerProvider, WithServerPropagator, WithServerLogger, WithUnaryInterceptors, WithStreamInterceptors, WithRawServerOptions`.
- Client options: `WithClientTracing, WithClientTracerProvider, WithClientPropagator, WithClientUnaryInterceptors, WithClientStreamInterceptors, WithRawDialOptions`.
- Func: `ToStatus(error) error`.

### grpc/internal/testsvc (internal)
- Consts `ServiceName, UnaryMethod, ServerStreamMethod, ClientStreamMethod, BidiMethod`; vars `ServerStreamDesc, ClientStreamDesc, BidiStreamDesc`; `Register(grpc.ServiceRegistrar, Impl)`; interface `Impl`.

### postgres (package database)
- Interfaces: `Querier`, `Execer`, `ClientPort` (Querier+Execer+`Begin`+`Close`), `StoredProcedurer[T]` (`QueryRow, Query, QueryRowJSON, QueryJSON`).
- Structs: `Config{DSN, MaxConnLifetime, MaxConnIdleTime, MaxConns, MinConns}`, `StoredProcedure[T]{Pool Querier}` (value-receiver methods implementing `StoredProcedurer[T]`).
- Constructors: `NewPostgresClient(Config) (*pgxpool.Pool, error)`, `NewPostgresClientFromEnv(Config)`, `NewStoredProcedure[T](Querier) StoredProcedurer[T]` (traced), `WrapWithTracing[T]`, `WrapWithInstrumenter[T]`.
- Registration: `ErrorMetadata`, `RegisterErrors`.
- Consts: `PostgresEnvVar = "POSTGRES_DSN"`; `ERR_DB_POSTGRES_{UNAVAILABLE,TIMEOUT,AUTH_FAILED,CONNECTION_FAILED,DEADLOCK,ERROR}`.

### mongo (package mongodb)
- Interfaces: `ClientPort` (`Collection, Ping, Close`), `Collection` (9 ops: `InsertOne, InsertMany, Find, FindOne, BulkWrite, ReplaceOne, CountDocuments, UpdateOne, DeleteOne`), `Cursor`, `SingleResult`, `InsertOneResult`, `InsertManyResult`, `UpdateResult`, `DeleteResult`, `BulkWriteResult` (empty interface).
- Structs: `Config{URI, Database, ConnectTimeout, ServerSelectionTimeout, MaxPoolSize, MinPoolSize, MaxConnIdleTime, RetryWrites *bool}`, `Client` (+ `Close, Collection, Database, Ping`).
- Constructors: `New(ctx, Config) (ClientPort, error)`, `NewWithoutTracing`, `NewWithInstrumenter`; wrappers `WrapWithTracing`, `WrapWithInstrumenter`.
- Funcs: `WrapOperationError`, `WrapDecodeError`, `ParseObjectID`, `ExtractInsertedIDHex`; registration `ErrorMetadata`, `RegisterErrors`.
- Consts: `ERR_DB_MONGO_{UNAVAILABLE,TIMEOUT,AUTH_FAILED,ERROR,NOT_FOUND,DECODE_FAILED}`.

### messaging
- Interface: `Messager` (`Publish, Subscribe, Close`); func type `MessageHandler`, `PubSubOption`.
- Structs: `PubSub`, `Publisher`, `Subscriber`, `Topology{Version, Exchanges, Queues, Bindings}`, `Exchange`, `QueueTopology` (TTL, MaxLength, Overflow, Prefetch, QueueMode, Arguments, DLX, Retry), `Binding`, `DLXConfig`, `RetryConfig{MaxAttempts, InitialDelay, MaxDelay, BackoffMult}`, `Event`, `EventMessage`, `Message`.
- Constructors: `NewPubSub(amqpURL, ...PubSubOption) (Messager, error)`, `NewPublisher(amqpURL, []Exchange)`, `NewSubscriber(amqpURL, Exchange, QueueTopology, consumer, amqp.Table, []string)`.
- Methods: `PubSub.{Publish, Subscribe, Close}`, `Publisher.{Publish, PublishWithHeaders, Close}`, `Subscriber.{Consume, Reconnect, Close}`, `Topology.{GetQueueByName, GetBindingByQueue, GetExchangeByName}`.
- Options: `WithTracer(trace.Tracer)`, `WithPropagator`, `WithTopology`, `WithTopologyFile`, `WithLogger`.
- Loaders: `LoadTopology` (legacy), `LoadTopologyFromBytes`, `LoadTopologyFromFile`, `LoadTopologyFromEnv`; registration `ErrorMetadata`, `RegisterErrors`.
- Consts: `TopologyEnvVar`, `DefaultHeartbeat = 60s`, `DefaultReconnectDelay = 5s`, `MaxReconnectDelay = 60s`; `ERR_MQ_{UNAVAILABLE,TIMEOUT,AUTH_FAILED,CONNECTION_FAILED,CHANNEL_ERROR,PUBLISH_FAILED,CONSUME_FAILED,QUEUE_ERROR,EXCHANGE_ERROR,ERROR}`.

### storage
- Interfaces: `KVStorager[T]` (`Put, Get, Delete, Clear`), `AppendLogStorager[T]` (`Append, List, Delete, Clear`).
- Structs/constructors: `MemoryStorage[T]` / `NewMemory[T]()` (methods `Put, Get, Append, List, Delete, Clear`); `FileStorage[T]` / `NewFileStorage[T](baseDir) (*FileStorage[T], error)` (methods `Append, List, Delete, Clear, GetAll`).
- Registration: `ErrorMetadata`, `RegisterErrors`. Consts `ERR_STORAGE_{UNAVAILABLE,TIMEOUT,ACCESS_DENIED,INVALID_CREDENTIALS,ERROR}`.

### llmclient
- Types: `LLMProvider` (`Execute(ctx, LLMRequestConfig) (string, error)`), `Client`, `Config{APIKey, Endpoint, Timeout}`, `LLMRequestConfig{Model, SystemPrompt, UserMessage}`, wire types `LLMRequest`, `LLMResponse`, `Message`, `Choice`, `Usage`.
- Constructors: `NewClient(httprequest.HTTPRequester, Config) *Client`, `New`, `NewWithoutTracing`, `NewWithInstrumenter`; wrappers `WrapWithTracing`, `WrapWithInstrumenter`. Method `Client.Execute`.

### Utility packages
- `envs`: `NewLoader(c any) error`.
- `urlkit`: `BuildURL`, `BuildURLString`, `MustBuildURL` (panics), `JoinPath`.
- `validators`: `IsValidDatePointer`, `IsAlphanumericWithSpaces`, `IsWithinRange(min, max) func(any) error`, `ValidateDateRange`; const `DateLayout`.
- `marshal`: `NoEscape`, `NoEscapeBytes`, `NoEscapeNoNewline`, `NoEscapeBytesNoNewline`.
- `slices`: `Chunk[T]`, `MergeUniqueStrings`.
- `stringutils`: `NormalizeText`, `RemoveMarkdownCodeBlock`.
- `algorithms`: `ExponentialBackoffConfig`, `DefaultExponentialBackoffConfig()`, `NewExponentialBackoff(cfg) backoff.BackOff`.
- `ioutils`: `WriteJSON(io.Writer, any)`, `SaveJSON(filename, any)`, `PrintJSON(any)`, `SaveProductsToJSON` (deprecated alias).

### Constructor-shape consistency (input for P2.X1)

| Shape | Packages |
| --- | --- |
| Tracing triplet + `WrapWith*` | `httprequest`, `llmclient`, `mongo` |
| Traced constructor + `WrapWith*`, no untraced constructor | `postgres` (`NewStoredProcedure`; untraced use = struct literal `StoredProcedure[T]{Pool: p}`) |
| Functional options (`With*`) | `grpc`, `messaging`, `httpclient`, `httprequest.NewWithOptions` |
| Options struct | `httpserver/hertz` (`ServerOptions`), `logger.New` (`Config`) |
| Returns interface | `httprequest.*` (`HTTPRequester`), `llmclient.New*` (`LLMProvider`), `mongo.New*` (`ClientPort`), `messaging.NewPubSub` (`Messager`), `hertz.New*` (`HTTPServer`), `postgres.NewStoredProcedure` |
| Returns concrete | `grpc.New` (`*Server`), `grpc.Dial` (`*grpc.ClientConn`), `postgres.NewPostgresClient` (`*pgxpool.Pool`), `httpclient.*` (`*http.Client`), `llmclient.NewClient`, `storage.*`, `logger.New`, `messaging.NewPublisher/NewSubscriber` |

---

## 5. Constructors, lifecycle and concurrency — documented vs implemented

| Package | Documented | Implemented (evidence) | Match |
| --- | --- | --- | --- |
| errorkit | Registry helpers "safe for concurrent use, but mutate process-global state" (errorkit/README.md "Registry API") | Global map guarded by `errorRegistryMu` (registry.go:5-8, 194-200; register.go:52-69, 171-183); instance `Registry.mu` (registry.go:165). But `ErrorRegistry` is an exported map that callers can read/write without the lock, and `MaxStackDepth` (config.go:15), `ColorsEnabled` (colors.go:18), `GenerateID` are unsynchronized globals mutated by `SetMaxStackDepth`/`DisableColors`/`SetIDGenerator`/`InitFromEnv` | Partial — DN-30 |
| logger (global) | `Init` sets package global + `zerolog/log.Logger`; "do not call Init concurrently or after the service has started" (logger/README.md "Lifecycle") | logger.go:60-61 plain assignments to `global` and `log.Logger`; `globalLogger()` reads `global` unsynchronized (logger.go:14-21); `newLoggerWithWriter` also calls `zerolog.SetGlobalLevel` and sets `zerolog.TimeFieldFormat` (config.go:303-310) | Matches the documented constraint |
| logger (instance) | `New` "does not mutate the package-level global" (instance.go:42; README "Instance API"); AGENTS.md:119 "No SDK package mutates the global zerolog logger except logger.Init" | `New` → `newLoggerWithWriter` (instance.go:59) → `zerolog.SetGlobalLevel(...)` and `zerolog.TimeFieldFormat = ...` (config.go:303-310): process-wide zerolog state is mutated on every `New` | **Mismatch** — DN-01 |
| logger (OTLP) | `InitLogProvider` registers a global provider; "uses grpc.WithBlock() so connection errors surface at startup" (logger/README.md:176); `Shutdown` flushes and closes | logprovider.go:61-91: `otlploggrpc.New` with `WithInsecure()` only, no `WithBlock`; errors printed to stderr and a no-op provider returned (66-69); global `otelglobal.SetLoggerProvider` (88). `Shutdown` returns nothing, prints errors to stderr (23-27) | **Mismatch** (WithBlock, TLS not configurable, errors not returned) — DN-02 |
| telemetry | `Instrumenter` explicit and immutable; compatibility globals | `Instrumenter{tracer}` set once (trace.go:56-82). Global `tracer` (trace.go:53) reassigned by `InitTracer` without synchronization (trace.go:115-117) and read by every compatibility call (trace.go:308-310) | Instrumenter matches; globals race-prone — DN-29 |
| httpclient | `*http.Client` safe for concurrent use; no Close, use `CloseIdleConnections` (README "Lifecycle") | Plain `*http.Client`/`*http.Transport` (httpclient.go:222-285) | Matches |
| httprequest | "Fully immutable after construction … safe for concurrent use"; no Close (README "Lifecycle") | `Do` copies `retryConfig` (httprequest.go:184), never writes to `h`; wrapper holds only immutable fields (tracing.go:52-61) | Matches |
| httpserver | `Run()` blocks; `Shutdown(ctx)` graceful drain bounded by ctx; `HTTPContext` single-goroutine (httpserver/README.md "Lifecycle") | Contracts only (contracts.go:147-177) | n/a |
| httpserver/hertz | `Run()` blocks until termination; `Shutdown` delegates to `server.Hertz.Shutdown`; recovery/audit opt-in via `ServerOptions` | `Run()` calls `engine.Spin()` and always returns `nil` (wrapper.go:58-61); `Spin` installs SIGINT/SIGHUP/SIGTERM handlers and runs its own `Shutdown(context.Background())`, logging (not returning) serve errors (hertz `pkg/app/server/hertz.go:56-79,90-113`). `server.Default` always adds Hertz's own recovery (hertz.go:48-53) regardless of `EnableRecovery` (server.go:48,51) | **Mismatch** — DN-15, DN-16 |
| grpc | `Serve` blocks; `Shutdown(ctx)` graceful then hard stop on ctx expiry with `ERR_SYSTEM_TIMEOUT_INTERNAL`; caller closes `ClientConn` | server.go:94-102, 108-125 exactly as documented; `Dial` lazy via `grpc.NewClient` (client.go:124) | Matches |
| postgres | One pool per process, `pool.Close()` once; `StoredProcedure[T]` and wrappers stateless | `NewPostgresClient` has no ctx parameter; fixed 5 s `context.Background()` for connect+ping (postgres.go:62-80); value-receiver `StoredProcedure[T]` (stored_procedure.go:19-99); wrapper holds immutable fields (tracing.go:41-44) | Matches (no ctx — P2.X1/C4 question) |
| mongo | `ClientPort` safe after `New`; `Close(ctx)` once | `New` pings with `context.WithTimeout(ctx, 5s)` (client.go:67-73), disconnects on failure with `context.Background()` (client.go:71); `Close` → `Disconnect(ctx)` (client.go:101-105) | Matches |
| messaging | `PubSub` owns one shared thread-safe Publisher; one Subscriber per `Subscribe`; `Subscribe` blocks until ctx cancelled; Close closes the publisher | `Publisher.mu` held for the whole publish incl. reconnect I/O (amqp.go:94-95, 139-140, 181-182); `Subscriber` has no lock but is confined to the `Subscribe` goroutine (messaging.go:247-283); `PubSub.Close` closes only the publisher (messaging.go:339-341) — active subscribers stop only when their ctx is cancelled (`defer sub.Close()` messaging.go:254) | Matches; note `Close` does not stop subscribers |
| storage | `MemoryStorage` guarded by `sync.RWMutex` (README "MemoryStorage"); FileStorage concurrency not documented | `MemoryStorage.mu` (memory_storage.go:15, 33, 41, 59, 69, 81, 90); `FileStorage.mu` (file_storage.go:20, 38, 56, 78, 93, 120) — in-process only, no cross-process locking, writes via temp file + rename (file_storage.go:200-221) | Matches; FileStorage undocumented — DN-34 |
| llmclient | `Config` consumed at construction; safe for concurrent `Execute` | `Client` fields never written after `NewClient` (client.go:69-77, 96-144) | Matches |
| algorithms | Returned `BackOff` not safe for concurrent use; `MaxRetries = 0` wraps to `MaxUint64` (algorithms/README.md:131-132) | New `BackOff` per call (backoff.go:33); `MaxRetries < 1` is clamped to 1 (backoff.go:29-31) so 0 means one attempt, not unbounded | Concurrency matches; edge case **mismatch** — DN-26 |
| envs, urlkit, validators, marshal, slices, stringutils, ioutils | Pure/stateless, safe for concurrent use | No package state besides compiled regexps (stringutils) | Matches |

---

## 6. Critical flows (file:line call paths)

### 6.1 HTTP out with retries (`httpclient` + `httprequest`)

1. Client: `httpclient.NewDefaultClient` (httpclient.go:296) → `NewClient` (httpclient.go:222): dialer (228), TLS `MinVersion: tls.VersionTLS13` when no custom config (235), transport with pool/timeouts (246-252), optional `TransportWrapper` (257-258), `http.Client{Timeout}` (263), `CheckRedirect` policy (269-285).
2. Requester: `httprequest.New` (httprequest.go:142) → `newPlain` (132-138; retry defaults via `normalizeRetryConfig` 117-128, redactor `DefaultBodyRedactor` 55-60) → `WrapWithTracing` (tracing.go:38) → `WrapWithInstrumenter` (tracing.go:45-50; nil → unwrapped).
3. `tracingHTTPRequester.Do` (tracing.go:57-61): span `http.<host keyword>.<METHOD>.<last two path segments>` (tracing.go:63-105). No error/status is recorded on the span.
4. `HTTPRequest.Do` (httprequest.go:181-223): per attempt `ctx.Err()` check (190-192), span event `http.attempt` (194-200), `executeRequest` (203).
5. `executeRequest` (235-274): `buildRequest` (277-306: body reader, `Content-Type` then `Headers` override, W3C propagation via global propagator at 303) → `client.Do` (245) → transport error → `buildNetworkError` (httperror.go:29) → `defer res.Body.Close()` (249).
   - non-2xx: `io.ReadAll(res.Body)` with no size limit (253), optional audit (255 → audit.go:38-62; logs full `url` at audit.go:54, headers sanitized by deny-list audit.go:14-33, bodies redacted), `buildHTTPError` (257 → httperror.go:78-158: status→code switch 106-146, `insufficient_quota` 429 forced non-retriable 122-125/153-155, **global `logger.Error(appErr)` on every non-2xx at 156**).
   - 2xx with `out`: `processResponse` over `io.TeeReader` (263 → decode.go:15-49), audit (264-266).
6. Retry decision: `isRetryable` uses `errors.As` + `Metadata.Retriable` (211, 309-317); no method/idempotency check; backoff `waitForRetry` with `time.After` (216, 321-328), `nextDelay` doubling capped at `MaxDelay` (220, 226-232). Retriable codes come from `ErrorMetadata` (httprequest/errors_codes.go, httpclient/errors_codes.go: 503/504/5xx/network/429).

### 6.2 Inbound HTTP middleware (`httpserver/hertz`)

`hertz.New` (server.go:23) → `NewWithOptions` (server.go:30-76):

1. `applyOptionDefaults` (31, 81-99) backfills statuses, redactor, reply layout/clock — **not** the `Enable*` booleans.
2. Address/timeouts (33-39).
3. Engine: tracing on → `hertztracing.NewServerTracer()` + `server.Default(tracerOpt, …)` + `engine.Use(hertztracing.ServerMiddleware)` (45-49); off → `server.Default(opts...)` (51). `server.Default` always prepends Hertz's own `recovery.Recovery()` (hertz `pkg/app/server/hertz.go:48-53`).
4. SDK recovery `recovery.Recovery(WithRecoveryHandler(RecoveryHandler))` (54-56; handler 103-109 logs via global `logger.ErrorCtx`, aborts 500).
5. Inbound audit `hertzInboundAuditMiddleware` (57-59, 152-205): copies the request body and both header maps on every request, calls `c.Next`, and then **does nothing** — the logging block is commented out (158-161, 176-177, 186-203); `redact` and `Audit.CaptureBodies` are never used (155 is the ineffassign reported by golangci-lint in V9).
6. NoRoute/NoMethod (61-66, 111-147), `/health` (67-69, 222-226).
7. Effective order for a request: Hertz recovery → OTel server middleware → SDK recovery → audit (no-op) → user middleware registered later via `HTTPServer.Use` (wrapper.go:24-28, adapter 43-49) → handler (wrapper.go:51-56; `hertzContext` per request).
8. `RequestLoggerMiddleware` (request_logger.go:16-37) is not installed by `NewWithOptions` and cannot be installed through `httpserver.HTTPServer.Use` (it returns `app.HandlerFunc`, the engine is unexported in `wrapperHertzServer`, wrapper.go:13-17). DN-17.
9. Lifecycle: `Run` → `engine.Spin()` (wrapper.go:58-61), `Shutdown` → `engine.Shutdown(ctx)` (65-67).

### 6.3 gRPC interceptors (`grpc`)

Server `New` (server.go:24-84): address required (25-29; the address is stored but never used to listen, `Serve` takes a listener — DN-21) → options (31-34) → keepalive params/enforcement (38-43) → message sizes (44-49) → interceptor chain with recovery **outermost**, then user interceptors (53-66) → `otelgrpc.NewServerHandler` stats handler for all four call modes (70-79) → `WithRawServerOptions` appended last (81) → `grpc.NewServer` (83).

Recovery: unary (interceptors.go:17-26) and stream (30-39) → `recoverToStatus` (43-54): `ERR_SYSTEM_UNEXPECTED` with the panic value in the reason and `debug.Stack()` in the payload (44-47), logged via injected `*logger.Logger` or global (48-52), returned to the caller as `status.New(codes.Internal, appErr.Error())` (53) — the panic value reaches the client.

Lifecycle: `Serve` (94-102), `Shutdown` → `GracefulStop` in a goroutine, hard `Stop` on ctx expiry (108-125), `Stop` (128).

Client `Dial` (client.go:73-132): target required (74-78) → default `insecure.NewCredentials()` when `Creds` nil (85-88) → keepalive (92-94) → default call options (95-104) → user interceptors (105-110) → `otelgrpc.NewClientHandler` (111-120) → raw dial options (122) → `grpc.NewClient` (124).

Error mapping: `ToStatus` (status.go:48-66) → `codeFor` (14-38): only the 15 errorkit core codes are mapped; package codes (`ERR_DB_*`, `ERR_MQ_*`, `ERR_NETWORK_*`, `ERR_AUTH_*`, `ERR_EXTERNAL_*`) fall to `codes.Internal`; `ErrorInfo{Reason: code, Domain: "go-sdk"}` attached (56-61).

### 6.4 Postgres stored procedures (`postgres`)

1. Pool: `NewPostgresClientFromEnv` (postgres.go:88-96, reads `POSTGRES_DSN`) → `NewPostgresClient` (30-83): DSN required (31-35) → `normalizePostgresDSN` (37, 98-136) → `pgxpool.ParseConfig` → tuning only for non-zero fields (48-59) → `context.WithTimeout(context.Background(), 5s)` (62) → `pgxpool.NewWithConfig` (65) → `Ping`, `pool.Close()` on failure (74-80). Errors attach the cause with `WithPayload(err)`, not `WithWrapped` (41, 69, 78), so `errors.Is/As` cannot reach the pgx error (DN-12).
2. Runner: `NewStoredProcedure[T]` (stored_procedure.go:23-25) → `WrapWithTracing` (tracing.go:29-31) → `WrapWithInstrumenter` (tracing.go:34-39; always wraps, even for nil).
3. Call: `tracingStoredProcedurer.Query` (tracing.go:52-56) → `pgSpan` (70-72) → span `postgres.<proc>` from regex `(?i)\bFROM\s+(\w+)\s*\(` (11, 74-79) → `StoredProcedure.Query` (stored_procedure.go:52-72): `Pool.Query(ctx, query, args...)` (57, `nosemgrep: defer-rows-close` at 56) → `pgx.CollectRows` closes rows (64) → `ERR_DB_POSTGRES_ERROR` with `WithWrapped` (59-62, 66-69).
4. `QueryRow` uses `Pool.Query` + `pgx.CollectOneRow` (27-50, `nosemgrep` at 31). `QueryRowJSON`/`QueryJSON` (74-99) → `appendJSONArg` (108-121): `json.Marshal(jsonArg)` appended as the last positional argument as a string (119).
5. Row mapping: `pgRowMapper` → `RowToStructByNameLax` for structs, `RowTo` otherwise (101-106).
6. SQL text is passed verbatim from the caller; values are positional parameters. No SQL is built by the SDK.

### 6.5 RabbitMQ publish/consume with topology (`messaging`)

1. Build: `NewPubSub` (messaging.go:88-112) → options (89-92) → `resolveTopology` (114-123): `WithTopology` > `WithTopologyFile` → `LoadTopologyFromFile` (topology.go:82-96) > `LoadTopologyFromEnv` (`RABBITMQ_TOPOLOGY_FILE`, topology.go:101-109). `LoadTopologyFromBytes` is a bare `json.Unmarshal` with no validation (topology.go:69-78).
2. `NewPublisher` (amqp.go:29-41) → `connect` (43-81): `amqp.DialConfig` with 60 s heartbeat (44-46), `conn.Channel()` (54), `ExchangeDeclare` for every topology exchange (63-76). No publisher confirms (`Channel.Confirm` never called) and `mandatory=false` on publish (108, 123, 159, 167).
3. Publish: `PubSub.Publish` (messaging.go:143-225) → `getQueueAndBinding` (144, 349-365) → `EventMessage` + `json.Marshal` (153-167) → producer span `messaging.publish <queue>` (170-183) → correlation baggage (187-193) → `publishOnce` per routing key: headers + propagator inject (195-213) → `Publisher.PublishWithHeaders` (amqp.go:138-176: lock, reconnect if closed 142-150, `channel.Publish` 159, one reconnect+retry 160-173) → `executeWithRetry` (215, 371-382: `algorithms.NewExponentialBackoff` + `backoff.Retry`, no context) → `ERR_MQ_PUBLISH_FAILED` (218-221). The `ctx` is not passed to the AMQP call.
4. Consume: `PubSub.Subscribe` (231-284) → `GetExchangeByName` (237-243) → `buildQueueArgs` (245, 424-455: TTL, max-length, overflow, queue mode, DLX, custom args float→int64) → `NewSubscriber` (amqp.go:224-244) → `connect` (246-314: dial, `ExchangeDeclare` 267, `QueueDeclare` 279, `QueueBind` per routing key 293-301, `Qos(prefetch)` 303) → `Consume` with `autoAck=false` (334-350) → loop (264-283): ctx done → return nil (266-267); closed delivery channel → `reconnectWithBackoff` (271, 388-409: 5 s doubling to 60 s, `time.After`, ctx-aware) → `Consume` again (274).
5. Per delivery: `handleDelivery` (286-336): extract trace context from headers (287-288), consumer span (290-301), per-attempt process span + handler (311-326), `executeWithRetry` (328), `Nack(false, false)` after exhaustion → DLX (331), `Ack(false)` on success (335). Ack/Nack errors discarded; handler panics are not recovered.
6. Close: `PubSub.Close` → `Publisher.Close` (messaging.go:339-341 → amqp.go:180-202).

### 6.6 OTLP logging setup/shutdown (`logger`)

1. `SetupOTLP` (logprovider.go:104-107) → `InitLogProvider` (61-91): `otlploggrpc.New(WithEndpoint, WithInsecure)` (62-65), failure → stderr + no-op `log.NewLoggerProvider()` (66-69), resource `service.name` (71-78), `log.NewLoggerProvider(WithProcessor(NewBatchProcessor(exporter)), WithResource)` (80-85), `otelglobal.SetLoggerProvider` (88; deprecated API, SA1019 in V7).
2. `NewOTLPWriter` (otlp.go:39-43) → `otelglobal.GetLoggerProvider().Logger(serviceName)` (41; SA1019).
3. `Init` (logger.go:44-63): `DetectEnv` (45; reads `ENVIRONMENT`, config.go:32-39) → `zerolog.MultiLevelWriter(newWriter(env…), otlpWriter)` (53-54) → `newLoggerWithWriter` (55 → config.go:301-313: global level, `TimeFieldFormat`, `redactingWriter` wraps the multi-writer so both console and OTLP get redacted JSON) → `global = &l`, `log.Logger = l` (60-61) → returns `io.NopCloser(nil)` (62).
4. Per event: `redactingWriter.Write` (config.go:323-344) → console writer (Dev: `zerolog.ConsoleWriter` whose `FormatExtra` re-reads `ENVIRONMENT` on every line, config.go:56-58) and `OTLPWriter.Write` (otlp.go:47-126): JSON unmarshal (50; malformed → plain record emitted with `context.Background()` 51-57), timestamp (64-68), severity (71-74), body (77-79), `trace_id`/`span_id` → reconstructed `SpanContext` (89-103), attributes (106-123), `Emit` (125).
5. Shutdown: `LogProviderHandle.Shutdown(ctx)` → `provider.Shutdown(ctx)`; error printed to stderr, not returned (logprovider.go:23-27).

### 6.7 LLM calls (`llmclient`)

`llmclient.New` (client.go:80-82) → `WrapWithTracing` (tracing.go:20-22) → `tracingLLMProvider.Execute` (tracing.go:38-42; span `llm.<model>` or `llm.execute`, 44-49) → `Client.Execute` (client.go:96-144): model required → `ERR_VALIDATION_MISSING_FIELD` (97-101) → optional `context.WithTimeout(cfg.Timeout)` (103-107) → `marshal.NoEscapeBytes(LLMRequest{system, user})` (109-121) → `RequestConfig{POST, Endpoint, Content-Type JSON, Authorization: "Bearer " + APIKey}` (124-132) → `httpRequest.Do(ctx, …, &response)` (133; continues in §6.1 — retry count is whatever the injected requester was built with, default 5) → no choices → `ERR_EXTERNAL_SERVICE_ERROR` (137-141) → first choice content (143). The API key travels only in a request header; `httprequest` audit sanitizes `Authorization` (audit.go:14-21). There is no `*FromEnv` constructor and no Go code reads `LLM_API_KEY` (DN-22).

---

## 7. Test inventory

Counted with `grep -E '^func (Test|Benchmark|Fuzz|Example)'` per file. No
`Benchmark`, `Fuzz` or `Example` functions exist anywhere. No
`//go:build` tags, no `testing.Short()` gates; the only `t.Skip` calls are
`storage/file_storage_phase11_test.go:21,72` ("chmod unsupported"). All test
files use the in-package name (white-box). No test dials an external
service: the only network use is `httptest.NewServer` (httpclient,
httprequest), gRPC `bufconn` (grpc) and a Hertz engine built but never
started (`httpserver/hertz`). Connection paths of `postgres`
(`NewPostgresClient` beyond DSN validation), `mongo` (`New*`) and
`messaging` (`NewPubSub`, `Publisher`, `Subscriber`, `Subscribe`) have no
tests at all. Coverage is from baseline.md V6 (total 47.7%).

| Package | Test files | Test funcs | `t.Parallel` | Fakes / helpers | Coverage |
| --- | --- | --- | --- | --- | --- |
| algorithms | backoff_test.go | 3 | 0 | — | 100.0% |
| envs | loader_test.go | 1 | 0 | `os.Unsetenv` (loader_test.go:14) | 66.7% |
| errorkit | error_test.go (7), factory_test.go (2), register_test.go (12), registry_api_test.go (3) | 24 | 0 | table tests (11 `t.Run`) | 60.0% |
| grpc | grpc_test.go (7), recovery_test.go (4), roundtrip_test.go (9), status_test.go (4) | 24 | 0 | `bufconn` + `internal/testsvc`; `fakeServerStream` | 77.2% |
| grpc/internal/testsvc | — | 0 | — | is itself test support | 0.0% (no tests) |
| httpclient | errors_codes_test.go (3), httpclient_test.go (11) | 14 | 11 | `httptest.NewServer` | 57.1% |
| httprequest | errors_codes_test.go (3), httprequest_test.go (3), options_test.go (7), tracing_test.go (3) | 16 | 12 | `httptest.NewServer`; unroutable `127.0.0.1:1` for network errors | 44.9% |
| httpserver | options_test.go | 4 | 4 | — | 75.8% |
| httpserver/hertz | wrapper_test.go | 2 | 2 | engine built, never `Run` | 19.4% |
| ioutils | ioutils_test.go | 4 | 4 | `t.TempDir` | 68.8% |
| llmclient | client_test.go (6), tracing_test.go (3) | 9 | 9 | stub `HTTPRequester`, `stubProvider` | 60.0% |
| logger | config_test.go (4), instance_test.go (5), wrapper_test.go (1) | 10 | 0 | `t.Setenv` | 52.7% |
| marshal | json_test.go | 4 | 3 | — | 87.5% |
| messaging | errors_codes_test.go (3), options_test.go (5), topology_test.go (8) | 16 | 9 | `t.Setenv`, `t.TempDir`; no broker fake | 10.5% |
| mongo | client_test.go (4), errors_codes_test.go (3), errors_test.go (2), tracing_test.go (2) | 11 | 7 | `stubClientPort`, `stubCollection` | 25.3% |
| postgres | errors_test.go (3), postgres_test.go (5), stored_procedure_test.go (4), tracing_test.go (1) | 13 | 3 | `fakeQuerier`, `fakeRows`, `fakeStoredProcedurer`, `tracetest.InMemoryExporter`, `t.Setenv` | 52.7% |
| slices | chunks_test.go | 6 | 6 | — | 100.0% |
| storage | errors_codes_test.go (3), file_storage_phase11_test.go (6), storage_test.go (11) | 20 | 6 | `t.TempDir`, chmod-based failures | 79.8% |
| stringutils | normalize_test.go | 2 | 0 | table tests | 100.0% |
| telemetry | trace_test.go | 2 | 0 | `tracetest.InMemoryExporter` | 35.3% |
| urlkit | url_test.go | 3 | 4 | testify `assert` (only testify user) | 70.3% |
| validators | validators_test.go | 1 | 0 | — | 9.7% |

Totals: 44 test files, 189 `Test*` functions, 4,320 test lines.
AGENTS.md §6 prescribes `require`/`assert` from testify; only `urlkit` uses
it (DN-37). `go test -short ./...` and `go test -race -short ./...` pass
(baseline V4, V5).

---

## 8. Existing tooling and CI

### 8.1 Task (`Taskfile.yml`)

| Task | What it runs | Notes |
| --- | --- | --- |
| `default` | `task --list` | |
| `tools:install` | `proto install`; `go install golang.org/x/tools/cmd/goimports@latest`; `pip install --upgrade pre-commit semgrep==1.95.0 git-cliff==2.14.2` (32-42) | goimports and pre-commit unpinned |
| `skills:install` | `skillshare install && skillshare sync` (44-51) | |
| `precommit:install` / `precommit:run` | `pre-commit install --install-hooks` / `pre-commit run --all-files` (55-69) | |
| `staticanalysis` | `QG_FULL=1 bash .skillshare/skills/go-code-quality-check/scripts/staticanalysis.sh` (15, 71-79) | Script lives in a gitignored, externally installed skill |
| `test` → `test:package` | `go test -short ./<pkg>/...` per top-level package in parallel (81-92) | `ALL_PACKAGES` = 20 top-level dirs (17-18); no `-race` |
| `check` | `test`, `staticanalysis`, `precommit:run` (94-99) | |
| `changelog` | `git-cliff --unreleased --strip all` (103-109) | |
| `release:check` | `go run golang.org/x/exp/cmd/gorelease@latest -base=<last v tag>` with clean-tree precondition (111-120) | `@latest` unpinned; precondition `test -z "$(git status --porcelain)"` at 117 |
| `packages` | lists `ALL_PACKAGES` (124-129) | |
| `deps:outdated` | per-package `go list -m -u` (131-147) | |
| `deps:update` | single `go get $UPGRADE ./pkg/...`, `go mod tidy`, then `deps:verify` (149-166) | |
| `deps:verify` | per-package build, vet, `go test -short` (168-181) | |

No task runs `-race`, coverage, `govulncheck`, `golangci-lint` or sentrux.

### 8.2 proto (`.prototools`)

`go = "1.27.3"`, `skillshare = "0.25.5"`, `staticcheck = "2026.2.2"`,
`sentrux = "0.5.7"`, `python = "3.15.2"`, `task = "3.54.0"`; plugins from
`trypanic/proto-plugins`; settings `auto-install`, `auto-clean`,
`telemetry = false`; `[env] file = ".env.shared"` (line 10) — that file does
not exist in the repository (DN-39). Baseline recorded PATH tools differing
from the pins (go 1.26.2, staticcheck 2026.1, skillshare 0.28.2).

### 8.3 pre-commit (`.pre-commit-config.yaml`)

`default_install_hook_types: [pre-commit, commit-msg]`, `fail_fast: false`.
Hooks: `pre-commit-hooks@v5.0.0` (trailing-whitespace, end-of-file-fixer,
check-yaml, check-merge-conflict, check-added-large-files 512 KB,
detect-private-key); `conventional-pre-commit@v4.5.0` (commit-msg); local
`gofmt -l -w`, `goimports -w`, `go test -short ./...` (line 37),
`staticanalysis` = same skill script in staged mode (line 44-50). The
staged-mode script exits 0 with "no staged .go files" under `--all-files`
(baseline V11). The script's banner `pkgs=1 files=1` in full mode counts the
targets `./...` and `.` (script lines 64-86), so full mode does scan the
whole tree (answers the question raised in baseline V7).

### 8.4 semgrep (`.semgrep.yml`)

11 project rules: `no-fmt-print-in-library` (W), `no-panic-outside-init`
(E), `no-os-exit-in-library` (E), `context-must-be-first-param` (W),
`no-context-background-in-request-path` (W, httpserver/httpclient/messaging
only), `error-wrapping-required` (W), `no-hardcoded-credentials` (E),
`sql-string-concat` (E), `weak-crypto-md5-sha1` (W),
`math-rand-for-security` (W), `http-no-timeout` (W), `defer-rows-close`
(W, lines 135-147). In-code suppressions exist: logger/wrapper.go:52,
errorkit/register.go:83, 132, urlkit/url.go:96,
ioutils/printer.go:34,37, postgres/stored_procedure.go:31,56.
The script runs the Docker image with `.semgrep.yml + p/golang + p/gosec +
p/secrets` when Docker is available, else the local CLI with `.semgrep.yml`
only (script lines 332-357). Docker is available on this host.

There is no `.semgrepignore`; semgrep 1.95.0's built-in default ignore list
skips `*_test.go`: a direct run `semgrep scan --config=.semgrep.yml postgres/`
scanned 5 files (`contracts.go, errors.go, postgres.go,
stored_procedure.go, tracing.go`) and reported 0 results. Test files are
therefore never scanned by any semgrep rule.

### 8.5 GitHub workflows and release

- Only `.github/workflows/release.yml` exists (`git ls-files .github`).
  Trigger: push to `main` (15-17). Jobs: `release-please`
  (`googleapis/release-please-action@45996ed…` v5.0.0, contents+PR write)
  and `assets` (only when a release is created): checkout by SHA,
  `actions/setup-go` with `go-version-file: go.mod` (59; resolves Go 1.26.1),
  `go build`, `go vet`, `go test -short` (61-65), CycloneDX SBOM via
  `go run …cyclonedx-gomod@v1.12.0`, `git archive`, `sha256sum`,
  `gh release upload`, proxy warm-up. Actions pinned by SHA; workflow-level
  `permissions: contents: read`.
- No workflow runs on pull requests or on non-release pushes: no CI for
  tests, race, lint, staticcheck, semgrep, govulncheck or gorelease.
- `release-please-config.json`: single package `.`, `release-type: go`,
  `include-v-in-tag`, no component in tag, visible sections feat/fix/perf/
  revert/refactor/docs, hidden build/chore/ci/style/test.
  `.release-please-manifest.json` = `{".": "1.0.0"}`. `cliff.toml` mirrors
  the sections for `task changelog` previews. `CHANGELOG.md` is empty
  (0 bytes) although README.md:165 calls it "the full per-phase change log"
  (DN-40).
- `main` shows Dependabot merge commits (`56870d8`), but there is no
  `.github/dependabot.yml` in either branch; Dependabot must be configured in
  repository settings (not verifiable from the tree).

### 8.6 SECURITY.md claims vs. reality

| Claim (SECURITY.md) | Reality |
| --- | --- |
| "the same tools run again in CI" (line 5-6) | No CI job runs them (§8.5) |
| `task staticanalysis` invokes `./staticanalysis.sh` (line 14); `bash staticanalysis.sh` examples (170-171) | No script at repo root; the runner is `.skillshare/skills/go-code-quality-check/scripts/staticanalysis.sh`, gitignored (`.gitignore` `.*/skills/*`) and installed by `task skills:install` |
| "CI gate (`.github/workflows/semgrep.yml`)" with `staticcheck` and `semgrep` jobs, PR baseline mode, weekly SARIF (127-141) | File does not exist |
| Pre-commit runs `go test -short` and the script on staged files (100-115) | True (`.pre-commit-config.yaml`), but the script is a no-op under `--all-files` |
| Docker variant uses `.semgrep.yml`, `p/golang`, `p/gosec`, `p/secrets`; local fallback `.semgrep.yml` only (30-35) | True (script 332-357) |
| `ERROR` rules block commit + CI (119-123) | Commit only; there is no CI |

---

## 9. §1 re-verification

| §1 fact | Verdict | Corrected value / evidence |
| --- | --- | --- |
| Module: single module, one `go.mod`, one version | Verified | One `go.mod` (`find -name go.mod`), module `github.com/trypanic/go-sdk` |
| Released version `v1.0.0` | Verified | `git tag` → `v1.0.0` only; `.release-please-manifest.json` `1.0.0`; 8 commits on top (`git log v1.0.0..HEAD`) |
| Go: `go 1.26.1` directive; toolchain `1.26.2` pinned in `.prototools` | **Wrong** (second half) | `go.mod` `go 1.26.1`, no `toolchain` line. `.prototools:1` pins `go = "1.27.3"`. 1.26.2 is only the Go on `PATH` (`go version`). CI (`release.yml:59`) uses `go-version-file: go.mod` → 1.26.1 |
| Size: 22 packages | Verified, with precision | `go list ./...` → 22 = 20 top-level dirs + `httpserver/hertz` + `grpc/internal/testsvc`; 21 importable; Taskfile `ALL_PACKAGES` has 20 entries |
| Size: ~14k lines | Verified, with precision | 13,979 lines of tracked `.go` = 9,659 non-test + 4,320 test |
| Per-package sizes (errorkit 2.3k, messaging 1.5k, logger 1.4k, httprequest 1.3k, grpc 1.3k, mongo 1k, postgres 0.9k, storage 0.9k) | Verified as **totals including tests** | Total/non-test: errorkit 2,327/1,685; messaging 1,463/1,234; logger 1,368/1,098; httprequest 1,314/980; grpc 1,266/610 (+ testsvc 87); mongo 974/732; postgres 904/480; storage 853/424; httpserver/hertz 515/473; httpclient 584/385; httpserver 479/388; llmclient 371/193; telemetry 476/364; urlkit 242/134; ioutils 139/80; validators 126/103; marshal 122/63; slices 112/43; algorithms 102/42; stringutils 92/43; envs 63/18 |
| Package name ≠ directory: `postgres`→`database`, `mongo`→`mongodb` | Verified | `go doc` headers; test files declare `package database` / `package mongodb` |
| Contracts: AGENTS.md §2 + package READMEs | Verified | 21 READMEs present (one per package dir; none for `grpc/internal/testsvc`) |
| Task runner Task; tools pinned via proto | Verified | `Taskfile.yml` (version 3), `.prototools` |
| Quality gates `task test`, `task staticanalysis`, `task precommit:run`, `task release:check` | Verified | Taskfile.yml:81-92, 71-79, 63-69, 111-120. `task staticanalysis` depends on the gitignored skill script; `release:check` uses `gorelease@latest` |
| CI: only `release.yml`; SECURITY.md semgrep jobs not present | Verified | `git ls-files .github` → `release.yml` + `.gitkeep`s; no `semgrep.yml` (§8.6) |
| Tests: no `//go:build integration`, no `testing.Short()`; external services faked | Verified, with precision | No build tags or Short gates (grep). Services are not faked so much as **not exercised**: connection paths of postgres/mongo/messaging have no tests; fakes exist only for postgres `Querier`, mongo `ClientPort`/`Collection`, llmclient requester, gRPC bufconn, `httptest` |
| Project skills `.skillshare/skills/{go-code-quality-check,go-design-principles,go-testing-strategy}` gitignored; `task skills:install` | Verified | `.gitignore` `.*/skills/*`; directories present; `git status --ignored` lists `.skillshare/skills/` |
| Known pre-existing: semgrep `defer-rows-close` at `postgres/stored_procedure_test.go:47` | **Wrong** | Not reproducible: semgrep 1.95.0 default ignore skips `*_test.go` (direct run scanned 5 non-test files of `postgres/`, 0 results), and the non-test call sites carry `// nosemgrep: defer-rows-close` (stored_procedure.go:31, 56). Baseline V7 also reports semgrep 0. Line 47 of the test is `got, err := sut.Query(...)`, which would match the rule pattern only if tests were scanned |
| Gotcha: `gorelease` refuses a dirty tree incl. untracked `.moon/` | Verified, with precision | `.moon/` (`cache/` only) is untracked and not ignored (`git check-ignore .moon` → exit 1; `git status --porcelain` → `?? .moon/`). It is a leftover of `main`, which still tracks `.moon/workspace.yml` and `moon.yml` (`git ls-tree main`). The refusal comes from the Taskfile precondition (Taskfile.yml:117), before gorelease runs |

Additional §1-adjacent facts for the orchestrator:

- `main` does not contain `Taskfile.yml` (`git show main:Taskfile.yml` fails);
  only `audit/2026-11` and `chore/migrate-moon-to-taskfile` contain `d966f55`.
  `main` is an ancestor of `audit/2026-11`.
- The on-disk `AGENTS.md` already uses `task …` in §7; a copy that still
  mentions `moon run repo:*` is stale.

---

## 10. Discovery notes (documentation vs. implementation)

Not findings. Each names the P2 task that should confirm, classify and
decide. Contract-relevant ones are marked (C).

| ID | Note | Evidence | For |
| --- | --- | --- | --- |
| DN-01 (C) | `logger.New` claims not to touch globals but mutates zerolog's global level and `TimeFieldFormat` | instance.go:42,59 → config.go:303-310; AGENTS.md:119 | P2.C1 |
| DN-02 | `InitLogProvider`: README promises `grpc.WithBlock()`; code has no blocking dial, forces `WithInsecure()`, returns no error, `Shutdown` returns no error | logger/README.md:176; logprovider.go:23-27, 61-69 | P2.C1 |
| DN-03 | logger README says "The logger has no `Config` struct" | logger/README.md:126-128 vs instance.go:14-33 | P2.C1 / X2 |
| DN-04 | logger README points to `httpserver.RequestLoggerMiddleware`; it lives in `httpserver/hertz` | logger/README.md:415, 616 vs request_logger.go:16 | P2.X2 |
| DN-05 | `DetectEnv` doc says it reads `APP_ENV`; it reads `ENVIRONMENT`. Dev console writer re-reads `ENVIRONMENT` on every line. Implicit env read inside `Init`/`New` vs "no silent env reads" | config.go:31-39, 56-58; AGENTS.md §2.4 | P2.C1 |
| DN-06 | `httprequest` README: zero `MaxRetries` → one attempt; code normalizes 0 → 5 | httprequest/README.md:99 vs httprequest.go:117-120, 147-150 | P2.C2 |
| DN-07 | `httprequest` README says `New`/`NewHTTPRequestWithRetry` return `*HTTPRequest` (concrete); both return `HTTPRequester` wrapped with tracing | httprequest/README.md:125, 135 vs httprequest.go:142-151 | P2.C2 / X1 |
| DN-08 | Comment refers to non-existent `telemetry.InitProvider` | httprequest.go:300-303 | P2.X2 |
| DN-09 | `httprequest` logs every non-2xx through the global logger, ignoring any per-request logger; library logging on an error it also returns | httperror.go:156 | P2.C2 |
| DN-10 | Tracing wrappers (`httprequest`, `llmclient`, `mongo`, `postgres`) never record errors or span status | tracing.go files; only `messaging` calls `RecordError` (messaging.go:216-217, 322-323, 329-330) | P2.X1 |
| DN-11 (C) | `postgres.WrapWithInstrumenter(sp, nil)` documented as "disable tracing / skip the wrapper"; implementation always wraps and a nil instrumenter falls back to the global tracer | postgres/README.md:186, 221 vs postgres/tracing.go:34-39 and telemetry/trace.go:143-146 | P2.C4 |
| DN-12 | `NewPostgresClient` errors carry the cause as `Payload`, not `Wrapped` (no `errors.Is/As`); README documents payload, AGENTS §2.1 favours wrapping | postgres.go:39-42, 67-70, 76-79 | P2.C4 |
| DN-13 | Wrong import paths in README quick starts: `go-sdk/database`, `go-sdk/mongodb` | postgres/README.md:63; mongo/README.md:52 | P2.X2 |
| DN-14 | mongo README: "the package itself imports no telemetry package by default"; it imports `telemetry` | mongo/README.md:7 vs `go list` (mongo → telemetry) | P2.X2 |
| DN-15 (C) | Inbound audit middleware is a no-op: logging commented out, `CaptureBodies`/`BodyRedactor` unused, yet bodies and headers are copied per request. Contracts promise redacted inbound audit by default | server.go:152-205 (ineffassign at 155); AGENTS.md:137; httpserver/README.md "Body redaction" | P2.C2 |
| DN-16 | `EnableRecovery=false` cannot remove Hertz's built-in recovery (`server.Default`); with `true` there are two recovery layers. `Run()` always returns nil and installs OS signal handlers via `Spin` | server.go:48,51,54-56; wrapper.go:58-61; hertz pkg/app/server/hertz.go:48-53, 56-79, 90-113 | P2.C2 |
| DN-17 | `RequestLoggerMiddleware` returns `app.HandlerFunc` and cannot be installed through `httpserver.HTTPServer.Use`; engine is unexported | request_logger.go:16; wrapper.go:13-28 | P2.C2 / X1 |
| DN-18 | hertz/httpserver READMEs say zero-valued fields are backfilled in `NewWithOptions`; `Enable*` booleans are not (cannot be) backfilled | httpserver/README.md:159; hertz/README.md:16 vs server.go:81-99 | P2.X2 |
| DN-19 | Root README and llms.txt describe `grpc` as Kitex-backed and requiring `*serviceinfo.ServiceInfo`; it wraps `google.golang.org/grpc` | README.md:16, 45; llms.txt:22 vs grpc/README.md, server.go:7-8 | P2.X2 |
| DN-20 | gRPC recovery returns `appErr.Error()` (contains the panic value) to the client | interceptors.go:44-53 | P2.C3 |
| DN-21 | `grpc.ServerConfig.Address` is required but not used for listening | server.go:25-29, 83, 94 | P2.C3 / X1 |
| DN-22 (C) | README/AGENTS promise a `*FromEnv` constructor for `LLM_API_KEY`; none exists, no Go code reads it | README.md:65-67; AGENTS.md:130; `grep LLM_API_KEY` only in llmclient/README.md:11 | P2.C2 / X1 |
| DN-23 | `ToStatus` maps only the 15 core codes; package-owned codes (`ERR_DB_*`, `ERR_MQ_*`, `ERR_NETWORK_*`, `ERR_AUTH_*`, `ERR_EXTERNAL_*`) become `Internal` | status.go:14-38 | P2.C3 |
| DN-24 | `messaging.WithLogger` only affects reconnect warnings; `Publisher` always logs through the global logger | messaging.go:72-77, 411-417; amqp.go:98, 105, 116, 143, 160 | P2.C5 |
| DN-25 | messaging README references `schemas/rabbit-topology.json`, which does not exist; topology is not validated after unmarshal | messaging/README.md:230; topology.go:69-78 | P2.C5 / X2 |
| DN-26 | algorithms README: `MaxRetries = 0` wraps to `MaxUint64` (unbounded); code clamps to 1 | algorithms/README.md:131-132 vs backoff.go:29-31 | P2.C6 |
| DN-27 | marshal README: `ioutils.PrintJSON` uses `marshal.NoEscape`; `ioutils` does not import `marshal` | marshal/README.md:112-118 vs ioutils/printer.go:33-39, `go list` | P2.X2 |
| DN-28 | README purpose mentions "metrics"; no metrics API exists | README.md:3-5 | P2.X2 |
| DN-29 | `telemetry.InitTracer` writes a package global without synchronization; every compatibility helper reads it | trace.go:53, 115-117, 308-310 | P2.C1 |
| DN-30 | `errorkit.ErrorRegistry` exported map bypasses `errorRegistryMu`; `MaxStackDepth`, `ColorsEnabled`, `GenerateID` are unsynchronized mutable globals; `InitFromEnv` reads `ERROR_KIT_MAX_STACK` | registry.go:5-8; config.go:15, 44-52; colors.go:18 | P2.C1 |
| DN-31 (C) | AGENTS §1 says `validators` covers "email, ULID, etc."; it has none (ULID helpers are in `errorkit`) | AGENTS.md:63; llms.txt:32 vs validators `go doc` | P2.X2 |
| DN-32 (C) | AGENTS anti-pattern list recommends `envs.Load[T]()`, which does not exist (`envs.NewLoader`) | AGENTS.md:268 | P2.X2 |
| DN-33 | `httprequest` README examples call `llmclient.New(httpRequest, llmclient.ClientConfig{…})` (type is `Config`) and `llmclient` README passes an `*http.Client` to `New`; references to `internal/platform` from another repo | httprequest/README.md "Platform Builder"; llmclient/README.md "Execute" | P2.X2 |
| DN-34 | `FileStorage` concurrency (in-process `RWMutex`, no cross-process lock, no fsync) undocumented | storage/README.md; file_storage.go:20, 200-221 | P2.C4 |
| DN-35 | Package doc comments missing in 15 packages; `messaging` package doc is a stray file-path comment | `go doc` headers (§4) | P2.X1 |
| DN-36 | No `Example*` tests anywhere (godoc examples absent) | grep (§7) | P2.X1 / C6 |
| DN-37 | AGENTS §6 prescribes testify; only `urlkit` tests use it | AGENTS.md:290; TestImports | P2.X2 |
| DN-38 | `.sentrux/rules.toml` omits `grpc`, `httpserver/hertz`, `testsvc`; nothing runs sentrux although README lists it | .sentrux/rules.toml; Taskfile.yml | P2.X2 |
| DN-39 | `.prototools` loads `.env.shared`, which does not exist | .prototools:10 | P2.X2 |
| DN-40 | `CHANGELOG.md` is empty although README calls it the full change log | README.md:163-165 | P2.X2 |
| DN-41 | SECURITY.md describes a root `staticanalysis.sh` and a `semgrep.yml` CI workflow that do not exist; test files are never scanned by semgrep | §8.6, §8.4 | P2.X2 |
| DN-42 | `httprequest`/`llmclient` consumers transitively link the OTLP gRPC exporter and `google.golang.org/grpc` through `logger` | §3.3 | P2.X1 / X3 |

---

## 11. Not verified / limits

- Which semgrep path (Docker with `p/golang`, `p/gosec`, `p/secrets` or the
  local CLI with `.semgrep.yml` only) produced the baseline's 0 findings:
  Docker is available on this host, but `.staticanalysis/semgrep.stderr.log`
  is empty and the report does not record the path. Not re-run (P0.2 owns V7).
- Repository settings (Dependabot, branch protection, Actions permission for
  release-please PRs) are not visible from the tree.
- `go doc -all` was used for the inventory; unexported behaviour was read
  only where a flow required it. Struct field lists are summarized from `go doc`,
  not copied verbatim.
- Hertz behaviour (`server.Default`, `Spin`) was read from the module cache
  at the version in `go.mod` (`cloudwego/hertz v0.10.6`), not executed.
- No tests were written or run for this task; test and coverage numbers come
  from static counting and `baseline.md`.
