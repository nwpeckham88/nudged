package webui

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// DistFS is the filesystem root for the built UI. We prefer to read the
// static files from the local `internal/webui/dist` directory which the
// hub Dockerfile copies into the source tree during image build.
var DistFS fs.FS

func init() {
	// prefer local dist folder (useful during development)
	if _, err := os.Stat("internal/webui/dist"); err == nil {
		DistFS = os.DirFS("internal/webui/dist")
		return
	}
	// also check absolute path in case the final image copied files to /internal/webui/dist
	if _, err := os.Stat("/internal/webui/dist"); err == nil {
		DistFS = os.DirFS("/internal/webui/dist")
		return
	}

	DistFS = nil
}

// Register mounts the web UI at the given prefix (must end with '/').
// Example: Register(mux, "/ui/") will serve index at /ui and assets under /ui/*
func Register(mux *http.ServeMux, prefix string) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	if DistFS == nil {
		// no UI available at runtime; register a simple placeholder
		mux.HandleFunc(strings.TrimSuffix(prefix, "/"), func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "ui not available", http.StatusNotFound)
		})
		return
	}

	// serve static assets
	fileServer := http.FileServer(http.FS(DistFS))
	mux.Handle(prefix, http.StripPrefix(prefix, fileServer))

	// serve index at prefix without trailing slash
	mux.HandleFunc(strings.TrimSuffix(prefix, "/"), func(w http.ResponseWriter, r *http.Request) {
		// try to read index.html
		b, err := fs.ReadFile(DistFS, "index.html")
		if err != nil {
			http.Error(w, "ui not available", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// if base path is not '/', rewrite any relative asset paths
		if prefix != "/" {
			s := string(b)
			s = strings.Replace(s, "<head>", "<head><base href='"+path.Clean(prefix)+"/'>", 1)
			_, _ = w.Write([]byte(s))
			return
		}
		_, _ = w.Write(b)
	})
}
