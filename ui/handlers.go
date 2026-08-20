package ui

import "encoding/json"

// Page is the only legal way to answer with HTML — the inner function
// must return a Node, so it's structurally impossible to hand back
// arbitrary bytes or skip templating.
func Page(fn func(ctx *Context) (Node, error)) HandlerFunc {
	return func(ctx *Context) error {
		ctx.Kind = KindPage
		node, err := fn(ctx)
		if err != nil {
			return err
		}
		return Write(ctx.W, node)
	}
}

// Json is the only legal way to answer with data.
func Json(fn func(ctx *Context) (any, error)) HandlerFunc {
	return func(ctx *Context) error {
		ctx.Kind = KindJson
		data, err := fn(ctx)
		if err != nil {
			return err
		}
		ctx.W.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(ctx.W).Encode(data)
	}
}
