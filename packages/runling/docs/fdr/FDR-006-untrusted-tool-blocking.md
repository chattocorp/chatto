# FDR-006: Tool blocking after untrusted content

**Status:** Experimental
**Last reviewed:** 2026-09-27

## Overview

An application can mark tools whose results are untrusted and tools that must not
run after such results. Runling blocks those tools for the rest of the agent's
lifetime.

## Behavior

- `trust.untrusted` lists tool names. Any result from these tools, including an
  error result, marks the agent's context untrusted.
- `trust.blockAfterUntrusted` lists tool names that Runling blocks while the mark is
  set. The tool does not run. The model receives an error result with a fixed
  reason.
- `trust.onBlocked` runs before the model receives the refusal. Hosts can use it to
  show their own message, so the model does not describe the refusal. A failure in
  this callback writes an agent log line with the tool name only, and the block
  still applies.
- The mark lasts for the agent's lifetime and is copied to forks. User messages and
  notifications do not remove it.
- Each block writes an agent log line with the tool name. The line contains no
  arguments or content.
- Without a policy, Runling installs no hook and tool behavior is unchanged.
- The same behavior is available to plain Pi projects through
  `createTrustExtension` from `runling/extensions/trust`.
- Calls made in the same model step as the untrusted tool, before its result, are
  not blocked. The model has not seen the content at that point.
- Calls from codemode scripts pass the same checks. See
  [FDR-008](FDR-008-codemode.md).

## Design Decisions

### 1. Keep the mark for the agent's lifetime

**Decision:** Do not let a later message clear the mark.
**Why:** The untrusted content remains in the history, and message timing is
difficult to track correctly.
**Tradeoff:** The user must start a new conversation to use a blocked tool.

### 2. Name tools, not content

**Decision:** The policy uses tool names.
**Why:** Runling cannot decide whether content is safe.
**Tradeoff:** Applications must list every relevant tool, including tools that
return untrusted content in other ways.

## Related

- [ADR-005: Block tools after untrusted context](../adr/ADR-005-untrusted-context.md)
- [ADR-002: Agent ownership](../adr/ADR-002-agent-ownership.md)
- [FDR-007: Authorization classifier](FDR-007-authorization-classifier.md)
- [FDR-008: Codemode](FDR-008-codemode.md)
- [Agent API guide](../agents.md)

## Open Questions

- Should applications be able to mark content untrusted when they build a prompt?
