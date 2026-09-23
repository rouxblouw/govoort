package ui

// CSP sets a Content-Security-Policy header on every response. With all
// component CSS/JS served from same-origin static URLs.
// A policy this strict needs no 'unsafe-inline' anywhere.
func CSP(policy string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Context) error {
			ctx.W.Header().Set("Content-Security-Policy", policy)
			return next(ctx)
		}
	}
}
