package likeablecard

import (
	"bytes"
	_ "embed"
	"html/template"

	"github.com/rouxblouw/govoort/ui"
)

//go:embed likeable-card.gohtml
var src string

//go:embed likeable-card.css
var css []byte

//go:embed likeable-card.js
var js []byte

var tmpl = template.Must(template.New("likealbe-card").Parse(src))

func init() {
	ui.RegisterAsset("card", ui.AssetCSS, css)
	ui.RegisterAsset("card", ui.AssetJS, js)
}

type Props struct {
	Title     string
	Content   ui.Node
	LikeCount int
	Threshold int
}

type Component struct{ Props Props }

func New(p Props) *Component { return &Component{Props: p} }

func (c *Component) Render() (template.HTML, error) {
	body, err := c.Props.Content.Render()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Title                string
		Body                 template.HTML
		LikeCount, Threshold int
	}{c.Props.Title, body, c.Props.LikeCount, c.Props.Threshold})
	return template.HTML(buf.String()), err
}

// Assets reports what THIS component needs, plus whatever its child needs —
// same recursive shape as Render() resolving child Nodes.
func (c *Component) Assets() []ui.AssetRef {
	mine := []ui.AssetRef{
		{Name: "card", Kind: ui.AssetCSS},
		{Name: "card", Kind: ui.AssetJS},
	}
	return append(mine, ui.CollectAssets(c.Props.Content)...)
}
