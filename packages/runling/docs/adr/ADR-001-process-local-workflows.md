# ADR-001: Process-local TypeScript workflows

**Status:** Accepted
**Date:** 2026-09-22

## Context

Runling provides a library, CLI, and web console for TypeScript workflows and
agents. It is an independent product. Applications need ordinary functions and
explicit lifecycle control without a distributed scheduler or a required datastore.

## Decision

Workflows and tasks are TypeScript functions. Schemas can validate task inputs
and outputs. Execution, channels, and active agent sessions belong to the current
process. Applications own their integrations and external side effects.

The web server records run history in journals. A journal is an observation of
execution, not a durable continuation. Loading a journal for an unfinished run
marks it interrupted; it does not resume the workflow.
Graceful server shutdown also marks unfinished runs interrupted; explicit user
cancellation remains cancelled. The server does not restore application state,
agent sessions, or JavaScript execution from journals.

Cancellation uses abort signals and cooperative cleanup. It cannot force arbitrary
JavaScript to stop or undo external effects. Runling does not coordinate replicas
or provide a durable inbox or an exactly-once execution guarantee.

Keep Runling APIs independent of Chatto and Authling. Keep its decision records
with the package, with separate ADR and FDR numbering.

## Consequences

Applications can compose tasks with normal TypeScript control flow. They must
handle retries, idempotency, persistence, and external resource cleanup where
needed. A visible run history does not imply restart recovery. Durable execution
would require a separate decision about state and external effects.
