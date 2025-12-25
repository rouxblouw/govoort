# Govoort

A lightweight Go web framework for building server-rendered web applications with Go templates. Govoort automatically discovers and routes your `.gohtml` templates, allowing you to focus on building features rather than configuring routes.

## Features

- **Convention-based routing**: Page templates in `web/pages/` are automatically mapped to routes
- **Layout system**: Reusable layouts in `web/layouts/` wrap your pages
- **Shared templates**: Common partials and components in `web/templates/`
- **Static file serving**: Automatic serving of files from `web/static/`
- **GET/POST handlers**: Easy-to-define data hooks and form actions per page
- **Per-route code hooks**: Put Go code next to your page to prepare data before rendering
- **Global middleware-like data**: Register global data providers (e.g., session, current user) merged into every page render
- **JSON support**: Automatic JSON responses when requested
- **Hot reload support**: Built-in `/__version` endpoint for development workflows
- **Custom template functions**: Add your own template helpers
- **Security headers**: Configurable common security headers on all responses

## Installation

```bash
go get github.com/yourusername/govoort
```

## Quick Start

### 1. Create Your Project Structure

```
myapp/
├── main.go
└── web/
    ├── layouts/
    │   └── base.gohtml
    ├── pages/
    │   ├── index.gohtml
    │   └── about.gohtml
    ├── templates/
    │   └── header.gohtml
    └── static/
        └── style.css
```

### 2. Create a Base Layout

**web/layouts/base.gohtml**:
```html
{{define "base"}}
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    {{template "content" .}}
</body>
</html>
{{end}}
```

### 3. Create Page Templates

**web/pages/index.gohtml**:
```html
{{define "content"}}
<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
{{end}}
```

**web/pages/about.gohtml**:
```html
{{define "content"}}
<h1>About Us</h1>
<p>This is the about page.</p>
{{end}}
```

### 4. Create Your Main Application

**main.go**:
```go
package main

import (
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"

    "github.com/yourusername/govoort"
)

func main() {
    addr := flag.String("addr", ":8080", "server listen address")
    flag.Parse()

    // Create configuration
    config := govoort.DefaultConfig()
    // Option A: Register hooks centrally
    config.RegisterHooks = registerPageHooks

    // Option B: Register global data providers (middleware-like)
    config.GlobalData = []govoort.GlobalDataFunc{
        func(r *http.Request) (any, error) {
            // example: fetch session & user
            user := map[string]any{"ID": 123, "Name": "Ada"}
            return map[string]any{"CurrentUser": user}, nil
        },
    }

    // Create and start server
    server, err := govoort.New(config)
    if err != nil {
        log.Fatalf("failed to create server: %v", err)
    }

    if err := server.ListenAndServe(*addr); err != nil {
        log.Fatal(err)
    }
}

// Option A: central registration of per-route page hooks
func registerPageHooks(pages map[string]*govoort.Page) {
    // Attach a GET data hook for "/"
    if p, ok := pages["/"]; ok {
        p.Data = func(r *http.Request) (any, error) {
            return struct{
                Title   string
                Message string
            }{
                Title:   "Welcome",
                Message: "Hello from Govoort",
            }, nil
        }
        // Optional POST actions
        p.Post["save"] = func(r *http.Request) (any, error) {
            // do something, then return data for re-render
            return map[string]any{"Saved": true}, nil
        }
    }
}

// Register data hooks and POST handlers for your pages
func registerPageHooks(pages map[string]*govoort.Page) {
    // Home page data
    if p, ok := pages["/"]; ok {
        p.Data = func(r *http.Request) (any, error) {
            return map[string]any{
                "Title":   "Home",
                "Message": "Welcome to Govoort!",
            }, nil
        }
    }

    // About page data
    if p, ok := pages["/about"]; ok {
        p.Data = func(r *http.Request) (any, error) {
            return map[string]any{
                "Title": "About",
            }, nil
        }
    }
}
```

## Per‑Route Code Files (init‑based) — Option B

You can colocate Go code with your pages by creating small `.go` files in your app that register hooks for the matching route during `init()`. Use `govoort.RegisterPage` to attach a data provider and/or POST actions. This is especially useful for pages that need per-request logic (load session, find logged-in user, etc.).

Example for a page at `web/pages/account/profile.gohtml` whose route is `/account/profile`:

```go
// file: internal/pages/account/profile.go (your app)
package account

import (
    "net/http"
    "github.com/yourusername/govoort"
)

func init() {
    govoort.RegisterPage("/account/profile", govoort.PageHook{
        Data: func(r *http.Request) (any, error) {
            // load user/session and return any shape (struct or map)
            return map[string]any{
                "Title":       "Your profile",
                "CurrentUser": map[string]any{"Name": "Ada"},
            }, nil
        },
        Post: map[string]func(*http.Request) (any, error){
            "update": func(r *http.Request) (any, error) {
                // update profile; on success, return data for re-render
                return map[string]any{"Updated": true}, nil
            },
        },
    })
}
```

Notes:
- `RegisterPage(route, hook)` can be called from any package. It’s common to mirror your `web/pages/...` path in your Go package layout to keep things organized.
- The returned values from `Data`/`Post` can be any struct or `map[string]any`. They are merged with any global data (see below) before template rendering. Field/key name collisions are resolved by last-writer-wins: later providers override earlier ones (globals first, then page).

## Global Data Providers (Middleware‑like)

Some values should be available to all pages (e.g., security headers are already handled; you might want to expose `CurrentUser`, CSRF token, feature flags). Use `config.GlobalData` to register one or more `GlobalDataFunc` providers. These run on every request and their results are shallow‑merged into the page data before rendering.

```go
config.GlobalData = []govoort.GlobalDataFunc{
    func(r *http.Request) (any, error) {
        // e.g., read session/cookie and fetch the user
        user := map[string]any{"ID": 123, "Name": "Ada"}
        return map[string]any{"CurrentUser": user}, nil
    },
    func(r *http.Request) (any, error) {
        // feature flags
        return struct{ Beta bool }{Beta: true}, nil
    },
}
```

Merging rules:
- Providers run in the order supplied; later keys override earlier ones.
- The page’s own data overrides global data on key/field conflicts.
- Struct fields are exported by name; `map[string]any` keys are copied as-is.

## JSON responses for POST

When a POST action returns data and the client sets `Accept: application/json` (or posts JSON), Govoort returns JSON instead of re-rendering the HTML page. For normal form submissions (no JSON Accept), the page is re-rendered and its data is merged with global data.

### 5. Run Your Application

```bash
go run main.go
```

Visit `http://localhost:8080` in your browser!

## Routing Conventions

Govoort automatically maps template files to routes:

| File Path | Route |
|-----------|-------|
| `web/pages/index.gohtml` | `/` |
| `web/pages/about.gohtml` | `/about` |
| `web/pages/blog/index.gohtml` | `/blog` |
| `web/pages/blog/post.gohtml` | `/blog/post` |
| `web/pages/user/profile.gohtml` | `/user/profile` |

Both `/route` and `/route/` are handled automatically.

## Configuration

### Default Configuration

```go
config := govoort.DefaultConfig()
```

### Custom Configuration

```go
config := &govoort.Config{
    PagesDir:              "web/pages",
    LayoutsDir:            "web/layouts",
    TemplatesDir:          "web/templates",
    StaticDir:             "web/static",
    StaticRoute:           "/static/",
    EnableVersionEndpoint: true,
    TemplateFuncs:         template.FuncMap{
        "upper": strings.ToUpper,
    },
    CommonHeaders: map[string]string{
        "X-Content-Type-Options": "nosniff",
        "X-Frame-Options":        "DENY",
    },
    RegisterHooks: registerPageHooks,
}

server, err := govoort.New(config)
```

## Data Hooks and POST Handlers

### GET Data Hooks

Provide data to your templates on GET requests:

```go
func registerPageHooks(pages map[string]*govoort.Page) {
    if p, ok := pages["/products"]; ok {
        p.Data = func(r *http.Request) (any, error) {
            products, err := db.GetProducts()
            if err != nil {
                return nil, err
            }
            return map[string]any{
                "Title":    "Products",
                "Products": products,
            }, nil
        }
    }
}
```

### POST Handlers

Handle form submissions:

**web/pages/contact.gohtml**:
```html
{{define "content"}}
<h1>Contact Us</h1>
{{if .Success}}
    <p>Thanks for contacting us!</p>
{{else}}
    <form method="POST">
        <input type="hidden" name="action" value="submit">
        <input type="email" name="email" required>
        <textarea name="message" required></textarea>
        <button type="submit">Send</button>
    </form>
{{end}}
{{end}}
```

**main.go**:
```go
if p, ok := pages["/contact"]; ok {
    p.Post["submit"] = func(r *http.Request) (any, error) {
        email := r.FormValue("email")
        message := r.FormValue("message")

        // Process the form...
        err := sendEmail(email, message)
        if err != nil {
            return nil, err
        }

        return map[string]any{
            "Success": true,
        }, nil
    }
}
```

## JSON API Support

Govoort automatically returns JSON when the client requests it:

```bash
curl -H "Accept: application/json" http://localhost:8080/api/data
```

Your POST handlers can return any data structure, and it will be JSON-encoded if requested.

## Custom Template Functions

Add your own template functions:

```go
config := govoort.DefaultConfig()
config.TemplateFuncs = template.FuncMap{
    "formatDate": func(t time.Time) string {
        return t.Format("Jan 2, 2006")
    },
    "upper": strings.ToUpper,
}
```

Use in templates:
```html
<p>{{.Date | formatDate}}</p>
<h1>{{.Title | upper}}</h1>
```

## Development with Hot Reload

Govoort includes a `/__version` endpoint that returns the server start time. Combine this with [Air](https://github.com/cosmtrek/air) for automatic reloading:

**.air.toml**:
```toml
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "node_modules"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "gohtml"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
```

Run with Air:
```bash
air
```

## Using Govoort in Another Project

### Option 1: Local Development (Replace Import Path)

If you're developing this library locally and want to use it in another project:

1. **In the Govoort library**, update the import path reference in main.go from:
   ```go
   "github.com/yourusername/govoort"
   ```
   to your actual GitHub username and repository name.

2. **In your new project**, initialize a Go module and use a replace directive:
   ```bash
   mkdir myapp
   cd myapp
   go mod init myapp
   ```

3. **Create your go.mod with a replace directive**:
   ```go
   module myapp

   go 1.21

   require github.com/yourusername/govoort v0.0.0

   replace github.com/yourusername/govoort => /path/to/local/govoort
   ```

4. **Create your application** following the Quick Start guide above.

5. **Run**:
   ```bash
   go mod tidy
   go run main.go
   ```

### Option 2: Published Module

Once you publish Govoort to GitHub:

1. Tag a release:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. In your new project:
   ```bash
   go get github.com/yourusername/govoort@v0.1.0
   ```

## Advanced Usage

### Custom Handler

If you need to integrate Govoort with an existing HTTP server or middleware:

```go
server, err := govoort.New(config)
if err != nil {
    log.Fatal(err)
}

// Get the handler
handler := server.Handler()

// Use with your own server
http.ListenAndServe(":8080", yourMiddleware(handler))
```

### Accessing Loaded Pages

```go
server, err := govoort.New(config)
pages := server.Pages()

// Inspect or modify pages programmatically
for route, page := range pages {
    log.Printf("Route: %s", route)
}
```

## License

MIT

## Contributing

Contributions welcome! Please open an issue or pull request.
