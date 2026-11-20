---
type: feature
---

Standalone search providers (`chatto search-provider`) can serve `/livez`, `/readyz`, and `/metrics` on a separate health listener. Enable it with `[search_provider.health]`. Liveness stays healthy while the provider builds its index, and readiness succeeds only while the provider answers queries. With `diagnostics = true`, the listener also serves goroutine stacks, profiles, and index diagnostics to local connections. Each provider now checks its own index with a regular test query. While this query fails or blocks, the provider stops answering queries so that other providers answer them. Blocked index work no longer blocks provider shutdown indefinitely.
