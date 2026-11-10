---
type: feature
---

Inspect media processing and outbound webhook jobs in Server Settings → Jobs, and retry eligible failed jobs with the `server.jobs.manage` permission. Webhook deliveries now survive server restarts. Temporary failures retry automatically without holding a worker slot while waiting.
