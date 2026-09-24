// Package web serves the two browser apps compiled into the binary: the admin
// console (on the hub) and the customer portal (on each gate).
//
// Everything is self-hosted - fonts included. Nothing is fetched from a CDN,
// because customers inside a filtered network often cannot reach one.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// App returns an http.Handler for dist/<name>: static files where they exist,
// index.html for every other path so client-side routes survive a reload.
func App(name string) http.Handler {
	sub, err := fs.Sub(dist, "dist/"+name)
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	index, _ := fs.ReadFile(sub, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SecurityHeaders(w)
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "index.html" {
			if st, err := fs.Stat(sub, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(p, "assets/") || strings.HasPrefix(p, "api/") {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if len(index) == 0 {
			_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><title>Gozar</title><p>The web app was not built into this binary. Run <code>make web</code> before <code>go build</code>.</p>"))
			return
		}
		_, _ = w.Write(index)
	})
}

// SecurityHeaders are sent with every page and API response.
func SecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}
