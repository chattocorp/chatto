# ADR-007: Classify authorization in a separate model call

**Status:** Accepted
**Date:** 2026-09-30

## Context

Some agent tools must run only when a person asked for them. Examples are tools
that publish a change or write to an issue tracker. Instructions in the agent's
prompt cannot enforce this reliably. The agent can misread a question as a
request. Content from other people, such as a web page or an issue body, can also
tell the agent to call the tool.

[ADR-005](ADR-005-untrusted-context.md) blocks tools after untrusted content. It
does not check whether a person asked for a call in a clean context. It also
rejected confirmations that the acting agent reads, because their timing was
difficult to track and the untrusted content stayed in the history.

## Decision

Runling provides an authorization classifier. It is a separate model call that
answers one question: do these messages from authorized people authorize this
action?

- The application supplies the action as text, the messages, and an optional
  policy and context. The application selects the messages from a trusted
  source, such as server-authenticated authors.
- The classifier never sees the acting agent's conversation or tool results.
  Content there cannot argue with the decision. The application can add context
  that comes from agent output, such as a message that people answered. That
  context reaches the classifier, so the application must choose it with care.
- The classifier fails closed. Errors, timeouts, and a missing decision give
  `unclear`. Callers treat `unclear` like `deny`.
- `authorizationGate` applies the classifier to selected tool calls in a Pi
  `tool_call` hook. Applications can also call the classifier directly, for
  example to decide whether a reply approves a pending action.

The classifier does not replace deterministic checks. Applications install it
after cheaper gates, such as a role check.

## Consequences

Applications can enforce "only when a person asked" without trusting the acting
agent. The check costs one model call for each gated action and adds its latency.
A classifier can still misjudge an ambiguous message. The fail-closed default
turns such cases into a request for clearer confirmation, not an unwanted action.

Text in the action itself, such as an issue body that the agent wrote, reaches
the classifier. The classifier treats it as data, but a hostile action text can
still try to influence it. Applications that need exact approval must show the
exact action to people and accept only messages that arrive after it.

## Related

- [ADR-005: Block tools after untrusted context](ADR-005-untrusted-context.md)
- [FDR-007: Authorization classifier](../fdr/FDR-007-authorization-classifier.md)
