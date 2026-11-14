package http_server

import "bytes"

// frontendConfigPlaceholder marks where the bundled frontend's app template
// receives server-supplied frontend settings. The standalone frontend and the
// desktop app serve the template without replacement, so the frontend must
// treat a missing setting as its standalone default.
const frontendConfigPlaceholder = "<!-- FRONTEND_CONFIG_PLACEHOLDER -->"

// frontendGutterMetaName is the meta element name that the frontend reads in
// `apps/frontend/src/lib/frontendConfig.ts`.
const frontendGutterMetaName = "chatto-gutter"

// injectFrontendConfig replaces the frontend config placeholder with meta
// elements that carry operator settings for the bundled frontend. Meta
// elements need no CSP script hash.
func (s *HTTPServer) injectFrontendConfig(content []byte) []byte {
	value := "false"
	if s.config.Frontend.Gutter.Enabled {
		value = "true"
	}
	tag := `<meta name="` + frontendGutterMetaName + `" content="` + value + `" />`
	return bytes.Replace(content, []byte(frontendConfigPlaceholder), []byte(tag), 1)
}
