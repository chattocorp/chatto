---
type: feature
---

Account erasure now runs on the shared durable job queue. Authling creates the `AUTHLING_JOBS` stream and the `authling-jobs` consumer at startup. Account deletion behavior is unchanged, except for retry timing: a failed erasure attempt now waits one second before the next attempt, and the delay doubles up to 30 minutes. Before, every retry waited one second.
