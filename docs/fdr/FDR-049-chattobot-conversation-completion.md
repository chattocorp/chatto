# FDR-049: ChattoBot Conversation Completion

**Status:** Experimental
**Last reviewed:** 2026-10-04

## Overview

ChattoBot answers people in Chatto threads. A supervisor writes its messages
and selects how each turn ends. The host delivers that choice. This prevents
an extra model response from duplicating a completed reaction or reply.

## Behavior

- Each turn ends with one selected reply, one reaction, or silence.
- Questions and task results need a text reply. A simple acknowledgement can
  receive a reaction. The model decides which response fits the message.
- The supervisor can send a brief acknowledgement before research or other
  longer work. Short answers do not need a separate acknowledgement. A source
  investigation or implementation has its own announcement.
- After a reaction succeeds, further model text does not become a message.
  A failed reaction permits a text reply, but no second reaction attempt for
  the same user message.
- A completed turn does not prevent replies to later messages or task results.
- If the supervisor omits its final choice, it gets one attempt to supply it.
  This attempt cannot start more work. If it still omits the choice, the
  conversation fails through the existing reply-failure path.
- New pull request URLs still reach the user when a selected reply omits them
  or the supervisor selects silence. Existing authorization refusals remain
  separate host responses; they do not post after a final choice.

## Design Decisions

### 1. The host delivers an explicit choice

**Decision:** The supervisor selects its final response. Ordinary model text
does not go directly to the thread. The host accepts at most one final choice
per turn and blocks further tools.

**Why:** A prompt alone did not prevent the supervisor from adding an emoji
message after a successful reaction.

**Tradeoff:** Selecting the wrong response is still possible. Conversation
evaluations must check relevance and tone as well as delivery counts. A model
can also need a corrective turn, which adds latency.

### 2. Conversation policy belongs to ChattoBot

**Decision:** ChattoBot owns the reply, reaction, and silence choices. It uses
Runling's existing custom tools and agent conversation lifecycle. Chatto
requests continue through `@chatto/client`.

**Why:** Runling is an independent framework. It does not need Chatto message
or reaction concepts to execute agents and validate tool calls.

**Tradeoff:** Other Runling consumers define their own delivery policy. This
change does not add a generic structured-result API to Runling.

## Gates

- Reactions require a current message addressed to the bot and the bot's
  existing Chatto reaction permission.
- A background notification cannot receive a reaction.
- Only the model can select the final response. Codemode cannot select it.
- Existing maintainer, authorization, and untrusted-content checks still apply
  to work tools.

## Related

- [FDR-005: Reactions](FDR-005-reactions.md)
- [FDR-038: Bot Accounts](FDR-038-bot-accounts.md)
- [ChattoBot guide](../../packages/chattobot/README.md)
