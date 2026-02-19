package tests_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rouxblouw/govoort"
)

func setupTestPages(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "govoort_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	pagesDir := filepath.Join(tmpDir, "web/pages")
	layoutsDir := filepath.Join(tmpDir, "web/layouts")

	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatalf("failed to create pages dir: %v", err)
	}
	if err := os.MkdirAll(layoutsDir, 0755); err != nil {
		t.Fatalf("failed to create layouts dir: %v", err)
	}

	// Create a simple layout
	baseLayout := `{{define "base"}}<html><body>{{template "content" .}}</body></html>{{end}}`
	if err := os.WriteFile(filepath.Join(layoutsDir, "base.gohtml"), []byte(baseLayout), 0644); err != nil {
		t.Fatalf("failed to create base layout: %v", err)
	}

	// Create some pages
	indexPage := `{{define "content"}}<h1>Index</h1>{{end}}`
	if err := os.WriteFile(filepath.Join(pagesDir, "index.gohtml"), []byte(indexPage), 0644); err != nil {
		t.Fatalf("failed to create index page: %v", err)
	}

	aboutPage := `{{define "content"}}<h1>About</h1><p>{{.Message}}</p>{{end}}`
	if err := os.WriteFile(filepath.Join(pagesDir, "about.gohtml"), []byte(aboutPage), 0644); err != nil {
		t.Fatalf("failed to create about page: %v", err)
	}

	return tmpDir
}

func TestRouter(t *testing.T) {
	tmpDir := setupTestPages(t)
	defer os.RemoveAll(tmpDir)

	config := govoort.DefaultConfig()
	config.PagesDir = filepath.Join(tmpDir, "web/pages")
	config.LayoutsDir = filepath.Join(tmpDir, "web/layouts")
	config.AutoRegisterPages = false // explicitly disable auto-registration

	rootRouter := govoort.NewRouter()
	rootRouter.RegisterPage("/", govoort.PageHook{})

	aboutRouter := govoort.NewRouter()
	aboutRouter.RegisterPage("", govoort.PageHook{
		Data: func(w http.ResponseWriter, r *http.Request) (any, error) {
			return map[string]any{"Message": "Hello from Router!"}, nil
		},
	})

	rootRouter.UseRouter("/about", aboutRouter)

	config.Router = rootRouter

	server, err := govoort.New(config)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Test Index
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for index, got %d", resp.StatusCode)
	}

	// Test About
	resp, err = http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for about, got %d", resp.StatusCode)
	}
}

func TestNestedRouter(t *testing.T) {
	tmpDir := setupTestPages(t)
	defer os.RemoveAll(tmpDir)

	config := govoort.DefaultConfig()
	config.PagesDir = filepath.Join(tmpDir, "web/pages")
	config.LayoutsDir = filepath.Join(tmpDir, "web/layouts")
	config.AutoRegisterPages = false

	// Create a nested page template
	if err := os.MkdirAll(filepath.Join(config.PagesDir, "admin"), 0755); err != nil {
		t.Fatal(err)
	}
	adminPage := `{{define "content"}}<h1>Admin</h1>{{end}}`
	if err := os.WriteFile(filepath.Join(config.PagesDir, "admin/dashboard.gohtml"), []byte(adminPage), 0644); err != nil {
		t.Fatal(err)
	}

	rootRouter := govoort.NewRouter()
	adminRouter := govoort.NewRouter()
	adminRouter.RegisterPage("/dashboard", govoort.PageHook{
		Template: "admin/dashboard.gohtml",
	})

	rootRouter.UseRouter("/admin", adminRouter)
	config.Router = rootRouter

	server, err := govoort.New(config)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/admin/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /admin/dashboard, got %d", resp.StatusCode)
	}
}

func TestAutoRegisterPagesSetting(t *testing.T) {
	tmpDir := setupTestPages(t)
	defer os.RemoveAll(tmpDir)

	config := govoort.DefaultConfig()
	config.PagesDir = filepath.Join(tmpDir, "web/pages")
	config.LayoutsDir = filepath.Join(tmpDir, "web/layouts")

	// Default is false now
	server, err := govoort.New(config)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	// Should be 404 because auto-registration is off and no router provided
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for index with auto-registration off, got %d", resp.StatusCode)
	}

	// Enable auto-registration
	config.AutoRegisterPages = true
	server, err = govoort.New(config)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts2 := httptest.NewServer(server.Handler())
	defer ts2.Close()

	resp, err = http.Get(ts2.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for index with auto-registration on, got %d", resp.StatusCode)
	}
}
