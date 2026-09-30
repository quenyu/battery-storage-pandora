// Package swagger serves the bundled, offline-capable Swagger UI.
package swagger

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var resources embed.FS

// Handler serves /swagger and /swagger/..., including the canonical slash redirect.
// Register it for both paths without stripping their prefix.
func Handler() http.Handler {
	static, err := fs.Sub(resources, "static")
	if err != nil {
		panic(err) // The embedded directory is verified by the compiler.
	}
	files := http.StripPrefix("/swagger/", http.FileServer(http.FS(static)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/swagger" {
			http.Redirect(w, r, "/swagger/", http.StatusPermanentRedirect)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(w, r)
	})
}
