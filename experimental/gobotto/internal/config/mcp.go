package config

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// MCP configures operator-selected remote capabilities. Missing configuration
// disables MCP. Only explicitly listed tools are exposed to conversations.
type MCP struct {
	Servers map[string]MCPServer `toml:"servers"`
}

// MCPServer uses Streamable HTTP. Header values are read from environment
// variables at startup; an unset optional variable omits that header.
type MCPServer struct {
	URL                    string            `toml:"url"`
	Tools                  []string          `toml:"tools"`
	OptionalHeadersFromEnv map[string]string `toml:"optional_headers_from_env"`
}

// Validate rejects ambiguous names, unsafe transports, and protocol-header
// overrides. Local HTTP is permitted for development servers only.
func (cfg MCP) Validate() error {
	bad := errors.New("invalid MCP config: use named HTTP servers, explicit tool lists, and optional credential headers from environment variables")
	serverName := regexp.MustCompile(`^[a-z][a-z0-9_]{0,15}$`)
	toolName := regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	envName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	headerName := regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
	if len(cfg.Servers) > 16 {
		return bad
	}
	for name, server := range cfg.Servers {
		if !serverName.MatchString(name) || strings.Contains(name, "__") || len(server.Tools) == 0 || len(server.Tools) > 32 {
			return bad
		}
		u, err := url.Parse(server.URL)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
			return bad
		}
		local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
		if u.Scheme != "https" && !(u.Scheme == "http" && local) {
			return bad
		}
		seen := map[string]bool{}
		for _, name := range server.Tools {
			if !toolName.MatchString(name) || seen[name] {
				return bad
			}
			seen[name] = true
		}
		seen = map[string]bool{}
		for header, variable := range server.OptionalHeadersFromEnv {
			lower := strings.ToLower(header)
			if !headerName.MatchString(header) || !envName.MatchString(variable) || seen[lower] {
				return bad
			}
			// Keep this credential-only: transport framing and MCP session headers
			// belong to the SDK, not configuration.
			if lower != "authorization" && lower != "x-api-key" && lower != "api-key" {
				return bad
			}
			seen[lower] = true
		}
	}
	return nil
}
