---
type: feature
---

Chatto now supplies Prometheus alerting rules in `examples/prometheus/chatto-alerts.yml`. They report failed search queries, a missing search provider, and stalled search indexes and projections. A slow index rebuild that still makes progress does not fire them. The server metrics now also include `chatto_search_provider_query_probe_consecutive_failures` and `chatto_search_provider_query_probe_in_flight_seconds`, which show the test query of the search provider. See the Alert rules section of the server operations guide.
