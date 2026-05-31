# FDR-004: Message Editing & Deletion

**Status:** Active
**Last reviewed:** 2026-05-31

## Overview

Authors can edit and delete their own messages; moderators with the right permission can edit and delete others' messages, subject to outranking the author. Edits replace the message body; deletes remove the body and attachments and leave a "[Message deleted]" placeholder.

## Behavior

- Authors can edit their own messages within a 3-hour window from posting time. After the window closes, only moderators can edit. The window value is queryable via `Server.messageEditWindowSeconds` so the frontend can show countdown timers and disable the edit affordance at exactly the right moment.
- Only the message body text can be edited. Attachments aren't editable as text but can be removed individually.
- Deletions remove the message body and all attachments and replace the rendered message with a "[Message deleted]" placeholder.
- Deleting an already-deleted message is a no-op.
- Editing or deleting a thread reply that was echoed to the channel propagates to both visible artifacts automatically — they share the same body.
- Individual attachments and link previews can be removed from a message by the author without deleting the whole message.

## Design Decisions

### 1. 3-hour edit window for authors

**Decision:** Authors can edit their own messages only within 3 hours of posting. Moderators have no time limit. The 3-hour value is a Go constant (`core.MessageEditWindow`), exposed read-only over GraphQL as `Server.messageEditWindowSeconds`.
**Why:** Edits long after the fact (days or weeks later) damage the integrity of the conversation log — readers who already responded would be reacting to text that no longer exists. A short window covers genuine typo-fix cases; the moderation perm covers everything else. Exposing the constant via GraphQL (rather than hardcoding it in the frontend) lets the UI align countdown timers and disable-edit thresholds with the server's actual enforcement.
**Tradeoff:** Authors who notice a mistake a day later can't fix it themselves. They have to ask a moderator, or live with it. Operators who want a different window currently have to recompile — promoting it to a tunable server config is cheap if demand emerges.

### 2. Edit/delete changes are durable facts

**Decision:** Edits and deletions append durable message facts. The room timeline projection exposes the latest body, or a retracted placeholder after deletion.
**Why:** Message state is now event-sourced, so connected clients and rebuilt projections consume the same committed facts. This keeps edit/delete behavior consistent with the room event log. See ADR-033 and ADR-034.
**Tradeoff:** The user-facing timeline still exposes only the latest visible state. Showing prior versions would require a separate product decision and careful privacy handling.

### 3. Optimistic concurrency for edits

**Decision:** Edit mutations carry a revision token and fail if two edits race. The client must retry.
**Why:** A non-OCC update would risk silently overwriting concurrent edits — particularly bad when a moderator and the author both edit a message at once. See ADR-016.
**Tradeoff:** Clients need retry logic. In practice, conflicts are rare enough that a single retry almost always succeeds.

### 4. Shared body propagation

**Decision:** Thread replies and their channel echoes share one `messageBodyId`. An edit to either resolves to the same body and shows in both places.
**Why:** The alternative is fan-out edits with their own consistency problems. See FDR-003 and ADR-011.
**Tradeoff:** Frontend has to match edit/delete events against loaded messages by both `e.id` and `echoOfEventId` to refetch the right rows.

### 5. Delete physically removes the body, not just hides it

**Decision:** Delete removes the body and attachments from storage. Only the placeholder rendering remains.
**Why:** GDPR. Soft-delete leaves user-generated content in the database, which is the wrong default for an open-source chat app where users expect "delete" to mean delete. See ADR-007.
**Tradeoff:** No undo. Moderators can't restore a deleted message.

## Permissions

- `message.manage` — edit and delete *other* users' messages. Subject to outranking the author.
- (No separate permission for editing/deleting one's own messages — that's gated by authorship and the edit window only.)

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-011 (message body/event split), ADR-016 (OCC for message publishing), ADR-033 (event-sourced state), ADR-034 (single event stream)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo)
