# httpservertest

Framework-free `Context` recorder backed by `net/http/httptest`.

Import `github.com/trypanic/go-sdk/httpserver/httpservertest` in consumer tests. These helpers create
no network connections and require no infrastructure. The external-package
`*_test.go` files provide executable consumer examples.

Configure fields before invoking application code. HTTP contexts and cursors are
confined to one goroutine. Shared callback fakes require concurrency-safe
callbacks; they add no implicit locking. Callers own any resources their callbacks
create. Unconfigured Mongo and messaging operations panic to expose unexpected
calls. Capture arguments or inject errors in callbacks to assert application
behavior without duplicating the production implementation.

Assert `Context.Err` after each handler to catch request-read and response-encoding failures. Encoding failures produce an empty HTTP 500 response.
