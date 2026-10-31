package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPConfig(t *testing.T) {
	for _, extra := range []string{
		`[mcp.servers.exa]
url="https://mcp.exa.ai/mcp"
tools=["web_search_exa","web_fetch_exa"]
[mcp.servers.exa.optional_headers_from_env]
x-api-key="EXA_API_KEY"`,
		"",
	} {
		path := filepath.Join(t.TempDir(), "gobotto.toml")
		if err := os.WriteFile(path, []byte("[conversation]\nmodel='test'\n"+extra), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, server := range []MCPServer{
		{URL: "http://external.example/mcp", Tools: []string{"search"}},
		{URL: "https://user:secret@example.com/mcp", Tools: []string{"search"}},
		{URL: "https://example.com/mcp"},
		{URL: "https://example.com/mcp", Tools: []string{"search", "search"}},
		{URL: "https://example.com/mcp", Tools: []string{"*"}},
		{URL: "https://example.com/mcp", Tools: []string{"search"}, OptionalHeadersFromEnv: map[string]string{"Host": "HOST"}},
		{URL: "https://example.com/mcp", Tools: []string{"search"}, OptionalHeadersFromEnv: map[string]string{"x-api-key": "not an env name"}},
	} {
		if err := (MCP{Servers: map[string]MCPServer{"exa": server}}).Validate(); err == nil {
			t.Fatal("unsafe MCP config accepted")
		}
	}
}
