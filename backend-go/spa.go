package main

import (
	"net/http"
	"os"
	"path/filepath"
)

// SPAHandler serves the built React app, falling back to index.html for
// any path that isn't a real file on disk (client-side routing).
func SPAHandler(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	}
}
