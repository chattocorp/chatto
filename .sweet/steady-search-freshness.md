---
type: fix
---

A search provider whose index stops advancing now reports a degraded state instead of ready, and Admin → System shows stalled projections as a warning. New metrics report projection and search index stalls, waiting time, and the duration of the current and last index work: `chatto_projection_stalled`, `chatto_projection_pending_seconds`, `chatto_projection_current_operation_seconds`, `chatto_projection_last_commit_timestamp_seconds`, `chatto_projection_last_commit_duration_seconds`, and the `chatto_search_provider_*` family. Chatto logs a warning when a projection stalls and when it recovers.
