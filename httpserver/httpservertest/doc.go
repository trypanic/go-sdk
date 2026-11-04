// Package httpservertest provides a framework-free HTTP handler recorder.
//
// Configure Context before invoking a handler and assert Response and Err
// afterwards. A context is confined to one goroutine and owns no network resources.
package httpservertest
