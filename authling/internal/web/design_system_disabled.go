//go:build !bootstrap

package web

import "net/http"

// Production binaries do not expose the development component preview.
func mountDesignSystem(_ *http.ServeMux) {}
