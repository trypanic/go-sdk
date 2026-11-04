// Package httpserver defines framework-independent HTTP routing and reply contracts.
//
// Use the hertz subpackage for a concrete server. StatusFor and ReplyError
// map registered errors to client-safe transport responses. Context values are
// confined to one request; server lifecycle belongs to the caller.
package httpserver
