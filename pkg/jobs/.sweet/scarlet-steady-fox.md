---
type: feature
---

Add the shared durable job queue. One job stream holds protobuf facts for each job, and the state of a job is the fold of its facts. Enqueue and manual retry append facts under optimistic concurrency control. A durable consumer runs the jobs with capped exponential backoff, and an in-memory projection gives the job listing with a status filter. A succeeded job leaves one fact that expires after three days; other facts stay until the maximum age of the stream. An `Effect` adapter moves an existing durable consumer onto the queue.
