# ADR-011: In-process hosting and keyed batching

**Status:** Accepted
**Date:** 2026-10-26

## Context

Applications need to host workflows without a child-process launcher. Sources
also need to collect and limit incoming work without duplicating scheduling code.
Neither requirement needs a new execution model above ordinary tasks.

## Decision

Applications can start the packaged web host with a configuration object in their
own process. The host owns HTTP, source startup, signal handling, and cleanup.
Workspace hosting also serves the console from source under
[ADR-013](ADR-013-source-workspace-exports.md).
It uses the installed runtime so application task events reach the same console
and journal. One web host owns a process at a time. File-based CLI serving uses
the same host API; object configurations are fixed for that host's lifetime.

A bounded, process-local batch queue owns execution order, capacity, and
quiet-period timers. It runs one batch per key at a time and permits separate
keys to run concurrently. Applications supply item identities, grouping, and
quiet-period policy. The queue calls application code; it does not create tasks,
own workflow identities, or restore external input.

Long-lived workflows remain ordinary root tasks. Applications decide how sources
find or start them, admit work, handle child results, and close their roots.
Parents retain and await their children as specified in
[ADR-009](ADR-009-task-invocations.md). Recovery follows
[ADR-010](ADR-010-workflow-state-snapshots.md): restore root identity and state,
not queued work or external effects.

## Consequences

Applications can share a host and a scheduling utility without adopting an
agent or conversation model. Application-specific lifecycle and routing stay
outside Runling. The queue provides no durable delivery. Callers must stop
admission, signal active work, and await cleanup before releasing resources.
