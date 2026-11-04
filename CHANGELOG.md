# Changelog

## [2.0.0](https://github.com/trypanic/go-sdk/compare/v1.0.1...v2.0.0) (2026-11-04)


### ⚠ BREAKING CHANGES

* the module path is now github.com/trypanic/go-sdk/v2. Consumers must update their go.mod requirement and every SDK import to the /v2 path and apply the approved API migration guide.
* **postgres:** JSON argument marshal failures return ERR_DB_POSTGRES_ENCODE_FAILED instead of retriable ERR_DB_POSTGRES_ERROR. Callers should fix the argument instead of retrying.
* **urlkit:** JoinPath and BuildURL reject opaque bases such as mailto:user@example.com with ERR_VALIDATION_INVALID_FORMAT instead of silently ignoring appended paths.
* **api:** HTTPRequest/HTTPRequester become Request/Requester; HTTPServer/HTTPContext become Server/Context; LLMProvider/LLMRequest/LLMRequestConfig/LLMResponse lose LLM prefixes; PostgreSQL constructors become New/NewFromEnv with Context variants and PostgresEnvVar becomes DSNEnvVar. KVStorager/AppendLogStorager become KVStore/AppendLog; Messager becomes Broker; NewLoader becomes LoadInto. OTLP types/functions use Writer, Provider, Config, NewWriter, NewProvider, InitProvider and Setup. NewHTTPRequestWithRetry and exported ANSI constants are removed without aliases.
* **packages:** import postgres and mongo with their directory-matching identifiers; alias the upstream MongoDB driver when both are needed. Replace github.com/trypanic/go-sdk/slices with github.com/trypanic/go-sdk/batch and use batch.Chunk/batch.MergeUniqueStrings. No compatibility package is retained.
* **errorkit:** validators input rejection errors now have an empty Trace. Use error IDs and explicit request context for correlation; configuration errors still include caller frames.
* **telemetry:** HTTP spans use external.http.<host>.<METHOD>, LLM spans external.llm.execute, PostgreSQL spans postgres.<routine>.query and Mongo client lifecycle spans mongo.client.<operation>. Update trace selectors and dashboards.
* **retry:** messaging retry and reconnect delays use full jitter below their exponential caps. Retry policies stop scheduling new attempts after 15 minutes; caller contexts still bound active work.
* **adapters:** HTTP/LLM/Mongo/messaging/Hertz constructors now return concrete adapters; Mongo constructors accept variadic ClientOption values. NewStoredProcedure returns *TracedProcedure with Unwrap. Replace assertions on constructor results with direct method calls. NewFromClient borrows its driver and Close does not disconnect it.
* **httpserver:** HTTPServer and RouterGroup implementers must provide PATCH, HEAD, OPTIONS and Handle methods.
* **logger:** OTLP APIs are available only from logger/otlp; logger.Init and Config.OTLPWriter now accept io.Writer. Import logger/otlp explicitly and retain provider lifecycle ownership.
* **algorithms:** NewExponentialBackoff returns algorithms.Backoff instead of the upstream backoff.BackOff interface.
* **messaging:** PubSub.Publish now returns the specific ERR_MQ_* code of the failure (ERR_MQ_CONNECTION_FAILED, ERR_MQ_EXCHANGE_ERROR, ERR_MQ_QUEUE_ERROR, ERR_MQ_AUTH_FAILED, ERR_MQ_TIMEOUT, ERR_MQ_CLOSED or ERR_MQ_PUBLISH_FAILED) instead of always ERR_MQ_PUBLISH_FAILED; callers that match ERR_MQ_PUBLISH_FAILED must use errorkit.HasCode or Metadata.Retriable instead. PubSub is no longer comparable with == (it holds a context.CancelFunc since f111a23), as gorelease reports.
* **logger:** logger.DetectEnv (and therefore logger.Init and logger.New with an empty Config.Env) now returns Prod (JSON output, Info level) when ENVIRONMENT is unset or holds an unrecognised value; it previously returned Dev. Set ENVIRONMENT=development (or dev/local) to keep the console output. Matching is now case-insensitive.
* **logger:** the "time" field of every log line written by logger (Init, New) is now an RFC 3339 UTC string with nanoseconds instead of a number of Unix epoch seconds, and Init sets zerolog.TimeFieldFormat to time.RFC3339Nano instead of zerolog.TimeFormatUnix. Log pipelines that parse "time" as an integer must be updated.
* **logger:** logger.Config is no longer comparable (new Redactor func field); code comparing Config values or using it as a map key must change.
* **llmclient:** Execute returns ERR_SYSTEM_CONFIG_INVALID for an empty APIKey or a non-https Endpoint; set Config.AllowInsecureEndpoint to keep using a plain http endpoint.
* **messaging:** span names change from "messaging.<op> <queue>" to "messaging.<op>.<queue>", and the attributes messaging.destination, messaging.operation and messaging.rabbitmq.routing_key are replaced by messaging.destination.name, messaging.operation.type plus messaging.operation.name, and messaging.rabbitmq.destination.routing_key. Dashboards and alerts keyed on the old names must be updated.
* **messaging:** NewPubSub no longer reads RABBITMQ_TOPOLOGY_FILE. Callers that relied on the fallback must call NewPubSubFromEnv (same signature) or pass WithTopologyFile(os.Getenv("RABBITMQ_TOPOLOGY_FILE")).
* **httpserver:** a nil value passed to WithDataOpt or WithMetadataOpt is now omitted from the reply JSON instead of being sent as null, and Reply.Data / Reply.Metadata hold the value itself rather than a *any.
* **llmclient:** llmclient.Config{Timeout: 0} now means a 5-minute deadline per Execute instead of none (use a negative Timeout for none); httpclient.DefaultConfig().DialTimeout is 10 s instead of 60 s.
* **mongo:** the Cursor and BulkWriteResult interfaces have new methods; third-party implementations and test fakes must add them.
* **hertz:** with ServerOptions.EnableRecovery=false a handler panic is no longer recovered by Hertz and crashes the process; keep it true (the default) or install your own recovery middleware. The recovery log line gains a panic_stack field and Hertz no longer prints "[Recovery]" lines.
* **storage:** FileStorage writes <hex key>.jsonl files in the JSON Lines format instead of <hex key>.json arrays; old files are read and converted on the next Append, but older SDK versions cannot read the new files. MaxFileKeyLength drops from 125 to 124 bytes.
* **hertz:** the Hertz adapter's Run no longer handles SIGINT, SIGHUP or SIGTERM. Services that relied on it must stop the server themselves, e.g. with signal.NotifyContext followed by Shutdown(ctx). Run now returns an error when the server cannot listen.
* **messaging:** queues whose topology omits prefetch now consume with a prefetch window of 1 instead of unlimited. Set prefetch explicitly to keep a larger window.
* **messaging:** topologies that loaded before are rejected when they contain unknown fields, more than one binding per queue, undeclared exchanges, or retry blocks without a valid maxAttempts or backoffMultiplier. Topology decode errors are now ERR_SYSTEM_CONFIG_INVALID instead of ERR_INTERNAL.
* **httprequest:** httprequest no longer writes an error-level log line for each non-2xx response. Log the returned error yourself, or enable WithAuditLog for one line per attempt.
* **httprequest:** retry delays are now random in [0, backoff] instead of the fixed doubling schedule; a Retry-After above RetryConfig.MaxDelay stops retrying; a retry wait that would pass the context deadline returns the last upstream error instead of ERR_NETWORK_TIMEOUT.
* **mongo:** options set in the MongoDB URI are no longer replaced by SDK defaults, and the default MinPoolSize is clamped to MaxPoolSize, so the effective pool and retry settings change for URIs that carry options.
* **messaging:** for queues with a retry policy, publish attempt and dial counts change: maxAttempts attempts per routing key instead of up to 2 x maxAttempts attempts for the whole event, and completed routing keys are not re-sent on retry.
* **storage:** FileStorage creates its directory 0700 and writes files 0600, so other accounts and groups lose read access; NewFileStorage now has a variadic options parameter, which breaks code that stores it as a func(string) value.
* **messaging:** PubSub.Close stops active Subscribe calls (they used to keep consuming until their own ctx ended), and Publish or Subscribe after Close return ERR_MQ_CLOSED instead of silently opening a new connection.
* **httprequest:** invalid request URLs now fail with ERR_SYSTEM_CONFIG_INVALID before any attempt (scheme must be http or https); TLS verification failures are ERR_EXTERNAL_INVALID_RESPONSE; context.Canceled is ERR_REQUEST_CANCELED instead of ERR_SYSTEM_UNEXPECTED; the httpclient redirect-limit error is returned as is and is no longer retried; llmclient reports empty choices as ERR_EXTERNAL_INVALID_RESPONSE.
* **storage:** FileStorage.Clear no longer removes *.json files in baseDir whose names are not hex-encoded keys.
* **mongo:** mongo errors for cancellation, closed clients, invalid arguments and invalid client options carry new codes and are no longer retriable; New* returns ERR_SYSTEM_CONFIG_INVALID instead of ERR_DB_MONGO_UNAVAILABLE for an invalid URI or option set.
* **messaging:** Publish now fails with ERR_MQ_PUBLISH_FAILED when the broker nacks a message or cannot route it (no queue bound for the routing key, for example before any Subscribe declared the queue); it used to return nil. Every publish waits for a broker confirm, which adds one round trip. Use WithDeclareTopology() to declare queues and bindings up front.
* **httprequest:** non-idempotent requests (POST, PATCH, ...) without an Idempotency-Key header are no longer retried after a 5xx, 429 or timeout; only dial failures are retried. Set RetryConfig.RetryNonIdempotent to restore the previous behavior.
* **httprequest:** successful responses larger than 32 MiB (after decompression) now fail with ERR_EXTERNAL_INVALID_RESPONSE; raise the cap with WithMaxResponseBytes. Non-2xx bodies captured with WithBodyCapture are truncated to 64 KiB.
* **hertz:** with the default ServerOptions (EnableAudit=true) every inbound request now emits one info-level log line through the request or global logger. Set EnableAudit=false to keep the previous silent behavior. Audit.CaptureBodies=false now always logs "[REDACTED]" bodies, even when BodyRedactor is RawBodyRedactor.
* **httpserver:** Reply.Error no longer contains err.Error(). For an *errorkit.AppError it holds the Reason (4xx) or Metadata.Description, and the code moves to the new `code` JSON field; any other error is reported as code ERR_INTERNAL with message "Internal server error". Clients that parsed the "[CODE] reason" text must read `code` instead. The Hertz adapter now logs every error passed to HTTPContext.WithError.
* **messaging:** messaging returns ERR_MQ_AUTH_FAILED, ERR_MQ_CHANNEL_ERROR, ERR_MQ_EXCHANGE_ERROR, ERR_MQ_QUEUE_ERROR and ERR_MQ_CONSUME_FAILED where it returned ERR_MQ_CONNECTION_FAILED, ERR_SYSTEM_CONFIG_INVALID where it returned ERR_INTERNAL for topology lookups, and the subscriber reconnect loop no longer retries non-retriable errors.
* **validators:** IsValidDatePointer returns ERR_VALIDATION_INVALID_FORMAT (was ERR_VALIDATION_MISSING_FIELD) for a value that is not a *string; IsWithinRange(lo, hi) with lo > hi returns ERR_SYSTEM_CONFIG_INVALID; the reasons of IsWithinRange, IsValidDatePointer and ValidateDateRange changed.
* **grpc:** grpc.New and grpc.Dial return ERR_SYSTEM_CONFIG_INVALID when ServerConfig.Creds / ClientConfig.Creds is nil and WithServerInsecure() / WithClientInsecure() is not passed. Code that relied on the plaintext default must set Creds or pass the insecure option; credentials passed only through WithRawServerOptions / WithRawDialOptions must move to Creds.
* **httprequest:** httprequest error payloads change. `url` and `original_error` no longer contain userinfo or query values (values are replaced by REDACTED), and `provider_error_message` is present only with WithBodyCapture(), redacted by the configured BodyRedactor ("[REDACTED]" by default). Read provider_error_code/provider_error_type instead.
* **postgres:** postgres errors carry cause-specific codes, HTTP statuses and retriability instead of ERR_DB_POSTGRES_ERROR for every failure; QueryRow with no row returns ERR_DB_POSTGRES_NOT_FOUND.
* **errorkit:** errorkit.MaxStackDepth, errorkit.ColorsEnabled, errorkit.GenerateID and errorkit.ErrorRegistry are no longer exported. Use SetMaxStackDepth, DisableColors/EnableColors, SetIDGenerator and the registry helpers (GetMetadata, RegisterMany, OverrideMetadata, Factory / Registry) instead.
* **grpc:** grpc.ToStatus returns codes.DeadlineExceeded or codes.Canceled (instead of codes.Unknown) for context.DeadlineExceeded and context.Canceled, including wrapped ones.
* **grpc:** grpc.ToStatus no longer sends the errorkit reason for server-side errors (registered HTTPStatus >= 500 or gRPC code Internal/Unknown/DataLoss); the status message is the registered Metadata.Description (or "internal error"). Use ToStatusWithPolicy(err, StatusPolicy{ExposeInternalReasons: true}) to keep the old message for trusted callers.
* **storage:** storage errors for nil input, over-long keys, permission failures and corrupt files carry new codes and are no longer retriable; FileStorage rejects keys longer than MaxFileKeyLength.
* **grpc:** grpc.ToStatus no longer returns codes.Internal for errorkit codes outside the core set; clients observe the gRPC code that the code's registered HTTPStatus/Retriable metadata calls for (e.g. ERR_DB_POSTGRES_UNAVAILABLE -> Unavailable, ERR_AUTH_UNAUTHENTICATED -> Unauthenticated).
* **algorithms:** NewExponentialBackoff no longer applies the cenkalti/backoff 15-minute MaxElapsedTime; set ExponentialBackoffConfig.MaxElapsedTime to keep an overall time budget. Non-positive intervals, multipliers below 1 and MaxRetries above 1000 are now replaced instead of passed through.
* **urlkit:** JoinPath returns ERR_VALIDATION_INVALID_FORMAT for a segment that contains a "." or ".." path element instead of joining it.
* **urlkit:** BuildURL, BuildURLString and MustBuildURL no longer concatenate base and path. A path containing "?" or "#", "." or ".." segments or invalid escapes now returns ERR_VALIDATION_INVALID_FORMAT (MustBuildURL panics), a path without a leading slash gets one, and a query on base is preserved instead of being corrupted.

### Features

* **connections:** add cancellable database and messaging setup ([d14d52c](https://github.com/trypanic/go-sdk/commit/d14d52c39fd56fed4395fdafcb83cc4503d361aa))
* **envs:** add Load[T] and LoadWithOptions ([4102bfe](https://github.com/trypanic/go-sdk/commit/4102bfea2c238bd860f1f84616c1a533823f6a83))
* **errorkit:** add As and HasCode error-chain helpers ([f6c6d5f](https://github.com/trypanic/go-sdk/commit/f6c6d5fd641160b26cc258d82f2e80ba585d497b))
* **errorkit:** add client-safe Public/PublicJSON serialization ([0272620](https://github.com/trypanic/go-sdk/commit/027262084618ef15cdaf1bf931aa93e4f3f9f246))
* **errorkit:** support stackless validation failures ([0d99d79](https://github.com/trypanic/go-sdk/commit/0d99d79e522530de5c88c1eaa6850e3d4d523564))
* **errorkit:** unexport mutable globals and back the setters with atomics ([194c67d](https://github.com/trypanic/go-sdk/commit/194c67de7e4f33893b503ca6651c9c5f52bf43ce))
* **grpc:** add public-endpoint tracing and otelgrpc option passthrough ([81db1c2](https://github.com/trypanic/go-sdk/commit/81db1c242d07ab9cf2ec291f3dc853bb85cc9d68))
* **grpc:** add server credentials and explicit insecure options ([e768a14](https://github.com/trypanic/go-sdk/commit/e768a14d1e7e84e3e97f837d68807d187b31d598))
* **grpc:** add Server.ListenAndServe using ServerConfig.Address ([f6c5e5f](https://github.com/trypanic/go-sdk/commit/f6c5e5f16d3f96346855db7a1259d046d5b9b574))
* **grpc:** add ServerConfig.MaxConcurrentStreams ([3dcf2f2](https://github.com/trypanic/go-sdk/commit/3dcf2f2c8657c8b1b56c85fba01b1b8c904ffda1))
* **grpc:** add WithClientDefaultTimeout ([50a884e](https://github.com/trypanic/go-sdk/commit/50a884ef1d0c0183868d3b74cba0f584f0a2bee9))
* **grpc:** ErrorInfo domain option and retriable metadata, recovery via ToStatus ([b0f8eb8](https://github.com/trypanic/go-sdk/commit/b0f8eb8a54705cb5d4607fd39a6ce6f835ddb3f3))
* **grpc:** require transport credentials or an explicit insecure opt-in ([ceceb00](https://github.com/trypanic/go-sdk/commit/ceceb00ad6e8b88f84bc230f8d234e1788f3f033))
* **hertz:** implement the redacted inbound audit log ([9e1e2b2](https://github.com/trypanic/go-sdk/commit/9e1e2b23af47d0db416e7511428a4a1fc7badc0e))
* **hertz:** installable request logger that honors X-Request-ID ([eda540b](https://github.com/trypanic/go-sdk/commit/eda540b77838ebd071ad2b2f71e995bc370253b1))
* **hertz:** provide default-on functional server options ([b454142](https://github.com/trypanic/go-sdk/commit/b454142f1cd56ba2690b6bdc80952b3f1b13811c))
* **httprequest:** cap response body sizes and drain before close ([8f9e4e3](https://github.com/trypanic/go-sdk/commit/8f9e4e35ee1dc06592ebdcaddf1931d6b6947d51))
* **httprequest:** jitter retry backoff and honor Retry-After ([f50efe1](https://github.com/trypanic/go-sdk/commit/f50efe1a8062c5844d6e851c550ee1d9f9c07cce))
* **httprequest:** retry only requests that are safe to resend ([536b83e](https://github.com/trypanic/go-sdk/commit/536b83e525eacb6f963bbd2c669a2697a03706e8))
* **httpserver:** complete router method contracts ([bf73dcc](https://github.com/trypanic/go-sdk/commit/bf73dccd7880e51a08a632a56f0509b67bcaf677))
* **httpserver:** configurable health path and readiness hook ([2d41186](https://github.com/trypanic/go-sdk/commit/2d41186874045811c7b2f79e520ae429ab48e272))
* **llmclient:** add ConfigFromEnv for LLM_API_KEY and LLM_ENDPOINT ([7905b6c](https://github.com/trypanic/go-sdk/commit/7905b6c545a6b4f1ee9c66e8788d05b6b78db3ef))
* **llmclient:** bound LLM calls end to end by default ([3623a53](https://github.com/trypanic/go-sdk/commit/3623a530e217664bed5ea1f43dc092a37910bf31))
* **logger:** add NewLogProvider with TLS, headers, timeout and errors ([7146d2d](https://github.com/trypanic/go-sdk/commit/7146d2dd8c6a50209eb7416a66bb67528091e85b))
* **logger:** default DetectEnv to Prod and match ENVIRONMENT case-insensitively ([62b57f5](https://github.com/trypanic/go-sdk/commit/62b57f57348bd49b079ec9bc8649d12d7582b5b4))
* **logger:** write nanosecond RFC 3339 timestamps and keep OTLP integer precision ([0a93051](https://github.com/trypanic/go-sdk/commit/0a93051c2ab545fa6d6f7f4b910aff2d9b47f3a3))
* **messaging:** add TLS, heartbeat, dial timeout and connection name options ([d6601d6](https://github.com/trypanic/go-sdk/commit/d6601d621f563d1dace8e810c17f6558e10515b4))
* **messaging:** align span names and attributes with telemetry and semconv ([717d0d3](https://github.com/trypanic/go-sdk/commit/717d0d34f78c7c0d4962804418ffaf6f635c650b))
* **messaging:** default prefetch of 1 instead of unlimited ([c551edc](https://github.com/trypanic/go-sdk/commit/c551edcd649406f324a72e7e5ef83b4c294e5c4c))
* **messaging:** make Close terminal and stop subscribers ([f111a23](https://github.com/trypanic/go-sdk/commit/f111a2318a0be0c5e0b25654538f2ef88db0f952))
* **messaging:** publisher confirms and mandatory publishing ([0a6bcd5](https://github.com/trypanic/go-sdk/commit/0a6bcd5c97c6336729e54f7a0c551dfa671edb02))
* **messaging:** read RABBITMQ_TOPOLOGY_FILE only in NewPubSubFromEnv ([328b36b](https://github.com/trypanic/go-sdk/commit/328b36b6d4cc9af1267cabc3688748349017473a))
* **messaging:** retry publishes per routing key with one retry layer ([210cba7](https://github.com/trypanic/go-sdk/commit/210cba7bd4d06c89bd5fbe05d4eb5d0d6d089f52))
* **messaging:** specific AMQP error codes and reconnect stop ([701b5ad](https://github.com/trypanic/go-sdk/commit/701b5ad6bf2294a40809edc861f2d23df55a00d1))
* **messaging:** validate topologies when they are loaded ([0fd2a40](https://github.com/trypanic/go-sdk/commit/0fd2a4065f53901823738b22aafdc7f6f2059bd7))
* migrate the SDK module and consumer imports to v2 ([1f40242](https://github.com/trypanic/go-sdk/commit/1f40242b5a34a6b034570c80037a8a11bb0c381d))
* **mongo:** classify cancellations, timeouts, network and config errors ([ebe31eb](https://github.com/trypanic/go-sdk/commit/ebe31eb7c8bdd3db4710890bbcfa4d4aa6b3095f))
* **mongo:** let URI options win over SDK defaults ([29c54ea](https://github.com/trypanic/go-sdk/commit/29c54ead00385af7d14dc72dbf1cb4477d8e4126))
* **mongo:** stream cursors and expose bulk write counts ([0fad067](https://github.com/trypanic/go-sdk/commit/0fad0677adf356b0a68f6b542c3ce2eef5b2785f))
* **postgres:** classify PostgreSQL errors by cause ([a69c832](https://github.com/trypanic/go-sdk/commit/a69c83234829f08eedbbe652251b6f53901c2583))
* **storage:** create FileStorage directories 0700 and files 0600 ([d2f0c55](https://github.com/trypanic/go-sdk/commit/d2f0c55af5695eb2a6347e4c288278bb4cd5582e))
* **storage:** return specific error codes for storage failures ([ab51fc8](https://github.com/trypanic/go-sdk/commit/ab51fc853643b1db673e614b7dc9f604f53fcac1))
* **storage:** store FileStorage logs as JSON Lines appended in place ([d27a2ff](https://github.com/trypanic/go-sdk/commit/d27a2ff919d5c28e710b7709a390391a11a3c9de))
* **telemetry:** share instrumenter providers with transport adapters ([6f02409](https://github.com/trypanic/go-sdk/commit/6f0240982bc30637cf03fd89782d00764594d065))
* **transports:** map classified errors across HTTP and gRPC ([ef2308a](https://github.com/trypanic/go-sdk/commit/ef2308ac5ff4c4b814e5dc8fab04a9bca266bec4))


### Bug Fixes

* **algorithms:** guard invalid backoff config and make MaxElapsedTime explicit ([922075c](https://github.com/trypanic/go-sdk/commit/922075c6376ae91975e5a37f62e2945474429770))
* **algorithms:** reset the backoff before returning it ([ab57956](https://github.com/trypanic/go-sdk/commit/ab57956efeb23440ab5c0b35369b160c1e083c9d))
* **ci:** keep each matrix job on its selected Go toolchain ([f32d1f3](https://github.com/trypanic/go-sdk/commit/f32d1f3afbdc3ec70165398b5b3a1bc29ee9bdb6))
* **ci:** validate major module transitions and bounded stack depth ([38c8b1d](https://github.com/trypanic/go-sdk/commit/38c8b1dfe3f180eedcf85cfbef55351ba217d9e0))
* **deps:** require the patched Testcontainers archive dependency ([dc965b4](https://github.com/trypanic/go-sdk/commit/dc965b4ea06bc8a3b28fd563e2440f08ccc04f06))
* **envs,ioutils,stringutils:** keep env values out of errors and dumps safe ([f87a70d](https://github.com/trypanic/go-sdk/commit/f87a70dd2850e05a1575d897658d1fa4d1f92ec5))
* **errorkit:** all-or-nothing registration, safe JSON fallback, writer colors, monotonic ULIDs ([edd32b8](https://github.com/trypanic/go-sdk/commit/edd32b8c70f223b0ed1dded771cc4bd6a9d8848a))
* **errorkit:** parse ERROR_KIT_MAX_STACK with strconv.Atoi ([123ea9e](https://github.com/trypanic/go-sdk/commit/123ea9e5ee72158aa9aa4a52d25686244e1e2161))
* **errorkit:** report the real import path of dotted packages in traces ([d449128](https://github.com/trypanic/go-sdk/commit/d4491288f1f0429f06be51313ada664c9d3f8640))
* **errorkit:** run OverrideMetadata updater outside the registry lock ([b997ed8](https://github.com/trypanic/go-sdk/commit/b997ed886887dd8bd6a4a4d08dff949f86565fc6))
* **errorkit:** skip typed-nil matches during error traversal ([f0808cb](https://github.com/trypanic/go-sdk/commit/f0808cbcf65f9a1483dd99725a2c8e12bf609f26))
* **errorkit:** walk the whole error tree in HasCode ([77ec696](https://github.com/trypanic/go-sdk/commit/77ec696d1b73c69eb2b323ffb4002efd4b5430cf))
* **errors:** skip typed-nil AppErrors at transport boundaries ([e9f8a6c](https://github.com/trypanic/go-sdk/commit/e9f8a6cead7924a68e6a06a52c03f26bf71a2e1c))
* **grpc:** keep internal error reasons off the wire in ToStatus ([8355dcd](https://github.com/trypanic/go-sdk/commit/8355dcdaf8d66d467663d9c004e9107da3a1f99c))
* **grpc:** map context errors to DeadlineExceeded and Canceled ([2a52d56](https://github.com/trypanic/go-sdk/commit/2a52d5664261f858a4a929fc310258dca7c42a8f))
* **grpc:** map non-core errorkit codes from registered metadata ([207ec87](https://github.com/trypanic/go-sdk/commit/207ec8779afd10d625b1178385fb50b102b7f32c))
* **grpc:** return a fixed message for recovered panics ([643631d](https://github.com/trypanic/go-sdk/commit/643631d802a9c8dce153bfd347c7680b70fd49d3))
* **hertz:** answer unknown methods with the NoMethod handler (405) ([f91fca8](https://github.com/trypanic/go-sdk/commit/f91fca88cf1dbf0a17f772aac93db5a8c2a32651))
* **hertz:** bound Shutdown by the caller's context and report timeouts ([af7d7a8](https://github.com/trypanic/go-sdk/commit/af7d7a8ee4da7febe50eecd55f9ea990d261fce2))
* **hertz:** make EnableRecovery the only, outermost recovery layer ([389f1fa](https://github.com/trypanic/go-sdk/commit/389f1fa2aaa7bceac57ce0a4d8ee0f5cd3581c43))
* **hertz:** return listen errors from Run and stop handling signals ([94bf3e5](https://github.com/trypanic/go-sdk/commit/94bf3e56835306e2684fd7581aabb4d9d642668f))
* **http:** redact URL-bearing audit headers ([9b930df](https://github.com/trypanic/go-sdk/commit/9b930dffe2f9b2dccf31f0806f7b98649d5fa8a9))
* **httprequest:** always send at least one request when MaxRetries &lt; 1 ([f1111cc](https://github.com/trypanic/go-sdk/commit/f1111cc61b44b349aed1ddd99a40dc71147caf2f))
* **httprequest:** classify permanent client failures as non-retriable ([c68d1da](https://github.com/trypanic/go-sdk/commit/c68d1da26c06f4ba2a9de577920d3f2ba261fb70))
* **httprequest:** decode named string/bool types and reject nil out pointers ([d17c97f](https://github.com/trypanic/go-sdk/commit/d17c97f075d5fc9358d1a7ba405e2218604a87cd))
* **httprequest:** enforce response limit after JSON decode ([48c1971](https://github.com/trypanic/go-sdk/commit/48c1971141cc4202a73073f0f1b222c553d8c13a))
* **httprequest:** name implicit GET spans consistently ([ef9798c](https://github.com/trypanic/go-sdk/commit/ef9798ca85fd34523b78b4066c958e5c4e0cea61))
* **httprequest:** prevent replay after redirected dial failures ([6ce5b4a](https://github.com/trypanic/go-sdk/commit/6ce5b4a05ccee60a0fca20e371acfff32e475941))
* **httprequest:** redact nested URL error diagnostics ([6586058](https://github.com/trypanic/go-sdk/commit/6586058b250954f4191f382a5f22e43ba98df713))
* **httprequest:** sanitize URLs and provider messages before payloads, logs and spans ([e645be3](https://github.com/trypanic/go-sdk/commit/e645be31f3e5f6b330acdfb414c74559a69e3454))
* **httprequest:** share one wider sensitive-header list and mask values ([6f09952](https://github.com/trypanic/go-sdk/commit/6f0995221b7a79b9af5bd30b6f57349345570d89))
* **httprequest:** stop logging the errors Do returns ([2671c79](https://github.com/trypanic/go-sdk/commit/2671c79601d09d963bd42d85824d3cddc2ae17d3))
* **httpserver:** never send internal error text in HTTP replies ([74082e8](https://github.com/trypanic/go-sdk/commit/74082e8eca801c0cc32b4aa33cab935e170866fa))
* **httpserver:** store Reply data and metadata values, not pointers ([6018384](https://github.com/trypanic/go-sdk/commit/60183840fd06261864ab4a9025c26384571458c2))
* **integration:** bind services and Ryuk to loopback ([40580e0](https://github.com/trypanic/go-sdk/commit/40580e09ed2cacba304ca6110e5665358c0f7b8b))
* **llmclient:** never print the API key ([97c5e5d](https://github.com/trypanic/go-sdk/commit/97c5e5d91f88ff9b73d94f06c1952793de8c069f))
* **llmclient:** require an API key and an https endpoint before sending ([bc9ab0c](https://github.com/trypanic/go-sdk/commit/bc9ab0cfb5c71b2c7656f92945f68b17173333c9))
* **logger:** keep InitLogProvider and SetupOTLP lenient and never failing ([2c3f3a0](https://github.com/trypanic/go-sdk/commit/2c3f3a094f9ffc70e570639a78289eb4c17d39d3))
* **logger:** log wrapped and nil errors correctly; RecordError(nil) no-op ([2137d99](https://github.com/trypanic/go-sdk/commit/2137d99d3c0e4ace8890caf5e028abfc9bd63161))
* **logger:** mask secret/access/signing keys and full URL passwords ([8f65f47](https://github.com/trypanic/go-sdk/commit/8f65f47b27f8c1a58315ea4f646ccddc6a98c0f3))
* **logger:** replace deprecated otel/log/global with otel APIs ([1f82bed](https://github.com/trypanic/go-sdk/commit/1f82bedcde44d45a3a1d939636cdb9f2a8109a4a))
* **logger:** send every line with a backslash through full redaction ([330963c](https://github.com/trypanic/go-sdk/commit/330963c7c9cfab925144624778c27ff0715a52ea))
* **logger:** stop New from mutating zerolog globals ([6d5b7eb](https://github.com/trypanic/go-sdk/commit/6d5b7ebf0403fb652fd12199eecce0b846a37ac0))
* **logger:** widen log redaction to tokens, secrets and URL credentials ([11e4780](https://github.com/trypanic/go-sdk/commit/11e478048b5d40fd0d7ba7506fde018fab6c5c07))
* **messaging:** check Close errors on teardown and reconnect paths ([f7508c2](https://github.com/trypanic/go-sdk/commit/f7508c2e8fbeba540a02463f038beb6d90d61bfe))
* **messaging:** detach the old connection before a reconnect closes it ([9676d59](https://github.com/trypanic/go-sdk/commit/9676d5905575ac1a3cf1b9c77c959490e641d5a2))
* **messaging:** fail Subscribe with ERR_MQ_CLOSED when Close races it ([d745334](https://github.com/trypanic/go-sdk/commit/d745334a0aefe6228c2bf84c9c3ce1c8cfa837f0))
* **messaging:** honour ctx in Publish and dial outside the mutex ([0476d0f](https://github.com/trypanic/go-sdk/commit/0476d0f4c247801477893e42aa1717ef982b7df0))
* **messaging:** log recovered handler panics at Error level ([59bbc56](https://github.com/trypanic/go-sdk/commit/59bbc56ee2a4ddfb9d22f80378095684eec85f69))
* **messaging:** recover handler panics and dead-letter the message ([307fae0](https://github.com/trypanic/go-sdk/commit/307fae0b1efccd7095e8ab26f07fc3108b9bb4a7))
* **messaging:** reopen the link after a nack that closed the channel ([4a15433](https://github.com/trypanic/go-sdk/commit/4a15433f360db4a72870e5cac5c08469c83a426e))
* **messaging:** report failed Ack and Nack instead of discarding them ([99dd969](https://github.com/trypanic/go-sdk/commit/99dd969288dee2b0bd8f1e410a2d2f99649a4cb1))
* **messaging:** requeue in-flight messages on subscription shutdown ([745bd0a](https://github.com/trypanic/go-sdk/commit/745bd0a2fa3c6c35eb96cc80b03a2ea21c6ee8fc))
* **messaging:** return SDK error codes unchanged ([23afe68](https://github.com/trypanic/go-sdk/commit/23afe68c3e7197c5b94cbe106a7155f6a6ad3420))
* **messaging:** route publisher diagnostics through the injected logger ([ef0d848](https://github.com/trypanic/go-sdk/commit/ef0d8486e10b5d30490922958534edc733dc08d5))
* **mongo:** classify decode failures apart from operation errors ([bd4b1e0](https://github.com/trypanic/go-sdk/commit/bd4b1e0eed7d63b1788908f7276fac0a2d85d1f8))
* **mongo:** trace cursor iteration and size the start-up ping budget ([3752993](https://github.com/trypanic/go-sdk/commit/37529936026f08a23872e077b887f99e6ed3628b))
* **otlp:** preserve typed-nil optional writer behavior ([242ffcb](https://github.com/trypanic/go-sdk/commit/242ffcb0a30e0bd4cb504bae77b05c04e47f766a))
* **postgres:** classify JSON argument encoding failures ([1e49097](https://github.com/trypanic/go-sdk/commit/1e49097fdfa263b36258037f7f026d4b32f1ffc1))
* **postgres:** classify statement and lock timeouts as timeouts ([422f509](https://github.com/trypanic/go-sdk/commit/422f509dd627dcb5b82cadb5a894fe1969d2cd6b))
* **postgres:** parse DSN passwords containing '/', '?' or '#' ([af223db](https://github.com/trypanic/go-sdk/commit/af223db732e93989fe70b54a82debde71b5716d6))
* **postgres:** return the inner StoredProcedurer for a nil instrumenter ([543f0d7](https://github.com/trypanic/go-sdk/commit/543f0d70684e4b2ff515214fe8845e0ad723f399))
* **postgres:** wrap pool errors instead of exposing the raw driver error ([880ed17](https://github.com/trypanic/go-sdk/commit/880ed17534e89bba6b5ca54e2f0065f2ff57e450))
* **slices:** clip Chunk sub-slice capacity ([c802dcb](https://github.com/trypanic/go-sdk/commit/c802dcbf6e37baf62083ea8f9447543360545e31))
* **storage:** drop a garbled last line like a torn one ([97027bd](https://github.com/trypanic/go-sdk/commit/97027bd23dbe48bd29b6070d83c54142f4a24aa5))
* **storage:** keep 125-byte keys written by earlier versions reachable ([7607354](https://github.com/trypanic/go-sdk/commit/760735407447982a2daaa91842477812cdf69200))
* **storage:** make FileStorage.Clear delete only files it owns ([1043208](https://github.com/trypanic/go-sdk/commit/1043208061c352ebed2df77a2e0a389b24124720))
* **storage:** never read or append through a symlink at a key's path ([d5fc424](https://github.com/trypanic/go-sdk/commit/d5fc424afa88e19243d58cdad057ce74d481673c))
* **storage:** reject FileStorage modes that lock the owner out ([7741c14](https://github.com/trypanic/go-sdk/commit/7741c1436dc97b797b83fe7f15356df37fb42326))
* **storage:** return an error from FileStorage.Append(nil) ([f0a8c2e](https://github.com/trypanic/go-sdk/commit/f0a8c2e1b6af939634b58e42f6df82847458c9cf))
* **storage:** write FileStorage files through unique synced temp files ([c8b1231](https://github.com/trypanic/go-sdk/commit/c8b12313538a998ff15dea44a8f74559ad36a28e))
* **stringutils:** parse RemoveMarkdownCodeBlock fences line by line ([acfcdfc](https://github.com/trypanic/go-sdk/commit/acfcdfc317869ae56f1cad374d9d4497a3dc9325))
* **telemetry:** make InitTracer safe to call concurrently with spans ([18ceb3e](https://github.com/trypanic/go-sdk/commit/18ceb3e70e7fda97e04fc788074e3f1e995ca62f))
* **telemetry:** record classified failures in client spans ([719de34](https://github.com/trypanic/go-sdk/commit/719de34b5756e13103824d7c70f74138d0ac3fe0))
* **urlkit:** preserve escaped base paths and reject opaque URLs ([2fb7625](https://github.com/trypanic/go-sdk/commit/2fb7625ffdec106ced42fe0cd48c83108d6b2904))
* **urlkit:** reject dot segments in JoinPath and add JoinHTTPPath ([d5c047b](https://github.com/trypanic/go-sdk/commit/d5c047b9f248afc745a5856749207dafe96a449a))
* **urlkit:** treat BuildURL path as a path, never as URL syntax ([d390f64](https://github.com/trypanic/go-sdk/commit/d390f64ccbb3869abc91f44d1a8da7200cf0eca1))
* **validators:** consistent error codes and messages ([5555207](https://github.com/trypanic/go-sdk/commit/555520772cd769924897f33c107a37759aec1dd3))


### Performance Improvements

* **errorkit:** capture stacks with runtime.Callers and start at the caller ([b9e2d5f](https://github.com/trypanic/go-sdk/commit/b9e2d5f8b3107bfe29900954d08481b7b4dd38bc))
* **httprequest:** stop buffering decoded responses a second time ([7b5e562](https://github.com/trypanic/go-sdk/commit/7b5e56277bf5ebd54c879b09b098ba2b98decf95))
* **logger:** skip JSON re-parsing for lines with nothing to redact ([6d4725f](https://github.com/trypanic/go-sdk/commit/6d4725f62f90cd6bdb98b417e4c478d7eb3ccead))
* **slices:** preallocate Chunk and MergeUniqueStrings, document size contract ([aaf363a](https://github.com/trypanic/go-sdk/commit/aaf363ac0031ef53940a410b05f12bb19cd73594))
* **stringutils:** normalize text in a single pass without regexp ([e5bd2c6](https://github.com/trypanic/go-sdk/commit/e5bd2c619486f808ebde764aad240eda96e7b352))


### Code Refactoring

* **adapters:** return concrete clients and expose escape hatches ([f9cca24](https://github.com/trypanic/go-sdk/commit/f9cca24f5314c64b5ba746ed0d5764ca7a4fe710))
* **algorithms:** expose an SDK-owned backoff contract ([2ecddb7](https://github.com/trypanic/go-sdk/commit/2ecddb7002560074a7c7a1c84ac009af24b47301))
* **api:** remove stuttering names and redundant constructors ([0a43004](https://github.com/trypanic/go-sdk/commit/0a430040c01f0a2a97625e4a866cf6c86101c091))
* **envs,validators,marshal,ioutils:** use errors.AsType in tests and samples ([785b1b4](https://github.com/trypanic/go-sdk/commit/785b1b4a4d7f2b799a2a17cb4a202cfa1183d313))
* **errorkit:** clamp the stack depth with min/max builtins ([de0e83d](https://github.com/trypanic/go-sdk/commit/de0e83d5348723a7b7d8331433e85682b24a4b35))
* **errorkit:** copy the default registry with maps.Clone ([68bdf18](https://github.com/trypanic/go-sdk/commit/68bdf18f7871c78b005e530482ef8d54a52968aa))
* **errorkit:** own external service error codes ([0eb616c](https://github.com/trypanic/go-sdk/commit/0eb616c0fa9e1f9df3a694de3a7d1528c968e2b2))
* **grpc:** use errors.AsType in ToStatusWithPolicy ([d4cd981](https://github.com/trypanic/go-sdk/commit/d4cd981ca2e5a2877cbfd087abc8aaef566b18c9))
* **hertz:** drop the context.Background fallback in hertzContext ([4e4cea8](https://github.com/trypanic/go-sdk/commit/4e4cea81c470e327dd601f5b0ce0d2f85535f3b7))
* **httpclient,httprequest:** use min and stop shadowing max ([c3d0794](https://github.com/trypanic/go-sdk/commit/c3d0794d02ec2bd2f08d5a1e14bceca4b7634f6e))
* **httprequest,httpserver:** use errors.AsType for typed error checks ([ca8b887](https://github.com/trypanic/go-sdk/commit/ca8b887b1730f3ef3088c1514eae83621819c1ea))
* **httprequest:** use reflect.Pointer and reflect.TypeFor ([6cfeb51](https://github.com/trypanic/go-sdk/commit/6cfeb5163bae8424b39ce79315a981cd828c868f))
* **llmclient:** use omitzero for numeric request tuning fields ([d737eb4](https://github.com/trypanic/go-sdk/commit/d737eb4e7640d6ef474439b854f7a2636eafa9d4))
* **logger:** isolate OTLP export in a dedicated package ([4504ec6](https://github.com/trypanic/go-sdk/commit/4504ec6dba927d803e99191a757db31059443061))
* **logger:** resolve golangci-lint ST1023 and errcheck reports ([98bd9ca](https://github.com/trypanic/go-sdk/commit/98bd9ca2be65420dd4b881bdc6b0a3641262bbe5))
* **logger:** sort console field keys with slices.Sorted(maps.Keys) ([04bdad2](https://github.com/trypanic/go-sdk/commit/04bdad2f4ea57a53521fe14bab785fe5f0a1077f))
* **logging:** share context lookup without exporter coupling ([39f7e07](https://github.com/trypanic/go-sdk/commit/39f7e07044585f6e67dd1f6f8e29b9c4ea515f58))
* **marshal,ioutils:** share one no-escape JSON encoder ([743d2d3](https://github.com/trypanic/go-sdk/commit/743d2d38334aef406ba8df3af74c319f1ab7179f))
* **messaging:** cap the reconnect delay with min ([3e27f2b](https://github.com/trypanic/go-sdk/commit/3e27f2b3d0924839266ba7910f5d2e754b3775e5))
* **messaging:** route AMQP calls through an unexported seam ([7034163](https://github.com/trypanic/go-sdk/commit/7034163d57c569c896e16c9956d1b777e472b929))
* **messaging:** use omitzero for numeric topology fields ([5f66ca0](https://github.com/trypanic/go-sdk/commit/5f66ca0d1a3dc6fac909317e24c8c3967c4136e5))
* **mongo:** drop the redundant loop-variable copy in TestNormalizeURI ([719806b](https://github.com/trypanic/go-sdk/commit/719806bcbc94f575b3be39394cbd25ab996cad04))
* **mongo:** use errors.AsType in tests ([82b0a9f](https://github.com/trypanic/go-sdk/commit/82b0a9f550bdbd062a82c3d8e734ddc8d5faa13f))
* **mongo:** use t.Context in tests ([af4975e](https://github.com/trypanic/go-sdk/commit/af4975e02298476fe2022e6ffef27a5d770364da))
* **packages:** align database names and isolate batch helpers ([ce3ebe4](https://github.com/trypanic/go-sdk/commit/ce3ebe4796bbb52bb7ee70bd8ce4f7d1431fbd5d))
* **postgres:** use errors.AsType in tests ([673a51c](https://github.com/trypanic/go-sdk/commit/673a51c0c6df819f4150e2af6fff5dec9e7778e1))
* **postgres:** use t.Context in tests ([16d9ec8](https://github.com/trypanic/go-sdk/commit/16d9ec87ba07d474eeefb2044f35e9fa15fe1839))
* **retry:** share bounded full-jitter transport backoff ([9c5f525](https://github.com/trypanic/go-sdk/commit/9c5f52595bc7e78d5b205ea54e22ee0725beef46))
* **storage:** use errors.AsType in tests ([c949925](https://github.com/trypanic/go-sdk/commit/c949925034cb373de13f4b354405ba6534b2fdc7))
* **telemetry:** default the instrumentation scope with cmp.Or ([298e515](https://github.com/trypanic/go-sdk/commit/298e5157d007e6d73ae700b0b9cf9e1d3c7b7ecd))
* **telemetry:** put context first in private span helpers ([02a1632](https://github.com/trypanic/go-sdk/commit/02a16320868223af2813033cb2f3347ab841b919))
* **telemetry:** use bounded operation span names ([3c7d7a1](https://github.com/trypanic/go-sdk/commit/3c7d7a166c5d3b7ee2a1f153024b818426768177))


### Documentation

* add a vulnerability reporting and supported-versions policy ([228a5e1](https://github.com/trypanic/go-sdk/commit/228a5e150299fca6651eaa93b92207805e7141e2))
* add the MIT licence ([dc33129](https://github.com/trypanic/go-sdk/commit/dc3312923927d8f62f3c4e06e628104480017cf1))
* **audit:** record R-DEPS review corrections and validation ([ddf7afd](https://github.com/trypanic/go-sdk/commit/ddf7afd873c661ef70e50f94882e4b01b98d8549))
* consumer README sections, release-notes wording and CONTRIBUTING.md ([e14d5e7](https://github.com/trypanic/go-sdk/commit/e14d5e7452e3cb5c5c9d527d18a65a394b333fd2))
* correct residual adapter contract references ([0c3c5ed](https://github.com/trypanic/go-sdk/commit/0c3c5ed9a6718f221db6f5fb172299ae0c1e4e41))
* do not hard-code the current release in SECURITY.md ([94beeda](https://github.com/trypanic/go-sdk/commit/94beeda1253b8b05c4abb1e61fe25cdfc5fcb880))
* document task check de-duplication and git-cliff in tools:install ([662d189](https://github.com/trypanic/go-sdk/commit/662d189fb44367abe16793b99eb1955d7b41949f))
* **errorkit,logger,telemetry:** package docs, exported-symbol docs and README fixes ([5213861](https://github.com/trypanic/go-sdk/commit/5213861a41165333a098a1cda162145732fb4d01))
* **grpc:** document that raw interceptors run outside recovery ([790ea74](https://github.com/trypanic/go-sdk/commit/790ea74f8c4f4c3451121defbcd324ab024bdb6a))
* **httpclient,httprequest,httpserver,llmclient:** correct package docs ([c7977c5](https://github.com/trypanic/go-sdk/commit/c7977c577956838c1ae4989f37bc739e746029d8))
* **messaging:** add package docs, concurrency notes and examples ([e366890](https://github.com/trypanic/go-sdk/commit/e36689074f35cad67a0a6c6b323250b13132b217))
* **messaging:** declare the specific error codes returned by Publish ([29e6a03](https://github.com/trypanic/go-sdk/commit/29e6a034d4cc62eb377a5ab7d940c941166b3969))
* **messaging:** document retry defaults and DLX/amq.* validation ([8ce445e](https://github.com/trypanic/go-sdk/commit/8ce445e6d50de7fdef77a96a30b90ef830c11bd5))
* **postgres,mongo,storage:** add package docs, symbol docs and fixed samples ([e7465c1](https://github.com/trypanic/go-sdk/commit/e7465c14e952f11594d61183f69b01fa003f22ff))
* recommend required CI checks on main, satisfied by the manual run ([dfc611e](https://github.com/trypanic/go-sdk/commit/dfc611ef62c09d12e3f973a2560879badcc30a24))
* **sdk:** complete package contracts and executable examples ([9be7035](https://github.com/trypanic/go-sdk/commit/9be70350f919b6d8876f913cc840bbd67013bafa))
* **sdk:** finalize v2 consumer and agent contracts ([6a390d4](https://github.com/trypanic/go-sdk/commit/6a390d4f0dbc21af740597a7b2314d09b9129c1b))
* **security:** verify the tagged crypto advisory decision ([73f81c6](https://github.com/trypanic/go-sdk/commit/73f81c6978892c8026cec9ddd77888170dc8275e))
* **storage:** state the version compatibility of the JSON Lines format ([b07f5ae](https://github.com/trypanic/go-sdk/commit/b07f5aeb6c4af2ce4648ef62fb856fce234e4f66))
* **urlkit,validators,marshal,slices,stringutils,algorithms:** add package docs and fix README claims ([5d5c63d](https://github.com/trypanic/go-sdk/commit/5d5c63d9abe37b204aeb94c11b968c4bbfa0b1e7))

## [1.0.1](https://github.com/trypanic/go-sdk/compare/v1.0.0...v1.0.1) (2026-11-04)


### Bug Fixes

* **deps:** update all module dependencies ([1e25390](https://github.com/trypanic/go-sdk/commit/1e253905079acf18972174681e3e8e6adf8a26e8))
