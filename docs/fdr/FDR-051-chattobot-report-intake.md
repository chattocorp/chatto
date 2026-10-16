# FDR-051: ChattoBot Report Intake

**Status:** Experimental
**Last reviewed:** 2026-10-16

## Overview

Operators can configure rooms where ChattoBot responds to new bug reports and
feature requests without a mention. This helps users prepare useful reports
while the bot stays silent for unrelated posts. In active bot conversations,
ordinary thread replies can continue the conversation without another mention.

## Behavior

- Intake is disabled by default. Operators select rooms by stable room ID.
- New top-level human posts in those rooms are classified. Replies, bot posts,
  forwarded copies, and existing history do not start automatic intake.
- Accepted reports continue into a normal conversation in the same workflow
  run and report thread. The bot
  can ask questions and inspect attachments. Incomplete reports can qualify.
- When GitHub is configured, the conversation searches existing issues as soon
  as a feature or bug symptom is specific enough. It reads and links plausible
  matches with a short summary. If several issues could apply, it asks which,
  if any, covers the request before continuing the interview or investigating
  causes. Users can discuss missing behavior in the thread. These reads need no
  maintainer approval; an existing issue is not proof of a shipped feature or fix.
- Relevant epic/task context comes from GitHub's native parent/sub-issue
  relationships. The bot distinguishes the parent's scope from its tasks and uses
  these relationships for authorized links between issues. Discussion alone does
  not authorize a link change or a move to another parent.
- Image-only posts qualify for inspection. The classifier sees attachment types,
  not image contents, so some accepted posts might not be reports.
- Ignored posts and failed classifications produce no reply, reaction, or typing.
  Users can mention the bot to request help after a missed report.
- Addressed messages bypass classification. An addressed message during
  classification cancels the classifier and starts the conversation in the same run.
- Active conversations check ordinary human replies every 90 seconds while the
  bot is not answering. This also applies to conversations started by a ping.
  The classifier considers all eligible messages since the bot's latest post,
  with a short preceding excerpt. Relevant replies continue the same conversation
  together. Thanks, unrelated discussion, and requests to leave the conversation
  to humans should stay silent.
- Empty or unchanged batches require no model call. Another reply causes the
  full batch since the bot's latest post to be considered again. Checks stop when
  the conversation ends. Only addressed messages and accepted follow-ups reset
  the one-hour inactivity timeout. Pings remain immediate and cancel pending checks.
  Current turns and background jobs can finish before the conversation expires.
  Expiry is checked with follow-ups, so an inactive conversation can remain open
  until the next polling tick.
- Intake and automatic follow-ups do not authorize GitHub changes or investigations.
  A maintainer must send an addressed request, even if they wrote the original report.
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
**Why:** Room renames must not change the policy. In other rooms, the bot must
be addressed before it joins a conversation.
**Tradeoff:** Operators must configure the IDs. Once the bot joins, automatic
follow-up checks also apply outside intake rooms.

### 3. Keep intake separate from authority

**Decision:** Automatic intake and follow-ups cannot authorize privileged work.
**Why:** A report from a maintainer is not necessarily a request for an issue
or an investigation. Classification must not expand the bot's authority.
**Tradeoff:** Maintainers must send an addressed request before that work starts.

### 4. Share only what classification needs

**Decision:** Send post text and attachment MIME types to the configured model
provider. Follow-up checks also include a short preceding excerpt ending with the
bot's latest post. Use low reasoning effort and a 30-second model deadline.
**Why:** Classification does not need tools, complete room history, or attachment contents.
**Tradeoff:** Even ignored posts reach the model provider and incur model costs.
The provider sees the bot host's IP address. Operators must tell room members
that the bot processes new posts and active thread replies without a mention.

### 5. Check quiet conversations periodically

**Decision:** Collect ordinary replies before deciding whether the bot should
participate again. Use the last bot post as the boundary, with one remembered
batch to skip unchanged checks. Keep each message's authenticated author.
**Why:** Users often answer in several short messages. A check can assess these
together without a conversation-agent turn for each post or a per-message ledger.
**Tradeoff:** Automatic replies can take about 90 seconds plus model time. A
classifier can miss useful replies or interrupt human discussion. A ping bypasses
the delay. Checks use the newest thread page; if it has no bot post, the bot stays
silent rather than guessing the boundary. Process restarts end these checks.

## Gates

- `CHATTO_AUTO_ENGAGE_ROOM_IDS` selects intake rooms; an empty value disables it.
- The bot account must have access to the room and the report.
- `CHATTO_ALLOWED_USER_ID`, when configured, also restricts intake and follow-ups.
- `CHATTO_MAINTAINER_USER_IDS` still gates investigations and GitHub changes,
  with a current addressed request required.

## Related

- [FDR-038: Bot Accounts](FDR-038-bot-accounts.md)
- [ChattoBot setup and evaluations](../../packages/chattobot/README.md)

## Open Questions

- Should failed intake survive process restarts and support retries?
