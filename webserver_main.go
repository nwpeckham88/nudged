//go:build embed
// +build embed

package main

import (
	"embed"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed web/dist
var embeddedFiles embed.FS

func main() {
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.FS(distFS))

	// Serve static files from the dist root
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			// serve index
			serveIndex(w)
			return
		}
		// normalize path and try to open
		p = strings.TrimPrefix(path.Clean(p), "/")
		// try to open file in embedded FS
		f, err := distFS.Open(p)
		if err != nil {
			// fallback to index.html for SPA
			serveIndex(w)
			return
		}
		f.Close()
		// set correct mime type for known file extensions
		if ext := path.Ext(p); ext != "" {
			if m := mime.TypeByExtension(ext); m != "" {
				w.Header().Set("Content-Type", m)
			}
		}
		fileServer.ServeHTTP(w, r)
	}))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func serveIndex(w http.ResponseWriter) {
	data, err := embeddedFiles.ReadFile("web/dist/index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
