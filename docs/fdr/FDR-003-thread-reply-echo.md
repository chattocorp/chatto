# FDR-003: Thread Reply Echo

**Status:** Active
**Last reviewed:** 2026-09-15

## Overview

When a user posts a reply inside a thread, they can also echo the reply into the
parent room timeline. The echo appears with other room messages and links back
to its thread. In a DM, the client labels this action **Also send to
conversation**.

## Behavior

- The thread composer shows an echo checkbox when the user has the required
  permissions. It says **Also send to conversation** in a DM.
- Ticking the checkbox and sending the reply produces two visible artifacts: the reply inside the thread pane, and a reference to the same message in the room timeline.
- The checkbox resets to unchecked after each successful send.
- A thread reply with an echo shows a megaphone icon after its text. The icon is not a control. It updates when the echo is added or removed.
- The echo in the room timeline shows a "Thread" indicator below the body; clicking it opens the thread.
- If the original reply was attributed to a specific message, the echo shows the same reply-attribution byline. Clicking the byline on the echo opens the thread and highlights the referenced message inside it.
- Editing or deleting the original reply automatically affects the echo too — edit/delete events target the original reply, and read models apply the change to the linked echo.
- Deleting the echo itself only hides that room-timeline copy. The original thread reply remains in the thread with its body readable.
- Reactions shown on the original reply and its channel echo are the same reaction set; reacting in either place targets the original reply.
- The thread's reply count is not incremented by the echo; the echo represents the same reply, not an additional one.
- Search returns the original reply once. Echoes do not create separate search matches.
- Mention notifications fire once for the reply, not twice (the echo doesn't re-notify).
- The main-room composer never shows the echo checkbox — the action only makes sense from inside a thread.
- Editing a thread reply shows the same "Also send to channel" checkbox while
  the author can edit the reply. Effective `message.manage` keeps this action
  available after the normal edit window. Saving with the checkbox selected
  creates or keeps the channel echo. Saving with it cleared hides the existing
  echo from the room timeline and keeps the thread reply readable. A Disabled
  room cannot gain a new echo from a historical reply, but an existing echo can
  still be removed.

## Design Decisions

### 1. Echoes reference the original reply

**Decision:** An echo stores its own timeline identity and a link to the original thread reply. It does not store a second body, attachment list, preview, mention list, or reply attribution. Reads use the original content. API responses still contain a complete message.
**Why:** One content source prevents stale copies and makes edits apply in both views without duplicate writes.
**Tradeoff:** Reads must resolve the link. If the original is unavailable, the echo cannot use a historical copy as a fallback. Historical copies remain subject to normal secure deletion; upgrades do not bulk-delete them.

### 2. Echo deletion hides the echo artifact

**Decision:** Deleting an echo emits the normal durable message retraction for the echo's own event ID, and the room read model hides that echo from the main timeline. It does not retract the original thread reply.
**Why:** The echo is a first-class `MessagePostedEvent`, so its counterpart is the same delete/retract fact used for other messages. The special case is rendering policy: retracting an echo removes the copy, while retracting the original removes the underlying content.
**Tradeoff:** Echo retractions are interpreted differently from original reply retractions. The projection has to know whether the target event is an echo.

### 3. Reactions canonicalize to the original reply

**Decision:** Reactions attach to the original thread reply event ID. Channel echo event IDs are accepted as aliases at API boundaries and during projection replay, but new durable reaction facts target the original reply.
**Why:** The echo represents the same contribution in a second timeline context. A single reaction set keeps the room and thread views consistent and avoids users seeing different counts for one reply.
**Tradeoff:** Reaction reads need the echo link to resolve aliases. Historical echo-keyed reaction facts are canonicalized during projection replay instead of rewriting EVT.

### 4. Echoes resolve mentions without new notifications

**Decision:** Reads resolve mentions from the original reply. Only the original triggers mention notifications.
**Why:** Mentions must display in both views, but each recipient must receive only one notification.
**Tradeoff:** Timeline and realtime responses must resolve the original mention metadata.

### 5. Echo publish is best-effort

**Decision:** If the echo publish fails, a warning is logged and the original thread reply still succeeds.
**Why:** The reply is the primary artifact. Failing the whole operation because the secondary copy didn't make it would be worse than missing the copy.
**Tradeoff:** Rarely, an echo can fail silently from the user's perspective. The reply is still posted in the thread, so no message is lost.

### 6. Echo only flows thread → room, never the reverse

**Decision:** `alsoSendToChannel` is only valid when posting inside a thread. Sending a plain room message with the flag is rejected.
**Why:** The feature exists to bridge thread visibility back to the room. The reverse (a room message that also shows in some thread) doesn't have a well-defined target.

### 7. Echo state follows author edit permission

**Decision:** The ConnectRPC `MessageService.UpdateMessage` API can optionally
reconcile a thread reply's channel echo state when the author can edit the
message through the shared core message model. Effective `message.manage`
bypasses the normal author edit window. Omitting the field preserves current
echo state for clients that do not intend to change it and for edits by other
users.
**Why:** Users often realize shortly after posting in a thread that the reply should have been visible in the room. Treating the checkbox as edit-time message state keeps the interaction aligned with the composer.
**Tradeoff:** Echo reconciliation is not a new persisted event type; adding an echo appends the existing echo-shaped `MessagePostedEvent`, and removing one appends a normal `MessageRetractedEvent` for the echo artifact.

## Permissions

- `message.echo` — permits the echo. A DM can override it at the Direct
  messages scope.
- `message.post` — permits the new artifact in the main room timeline.
- `message.post-in-thread` — required for the thread reply itself. Covers replies with `inReplyTo` attribution as well; there is no separate reply permission.

## Related

- **ADRs:** ADR-011 (message body / event split), ADR-026 (event identity via NanoID), ADR-038 (room-owned thread state)
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion), FDR-005 (Reactions)
