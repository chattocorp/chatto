# ADR-005: Block tools after untrusted context

**Status:** Accepted
**Date:** 2026-09-27

## Context

Tools such as web search or page reading return content that other people control.
That content can contain instructions that try to change what the agent does. An
agent that also has powerful tools, such as tools that publish changes or steer
other tasks, can then act for an attacker. Prompt instructions cannot prevent this
reliably.

Applications first tried to let a new user message confirm the untrusted content.
That approach depends on exactly when the model receives each message. Reviews
found several timing bugs in it, and the untrusted content stays in the history
after a confirmation.

## Decision

An agent can declare a trust policy with two tool lists. A result from a tool in
the `untrusted` list marks the agent's context untrusted. While the mark is set,
Runling blocks calls to tools in the `blockAfterUntrusted` list before they run.
The model receives a fixed reason.

The mark is never cleared. It lasts for the agent's lifetime and is copied to
forks, because the content remains in the model history. User messages do not
remove it. To use a blocked tool, the application starts a new agent, for example
for a new conversation.

Runling enforces the policy with a standalone Pi extension, `extensions/trust.ts`,
which uses Pi's `tool_result` and `tool_call` hooks. Runling agents install it
through the `trust` option. Plain Pi users can install the same extension. The
policy names tools; it does not inspect content.

## Consequences

The rule is simple to implement, test, and explain. Applications declare policy
instead of tracking message timing. A conversation that read untrusted content
cannot use the blocked tools again, even when the user wants that. Applications
should separate capabilities between agents, so that an agent that reads untrusted
content does not also need powerful tools.

This does not detect untrusted content that reaches the agent in other ways, such
as prompts that the application builds. It does not stop an agent from sending data
through tools that remain available.

[ADR-007](ADR-007-authorization-classifier.md) adds a separate check that people
authorized an action. It keeps the rule of this record: the acting agent does not
decide whether a confirmation is valid.
