// Package mongotest provides callback-based MongoDB test doubles without a server.
//
// Configure callbacks before use; unexpected calls panic. Callbacks own
// synchronization and any resources they create. Cursor follows the driver
// convention of one goroutine at a time.
package mongotest
