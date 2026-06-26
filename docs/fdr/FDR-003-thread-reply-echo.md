# FDR-003: Thread Reply Echo

**Status:** Active
**Last reviewed:** 2026-06-01

## Overview

When posting a reply inside a thread, the user can optionally "also send to channel" — echoing the reply into the parent room's timeline so people watching the room see it without opening the thread. The echo appears alongside other room messages and links back to its thread.

## Behavior

- The thread pane composer shows an "Also send to channel" checkbox when the user has the right permission.
- Ticking the checkbox and sending the reply produces two visible artifacts: the reply inside the thread pane, and a copy of the same message in the room timeline.
- The checkbox resets to unchecked after each successful send.
- The echo in the room timeline shows a "Thread" indicator below the body; clicking it opens the thread.
- If the original reply was attributed to a specific message, the echo shows the same reply-attribution byline. Clicking the byline on the echo opens the thread and highlights the referenced message inside it.
- Editing or deleting the original reply automatically affects the echo too — edit/delete events target the original reply, and read models apply the change to the linked echo.
- Deleting the echo itself only hides that room-timeline copy. The original thread reply remains in the thread with its body readable.
- Reactions on the original and the echo are independent — they're different events as far as reactions are concerned.
- The thread's reply count is not incremented by the echo; the echo represents the same reply, not an additional one.
- Mention notifications fire once for the reply, not twice (the echo doesn't re-notify).
- The main-room composer never shows the echo checkbox — the action only makes sense from inside a thread.
- During the normal edit window, editing a thread reply shows the same "Also send to channel" checkbox. Saving with it checked creates or keeps the channel echo; saving with it unchecked hides the existing echo from the room timeline while keeping the thread reply readable.

## Design Decisions

### 1. Echo links by event identity, not payload aliases

**Decision:** The echo and the original thread reply are two different EVT envelopes. The echo carries `echoOfEventId`, which points at the original reply envelope. The message identity itself lives on the envelope (`Event.id`), not inside the `MessagePostedEvent` payload.
**Why:** GraphQL and EVT now model the same wrapper/payload boundary. Echoes still render the same text, but edits and deletes are propagated through the event-link relationship instead of a shared `messageBodyId` payload crutch.
**Tradeoff:** Read models have to keep the echo link when applying edit/delete state. Reactions remain naturally independent because they already key on the envelope event ID.

### 2. Echo deletion hides the echo artifact

**Decision:** Deleting an echo emits the normal durable message retraction for the echo's own event ID, and the room read model hides that echo from the main timeline. It does not retract the original thread reply.
**Why:** The echo is a first-class `MessagePostedEvent`, so its counterpart is the same delete/retract fact used for other messages. The special case is rendering policy: retracting an echo removes the copy, while retracting the original removes the underlying content.
**Tradeoff:** Echo retractions are interpreted differently from original reply retractions. The projection has to know whether the target event is an echo.

### 3. Reactions key on event ID, not body ID

**Decision:** Reactions attach to the event ID, so the echo and original accumulate reactions independently.
**Why:** People reacting in the channel timeline are reacting to the appearance there; people reacting in the thread are reacting to the contribution inside the thread. Conflating them would mute one of those signals.
**Tradeoff:** Total reaction count on a "reply that was also sent to channel" is split — there's no single canonical number. In practice this matches the user's mental model.

### 4. Mentions copy to the echo, but don't re-notify

**Decision:** The echo carries the same `mentionedUserIds` as the original, but only the original triggers mention notifications.
**Why:** The mention rendering (highlight, link to profile) needs to work on the echo too, so the field has to be present. But getting two notifications for one mention would be noisy.
**Tradeoff:** Mention-driven indicators in the UI need to look at both events; the notification system has to know to skip the echo.

### 5. Echo publish is best-effort

**Decision:** If the echo publish fails, a warning is logged and the original thread reply still succeeds.
**Why:** The reply is the primary artifact. Failing the whole operation because the secondary copy didn't make it would be worse than missing the copy.
**Tradeoff:** Rarely, an echo can fail silently from the user's perspective. The reply is still posted in the thread, so no message is lost.

### 6. Echo only flows thread → room, never the reverse

**Decision:** `alsoSendToChannel` is only valid when posting inside a thread. Sending a plain room message with the flag is rejected.
**Why:** The feature exists to bridge thread visibility back to the room. The reverse (a room message that also shows in some thread) doesn't have a well-defined target.

### 7. Echo state is editable during the edit window

**Decision:** The message update API can optionally reconcile a thread reply's channel echo state during the author's normal edit window. GraphQL `updateMessage` remains the legacy compatibility path, and ConnectRPC `MessageService.UpdateMessage` exposes the same behavior through the shared core message model. Omitting the field preserves current echo state for older clients and moderation edits.
**Why:** Users often realize shortly after posting in a thread that the reply should have been visible in the room. Treating the checkbox as edit-time message state keeps the interaction aligned with the composer.
**Tradeoff:** Echo reconciliation is not a new persisted event type; adding an echo appends the existing echo-shaped `MessagePostedEvent`, and removing one appends a normal `MessageRetractedEvent` for the echo artifact.

## Permissions

- `message.echo` — granted to `everyone` by default. Gates the "Also send to channel" checkbox at the server-role and per-room scopes.
- `message.post-in-thread` — required for the thread reply itself. Covers replies with `inReplyTo` attribution as well; there is no separate reply permission.

## Related

- **ADRs:** ADR-011 (message body / event split), ADR-026 (event identity via NanoID), ADR-038 (room-owned thread state)
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion), FDR-005 (Reactions)
