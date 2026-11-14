---
type: feature
---

Inspect background jobs in Server Administration → Jobs, and retry or delete failed jobs with the `server.manage` permission. The page shows media processing, outbound webhooks, file cleanup, call-key and account-key deletion, push-subscription cleanup, and notification alerts. Webhook deliveries now survive server restarts. Temporary failures retry automatically without holding a worker slot while waiting. Succeeded jobs are kept for three days. Failed jobs stay until someone retries or deletes them; set `CHATTO_CORE_JOB_FAILED_RETENTION` to remove them automatically. Each job type has its own worker pool: `CHATTO_CORE_JOB_CONCURRENCY` sets the pool size of each webhook, cleanup, and key-deletion job type, and notification alerts and media processing have their own sizes.

Media processing, file cleanup, and key deletion still retry until they finish, but the delay between attempts now doubles from 30 seconds up to 30 minutes. After a long outage of a tool, the key service, or object storage, waiting work can therefore resume up to 30 minutes after the outage ends.

Do not roll back to an earlier server version while jobs are queued or running. An earlier version does not run jobs that this version has accepted, including account-key deletion; they wait until you upgrade again.

If you run standalone `chatto asset-processing` workers, upgrade them right after the servers. Older workers cannot process media jobs from upgraded servers, and a new worker exits until an upgraded server has created the job stream.
