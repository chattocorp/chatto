# FDR-004: Message Editing & Deletion

**Status:** Active
**Last reviewed:** 2026-08-10

## Overview

Authors can edit and delete their own messages; users with `message.manage` can edit and delete others' messages. Edits replace the message body; deletes remove the body and attachments and initially leave a "[Message deleted]" placeholder.

## Behavior

- Authors can edit their own messages within a 3-hour window from posting time. After the window closes, only moderators can edit. The window value is queryable via `Server.messageEditWindowSeconds` so the frontend can show countdown timers and disable the edit affordance at exactly the right moment.
- Only the message body text can be edited. Attachments aren't editable as text but can be removed individually.
- Edited message bodies are capped at the same 10,000-byte limit as newly posted message bodies.
- Deletions remove the message body and all attachments and initially replace the rendered message with a "[Message deleted]" placeholder.
- A deleted-message placeholder disappears after one hour when the message has no current attachments or link preview, reactions, or replies in its thread.
- Being a reply, a message inside a thread, or a channel echo does not by itself keep a deleted-message placeholder visible.
- Deleting an already-deleted message is a no-op.
- Editing a message does not re-resolve mentions. Mentions and mention notifications remain tied to the original posted message.
- A racing deletion always wins over an edit; a deleted message cannot be made visible again by a late edit retry.
- An edit retried after another message mutation keeps the latest attachments and preview metadata instead of restoring an older body snapshot.
- Every authorized edit, attachment removal, and preview removal rechecks mutable authority inside a room-OCC attempt and atomically guards the narrow authorization fence. A concurrent room or classified authorization change forces a retry before commit. Deletions still recheck mutable authority on each room-OCC attempt and retain request-time semantics for a cross-aggregate revocation.
- Editing or deleting a thread reply that was echoed to the channel propagates to both visible artifacts automatically through the echo's `echoOfEventId` link.
- Creating or removing a channel echo through an edit commits atomically with the parent edit. Echo creation also rechecks `message.echo` and `message.post` authority on each room-and-authorization-fence attempt.
- Deleting the echo artifact itself hides only the room-timeline echo. The original thread reply remains readable inside the thread.
- Individual attachments and link previews can be removed from a message by the author without deleting the whole message.
- ConnectRPC `MessageService.UpdateMessage`, `DeleteMessage`, `DeleteAttachment`, and `DeleteLinkPreview` expose message-management behavior through the shared core `MessageModel`.

## Design Decisions

### 1. 3-hour edit window for authors

**Decision:** Authors can edit their own messages only within 3 hours of posting. Moderators have no time limit. The 3-hour value is a Go constant (`core.MessageEditWindow`) exposed read-only through the public server-state API.
**Why:** Edits long after the fact (days or weeks later) damage the integrity of the conversation log — readers who already responded would be reacting to text that no longer exists. A short window covers genuine typo-fix cases; the moderation perm covers everything else. Exposing the constant through the API (rather than hardcoding it in the frontend) lets the UI align countdown timers and disable-edit thresholds with the server's actual enforcement.
**Tradeoff:** Authors who notice a mistake a day later can't fix it themselves. They have to ask a moderator, or live with it. Operators who want a different window currently have to recompile — promoting it to a tunable server config is cheap if demand emerges.

### 2. Edit/delete changes are durable facts

**Decision:** Edits and deletions append durable message facts. The room timeline projection exposes the latest body, or a retracted placeholder after deletion.
**Why:** Message state is now event-sourced, so connected clients and rebuilt projections consume the same committed facts. This keeps edit/delete behavior consistent with the room event log. See ADR-033 and ADR-034.
**Tradeoff:** The user-facing timeline still exposes only the latest visible state. Showing prior versions would require a separate product decision and careful privacy handling.

### 3. Optimistic concurrency for edits

**Decision:** Authorized edits use two OCC guards in one atomic JetStream batch: the replacement body is guarded by the room aggregate tail, and the semantic edit event is guarded by the narrow authorization-fence tail. Every attempt captures the authorization fence before the room tail, waits for current room, group, RBAC, actor, and message state, then rechecks room archive state, membership, current message identity and authorship, the exact author edit-window boundary, and applicable permissions. It rebuilds from the latest committed body and atomically commits the body, semantic edit, and any edit-driven echo change. A change to either boundary retries the complete decision. Internal linked-message propagation and deletions remain room-scoped. Message edits check but do not advance the authorization fence.
**Why:** Reusing a body prepared before a room OCC conflict could restore an attachment or preview removed by another mutation, while guarding edit facts independently could let a late body resurrect a deleted message. The room guard closes those lifecycle races. The authorization guard closes the cross-aggregate revocation race without making unrelated EVT traffic contend. Atomic echo reconciliation prevents partial success. See ADR-016, ADR-033, ADR-034, ADR-040, and ADR-068.
**Tradeoff:** Strict edit authorization depends on every authorization-changing writer advancing the fence. Deletions deliberately retain request-time authorization semantics and can overlap a cross-aggregate role or permission revocation until the serving replica projects it. The public API does not currently expose a client revision token, so concurrent full-text replacements resolve in commit order; the later successful edit supplies the visible text while retaining independently committed metadata changes.

### 4. Edits don't re-resolve mentions

**Decision:** Editing a message changes the visible body but does not add, remove, dismiss, or re-send mention notifications.
**Why:** Mentions are post-time attention facts, not mutable properties of the latest body. This prevents retroactive pings and keeps edit replay independent from mutable usernames and private body payload retention. See FDR-006.
**Tradeoff:** If an author needs to notify someone they forgot, they must send a new message. If they remove an `@name` while editing, the original notification still reflects that the mention happened.

### 5. Echo propagation

**Decision:** Thread replies and their channel echoes are separate message events linked by `echoOfEventId`. An edit or delete targeting the original reply is applied to both visible artifacts by the read model. A delete targeting the echo's own event ID hides only the echo artifact from the room timeline.
**Why:** Message identity belongs to the EVT envelope, and `MessagePostedEvent` remains payload-only. The link preserves the user-facing "same reply shown twice" behavior without duplicating envelope metadata into payload fields. See FDR-003.
**Tradeoff:** Frontend has to distinguish direct echo deletes from original-reply deletes: direct echo deletes remove the echo row, while original deletes tombstone any loaded echoes.

### 6. Delete physically removes the body payload, not just hides it

**Decision:** Message body content is stored in private body payload events separate from public post/edit facts. Delete appends the public retraction fact, removes attachments from storage, and securely deletes body payload events where the storage backend supports it. Only the placeholder rendering remains.
**Why:** GDPR. Soft-delete leaves user-generated content in the database, which is the wrong default for an open-source chat app where users expect "delete" to mean delete. Separating public message facts from body payloads preserves the conversation audit trail while allowing body material to be removed. See ADR-007.
**Tradeoff:** No undo. Moderators can't restore a deleted message. Older embedded-body EVT histories remain readable for compatibility but cannot be physically shredded at body granularity.

### 7. Context-free tombstones expire after one hour

**Decision:** Deleted-message placeholders remain visible for one hour. After that, the client removes placeholders that no longer carry visible attachments, previews, reactions, or thread replies. The same rule applies to deleted replies, thread messages, and channel echoes.
**Why:** A recent tombstone explains an abrupt gap to nearby readers, while a permanent placeholder adds noise when no surviving conversation depends on it.
**Tradeoff:** Timeline clients need deletion timestamps, and older clients can display more tombstones than newer clients during mixed-version rollouts. Replies that merely point at a deleted message do not retain its placeholder unless they are represented by the message's existing thread summary.

## Permissions

- `message.manage` — edit and delete *other* users' messages.
- (No separate permission for editing/deleting one's own messages — that's gated by authorship and the edit window only.)
- Attachment and link-preview removal is author-only; `message.manage` does not grant cross-user removal for those partial message edits.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-011 (message body/event split), ADR-016 (OCC for message publishing), ADR-033 (event-sourced state), ADR-034 (single event stream), ADR-038 (room-owned thread state)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-006 (@Mentions)
