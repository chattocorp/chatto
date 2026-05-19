# FDR-003: Thread Reply Echo

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

When posting a reply inside a thread, the user can optionally "also send to channel" — echoing the reply into the parent room's timeline so people watching the room see it without opening the thread. The echo appears alongside other room messages and links back to its thread.

## Behavior

- The thread pane composer shows an "Also send to channel" checkbox when the user has the right permission.
- Ticking the checkbox and sending the reply produces two visible artifacts: the reply inside the thread pane, and a copy of the same message in the room timeline.
- The checkbox resets to unchecked after each successful send.
- The echo in the room timeline shows a "Thread" indicator below the body; clicking it opens the thread.
- If the original reply was attributed to a specific message, the echo shows the same reply-attribution byline. Clicking the byline on the echo opens the thread and highlights the referenced message inside it.
- Editing or deleting the original reply automatically affects the echo too — they share the same message body.
- Reactions on the original and the echo are independent — they're different events as far as reactions are concerned.
- The thread's reply count is not incremented by the echo; the echo represents the same reply, not an additional one.
- Mention notifications fire once for the reply, not twice (the echo doesn't re-notify).
- The main-room composer never shows the echo checkbox — the action only makes sense from inside a thread.

## Design Decisions

### 1. Echo shares the message body, not the event identity

**Decision:** The echo and the original thread reply are two different events but point at the same `messageBodyId`. The body lives in one place; both events reference it.
**Why:** Edits and deletes operate on the body and need to apply atomically to both visible artifacts. Cloning the body would mean implementing edit fan-out, with its own consistency problems. See ADR-011.
**Tradeoff:** The two events have to be linked at read time so the frontend knows they share a body. The `echoOfEventId` field on the echo carries the link.

### 2. Reactions key on event ID, not body ID

**Decision:** Reactions attach to the event ID, so the echo and original accumulate reactions independently.
**Why:** People reacting in the channel timeline are reacting to the appearance there; people reacting in the thread are reacting to the contribution inside the thread. Conflating them would mute one of those signals.
**Tradeoff:** Total reaction count on a "reply that was also sent to channel" is split — there's no single canonical number. In practice this matches the user's mental model.

### 3. Mentions copy to the echo, but don't re-notify

**Decision:** The echo carries the same `mentionedUserIds` as the original, but only the original triggers mention notifications.
**Why:** The mention rendering (highlight, link to profile) needs to work on the echo too, so the field has to be present. But getting two notifications for one mention would be noisy.
**Tradeoff:** Mention-driven indicators in the UI need to look at both events; the notification system has to know to skip the echo.

### 4. Echo publish is best-effort

**Decision:** If the echo publish fails, a warning is logged and the original thread reply still succeeds.
**Why:** The reply is the primary artifact. Failing the whole operation because the secondary copy didn't make it would be worse than missing the copy.
**Tradeoff:** Rarely, an echo can fail silently from the user's perspective. The reply is still posted in the thread, so no message is lost.

### 5. Echo only flows thread → room, never the reverse

**Decision:** `alsoSendToChannel` is only valid when posting inside a thread. Sending a plain room message with the flag is rejected.
**Why:** The feature exists to bridge thread visibility back to the room. The reverse (a room message that also shows in some thread) doesn't have a well-defined target.

## Permissions

- `message.echo` — granted to `everyone` by default. Gates the "Also send to channel" checkbox at the server-role and per-room scopes.
- `message.post-in-thread` — required for the thread reply itself. Covers replies with `inReplyTo` attribution as well; there is no separate reply permission.

## Related

- **ADRs:** ADR-011 (message body / event split), ADR-026 (event identity via NanoID)
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion), FDR-005 (Reactions)
