package demo

import (
	likeablecard "devapp/components/likeable-card"
	"devapp/layout"

	"github.com/rouxblouw/govoort/ui"
)

var Route = ui.Page(func(ctx *ui.Context) (ui.Node, error) {
	return layout.New(layout.Props{
		Title:  "Home",
		Header: ui.Text("© 2026"),
		Content: likeablecard.New(likeablecard.Props{
			Title:     "About",
			Content:   ui.Text("Every route builds a Node tree; one Write() call renders it."),
			LikeCount: 1,
			Threshold: 100,
		}),
		Footer: ui.Text("© 2026"),
	}), nil
})
