// Package messagingtest provides callback-based messaging test doubles.
//
// Broker has explicit Publish, Subscribe and Close callbacks. Unset callbacks
// panic to expose unexpected operations. Callbacks own synchronization and
// lifecycle; the fake creates no connections or background goroutines.
package messagingtest
