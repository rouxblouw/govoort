package ui

import (
	"net/http"
	"strings"
)

// Kind tags what sort of response a route produces
type Kind int

const (
	KindPage Kind = iota
	KindJson
)

// Context is Go's answer to Express's (req, res) — one value threaded
// through the whole request lifecycle.
type Context struct {
	W    http.ResponseWriter
	R    *http.Request
	Kind Kind
}

// HandlerFunc is our equivalent of an Express route handler.
type HandlerFunc func(ctx *Context) error

// Middleware wraps a HandlerFunc to produce a new HandlerFunc
type Middleware func(next HandlerFunc) HandlerFunc

// The Router
type Router struct {
	mux        *http.ServeMux
	middleware []Middleware
}

func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Use registers middleware that applies to every route added after this
// call on this router (and to anything mounted under it — see Mount).
func (rt *Router) Use(mw Middleware) {
	rt.middleware = append(rt.middleware, mw)
}

func (rt *Router) Get(pattern string, h HandlerFunc)  { rt.handle("GET", pattern, h) }
func (rt *Router) Post(pattern string, h HandlerFunc) { rt.handle("POST", pattern, h) }

func (rt *Router) handle(method, pattern string, h HandlerFunc) {
	// wrap innermost-out: middleware registered first ends up outermost
	for i := len(rt.middleware) - 1; i >= 0; i-- {
		h = rt.middleware[i](h)
	}
	rt.mux.HandleFunc(method+" "+pattern, func(w http.ResponseWriter, r *http.Request) {
		ctx := &Context{W: w, R: r}
		if err := h(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// Raw registers a plain http.HandlerFunc, bypassing Page/Json and all
// framework middleware (Recover, ErrorMiddleware, Logging, etc). Intended
// for static assets and similar low-risk, framework-agnostic routes.
func (rt *Router) RawGet(pattern string, h http.HandlerFunc) {
	rt.mux.HandleFunc("GET "+pattern, h)
}

// ServeHTTP makes Router itself an http.Handler — this is what lets a
// Router be mounted inside another one
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mux.ServeHTTP(w, r)
}

func (rt *Router) Mount(prefix string, sub *Router) {
	rt.mux.Handle(prefix+"/", http.StripPrefix(strings.TrimSuffix(prefix, "/"), sub))
}
