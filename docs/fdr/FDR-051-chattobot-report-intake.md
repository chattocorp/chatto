# FDR-051: ChattoBot Report Intake

**Status:** Experimental
**Last reviewed:** 2026-10-15

## Overview

Operators can configure rooms where ChattoBot responds to new bug reports and
feature requests without a mention. This helps users prepare useful reports
while the bot stays silent for unrelated posts.

## Behavior

- Intake is disabled by default. Operators select rooms by stable room ID.
- New top-level human posts in those rooms are classified. Replies, bot posts,
  forwarded copies, and existing history do not start automatic intake.
- Accepted reports continue into a normal conversation in the same workflow
  run and report thread. The bot
  can ask questions and inspect attachments. Incomplete reports can qualify.
- Image-only posts qualify for inspection. The classifier sees attachment types,
  not image contents, so some accepted posts might not be reports.
- Ignored posts and failed classifications produce no reply, reaction, or typing.
  Users can mention the bot to request help after a missed report.
- Addressed messages bypass classification. Follow-up messages must address the
  bot as usual. An addressed message during classification cancels the classifier
  and starts the conversation in the same run.
- Intake does not authorize GitHub changes or investigations. A maintainer must
  send an addressed request, even if they wrote the original report.
- Duplicate deliveries are suppressed for 24 hours in the running bot process,
  including source reloads. Failed classifications are not retried automatically.

## Design Decisions

### 1. Classify before starting a conversation

**Decision:** The intake workflow calls a classifier with no tools or visible
Chatto effects. It then exits or runs the conversation as its child. Routing
only starts the workflow and delivers later messages; it does not act on decisions.
**Why:** Silence must not depend on a conversation agent deciding not to reply.
The normal conversation can focus on helping with accepted reports.
**Tradeoff:** Accepted reports need an extra model request. Classification errors
can miss reports or cause unwanted replies.

### 2. Select rooms explicitly

**Decision:** Require stable room IDs and process only new top-level posts.
**Why:** Room renames must not change the policy. Other conversations must keep
their existing mention and reply behavior.
**Tradeoff:** Operators must configure the IDs. Unaddressed follow-ups do not
continue the conversation automatically.

### 3. Keep intake separate from authority

**Decision:** Automatic intake cannot authorize privileged work.
**Why:** A report from a maintainer is not necessarily a request for an issue
or an investigation. Classification must not expand the bot's authority.
**Tradeoff:** Maintainers must send an addressed request before that work starts.

### 4. Share only what classification needs

**Decision:** Send post text and attachment MIME types to the configured model
provider. Use low reasoning effort and a 30-second model deadline.
**Why:** Classification does not need tools, room history, or attachment contents.
**Tradeoff:** Even ignored posts reach the model provider and incur model costs.
The provider sees the bot host's IP address. Operators must tell room members
that the bot processes new posts without a mention.

## Gates

- `CHATTO_AUTO_ENGAGE_ROOM_IDS` selects intake rooms; an empty value disables it.
- The bot account must have access to the room and the report.
- `CHATTO_ALLOWED_USER_ID`, when configured, also restricts intake.
- `CHATTO_MAINTAINER_USER_IDS` still gates investigations and GitHub changes,
  with a current addressed request required.

## Related

- [FDR-038: Bot Accounts](FDR-038-bot-accounts.md)
- [ChattoBot setup and evaluations](../../packages/chattobot/README.md)

## Open Questions

- Should accepted report threads also receive automatic follow-up intake?
- Should failed intake survive process restarts and support retries?
