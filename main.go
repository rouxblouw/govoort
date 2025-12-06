package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Page represents a routed page with optional GET data hook and POST action handlers.
type Page struct {
	Route    string
	Template *template.Template
	// Data is called on GET to provide data for the template. Return nil if not needed.
	Data func(r *http.Request) (any, error)
	// Post contains named POST actions. The action name is selected by form field "action".
	Post map[string]func(r *http.Request) (any, error)
}

func (p *Page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		p.serveGet(w, r)
	case http.MethodPost:
		p.servePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Page) serveGet(w http.ResponseWriter, r *http.Request) {
	var data any
	if p.Data != nil {
		var err error
		data, err = p.Data(r)
		if err != nil {
			log.Printf("GET data hook error for %s: %v", p.Route, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
	if err := executePageTemplate(w, p.Template, data); err != nil {
		log.Printf("template execute error for %s: %v", p.Route, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (p *Page) servePost(w http.ResponseWriter, r *http.Request) {
	// Parse form if not already parsed
	_ = r.ParseForm()
	action := r.FormValue("action")
	if action == "" && len(p.Post) == 1 {
		// If only one action is registered, allow empty to pick the sole action
		for k := range p.Post {
			action = k
			break
		}
	}
	handler, ok := p.Post[action]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("unknown action"))
		return
	}
	data, err := handler(r)
	if err != nil {
		log.Printf("POST action '%s' error for %s: %v", action, p.Route, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// If client explicitly wants JSON, serve JSON; otherwise re-render the page with returned data
	if wantsJSON(r) {
		writeJSON(w, data)
		return
	}
	if err := executePageTemplate(w, p.Template, data); err != nil {
		log.Printf("template execute (post) error for %s: %v", p.Route, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return true
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

// Template building and routing

type loadedPage struct {
	route string
	tmpl  *template.Template
	file  string
}

func loadPages() (map[string]*Page, error) {
	pageFiles, err := discoverTemplateFiles("web/pages")
	if err != nil {
		return nil, err
	}
	if len(pageFiles) == 0 {
		return nil, errors.New("no page templates found under web/pages")
	}

	// Build templates per page
	pages := make(map[string]*Page)
	for _, file := range pageFiles {
		route := routeFromPagePath("web/pages", file)
		tmpl, err := buildTemplateFor(file)
		if err != nil {
			return nil, fmt.Errorf("building template for %s: %w", route, err)
		}
		pages[route] = &Page{
			Route:    route,
			Template: tmpl,
			Data:     nil,
			Post:     map[string]func(r *http.Request) (any, error){},
		}
	}

	// Allow user to attach hooks for Data/POST actions
	registerPageHooks(pages)

	return pages, nil
}

func discoverTemplateFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".gohtml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func routeFromPagePath(root, full string) string {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		// On error, fall back to just using filename
		rel = filepath.Base(full)
	}
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	if rel == "index" {
		return "/"
	}
	if strings.HasSuffix(rel, "/index") {
		return "/" + strings.TrimSuffix(rel, "/index")
	}
	return "/" + rel
}

func buildTemplateFor(pageFile string) (*template.Template, error) {
	funcs := template.FuncMap{}

	// Start with a fresh template per page to avoid cross-page name clashes
	t := template.New("base").Funcs(funcs)

	// 1) Parse layouts first
	layoutFiles, _ := discoverTemplateFiles("web/layouts")
	if len(layoutFiles) > 0 {
		var err error
		t, err = t.ParseFiles(layoutFiles...)
		if err != nil {
			return nil, fmt.Errorf("parse layouts: %w", err)
		}
	}

	// 2) Parse shared templates next (partials, components)
	sharedFiles, _ := discoverTemplateFiles("web/templates")
	if len(sharedFiles) > 0 {
		var err error
		t, err = t.ParseFiles(sharedFiles...)
		if err != nil {
			return nil, fmt.Errorf("parse shared templates: %w", err)
		}
	}

	// 3) Parse the page body last
	var err error
	t, err = t.ParseFiles(pageFile)
	if err != nil {
		return nil, fmt.Errorf("parse page file: %w", err)
	}
	return t, nil
}

func executePageTemplate(w http.ResponseWriter, t *template.Template, data any) error {
	// Prefer executing the "base" template if present; otherwise execute the root
	if t.Lookup("base") != nil {
		return t.ExecuteTemplate(w, "base", data)
	}
	return t.Execute(w, data)
}

var appStart = time.Now().UnixMilli()

func main() {
	var (
		addr = flag.String("addr", getEnvDefault("ADDR", ":8080"), "server listen address")
	)
	flag.Parse()

	pages, err := loadPages()
	if err != nil {
		log.Fatalf("failed to load pages: %v", err)
	}

	mux := http.NewServeMux()
	for route, page := range pages {
		log.Printf("registering route %s for page", route)
		mux.Handle(route, page)
		// Also handle trailing-slash variant to avoid accidental 404s
		if route != "/" && !strings.HasSuffix(route, "/") {
			mux.Handle(route+"/", page)
		}
	}

	// Lightweight version endpoint for browser auto-reload during Air restarts
	mux.HandleFunc("/__version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		_, _ = fmt.Fprintf(w, `{"start":%d}`, appStart)
	})

	// Optionally serve static files if ./web/static exists (best-effort)
	if dirExists("web/static") {
		fs := http.FileServer(http.Dir("web/static"))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
	}

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, withCommonHeaders(mux)); err != nil {
		log.Fatal(err)
	}
}

func withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer-when-downgrade")
		next.ServeHTTP(w, r)
	})
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func getEnvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// registerPageHooks allows you to attach GET data hooks and POST action handlers per route.
// Customize this function to add your own logic.
func registerPageHooks(pages map[string]*Page) {
	// Example data hooks
	if p, ok := pages["/"]; ok {
		p.Data = func(r *http.Request) (any, error) {
			return map[string]any{
				"Title":   "Home",
				"Message": "Welcome to the home page",
			}, nil
		}
	}
	if p, ok := pages["/about"]; ok {
		p.Data = func(r *http.Request) (any, error) {
			return map[string]any{
				"Title": "About",
			}, nil
		}
	}
	if p, ok := pages["/blog/hello"]; ok {
		p.Data = func(r *http.Request) (any, error) {
			return map[string]any{
				"Title":   "Blog Hello",
				"Message": "Submit the form to be greeted",
			}, nil
		}
		p.Post["greet"] = func(r *http.Request) (any, error) {
			name := strings.TrimSpace(r.FormValue("name"))
			if name == "" {
				name = "friend"
			}
			return map[string]any{
				"Title":   "Blog Hello",
				"Message": fmt.Sprintf("Hello, %s!", name),
			}, nil
		}
	}
}
