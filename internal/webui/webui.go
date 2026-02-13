package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var content embed.FS

// Register mounts the UI handler on the given mux at the specified prefix.
// The prefix must end with a slash, e.g. "/ui/".
func Register(mux *http.ServeMux, prefix string) {
	dist, err := fs.Sub(content, "dist")
	if err != nil {
		panic(err)
	}

	handler := MakeHandler(dist, prefix)
	mux.Handle(prefix, handler)
}

// MakeHandler creates an http.Handler that serves static files from dist
// with SPA fallback to index.html for unknown routes.
func MakeHandler(dist fs.FS, prefix string) http.Handler {
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// clean path and strip prefix
		p := strings.TrimPrefix(r.URL.Path, prefix)
		p = path.Clean(p)

		// if path is empty or just slash, serve index
		if p == "" || p == "." || p == "/" {
			serveIndex(w, dist)
			return
		}

		// check if file exists in dist
		f, err := dist.Open(strings.TrimPrefix(p, "/"))
		if err == nil {
			_ = f.Close()
			// serve the file using http.FileServer which handles ranges, mime types, etc.
			http.StripPrefix(prefix, fileServer).ServeHTTP(w, r)
			return
		}

		// fallback to index.html for SPA routing
		serveIndex(w, dist)
	})
}

func serveIndex(w http.ResponseWriter, dist fs.FS) {
	f, err := dist.Open("index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, _ := f.Stat()
	http.ServeContent(w, &http.Request{}, "index.html", stat.ModTime(), f.(io.ReadSeeker))
}
