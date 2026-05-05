package hertz

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/trypanic/go-sdk/httpserver"
)

// wrapperHertzServer is the Hertz-backed httpserver.HTTPServer.
type wrapperHertzServer struct {
	engine    *server.Hertz
	config    httpserver.ServerConfig
	replyOpts httpserver.ReplyOptions
}

func (s *wrapperHertzServer) Group(prefix string) httpserver.RouterGroup {
	g := s.engine.Group(prefix)
	return &hertzRouterGroup{group: g, server: s}
}

func (s *wrapperHertzServer) Use(middleware ...httpserver.MiddlewareFunc) {
	for _, mw := range middleware {
		s.engine.Use(s.wrapMiddleware(mw))
	}
}

func (s *wrapperHertzServer) GET(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.GET(endpoint, s.wrapHandler(handler))
}
func (s *wrapperHertzServer) POST(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.POST(endpoint, s.wrapHandler(handler))
}
func (s *wrapperHertzServer) PUT(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.PUT(endpoint, s.wrapHandler(handler))
}
func (s *wrapperHertzServer) DELETE(endpoint string, handler httpserver.HandlerFunc) {
	s.engine.DELETE(endpoint, s.wrapHandler(handler))
}

func (s *wrapperHertzServer) wrapMiddleware(mw httpserver.MiddlewareFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{requestCtx: c, replyOpts: s.replyOpts}
		next := func(ctx context.Context, _ httpserver.HTTPContext) { c.Next(ctx) }
		mw(ctx, hctx, next)
	}
}

func (s *wrapperHertzServer) wrapHandler(handler httpserver.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{requestCtx: c, replyOpts: s.replyOpts}
		handler(ctx, hctx)
	}
}

func (s *wrapperHertzServer) Run() error {
	s.engine.Spin()
	return nil
}

// Shutdown gracefully stops the engine. The supplied context bounds the
// drain window before in-flight requests are forced to finish.
func (s *wrapperHertzServer) Shutdown(ctx context.Context) error {
	return s.engine.Shutdown(ctx)
}
