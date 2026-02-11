package webui

import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed dist/* dist/*/* dist/*/*/*
var embedded embed.FS

// Handler returns an http.Handler that serves the embedded web UI.
func Handler() (http.Handler, error) {
    sub, err := fs.Sub(embedded, "dist")
    if err != nil {
        return nil, err
    }
    return http.FileServer(http.FS(sub)), nil
}
