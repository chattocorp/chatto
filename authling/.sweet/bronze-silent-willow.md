---
type: feature
---

Account erasure now runs on the shared durable job queue. Authling creates the `AUTHLING_JOBS` stream and the `authling-jobs` consumer at startup. Account deletion behavior is unchanged.
