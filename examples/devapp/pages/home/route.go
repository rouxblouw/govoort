package home

import (
	"devapp/components/card"
	"devapp/components/nav"
	"devapp/layout"

	"github.com/rouxblouw/govoort/ui"
)

var siteNav = nav.New(nav.Props{
	Links: []nav.Link{
		{Label: "Home", Href: "/"},
		{Label: "About", Href: "/about"},
	},
})

var Route = ui.Page(func(ctx *ui.Context) (ui.Node, error) {
	return layout.New(layout.Props{
		Title:  "Home",
		Header: siteNav,
		Content: ui.Nodes{
			card.New(card.Props{Title: "Fast", Body: ui.Text("No build step.")}),
			card.New(card.Props{Title: "Simple", Body: ui.Text("Just Go.")}),
		},
		Footer: ui.Text("© 2026"),
	}), nil
})
