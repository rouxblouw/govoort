package ui

import (
	"encoding/json"
	"errors"
	"net/http"
)

// HTTPError lets a route's error carry its own status code.
type HTTPError interface {
	error
	StatusCode() int
}

type NotFoundError struct{ Resource string }

func (e NotFoundError) Error() string   { return e.Resource + " not found" }
func (e NotFoundError) StatusCode() int { return http.StatusNotFound }

// ErrorMiddleware is a global catch-all: put it on your root router so any
// error bubbling up from any route (page or JSON) gets a consistent
// response shaped correctly for that route's Kind.
func ErrorMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) error {
		err := next(ctx)
		if err == nil {
			return nil
		}
		code := http.StatusInternalServerError
		var he HTTPError
		if errors.As(err, &he) {
			code = he.StatusCode()
		}
		ctx.W.WriteHeader(code)
		if ctx.Kind == KindJson {
			_ = json.NewEncoder(ctx.W).Encode(map[string]string{"error": err.Error()})
		} else {
			_, _ = ctx.W.Write([]byte("<h1>" + http.StatusText(code) + "</h1><p>" + err.Error() + "</p>"))
		}
		return nil // handled — stop propagating
	}
}

// Recover converts a panic into an ordinary error so it flows through
// ErrorMiddleware instead of crashing the request. Register this FIRST
// (outermost), before ErrorMiddleware and everything else.
func Recover(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.New("internal error")
			}
		}()
		return next(ctx)
	}
}
