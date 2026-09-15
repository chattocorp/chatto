package web

import (
	"net/http"
	"net/url"
)

// requireSameOrigin writes the standard rejection response when browser origin
// evidence is invalid. Callers must return when it reports false.
func requireSameOrigin(w http.ResponseWriter, r *http.Request, publicOrigin *url.URL) bool {
	if !sameOrigin(r, publicOrigin) {
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		return false
	}
	return true
}

// guardFormRequest checks origin before limiting body reads. It does not read or
// parse the body; each route retains its parsing order and error response.
// Callers must return when it reports false.
func guardFormRequest(w http.ResponseWriter, r *http.Request, publicOrigin *url.URL, maxBytes int64) bool {
	if !requireSameOrigin(w, r, publicOrigin) {
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	return true
}
