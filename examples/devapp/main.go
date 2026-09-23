package main

import (
	"devapp/pages/about"
	"devapp/pages/demo"
	"devapp/pages/home"
	"log"
	"net/http"

	"github.com/rouxblouw/govoort/ui"
)

func main() {
	// api := ui.NewRouter()

	app := ui.NewRouter()
	app.Use(ui.RequestLogging)
	app.Use(ui.Recover)
	app.Use(ui.ErrorMiddleware)
	app.RawGet("/assets/{file...}", ui.AssetsHandler)
	app.Get("/", home.Route)
	app.Get("/about", about.Route)
	app.Get("/demo", demo.Route)

	// app.Mount("/api", api)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", app))
}
