# FDR-056: Background Jobs

**Status:** Experimental
**Last reviewed:** 2026-11-10

## Overview

The Jobs page in server settings lets authorised users inspect background work
and retry failed jobs. The first version covers outbound webhooks and media
processing.

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
- The page has explicit refresh, paginated results, and expandable details.
  It has no cancellation, payload editing, or bulk controls.
- V1 retains job records without automatic expiry.

## Design Decisions

### 1. Use one execution model

**Decision:** Media and webhook work use one managed queue. Other asynchronous
work remains outside this first version.
**Why:** Common execution and recovery controls reduce separate worker rules.
See ADR-123.
**Tradeoff:** Slow work can delay other job types.

### 2. Keep failed work available

**Decision:** Failed jobs stay in the list and can be retried where their
lifecycle permits it.
**Why:** Operators can recover work after fixing a tool or external service.
**Tradeoff:** Manual retry can repeat an external effect. Operators must check
its recipient when a duplicate matters.

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
