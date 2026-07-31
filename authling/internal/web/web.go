// Package web serves Authling's server-rendered user interface and embedded
// browser assets.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var embeddedAssets embed.FS

// Handler returns Authling's public HTTP handler. Its pages are rendered on
// the server and remain usable without client-side JavaScript.
func Handler() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic("open embedded web assets: " + err.Error())
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := homePage().Render(r.Context(), w); err != nil {
			http.Error(w, "render page", http.StatusInternalServerError)
		}
	})
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
