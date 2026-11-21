---
type: feature
---

Botto connects to Means collections through ordinary MCP settings, with explicit read and write tool lists. Use `serialize = true` to serialize calls to a server across conversations. Replace `[memory]` or `[brains]` with `[mcp.servers.<name>]`, running `means mcp --path` with the existing collection path. Keep all collection files unchanged. Collection descriptions and schemas come from the MCP `info` tool; Botto no longer preloads memories.

Botto includes skills for rendered SVG, Mermaid, and ANSI visuals, and for bug triage with environment details and repository-scoped duplicate searches. Issue drafts request authorization from the configured contact before publication when no owner has already approved them.

Klaus uses one collection for personal memory and shared Chatto knowledge. Its collection instructions encourage sourced knowledge capture and structured issue follow-up lookups; edits and deletions still require owner authorization. Positive acknowledgment reactions require silence without research or memory writes. Botto accepts supported Unicode emoji for reactions and prevents a reaction attempt from producing a progress message as well.
