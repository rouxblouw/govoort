package ui

import (
	"log"
	"time"
)

func RequestLogging(next HandlerFunc) HandlerFunc {
	return func(ctx *Context) error {
		start := time.Now()
		err := next(ctx)

		elapsedNs := time.Since(start).Nanoseconds() // raw int64 count of ns
		elapsedMs := float64(elapsedNs) / 1e6        // convert to ms as a float

		log.Printf("served %s in %.2fms", ctx.R.URL.Path, elapsedMs)
		return err
	}
}
