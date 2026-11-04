# messagingtest

Callback-based `Broker` implementing the public publish/subscribe port.

Import `github.com/trypanic/go-sdk/messaging/messagingtest` in consumer tests. These helpers create
no network connections and require no infrastructure. The external-package
`*_test.go` files provide executable consumer examples.

Configure fields before invoking application code. HTTP contexts and cursors are
confined to one goroutine. Shared callback fakes require concurrency-safe
callbacks; they add no implicit locking. Callers own any resources their callbacks
create. Unconfigured Mongo and messaging operations panic to expose unexpected
calls. Capture arguments or inject errors in callbacks to assert application
behavior without duplicating the production implementation.
