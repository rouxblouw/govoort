package about

import (
	"devapp/components/card"

	"github.com/rouxblouw/govoort/ui"
)

var Route = ui.Page(func(ctx *ui.Context) (ui.Node, error) {
	return card.New(card.Props{
		Title: "About",
		Body:  ui.Text("Every route builds a Node tree; one Write() call renders it."),
	}), nil
})
