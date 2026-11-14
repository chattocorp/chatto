---
type: feature
---

Inspect background jobs in Server Administration → Jobs, and retry eligible failed jobs with the `server.manage` permission. The page shows media processing, outbound webhooks, file cleanup, call-key and account-key deletion, push-subscription cleanup, and notification alerts. Webhook deliveries now survive server restarts. Temporary failures retry automatically without holding a worker slot while waiting. Succeeded jobs are kept for seven days; failed jobs stay until they are retried. Notification alerts and media processing have their own worker pools; `CHATTO_CORE_JOB_CONCURRENCY` limits the other jobs.

If you run standalone `chatto asset-processing` workers, upgrade them right after the servers. Older workers cannot process media jobs from upgraded servers, and a new worker exits until an upgraded server has created the job stream.
