package govoort

import (
	"encoding/json"
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

// Config holds configuration for the Govoort framework.
type Config struct {
	// PagesDir is the directory where page templates are stored (default: "web/pages")
	PagesDir string
	// LayoutsDir is the directory where layout templates are stored (default: "web/layouts")
	LayoutsDir string
	// TemplatesDir is the directory where shared templates/partials are stored (default: "web/templates")
	TemplatesDir string
	// StaticDir is the directory where static files are served from (default: "web/static")
	StaticDir string
	// StaticRoute is the URL prefix for static files (default: "/static/")
	StaticRoute string
	// TemplateFuncs allows you to add custom template functions
	TemplateFuncs template.FuncMap
	// RegisterHooks is called after pages are loaded, allowing you to attach Data and Post handlers
	RegisterHooks func(pages map[string]*Page)
	// EnableVersionEndpoint enables the /__version endpoint for hot-reload support (default: true)
	EnableVersionEndpoint bool
	// CommonHeaders are headers added to all responses
	CommonHeaders map[string]string
	// EnableHTTPCallLogs controls whether each HTTP request handling is logged with its response time in ms (default: true)
	EnableHTTPCallLogs bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		PagesDir:              "web/pages",
		LayoutsDir:            "web/layouts",
		TemplatesDir:          "web/templates",
		StaticDir:             "web/static",
		StaticRoute:           "/static/",
		TemplateFuncs:         template.FuncMap{},
		RegisterHooks:         nil,
		EnableVersionEndpoint: true,
		CommonHeaders: map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer-when-downgrade",
		},
		EnableHTTPCallLogs: true,
	}
}

var appStart = time.Now().UnixMilli()

// Server represents a Govoort server instance.
type Server struct {
	config *Config
	pages  map[string]*Page
	mux    *http.ServeMux
}

// New creates a new Govoort server with the given configuration.
func New(config *Config) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}

	pages, err := loadPagesWithConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load pages: %w", err)
	}

	mux := http.NewServeMux()
	for route, page := range pages {
		log.Printf("registering route %s", route)
		mux.Handle(route, page)
		// Also handle trailing-slash variant to avoid accidental 404s
		if route != "/" && !strings.HasSuffix(route, "/") {
			mux.Handle(route+"/", page)
		}
	}

	// Version endpoint for browser auto-reload during development
	if config.EnableVersionEndpoint {
		mux.HandleFunc("/__version", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			_, _ = fmt.Fprintf(w, `{"start":%d}`, appStart)
		})
	}

	// Serve static files if directory exists
	if dirExists(config.StaticDir) {
		fs := http.FileServer(http.Dir(config.StaticDir))
		mux.Handle(config.StaticRoute, http.StripPrefix(config.StaticRoute, fs))
	}

	return &Server{
		config: config,
		pages:  pages,
		mux:    mux,
	}, nil
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	if s.config.EnableHTTPCallLogs {
		h = s.withRequestLogging(h)
	}
	return s.withCommonHeaders(h)
}

// ListenAndServe starts the server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	log.Printf("listening on %s", addr)
	return http.ListenAndServe(addr, s.Handler())
}

// Pages returns the loaded pages (useful for testing or inspection).
func (s *Server) Pages() map[string]*Page {
	return s.pages
}

func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range s.config.CommonHeaders {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

// withRequestLogging logs each HTTP request and its response time.
// For durations under 1ms, it logs fractional milliseconds with microsecond precision (e.g., 0.12 ms).
func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		dur := time.Since(start)
		if dur < time.Millisecond {
			// Show sub-millisecond durations as fractional milliseconds (based on microseconds)
			ms := float64(dur) / float64(time.Millisecond)
			log.Printf("%s %s in %.2f ms", r.Method, r.URL.Path, ms)
			return
		}
		log.Printf("%s %s in %d ms", r.Method, r.URL.Path, dur.Milliseconds())
	})
}

// Template loading functions

func loadPagesWithConfig(config *Config) (map[string]*Page, error) {
	pageFiles, err := discoverTemplateFiles(config.PagesDir)
	if err != nil {
		return nil, err
	}
	if len(pageFiles) == 0 {
		return nil, fmt.Errorf("no page templates found under %s", config.PagesDir)
	}

	// Build templates per page
	pages := make(map[string]*Page)
	for _, file := range pageFiles {
		route := routeFromPagePath(config.PagesDir, file)
		tmpl, err := buildTemplateForWithConfig(file, config)
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
	if config.RegisterHooks != nil {
		config.RegisterHooks(pages)
	}

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

func buildTemplateForWithConfig(pageFile string, config *Config) (*template.Template, error) {
	funcs := config.TemplateFuncs
	if funcs == nil {
		funcs = template.FuncMap{}
	}

	// Start with a fresh template per page to avoid cross-page name clashes
	t := template.New("base").Funcs(funcs)

	// 1) Parse layouts first
	layoutFiles, _ := discoverTemplateFiles(config.LayoutsDir)
	if len(layoutFiles) > 0 {
		var err error
		t, err = t.ParseFiles(layoutFiles...)
		if err != nil {
			return nil, fmt.Errorf("parse layouts: %w", err)
		}
	}

	// 2) Parse shared templates next (partials, components)
	sharedFiles, _ := discoverTemplateFiles(config.TemplatesDir)
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

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
