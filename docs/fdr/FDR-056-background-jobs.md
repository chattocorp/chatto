# FDR-056: Background Jobs

**Status:** Experimental
**Last reviewed:** 2026-11-10

## Overview

The Jobs page in server settings lets authorised users inspect background work
and retry failed jobs. It covers outbound webhooks, media processing, file
cleanup, call-key deletion, push-subscription cleanup, account-key deletion,
and notification alerts.

## Behavior

- Accepted jobs survive restart. An external effect can occur more than once.
- Jobs show queued, running, succeeded, or failed state, attempt counts, times,
  and a safe failure category. Content and credentials are not shown.
- Temporary failures retry automatically with increasing delays. Delays are
  capped at thirty minutes. Exhausted or permanent failures remain visible.
- A failed, unexpired job can be retried manually. Retry keeps its identity and
  payload and starts a new attempt budget. A retry applies only to the attempt budget the operator viewed. Concurrent
  or stale actions cannot retry a later budget that has already failed again.
- An expired webhook job cannot be retried. Retry never extends its original
  delivery deadline. Current access and endpoint state still control delivery.
- A media job succeeds when processing reaches a terminal asset outcome. A
  failed conversion is shown on the attachment; retrying a worker failure does
  not restart an already completed conversion.
- A failed cleanup or key-deletion job is unresolved work. It is not complete
  or abandoned. A retry repeats the idempotent deletion.
- A notification alert job also succeeds when the alert expired or is no longer
  eligible. Retry does not extend the alert delivery window.
- The list refreshes every three seconds while it is visible, and loads more
  results as it scrolls. A job detail page refreshes until the job succeeds or
  fails. The list also has a manual refresh. The pages have no cancellation,
  payload editing, or bulk controls.
- V1 retains job records without automatic expiry.

## Design Decisions

### 1. Use one execution model

**Decision:** All durable asynchronous effects use one managed queue.
Periodic reconciliation passes remain separate.
**Why:** Common execution and recovery controls reduce separate worker rules.
See ADR-123.
**Tradeoff:** Slow work can delay other job types.

### 2. Keep failed work available

**Decision:** Failed jobs stay in the list and can be retried where their
lifecycle permits it.
**Why:** Operators can recover work after fixing a tool or external service.
**Tradeoff:** Manual retry can repeat an external effect. Operators must check
its recipient when a duplicate matters.

### 3. Poll instead of realtime events

**Decision:** Visible job pages poll the job API. Job state has no realtime
event.
**Why:** Only authorised operators view jobs, and only while the page is open.
Polling needs no new realtime contract or delivery filter.
**Tradeoff:** Changes appear up to three seconds late, and each open page reads
the job list repeatedly.

## Gates

- `server.jobs.manage`: view job summaries and retry eligible failed jobs.
  Human credential sessions must enable privileged mode.
- `asset_processing.enabled`: process media in the server process. Standalone
  media processes remain available when embedded media processing is disabled.

## Related

- **ADRs:** ADR-123, ADR-069, ADR-114
- **FDRs:** FDR-008, FDR-038, FDR-046

## Open Questions

- Add a retention and indexing contract when job volume requires it.
- Add email and delayed deletion when those features need durable jobs.
- Give privacy cleanup its own capacity if other work delays it.
