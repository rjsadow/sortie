package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEmbeddedFS verifies the embedded filesystem is accessible
func TestEmbeddedFS(t *testing.T) {
	// Verify we can access the embedded filesystem
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		t.Fatalf("Failed to access embedded files: %v", err)
	}

	// Check that index.html exists
	_, err = fs.Stat(distFS, "index.html")
	if err != nil {
		t.Fatalf("index.html not found in embedded files: %v", err)
	}
}

// TestRootHandler verifies the root path serves index.html
func TestRootHandler(t *testing.T) {
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		t.Fatalf("Failed to access embedded files: %v", err)
	}

	fileServer := http.FileServer(http.FS(distFS))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		if _, err := fs.Stat(distFS, path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)
	}
}

// TestSPARouting verifies unknown paths fall back to index.html
func TestSPARouting(t *testing.T) {
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		t.Fatalf("Failed to access embedded files: %v", err)
	}

	fileServer := http.FileServer(http.FS(distFS))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		if _, err := fs.Stat(distFS, path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// Test a non-existent path - should still return 200 (SPA routing)
	req := httptest.NewRequest("GET", "/some/unknown/path", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 for SPA route, got %d", rec.Code)
	}
}
