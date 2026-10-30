package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
)

//go:embed templates static
var assets embed.FS

//go:embed templates/base.html
var baseSource string

// staticTypes fixes Content-Type by extension so the answer does not depend on the
// host's MIME database (the Windows registry can map .js to text/plain).
var staticTypes = map[string]string{
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".ico":  "image/x-icon",
	".txt":  "text/plain; charset=utf-8",
	".json": "application/json",
}

// staticHandler serves the embedded static directory under /static/.
func staticHandler() http.Handler {
	sub, err := fs.Sub(assets, "static")
	if err != nil { // the embed directive guarantees the directory exists
		panic("web: embedded static directory missing: " + err.Error())
	}
	files := http.FileServerFS(sub)
	return http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct, ok := staticTypes[path.Ext(r.URL.Path)]
		if !ok {
			ct = mime.TypeByExtension(path.Ext(r.URL.Path))
		}
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
}
