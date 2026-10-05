# FDR-007: Authorization classifier

**Status:** Experimental
**Last reviewed:** 2026-09-30

## Overview

`createAuthorizationClassifier` creates a function that decides whether messages
from authorized people authorize an action. `authorizationGate` uses such a
function to block selected tool calls unless the decision is `allow`.

## Behavior

- A request has the action text, the messages (oldest first), and an optional
  policy and context. The classifier model receives them as one JSON document, so
  message text cannot pose as another part of the request.
- The classifier agent has one tool, `decide`, and loads no extensions, skills,
  prompt templates, themes, or context files. Only its first decision counts.
- The decision is `allow`, `deny`, or `unclear`, with a short reason for the
  acting agent. The reason is model text; do not show it as a host-written
  message.
- Without messages, the classifier returns `unclear` and makes no model request.
  An action longer than 60,000 characters also returns `unclear`.
- Errors, a timeout (60 seconds by default), and a missing decision return
  `unclear`. Cancellation of the workflow still throws.
- The classifier sends at most the 20 newest messages, each cut to 4,000
  characters, and at most the 10 newest context entries, each cut to 8,000
  characters.
- `authorizationGate` classifies only the listed tools. Each tool has a function
  that describes a call as the action text. Any decision other than `allow`
  blocks the call. The model receives a fixed reason with the decision and its
  reason. `onBlocked` runs first; its failure is ignored.
- Pi stops at the first `tool_call` hook that blocks a call. Install the gate
  after cheaper deterministic gates.

## Design Decisions

### 1. Separate the judge from the actor

**Decision:** The classifier never receives the acting agent's conversation or
tool results; it receives only what the application supplies.
**Why:** Untrusted content in that context must not influence the decision.
**Tradeoff:** The classifier does not see what the agent asked the people. A
short reply such as "yes" can be `unclear` unless the application passes the
question as context. That context is agent output, so an application should
accept a short agreement only for an action that the question names.

### 2. Fail closed

**Decision:** Every failure gives `unclear`.
**Why:** A gated action must not run because a model or provider failed.
**Tradeoff:** An outage of the classifier model blocks every gated action.

### 3. Leave message selection to the application

**Decision:** Runling does not choose which messages count.
**Why:** Only the application knows who can authorize an action and which
message source it can trust.
**Tradeoff:** Each application must select messages correctly, for example by
server-authenticated author and by order.

## Related

- [ADR-007: Classify authorization in a separate model call](../adr/ADR-007-authorization-classifier.md)
- [ADR-005: Block tools after untrusted context](../adr/ADR-005-untrusted-context.md)
- [FDR-006: Tool blocking after untrusted content](FDR-006-untrusted-tool-blocking.md)
- [Agent API guide](../agents.md)

## Open Questions

- Should the gate also receive the acting agent's tool calls, as some classifiers
  do, so that a short agreement can refer to an earlier proposal?
- Should the classifier record its decisions in the run journal for review?
