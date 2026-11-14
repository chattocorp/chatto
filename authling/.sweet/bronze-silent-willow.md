---
type: feature
---

Account erasure now runs on the shared durable job queue. Authling creates the `AUTHLING_JOBS` stream and the `AUTHLING_JOB_STATE` bucket at startup, and the `authling-jobs` consumer. Account deletion behavior is unchanged.
