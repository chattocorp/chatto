# ADR-006: Tasks communicate only with their parent and children

**Status:** Accepted
**Date:** 2026-09-28

## Context

A task can receive functions from its parent, such as a callback that posts to a
chat service. A child that calls such a function talks to the outside world
around its parent. The parent cannot filter, phrase, rate-limit, or even see
those messages, and one conversation can speak with several voices.

## Decision

A task communicates only with its parent and its children. A child sends
updates to its parent with `ctx.emit` and returns a result. A parent sends
messages to a child through the child's inbox. External communication, such as
a message to a user, belongs to the task that owns that conversation.

Runling supports this with typed updates and owner-side observation:

- A `notice` update carries a message that the owner should see now. It wakes
  the owner immediately.
- `observe(name, run, { onUpdate })` lets the owner's host code act on each
  child update inside the owner task, for example to report a verified fact
  deterministically instead of through a model.

Code that deliberately bypasses this rule must say so and explain why.

## Consequences

The owner is the single place that decides what reaches a user, in what words,
and how often. Children stay reusable, because they do not depend on the
owner's transport. A relayed message costs an owner turn when a model relays
it. Runling cannot enforce the rule, because JavaScript closures can reach any
function; it is a design rule for applications and reviews.
