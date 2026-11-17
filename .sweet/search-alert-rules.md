---
type: feature
---

Chatto now supplies Prometheus alerting rules in `examples/prometheus/chatto-alerts.yml`. They report failed search queries, a missing search provider, and stalled search indexes and projections. A slow index rebuild that still makes progress does not fire them. The server metrics now also include `chatto_search_provider_query_probe_consecutive_failures` and `chatto_search_provider_query_probe_in_flight_seconds`, which show the test query of the search provider. When `chatto run` includes the search provider, its metrics listener now also serves the provider's own `chatto_search_query_probe_*`, `chatto_search_provider_serving`, and `chatto_search_index_write_age_seconds` metrics, so that each replica reports its own provider. See the Alert rules section of the server operations guide.
