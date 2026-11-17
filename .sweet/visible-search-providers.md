---
type: feature
---

Admin → System has a new Search tab. It shows search availability and index freshness separately, and the state, version, indexing progress, current work, last progress, test query result, and a hint for each search provider process. A failed provider stays visible when another provider is healthy, and a provider with a missing or outdated status shows as unknown. The overall health now shows as unknown, not OK, when no check has data. The `chatto_search_provider_*` metrics now count all providers that answer instead of describing one of them: query `chatto_search_provider_stalled > 0` instead of `== 1`.
