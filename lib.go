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
	Data func(w http.ResponseWriter, r *http.Request) (any, error)
	// Post contains named POST actions. The action name is selected by form field "action".
	Post map[string]func(w http.ResponseWriter, r *http.Request) (any, error)
	// globals are global data providers executed before template rendering (merged with page data)
	globals []GlobalDataFunc
	// error500 is the template used for internal server errors
	error500 *template.Template
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
	tracker := &responseWriterTracker{ResponseWriter: w}
	var pageData any
	if p.Data != nil {
		var err error
		pageData, err = p.Data(tracker, r)
		if err != nil {
			log.Printf("GET data hook error for %s: %v", p.Route, err)
			if !tracker.wroteHeader {
				w.WriteHeader(http.StatusInternalServerError)
				_ = executePageTemplate(w, p.error500, nil)
			}
			return
		}
		if tracker.wroteHeader {
			return
		}
	}

	// collect global data
	merged, err := mergeWithGlobals(tracker, p.globals, pageData, r)
	if err != nil {
		log.Printf("global data hook error for %s: %v", p.Route, err)
		if !tracker.wroteHeader {
			w.WriteHeader(http.StatusInternalServerError)
			_ = executePageTemplate(w, p.error500, nil)
		}
		return
	}
	if tracker.wroteHeader {
		return
	}

	if err := executePageTemplate(w, p.Template, merged); err != nil {
		log.Printf("template execute error for %s: %v", p.Route, err)
		// Header might have been partially written if executePageTemplate failed mid-way,
		// but executePageTemplate currently doesn't write header before execution.
		// Actually, executePageTemplate calls t.Execute which writes to w.
		// If it fails mid-execution, we can't easily send a 500 template.
		// But let's try anyway if possible.
		if !tracker.wroteHeader {
			w.WriteHeader(http.StatusInternalServerError)
			_ = executePageTemplate(w, p.error500, nil)
		}
	}
}

func (p *Page) servePost(w http.ResponseWriter, r *http.Request) {
	tracker := &responseWriterTracker{ResponseWriter: w}
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
	data, err := handler(tracker, r)
	if err != nil {
		log.Printf("POST action '%s' error for %s: %v", action, p.Route, err)
		if !tracker.wroteHeader {
			w.WriteHeader(http.StatusInternalServerError)
			_ = executePageTemplate(w, p.error500, nil)
		}
		return
	}
	if tracker.wroteHeader {
		return
	}

	// If client explicitly wants JSON, serve JSON; otherwise re-render the page with returned data
	if wantsJSON(r) {
		writeJSON(w, data)
		return
	}

	// merge globals for HTML re-render
	merged, mErr := mergeWithGlobals(tracker, p.globals, data, r)
	if mErr != nil {
		log.Printf("global data hook error (post) for %s: %v", p.Route, mErr)
		if !tracker.wroteHeader {
			w.WriteHeader(http.StatusInternalServerError)
			_ = executePageTemplate(w, p.error500, nil)
		}
		return
	}
	if tracker.wroteHeader {
		return
	}

	if err := executePageTemplate(w, p.Template, merged); err != nil {
		log.Printf("template execute (post) error for %s: %v", p.Route, err)
		if !tracker.wroteHeader {
			w.WriteHeader(http.StatusInternalServerError)
			_ = executePageTemplate(w, p.error500, nil)
		}
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

type responseWriterTracker struct {
	http.ResponseWriter
	wroteHeader bool
}

func (rw *responseWriterTracker) WriteHeader(code int) {
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterTracker) Write(b []byte) (int, error) {
	rw.wroteHeader = true
	return rw.ResponseWriter.Write(b)
}

// GlobalDataFunc provides global data for all pages before rendering.
// It can return any struct or map[string]any; values will be merged.
// If it writes to http.ResponseWriter, further data providers and template rendering are skipped.
type GlobalDataFunc func(w http.ResponseWriter, r *http.Request) (any, error)

// PageHook allows per-route hooks to provide Data and Post actions.
// If Data or a Post action writes to http.ResponseWriter, template rendering is skipped.
// PageHook registers hooks for a route.
type PageHook struct {
	// Template is the relative path to the gohtml file from PagesDir.
	// If empty, it's assumed to be based on the route.
	Template string
	Data     func(w http.ResponseWriter, r *http.Request) (any, error)
	Post     map[string]func(w http.ResponseWriter, r *http.Request) (any, error)
}

// Router represents a group of routes.
type Router struct {
	routes  map[string]PageHook
	routers []routerMount
}

type routerMount struct {
	prefix string
	router *Router
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{
		routes: make(map[string]PageHook),
	}
}

// RegisterPage registers a route with the router.
func (r *Router) RegisterPage(route string, hook PageHook) {
	r.routes[route] = hook
}

// UseRouter mounts another router at a prefix relative to this router.
func (r *Router) UseRouter(prefix string, sub *Router) {
	r.routers = append(r.routers, routerMount{
		prefix: prefix,
		router: sub,
	})
}

// collectRoutes flattens the router hierarchy into a map of routes.
func (r *Router) collectRoutes(parentPrefix string) map[string]PageHook {
	allRoutes := make(map[string]PageHook)

	for route, hook := range r.routes {
		fullRoute := parentPrefix + route
		if !strings.HasPrefix(fullRoute, "/") {
			fullRoute = "/" + fullRoute
		}
		// Replace any double slashes
		fullRoute = strings.ReplaceAll(fullRoute, "//", "/")
		allRoutes[fullRoute] = hook
	}

	for _, mount := range r.routers {
		subRoutes := mount.router.collectRoutes(parentPrefix + mount.prefix)
		for route, hook := range subRoutes {
			allRoutes[route] = hook
		}
	}

	return allRoutes
}

// internal registry for page hooks registered by user packages at init().
var (
	pageHookRegistry = map[string]PageHook{}
)

// RegisterPage registers hooks for a route. Typically called from user code in an init() function.
// Deprecated: use Router instead to avoid blank imports and for better encapsulation.
// Example: govoort.RegisterPage("/account", govoort.PageHook{ Data: func(r *http.Request)(any,error){...} })
func RegisterPage(route string, hook PageHook) {
	// merge with any existing to allow multiple registrations to extend Post map
	if existing, ok := pageHookRegistry[route]; ok {
		if hook.Data != nil {
			existing.Data = hook.Data
		}
		if hook.Post != nil {
			if existing.Post == nil {
				existing.Post = map[string]func(w http.ResponseWriter, r *http.Request) (any, error){}
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
// If any global data provider writes to http.ResponseWriter, further processing is stopped.
func mergeWithGlobals(w http.ResponseWriter, globals []GlobalDataFunc, pageData any, r *http.Request) (any, error) {
	tracker, ok := w.(*responseWriterTracker)
	if !ok {
		tracker = &responseWriterTracker{ResponseWriter: w}
	}

	var out map[string]any
	// collect globals
	for _, gf := range globals {
		if gf == nil {
			continue
		}
		v, err := gf(tracker, r)
		if err != nil {
			return nil, err
		}
		if tracker.wroteHeader {
			return nil, nil
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
	// Router is the root router for the application. If set, routes registered on it will be added to the server.
	Router *Router
	// AutoRegisterPages controls whether the gohtml pages in the PagesDir should be automatically registered (default: true)
	AutoRegisterPages bool
	// GlobalData are executed for every request (GET and non-JSON POST responses) and merged into template data
	GlobalData []GlobalDataFunc
	// EnableVersionEndpoint enables the /__version endpoint for hot-reload support (default: true)
	EnableVersionEndpoint bool
	// CommonHeaders are headers added to all responses
	CommonHeaders map[string]string
	// EnableHTTPCallLogs controls whether each HTTP request handling is logged with its response time in ms (default: true)
	EnableHTTPCallLogs bool
	// NotFoundTemplate is the template name for 404 errors (default: "404")
	NotFoundTemplate string
	// InternalErrorTemplate is the template name for 500 errors (default: "500")
	InternalErrorTemplate string
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
		Router:                nil,
		AutoRegisterPages:     false,
		GlobalData:            nil,
		EnableVersionEndpoint: true,
		CommonHeaders:         map[string]string{},
		EnableHTTPCallLogs:    true,
		NotFoundTemplate:      "404",
		InternalErrorTemplate: "500",
	}
}

var appStart = time.Now().UnixMilli()

// Server represents a Govoort server instance.
type Server struct {
	config   *Config
	pages    map[string]*Page
	mux      *http.ServeMux
	error404 *template.Template
	error500 *template.Template
}

// New creates a new Govoort server with the given configuration.
func New(config *Config) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 1. Collect all routes from the router if provided
	routerRoutes := make(map[string]PageHook)
	if config.Router != nil {
		routerRoutes = config.Router.collectRoutes("")
	}

	// 2. Load pages (auto-discovery + merging router routes)
	pages, err := loadPagesWithConfig(config, routerRoutes)
	if err != nil {
		return nil, fmt.Errorf("failed to load pages: %w", err)
	}

	mux := http.NewServeMux()

	s := &Server{
		config: config,
		pages:  pages,
		mux:    mux,
	}

	// Setup error templates
	if p, ok := pages["/"+config.NotFoundTemplate]; ok {
		s.error404 = p.Template
		// remove from regular pages so it doesn't show up in listings or registered as normal route
		delete(pages, "/"+config.NotFoundTemplate)
	} else {
		s.error404 = template.Must(template.New("404").Parse(`<!DOCTYPE html><html><body><h1>404 Not Found</h1><p>The page you are looking for does not exist.</p></body></html>`))
	}

	if p, ok := pages["/"+config.InternalErrorTemplate]; ok {
		s.error500 = p.Template
		delete(pages, "/"+config.InternalErrorTemplate)
	} else {
		s.error500 = template.Must(template.New("500").Parse(`<!DOCTYPE html><html><body><h1>500 Internal Server Error</h1><p>Something went wrong on our end.</p></body></html>`))
	}

	for route, page := range pages {
		log.Printf("registering route %s", route)
		// attach global data providers
		page.globals = append(page.globals, config.GlobalData...)
		// assign error500 to page
		page.error500 = s.error500

		// Use exact match patterns (Go 1.22+) to allow the catch-all "/" for 404s
		if route == "/" {
			mux.Handle("GET /{$}", page)
			mux.Handle("POST /{$}", page)
		} else {
			mux.Handle("GET "+route, page)
			mux.Handle("POST "+route, page)
			// Also handle trailing-slash variant exactly to avoid accidental 404s
			if !strings.HasSuffix(route, "/") {
				mux.Handle("GET "+route+"/{$}", page)
				mux.Handle("POST "+route+"/{$}", page)
			}
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

	// Register 404 handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// http.ServeMux with "/" matches everything.
		// If we are here, it means no other route matched (including static files if they are under a prefix).
		w.WriteHeader(http.StatusNotFound)
		_ = executePageTemplate(w, s.error404, nil)
	})

	return s, nil
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	// Wrap with error recovery to use 500 template
	h = s.withErrorRecovery(h)
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

func (s *Server) withErrorRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic recovered: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				_ = executePageTemplate(w, s.error500, nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Template loading functions

func loadPagesWithConfig(config *Config, routerRoutes map[string]PageHook) (map[string]*Page, error) {
	pages := make(map[string]*Page)

	// 1. Auto-discover pages if enabled
	if config.AutoRegisterPages {
		pageFiles, err := discoverTemplateFiles(config.PagesDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}

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
				Post:     map[string]func(w http.ResponseWriter, r *http.Request) (any, error){},
			}
		}
	}

	// 2. Add routes from router (overwrites auto-discovered routes)
	for route, hook := range routerRoutes {
		pageFile := hook.Template
		if pageFile == "" {
			// Infer template path from route if not provided
			// /about -> about.gohtml
			// / -> index.gohtml
			// /blog/post -> blog/post.gohtml
			relPath := strings.TrimPrefix(route, "/")
			if relPath == "" {
				relPath = "index"
			}
			pageFile = filepath.Join(config.PagesDir, relPath+".gohtml")
		} else {
			// If provided, it's relative to PagesDir
			pageFile = filepath.Join(config.PagesDir, pageFile)
		}

		tmpl, err := buildTemplateForWithConfig(pageFile, config)
		if err != nil {
			return nil, fmt.Errorf("building template for router route %s (file: %s): %w", route, pageFile, err)
		}

		pages[route] = &Page{
			Route:    route,
			Template: tmpl,
			Data:     hook.Data,
			Post:     hook.Post,
		}
	}

	// 3. Add legacy registered hooks
	for route, hook := range pageHookRegistry {
		// This overwrites both auto-discovered and router routes if they collide
		if p, ok := pages[route]; ok {
			if hook.Data != nil {
				p.Data = hook.Data
			}
			if hook.Post != nil {
				if p.Post == nil {
					p.Post = map[string]func(w http.ResponseWriter, r *http.Request) (any, error){}
				}
				for k, v := range hook.Post {
					p.Post[k] = v
				}
			}
		} else {
			// If it doesn't exist, we need to load the template
			pageFile := hook.Template
			if pageFile == "" {
				relPath := strings.TrimPrefix(route, "/")
				if relPath == "" {
					relPath = "index"
				}
				pageFile = filepath.Join(config.PagesDir, relPath+".gohtml")
			} else {
				pageFile = filepath.Join(config.PagesDir, pageFile)
			}

			tmpl, err := buildTemplateForWithConfig(pageFile, config)
			if err != nil {
				// We log instead of returning error for legacy compatibility if template is missing
				log.Printf("warning: legacy RegisterPage route %s failed to load template %s: %v", route, pageFile, err)
				continue
			}

			pages[route] = &Page{
				Route:    route,
				Template: tmpl,
				Data:     hook.Data,
				Post:     hook.Post,
			}
		}
	}

	// 4. Allow legacy RegisterHooks (overwrites again)
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
