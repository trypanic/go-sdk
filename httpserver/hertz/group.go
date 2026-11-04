package hertz

import (
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/trypanic/go-sdk/httpserver"
)

type hertzRouterGroup struct {
	group  *route.RouterGroup
	server *wrapperHertzServer
}

func (g *hertzRouterGroup) GET(endpoint string, handler httpserver.HandlerFunc) {
	g.group.GET(endpoint, g.server.wrapHandler(handler))
}

func (g *hertzRouterGroup) POST(endpoint string, handler httpserver.HandlerFunc) {
	g.group.POST(endpoint, g.server.wrapHandler(handler))
}

func (g *hertzRouterGroup) PUT(endpoint string, handler httpserver.HandlerFunc) {
	g.group.PUT(endpoint, g.server.wrapHandler(handler))
}

func (g *hertzRouterGroup) DELETE(endpoint string, handler httpserver.HandlerFunc) {
	g.group.DELETE(endpoint, g.server.wrapHandler(handler))
}

func (g *hertzRouterGroup) Group(prefix string) httpserver.RouterGroup {
	subGroup := g.group.Group(prefix)
	return &hertzRouterGroup{group: subGroup, server: g.server}
}

func (g *hertzRouterGroup) Use(middleware ...httpserver.MiddlewareFunc) {
	for _, mw := range middleware {
		g.group.Use(g.server.wrapMiddleware(mw))
	}
}

// PATCH registers an HTTP PATCH route.
func (g *hertzRouterGroup) PATCH(endpoint string, handler httpserver.HandlerFunc) {
	g.group.PATCH(endpoint, g.server.wrapHandler(handler))
}

// HEAD registers an HTTP HEAD route.
func (g *hertzRouterGroup) HEAD(endpoint string, handler httpserver.HandlerFunc) {
	g.group.HEAD(endpoint, g.server.wrapHandler(handler))
}

// OPTIONS registers an HTTP OPTIONS route.
func (g *hertzRouterGroup) OPTIONS(endpoint string, handler httpserver.HandlerFunc) {
	g.group.OPTIONS(endpoint, g.server.wrapHandler(handler))
}

// Handle registers a route for an explicit HTTP method.
func (g *hertzRouterGroup) Handle(method, endpoint string, handler httpserver.HandlerFunc) {
	g.group.Handle(method, endpoint, g.server.wrapHandler(handler))
}
