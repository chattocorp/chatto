//go:build bootstrap

package web

import "net/http"

// mountDesignSystem exposes only static examples in development builds.
// It does not load accounts, sessions, configuration secrets, or workflow state.
func mountDesignSystem(mux *http.ServeMux) {
	mux.HandleFunc("GET /dev/ui", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, designSystemPage())
	})
}
