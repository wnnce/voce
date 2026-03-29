package handler

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/wnnce/voce/clients"
)

// WebHandler serves the built frontend embedded in the binary
func WebHandler(w http.ResponseWriter, r *http.Request) {
	assets, err := clients.WebAssets()
	if err != nil {
		http.Error(w, "Assets not found", http.StatusNotFound)
		return
	}

	fsrv := http.FileServer(http.FS(assets))

	path := r.URL.Path
	if path == "/" {
		fsrv.ServeHTTP(w, r)
		return
	}

	_, err = fs.Stat(assets, strings.TrimPrefix(path, "/"))
	if err != nil {
		// If the file doesn't exist, we fallback to index.html for SPA support
		r.URL.Path = "/"
	}

	fsrv.ServeHTTP(w, r)
}
