---
type: fix
---

The experimental MCP endpoint now limits requests separately for each caller. Unauthenticated requests or a flood from one bot or account can no longer block other authenticated bots and agents. Two shared limits remain: repeated invalid tokens briefly block callers on the same client network, and the first requests of new credentials share a safety ceiling. Each account gets 20 requests per second (burst 40) with at most 8 concurrent requests, and each replica applies the limits separately. If you run Chatto behind a reverse proxy, set `webserver.trusted_proxies`, so that Chatto limits each client network separately and not all callers together as the proxy.
