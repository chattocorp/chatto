# FDR-056: Background Jobs

**Status:** Experimental
**Last reviewed:** 2026-11-17

## Overview

The Jobs page in server settings lets authorised users inspect background work,
retry failed jobs, and delete failed jobs. It covers outbound webhooks, media processing, file
cleanup, message-body erasure, server logo and banner cleanup, call-key deletion, push-subscription cleanup, runtime credential cleanup, account-key deletion,
Neighborhood refresh, notification alerts, and transactional email.

## Behavior

- Accepted jobs survive restart. An external effect can occur more than once.
- Jobs show queued, running, succeeded, or failed state, attempt counts, times,
  and a safe failure category. Content and credentials are not shown.
- Temporary failures retry automatically with increasing delays. Delays are
  capped at thirty minutes. Exhausted or permanent failures remain visible.
- An attempt that a server restart interrupts counts as an attempt.
- Webhook and notification alert jobs have an attempt limit. Media, cleanup,
  Neighborhood refresh, key-deletion, and email jobs have no attempt limit; email still stops at its
  delivery deadline. The page shows only their attempt count.
- A failed, unexpired job can be retried manually. Retry keeps its identity and
  payload and starts a new attempt budget. A retry applies only to the failure
  that the operator viewed. Concurrent or stale actions cannot retry a later
  budget that has already failed again.
- An expired webhook or email job cannot be retried. Retry never extends its original
  delivery deadline. Current access and endpoint state still control delivery.
- Email jobs retry transport failures until the original code or reset-link expiry. They retain no addresses or mail content in the job history. See [FDR-041](FDR-041-transactional-email-delivery.md).
- Neighborhood refresh appears in Jobs when the server has Neighbors to check. A server with no Neighbors updates its empty directory directly. A failed refresh attempt retries after one minute, then at doubling intervals up to thirty minutes.
- A media job succeeds when processing reaches a terminal asset outcome. A
  failed conversion is shown on the attachment; retrying a worker failure does
  not restart an already completed conversion.
- A cleanup or key-deletion job that keeps failing is unresolved work. It is
  not complete or abandoned. A manual retry of a permanently failed job repeats
  the idempotent deletion.
- A notification alert job also succeeds when the alert expired or is no longer
  eligible. Retry does not extend the alert delivery window.
- The list shows the most recently updated jobs first and can show all, failed,
  or queued jobs. The selected filter is part of the page address, so it stays
  after the operator opens a job and goes back. The list refreshes every three
  seconds while it is visible, and loads more results as it scrolls. A job
  detail page refreshes until the job succeeds or fails. The pages have no
  cancellation of unfinished jobs, payload editing, or bulk controls.
- Servers older than `0.6.0-beta.0` have no job API. The settings navigation then omits
  Jobs, and the Jobs page shows a compatibility notice
  ([FDR-031](FDR-031-client-server-compatibility-discovery.md)).
- A failed job can be deleted, also when it cannot be retried. The page asks
  for confirmation. Delete abandons the work of the job; it cannot be retried
  later. A delete applies only to the failure that the operator viewed, so it
  cannot remove a job that another operator retried.
- Succeeded jobs leave the list three days after completion. Unfinished jobs
  stay until they finish. Failed jobs stay until someone retries or deletes
  them. A server operator can set `core.job_failed_retention` to remove failed
  jobs automatically after that time.

## Design Decisions

### 1. Use one execution model

**Decision:** All durable asynchronous effects use one managed queue.
Periodic reconciliation passes remain separate.
**Why:** Common execution and recovery controls reduce separate worker rules.
See ADR-123.
**Tradeoff:** Operators have one more stream to back up. Each job type has its
own worker pool, so slow work of one type does not delay another type.

### 2. Keep failed work available

**Decision:** Failed jobs stay in the list and can be retried where their
lifecycle permits it. They do not expire by default.
**Why:** Operators can recover work after fixing a tool or external service. A
failed cleanup or key-deletion job is unresolved work and must not disappear.
**Tradeoff:** Manual retry can repeat an external effect. Operators must check
its recipient when a duplicate matters. Failed jobs accumulate until someone
handles them.

### 3. Let operators delete failed jobs

**Decision:** An operator can delete a failed job after confirmation. Automatic
removal is an optional server setting, off by default.
**Why:** Some failures can never succeed, such as a delivery to an endpoint
that no longer exists. Without delete, these jobs would stay forever.
**Tradeoff:** Delete works on one job at a time. An operator with many dead
jobs must set the retention or delete them one by one.

### 4. Poll instead of realtime events

**Decision:** Visible job pages poll the job API. Job state has no realtime
event.
**Why:** Only authorised operators view jobs, and only while the page is open.
Polling needs no new realtime contract or delivery filter.
**Tradeoff:** Changes appear up to three seconds late, and each open page reads
the job list repeatedly.

## Gates

- `server.manage`: view job summaries, retry eligible failed jobs, and delete
  failed jobs.
- `asset_processing.enabled`: process media in the server process. Standalone
  media processes remain available when embedded media processing is disabled.

## Related

- **ADRs:** ADR-123, ADR-069, ADR-114
- **FDRs:** FDR-008, FDR-031, FDR-038, FDR-046

## Open Questions

- Add delayed deletion when it needs durable jobs.
- Add a bulk delete for failed jobs when operators need it.
