# ADR-002: Explicit agent connections and child ownership

**Status:** Accepted
**Date:** 2026-09-22

## Context

An agent can receive messages while a tool or child task is running. Input
acceptance, model consumption, output delivery, and workflow completion happen
at different times. Implicit ownership makes messages easy to lose and resources
easy to leave running.

## Decision

Use tasks, runs, and messages as the public execution model. A task takes input,
can exchange messages while running, and returns a result. A workflow is a task
that coordinates tasks. An agent is an implementation of a task, not another
kind of child execution.

Keep task channels independent of agents. A connection explicitly connects an
inbox and output callbacks to an agent. It owns the inbox iterator, permits
sequential turns, and awaits ordered text delivery. It does not own the supplied
agent; callers dispose the connection before the agent.

Distinguish queue acceptance from model consumption. Steering is delivered at a
model-call boundary. It does not interrupt an active tool or guarantee compliance.
Callers retain input that was not consumed. A conversation coordinator can own
that retention and distinguish user input from task notifications.

Spawned runs own their identity, channels, status, result, and cleanup promise.
Cancellation can settle the result before the underlying work stops. Async
disposal cancels unfinished work and awaits cooperative cleanup.

Runs are caller-owned. Returning from a parent does not join all children
automatically. Agent supervision adopts ordinary runs for snapshots and
notifications, using the same identity and lifecycle. Its disposal cancels adopted
runs and awaits their cleanup. Parent cancellation propagates to children;
cancelling one child does not cancel its siblings.

## Consequences

The same task can serve an agent, another workflow, or a transport adapter.
Applications must choose and dispose the appropriate owner. A cancelled task
handle can settle before uncooperative underlying code stops. Agent tools retain
the host's permissions; delegation is not a security sandbox.
