# Govoort

A lightweight Go web framework for building server-rendered web applications with Go templates. Govoort automatically discovers and routes your `.gohtml` templates, allowing you to focus on building features rather than configuring routes.

## Features

- **Convention-based routing**: Page templates in `web/pages/` are automatically mapped to routes
- **Layout system**: Reusable layouts in `web/layouts/` wrap your pages
- **Shared templates**: Common partials and components in `web/templates/`
- **Static file serving**: Automatic serving of files from `web/static/`
- **GET/POST handlers**: Easy-to-define data hooks and form actions per page
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
    config.RegisterHooks = registerPageHooks

    // Create and start server
    server, err := govoort.New(config)
    if err != nil {
        log.Fatalf("failed to create server: %v", err)
    }

    if err := server.ListenAndServe(*addr); err != nil {
        log.Fatal(err)
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
