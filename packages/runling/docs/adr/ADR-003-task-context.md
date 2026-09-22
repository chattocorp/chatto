# ADR-003: Separate task state, output, and notifications

**Status:** Accepted
**Date:** 2026-09-22

## Context

A supervising agent needs to answer progress questions while child workflows
continue. A single progress string loses earlier findings. Forwarding every tool
event or assistant message wakes the supervisor too often and can cause repeated
user replies.

## Decision

The task manager keeps three separate forms of context:

- A workflow-owned JSON object describes the latest task state. Each update
  replaces the previous object.
- A bounded output history retains explicitly published commentary and findings.
- Notifications request the owner's attention. They can be coalesced without
  deleting the current state or retained output.

Lifecycle status and the final result remain separate from workflow-owned state.
The lifecycle status is authoritative when a last published phase is stale.
Snapshots are detached copies with timestamps and ages. The host decides when
to include them in an agent prompt and what to send to the user.

State updates and buffer-only output do not wake the supervisor. Findings and
selected lifecycle or failure events can notify it. Do not automatically collect
private reasoning or raw tool output into the context buffer.

This state belongs to the task manager in the current process. It is not a
durable memory service. Exact buffer sizes are provisional implementation limits,
not an architectural requirement.

## Consequences

The owner can answer status questions from a snapshot without asking a busy
child to repeat itself. Producers must publish useful state and output explicitly.
Bounded history can lose earlier information; timestamps and truncation metadata
help the owner identify incomplete context. Applications still control privacy
and which retained data is sent to a model provider.
