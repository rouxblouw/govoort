package nav

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed nav.gohtml
var src string

var tmpl = template.Must(template.New("nav").Parse(src))

type Link struct {
	Label string
	Href  string
}

type Props struct{ Links []Link }

type Component struct{ Props Props }

func New(p Props) *Component { return &Component{Props: p} }

func (c *Component) Render() (template.HTML, error) {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, c.Props)
	return template.HTML(buf.String()), err
}
