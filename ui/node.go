package ui

import (
	"bytes"
	"html/template"
)

// An item that can render itself as HTML
type Node interface {
	Render() (template.HTML, error)
}

// String is the simplest node for silly responses
type Text string

func (t Text) Render() (template.HTML, error) {
	return template.HTML(template.HTMLEscapeString(string(t))), nil
}

// Nodes lets a slice of children act as a single Node
type Nodes []Node

func (ns Nodes) Render() (template.HTML, error) {
	var buf bytes.Buffer
	for _, n := range ns {
		html, err := n.Render()
		if err != nil {
			return "", err
		}
		buf.WriteString(string(html))
	}
	return template.HTML(buf.String()), nil
}
