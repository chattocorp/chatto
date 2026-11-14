---
type: feature
---

Botto connects to Means collections through ordinary MCP settings, with explicit read and write tool lists. Use `serialize = true` to serialize calls to a server across conversations. Replace `[memory]` or `[brains]` with `[mcp.servers.<name>]`, running `means mcp --path` with the existing collection path. Keep all collection files unchanged. Collection descriptions and schemas come from the MCP `info` tool; Botto no longer preloads memories.

Botto includes skills for rendered SVG, Mermaid, and ANSI visuals, and for bug triage with environment details and repository-scoped duplicate searches. Issue drafts request authorization from the configured contact before publication when no owner has already approved them.
