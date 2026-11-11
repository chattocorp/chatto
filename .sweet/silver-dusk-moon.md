---
type: feature
---

Inspect background jobs in Server Settings → Jobs, and retry eligible failed jobs with the `server.jobs.manage` permission. The page shows media processing, outbound webhooks, file cleanup, call-key and account-key deletion, push-subscription cleanup, and notification alerts. Webhook deliveries now survive server restarts. Temporary failures retry automatically without holding a worker slot while waiting. Succeeded jobs are kept for seven days; failed jobs stay until they are retried.
