package layout

import (
	"bytes"
	_ "embed"
	"html/template"

	"github.com/rouxblouw/govoort/ui"
)

//go:embed page.gohtml
var src string

var tmpl = template.Must(template.New("page").Parse(src))

type Props struct {
	Title   string
	Header  ui.Node
	Content ui.Node
	Footer  ui.Node
}

type Component struct{ Props Props }

func New(p Props) *Component { return &Component{Props: p} }

func (c *Component) Render() (template.HTML, error) {
	header, err := c.Props.Header.Render()
	if err != nil {
		return "", err
	}
	content, err := c.Props.Content.Render()
	if err != nil {
		return "", err
	}
	footer, err := c.Props.Footer.Render()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Title                   string
		Header, Content, Footer template.HTML
	}{c.Props.Title, header, content, footer})
	return template.HTML(buf.String()), err
}
