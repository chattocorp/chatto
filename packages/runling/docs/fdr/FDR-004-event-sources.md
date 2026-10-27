# FDR-004: Event source lifecycle

**Status:** Experimental
**Last reviewed:** 2026-10-14

## Overview

Named event sources let a Runling server receive external input without requiring
a webhook endpoint. Webhooks and sources can coexist or be configured separately.

## Behavior

- Sources start eagerly in development and packaged serving, without a browser
  request. They use the same run store as HTTP routes.
- Applications can host the packaged console and sources in their own process.
  Object configuration loads once. The host owns signal cleanup by default and
  also provides explicit, awaited shutdown. One host can run in a process at a time.
- Sources can use a bounded batch queue to serialize work per application key
  while separate keys run concurrently. Capacity includes running and queued
  items. Applications select quiet periods and can extend them for queued work.
- Batch duplicate detection is process-local. Failed items can be admitted again;
  the source decides whether retry is safe. Closing the queue discards queued
  items and awaits running callbacks. The queue does not create or recover tasks.
- Each dispatch receives a fresh routing context and waits for all run
  registrations it initiated, not workflow completion. A failed registration
  rejects dispatch; successful registrations from the same delivery remain.
- A valid reload aborts and awaits old sources before starting replacements.
  Invalid configuration leaves current sources running.
- File watching is opt-in through `runling serve --watch`. Without the flag,
  configuration loads once and code changes require a server restart.
- Retained state survives a valid reload for unchanged names. Removing or renaming
  a source starts a separate state lifetime. The adapter must scope state to its
  connection identity and handle changes to its retained data format.
- A router can keep the run handles of the runs that it starts, for example in
  the retained state. It can later send messages to a run, close its input, or
  cancel it.
- Reload does not replace active workflows with new code. Adapters that keep
  run handles must account for their original code remaining live.
- An unhandled source failure produces a safe log entry and leaves that source
  stopped until reload. The source owns transport retries.
- Shutdown stops sources and awaits pending dispatches before interrupting active
  runs and flushing journals. Explicit user cancellation remains cancelled.
  Cleanup is cooperative. Registered roots can re-enter after a process restart
  with snapshot state. Sources can use `ctx.runs()` to rebuild their routing.
- Run records identify source origin and name. Older journals remain readable;
  run history does not restore source state or active execution.

## Design Decisions

### 1. Accept delivery at registration

**Decision:** Dispatch resolves after registration rather than workflow completion.
**Why:** A transport can continue receiving events while long workflows run.
**Tradeoff:** Acceptance is not workflow success, and a multi-run delivery is not
atomic. Adapters need their own retry and deduplication policy.

### 2. Retain state by source name

**Decision:** Keep the state map for unchanged names across reloads.
**Why:** Connections can preserve process-local routing and recovery context.
**Tradeoff:** Name stability does not establish identity compatibility. The adapter
must reset incompatible credentials, identity, or routing state itself.

## Related

- [ADR-012: Router-controlled workflow reactivation](../adr/ADR-012-router-controlled-reactivation.md)

- [ADR-011: In-process hosting and keyed batching](../adr/ADR-011-hosting-and-batching.md)
- [ADR-001: Process-local workflows](../adr/ADR-001-process-local-workflows.md)
- [ADR-004: Event sources](../adr/ADR-004-event-sources.md)
- [ADR-009: Typed task invocations](../adr/ADR-009-task-invocations.md)
- [ADR-010: Periodic workflow state snapshots](../adr/ADR-010-workflow-state-snapshots.md)
- [FDR-010: Messages between hosts, tasks, and runs](FDR-010-host-messages.md)
- [Event source guide](../event-sources.md) and [webhook routing](../webhook-routing.md)

## Open Questions

- Should the console expose source health independently of the runs it creates?
