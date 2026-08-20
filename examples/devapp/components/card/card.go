package card

import (
	"bytes"
	_ "embed"
	"html/template"

	"github.com/rouxblouw/govoort/ui"
)

//go:embed card.gohtml
var src string

var tmpl = template.Must(template.New("card").Parse(src))

// Props is the component's public "input" — its API, in Angular terms.
type Props struct {
	Title string
	Body  ui.Node // kept UNRENDERED — resolved on demand in Render()
}

type Component struct{ Props Props }

func New(p Props) *Component { return &Component{Props: p} }

func (c *Component) Render() (template.HTML, error) {
	body, err := c.Props.Body.Render()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Title string
		Body  template.HTML
	}{c.Props.Title, body})
	return template.HTML(buf.String()), err
}
