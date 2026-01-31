package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed web/dist/*
var embeddedFiles embed.FS

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	// Get the subdirectory from the embedded filesystem
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		log.Fatal("Failed to access embedded files:", err)
	}

	// Create file server handler
	fileServer := http.FileServer(http.FS(distFS))

	// Handle all routes - serve index.html for SPA routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists
		if _, err := fs.Stat(distFS, path[1:]); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing, serve index.html for non-existent paths
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Launchpad server starting on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("Server error:", err)
		os.Exit(1)
	}
}
