package ui

import (
	"io"
	"net/http"
)

// Write renders a Node and streams it to the response.
func Write(w http.ResponseWriter, n Node) error {
	html, err := n.Render()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = io.WriteString(w, string(html))
	return err
}
