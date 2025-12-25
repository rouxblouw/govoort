package govoort

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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
	// globals are global data providers executed before template rendering (merged with page data)
	globals []GlobalDataFunc
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
	var pageData any
	if p.Data != nil {
		var err error
		pageData, err = p.Data(r)
		if err != nil {
			log.Printf("GET data hook error for %s: %v", p.Route, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	// collect global data
	merged, err := mergeWithGlobals(p.globals, pageData, r)
	if err != nil {
		log.Printf("global data hook error for %s: %v", p.Route, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := executePageTemplate(w, p.Template, merged); err != nil {
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

	// merge globals for HTML re-render
	merged, mErr := mergeWithGlobals(p.globals, data, r)
	if mErr != nil {
		log.Printf("global data hook error (post) for %s: %v", p.Route, mErr)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := executePageTemplate(w, p.Template, merged); err != nil {
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

// GlobalDataFunc provides global data for all pages before rendering.
// It can return any struct or map[string]any; values will be merged.
type GlobalDataFunc func(r *http.Request) (any, error)

// PageHook allows per-route hooks to provide Data and Post actions.
type PageHook struct {
	Data func(r *http.Request) (any, error)
	Post map[string]func(r *http.Request) (any, error)
}

// internal registry for page hooks registered by user packages at init().
var (
	pageHookRegistry = map[string]PageHook{}
)

// RegisterPage registers hooks for a route. Typically called from user code in an init() function.
// Example: govoort.RegisterPage("/account", govoort.PageHook{ Data: func(r *http.Request)(any,error){...} })
func RegisterPage(route string, hook PageHook) {
	// merge with any existing to allow multiple registrations to extend Post map
	if existing, ok := pageHookRegistry[route]; ok {
		if hook.Data != nil {
			existing.Data = hook.Data
		}
		if hook.Post != nil {
			if existing.Post == nil {
				existing.Post = map[string]func(r *http.Request) (any, error){}
			}
			for k, v := range hook.Post {
				existing.Post[k] = v
			}
		}
		pageHookRegistry[route] = existing
		return
	}
	pageHookRegistry[route] = hook
}

func getRegisteredPageHook(route string) (PageHook, bool) {
	h, ok := pageHookRegistry[route]
	return h, ok
}

// mergeWithGlobals merges data from global providers with page-specific data.
// Merge order: globals (in provided order) then pageData; later keys overwrite earlier ones.
// Both structs and map[string]any are supported; result is map[string]any unless all are nil.
func mergeWithGlobals(globals []GlobalDataFunc, pageData any, r *http.Request) (any, error) {
	var out map[string]any
	// collect globals
	for _, gf := range globals {
		if gf == nil {
			continue
		}
		v, err := gf(r)
		if err != nil {
			return nil, err
		}
		if v == nil {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		mergeInto(out, v)
	}
	if pageData != nil {
		if out == nil {
			out = map[string]any{}
		}
		mergeInto(out, pageData)
	}
	if out == nil {
		return nil, nil
	}
	return out, nil
}

// mergeInto merges fields/keys from src into dst (map). Struct fields must be exported.
func mergeInto(dst map[string]any, src any) {
	if src == nil {
		return
	}
	// if already a map[string]any
	if m, ok := src.(map[string]any); ok {
		for k, v := range m {
			dst[k] = v
		}
		return
	}
	val := reflect.ValueOf(src)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		t := val.Type()
		for i := 0; i < val.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" { // unexported
				continue
			}
			name := f.Name
			dst[name] = val.Field(i).Interface()
		}
		return
	}
	// fallback: store under generic key
	dst["Value"] = src
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
	// GlobalData are executed for every request (GET and non-JSON POST responses) and merged into template data
	GlobalData []GlobalDataFunc
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
		GlobalData:            nil,
		EnableVersionEndpoint: true,
		CommonHeaders:         map[string]string{},
		EnableHTTPCallLogs:    true,
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
		// attach global data providers
		page.globals = append(page.globals, config.GlobalData...)
		// attach any pre-registered page hooks from code (by route)
		if hook, ok := getRegisteredPageHook(route); ok {
			if hook.Data != nil {
				page.Data = hook.Data
			}
			if hook.Post != nil {
				if page.Post == nil {
					page.Post = map[string]func(r *http.Request) (any, error){}
				}
				for k, v := range hook.Post {
					page.Post[k] = v
				}
			}
		}

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
