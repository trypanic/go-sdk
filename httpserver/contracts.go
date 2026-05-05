// Package httpserver provides a framework-agnostic HTTP interface for Go.
// It defines common interfaces that can be implemented by various HTTP frameworks
// (Gin, Echo, Hertz, etc.), enabling framework-independent handlers and middleware.
package httpserver

import (
	"context"
)

// HTTPContext provides framework-agnostic access to HTTP request/response operations.
// Methods are safe within a single request but not for concurrent use across goroutines.
type HTTPContext interface {
	// ===== Request Methods =====

	// Param returns the URL path parameter value by name.
	// Example: For route "/users/:id" and URL "/users/123", Param("id") returns "123".
	// Returns empty string if parameter doesn't exist.
	Param(key string) string

	// Query returns the URL query parameter value by key.
	// Example: For URL "/search?q=golang", Query("q") returns "golang".
	// Returns empty string if parameter doesn't exist.
	Query(key string) string

	// BindJSON parses the JSON request body into the provided struct.
	// Returns error if parsing fails.
	BindJSON(obj any) error

	// GetBody returns the raw request body as bytes.
	GetBody() []byte

	// GetHeader retrieves an HTTP request header value by name.
	GetHeader(key string) string

	// ===== Reply Methods =====

	// JSON sends a JSON response with the given status code.
	// Data is automatically marshaled to JSON.
	JSON(statusCode int, data any)

	// String sends a plain text response with the given status code.
	String(statusCode int, message string)

	// Status sets the HTTP status code without a response body.
	Status(statusCode int)

	// SetHeader sets an HTTP response header.
	SetHeader(key, value string)

	// Redirect performs an HTTP redirect to the specified location.
	// Common status codes:
	//   301 - Permanent redirect (cached by browsers)
	//   302 - Temporary redirect (most common)
	//   303 - Redirect after POST (changes method to GET)
	//   307 - Temporary redirect (preserves method)
	//   308 - Permanent redirect (preserves method)
	//
	// Location can be absolute ("https://example.com") or relative ("/login").
	Redirect(statusCode int, location []byte)

	Reply(status int, options ...OptionReply)

	WithMessage(message string) OptionReply
	WithError(err error) OptionReply
	WithMetadata(metadata any) OptionReply
	WithData(data any) OptionReply
}

// HandlerFunc is a framework-agnostic HTTP handler function.
//
// The context.Context (first parameter) should be propagated to all I/O operations,
// database calls, and external API calls, following Go conventions.
//
// Example:
//
//	func GetUser(ctx context.Context, c interfaces.HTTPContext) {
//	    userID := c.Param("id")
//	    user, err := userRepo.FindByID(ctx, userID)
//	    if err != nil {
//	        c.JSON(500, map[string]string{"error": err.Error()})
//	        return
//	    }
//	    c.JSON(200, user)
//	}
type HandlerFunc func(ctx context.Context, c HTTPContext)

// MiddlewareFunc is a framework-agnostic middleware function.
// Middleware executes before/after handlers and must call next() to continue the chain.
//
// Example with logging:
//
//	func LoggerMiddleware(ctx context.Context, c interfaces.HTTPContext, next interfaces.HandlerFunc) {
//	    start := time.Now()
//	    next(ctx, c)
//	    log.Printf("Request completed in %v", time.Since(start))
//	}
//
// Example with authentication:
//
//	func AuthMiddleware(ctx context.Context, c interfaces.HTTPContext, next interfaces.HandlerFunc) {
//	    if c.GetHeader("Authorization") == "" {
//	        c.JSON(401, map[string]string{"error": "unauthorized"})
//	        return // Don't call next() to stop the chain
//	    }
//	    next(ctx, c)
//	}
type MiddlewareFunc func(ctx context.Context, c HTTPContext, next HandlerFunc)

// RouterGroup represents a group of routes with a common prefix and middleware.
//
// Example:
//
//	api := server.Group("/api")
//	api.Use(AuthMiddleware)
//
//	v1 := api.Group("/v1")           // Creates prefix "/api/v1"
//	v1.GET("/users", GetUsers)       // Route: GET /api/v1/users
//	v1.POST("/users", CreateUser)    // Route: POST /api/v1/users
type RouterGroup interface {
	// GET registers a handler for HTTP GET requests.
	// Endpoint is relative to the group's prefix.
	GET(endpoint string, handler HandlerFunc)

	// POST registers a handler for HTTP POST requests.
	POST(endpoint string, handler HandlerFunc)

	// PUT registers a handler for HTTP PUT requests.
	PUT(endpoint string, handler HandlerFunc)

	// DELETE registers a handler for HTTP DELETE requests.
	DELETE(endpoint string, handler HandlerFunc)

	// Group creates a sub-group with the given prefix appended to the current prefix.
	Group(prefix string) RouterGroup

	// Use adds middleware to this group and all its sub-groups.
	// Middleware executes in registration order.
	Use(middleware ...MiddlewareFunc)
}

// HTTPServer represents the HTTP server interface (Primary Port).
//
// Benefits:
// - Framework-agnostic: Switch between Gin/Echo/Hertz with only adapter changes
// - Clean architecture: Business logic never imports framework packages
// - Testable: Easy to mock for unit tests
type HTTPServer interface {
	// GET registers a handler for HTTP GET requests.
	// Supports path parameters using :param syntax (e.g., "/user/:id").
	GET(endpoint string, handler HandlerFunc)

	// POST registers a handler for HTTP POST requests.
	POST(endpoint string, handler HandlerFunc)

	// PUT registers a handler for HTTP PUT requests.
	PUT(endpoint string, handler HandlerFunc)

	// DELETE registers a handler for HTTP DELETE requests.
	DELETE(endpoint string, handler HandlerFunc)

	// Group creates a router group with the given prefix.
	// Useful for organizing routes and applying middleware to specific route sets.
	Group(prefix string) RouterGroup

	// Use adds global middleware applied to ALL routes.
	// Middleware executes in registration order.
	Use(middleware ...MiddlewareFunc)

	// Run starts the HTTP server and blocks until stopped.
	// Returns error if server fails to start.
	Run() error

	// Shutdown stops the server gracefully. The supplied context bounds
	// how long Shutdown waits for in-flight requests to finish before
	// forcing termination.
	Shutdown(ctx context.Context) error
}
