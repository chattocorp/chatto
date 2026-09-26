---
name: "chatto-event-sourcing"
description: "Use when designing, implementing, reviewing, debugging, or documenting Chatto event-sourced domain behavior, including EVT subjects, aggregate boundaries, Services, projections, optimistic concurrency control, read-your-writes, live/reconnect delivery, replay compatibility, migration safety, and rollback/deployment implications."
---

## Core Rules

- Durable domain facts go into `EVT`.
- Avoid adding new events to be stored in `EVT` unless they are required to record a durable domain fact.
- Multi-replica safety comes from JetStream OCC plus projection catch-up, not from in-process mutexes or "only one server will do this" assumptions.
- Every successful write that needs read-your-writes must wait for the local projector(s) that serve the next read path.
- Event subjects are part of the persisted data model. Changing an aggregate lane is a compatibility decision, not a refactor.
