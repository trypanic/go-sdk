package hertz

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
)

// Server is the Hertz-backed httpserver.HTTPServer.
type Server struct {
	engine    *server.Hertz
	config    httpserver.ServerConfig
	replyOpts httpserver.ReplyOptions
	// inFlight counts requests inside the handler chain, so Shutdown can
	// tell in-flight work from idle keep-alive connections.
	inFlight atomic.Int64
}

// shutdownWaitCeiling replaces Hertz's ExitWaitTimeout so that only the
// context passed to Shutdown bounds the drain.
const shutdownWaitCeiling = 24 * time.Hour

// drainPollInterval is how often Shutdown checks for in-flight requests.
const drainPollInterval = 10 * time.Millisecond

// trackInFlight is the first middleware of every engine.
func (s *Server) trackInFlight(ctx context.Context, c *app.RequestContext) {
	s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	c.Next(ctx)
}

func (s *Server) Group(prefix string) httpserver.RouterGroup {
	g := s.engine.Group(prefix)
	return &hertzRouterGroup{group: g, server: s}
}

func (s *Server) Use(middleware ...httpserver.MiddlewareFunc) {
	for _, mw := range middleware {
		s.engine.Use(s.wrapMiddleware(mw))
	}
}

func (s *Server) GET(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.GET(endpoint, s.wrapHandler(handler))
}
func (s *Server) POST(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.POST(endpoint, s.wrapHandler(handler))
}
func (s *Server) PUT(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.PUT(endpoint, s.wrapHandler(handler))
}
func (s *Server) DELETE(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.DELETE(endpoint, s.wrapHandler(handler))
}

func (s *Server) wrapMiddleware(mw httpserver.MiddlewareFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{ctx: ctx, requestCtx: c, replyOpts: s.replyOpts}
		next := func(ctx context.Context, _ httpserver.HTTPContext) { c.Next(ctx) }
		mw(ctx, hctx, next)
	}
}

func (s *Server) wrapHandler(handler httpserver.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{ctx: ctx, requestCtx: c, replyOpts: s.replyOpts}
		handler(ctx, hctx)
	}
}

// Run listens on the configured address and serves until Shutdown is
// called, then returns nil. A listen failure (address in use, permission
// denied) is returned as an *errorkit.AppError instead of crashing the
// process. Run installs no signal handlers: the caller owns the process
// lifecycle (for example with signal.NotifyContext) and calls Shutdown.
func (s *Server) Run() (err error) {
	addr := s.config.Address()
	// Hertz's netpoll transport panics when it cannot bind. Run calls the
	// engine on this goroutine, so the panic can be turned into an error.
	defer func() {
		if r := recover(); r != nil {
			err = listenError(addr, fmt.Errorf("%v", r))
		}
	}()
	if runErr := s.engine.Run(); runErr != nil {
		return listenError(addr, runErr)
	}
	return nil
}

// listenError reports a server that could not start on addr.
func listenError(addr string, cause error) error {
	return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
		errorkit.WithReason("http server cannot listen on %s", addr),
		errorkit.WithWrapped(cause),
	)
}

// Shutdown stops accepting connections and waits until no request is in
// flight or ctx is done, whichever comes first. Idle keep-alive
// connections do not delay it. When ctx ends while requests are still in
// flight it returns ERR_SYSTEM_TIMEOUT_INTERNAL. Calling it before Run
// returns Hertz's "engine is not running" error unchanged.
func (s *Server) Shutdown(ctx context.Context) error {
	// Hertz waits for every open connection, idle keep-alive ones
	// included; end its wait as soon as no request is in flight.
	drainCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		ticker := time.NewTicker(drainPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-drainCtx.Done():
				return
			case <-ticker.C:
				if s.inFlight.Load() == 0 {
					cancel()
					return
				}
			}
		}
	}()

	if err := s.engine.Shutdown(drainCtx); err != nil {
		return err
	}
	if n := s.inFlight.Load(); ctx.Err() != nil && n > 0 {
		return errorkit.NewError(errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL).With(
			errorkit.WithReason("http server shutdown ended with %d request(s) in flight", n),
			errorkit.WithWrapped(ctx.Err()),
		)
	}
	return nil
}

// PATCH registers an HTTP PATCH route.
func (s *Server) PATCH(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.PATCH(endpoint, s.wrapHandler(handler))
}

// HEAD registers an HTTP HEAD route.
func (s *Server) HEAD(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.HEAD(endpoint, s.wrapHandler(handler))
}

// OPTIONS registers an HTTP OPTIONS route.
func (s *Server) OPTIONS(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.OPTIONS(endpoint, s.wrapHandler(handler))
}

// Handle registers a route for an explicit HTTP method.
func (s *Server) Handle(method, endpoint string, handler httpserver.HandlerFunc) {
	s.engine.Handle(method, endpoint, s.wrapHandler(handler))
}

// Engine returns the underlying Hertz engine. Configure it before serving.
func (s *Server) Engine() *server.Hertz { return s.engine }
