# FDR-004: Message Editing & Deletion

**Status:** Active
**Last reviewed:** 2026-09-01

## Overview

Authors can edit and delete their own messages; users with `message.manage` can edit and delete others' messages. Edits replace the message body; deletes remove the body and attachments and initially leave a "[Message deleted]" placeholder.

## Behavior

- Authors can edit their own messages within a 3-hour window from posting time. After the window closes, only moderators can edit. The window value is queryable via `Server.messageEditWindowSeconds` so the frontend can show countdown timers and disable the edit affordance at exactly the right moment.
- Editing requires current room membership. In a channel room, it also requires
  broad `message.read`, or `message.read-interactions` with a relationship to
  the message's thread. The operation reads and returns the current message.
  DM membership authorizes the read. Posting and deletion remain independently
  authorized and do not return surrounding message state.
- Only the message body text can be edited. Attachments aren't editable as text but can be removed individually.
- Edited message bodies are capped at the same 10,000-byte limit as newly posted message bodies.
- Edited messages show a pen icon after their text. The icon is not a control.
- Deletions remove the message body and all attachments. The client removes the
  row immediately when no visible context remains.
- Attachment bytes are deleted only when the durable asset owner is the exact message being changed; a duplicate reference left by an older vulnerable server is removed without damaging the owning message.
- A deleted-message placeholder remains visible only while the message has a
  current attachment or link preview, a reaction, or a reply in its thread.
- Being a reply, a message inside a thread, or a channel echo does not by itself keep a deleted-message placeholder visible.
- Deleting an already-deleted message is a no-op.
- Editing a message does not re-resolve mentions. Mentions and mention notifications remain tied to the original posted message.
- Retracting a message removes notification occurrences whose exact target is
  that message. Retracting only a channel echo removes the echo artifact, not
  occurrences targeting the canonical thread reply.
- A racing deletion always wins over an edit; a deleted message cannot be made visible again by a late edit retry.
- An edit retried after another message mutation keeps the latest attachments and preview metadata instead of restoring an older body snapshot.
- Every authorized edit, attachment removal, preview removal, and deletion rechecks mutable authority inside a room-OCC attempt. The authorization read repeats when RBAC, room-group, or user inputs change during the decision. A concurrent room change forces the complete command attempt to retry. A cross-aggregate authorization change after the stable decision can overlap the command.
- Editing or deleting a thread reply that was echoed to the channel propagates to both visible artifacts automatically through the echo's `echoOfEventId` link.
- Creating or removing a channel echo through an edit commits atomically with the parent edit. Echo creation also rechecks `message.echo`, `message.post`, and the room's Threading Mode on each room-OCC attempt. Disabled rooms reject new echoes while still allowing an existing historical echo to be removed.
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

**Decision:** Authorized edits use the room aggregate tail as their OCC boundary. Every attempt waits for current room and message state, validates stable room-group, RBAC, and user authorization inputs, and rechecks room archive state, membership, current message identity and authorship, the exact author edit-window boundary, and applicable permissions. It rebuilds from the latest committed body and atomically commits the body, semantic edit, and any edit-driven echo change. A room conflict retries the complete decision. Internal linked-message propagation and deletions remain room-scoped.
**Why:** Reusing a body prepared before a room OCC conflict could restore an attachment or preview removed by another mutation, while guarding edit facts independently could let a late body resurrect a deleted message. The room guard closes those lifecycle races. Stable request-time authorization gives one clear decision point without a synthetic domain event. Atomic echo reconciliation prevents partial success. See ADR-016, ADR-033, ADR-034, ADR-040, ADR-068, and ADR-087.
**Tradeoff:** A cross-aggregate revocation after the final authorization validation can overlap a successful edit. The public API does not currently expose a client revision token, so concurrent full-text replacements resolve in commit order; the later successful edit supplies the visible text while retaining independently committed metadata changes.

### 4. Edits don't re-resolve mentions

**Decision:** Editing a message changes the visible body but does not add, remove, dismiss, or re-send mention notifications.
**Why:** Mentions are post-time attention facts, not mutable properties of the latest body. This prevents retroactive pings and keeps edit replay independent from mutable usernames and private body payload retention. See FDR-006.
**Tradeoff:** If an author needs to notify someone they forgot, they must send a new message. If they remove an `@name` while editing, the original notification still reflects that the mention happened.

### 5. Echo propagation

**Decision:** Thread replies and their channel echoes are separate message events linked by `echoOfEventId`. An edit or delete targeting the original reply is applied to both visible artifacts by the read model. A delete targeting the echo's own event ID hides only the echo artifact from the room timeline.
**Why:** Message identity belongs to the EVT envelope, and `MessagePostedEvent` remains payload-only. The link preserves the user-facing "same reply shown twice" behavior without duplicating envelope metadata into payload fields. See FDR-003.
**Tradeoff:** Frontend has to distinguish direct echo deletes from original-reply deletes: direct echo deletes remove the echo row, while original deletes tombstone any loaded echoes.

### 6. Delete physically removes the body payload, not just hides it

**Decision:** Message body content is stored in private body payload events separate from public post/edit facts. Delete appends the public retraction fact, securely deletes body payload events where the storage backend supports it, and removes attachment storage only after verifying that the asset is durably attached to that exact message. Only the placeholder rendering remains.
**Why:** GDPR. Soft-delete leaves user-generated content in the database, which is the wrong default for an open-source chat app where users expect "delete" to mean delete. Separating public message facts from body payloads preserves the conversation audit trail while allowing body material to be removed. See ADR-007.
**Tradeoff:** No undo. Moderators can't restore a deleted message. Older embedded-body EVT histories remain readable for compatibility but cannot be physically shredded at body granularity.

### 7. Context-free tombstones disappear immediately

**Decision:** The client immediately removes deleted-message placeholders that
have no visible attachments, previews, reactions, or thread replies. The same
rule applies to deleted replies, thread messages, channel echoes, and
attachment-only messages after removal of their final attachment.
**Why:** A placeholder has value only when visible conversation state still
refers to the deleted message. An empty placeholder adds noise and does not help
the reader.
**Tradeoff:** A deletion can create an immediate gap in a timeline. Replies that
merely point at a deleted message do not retain its placeholder unless the
message's thread summary contains a reply.

## Permissions

- `message.manage` — edit and delete *other* users' messages.
- `message.read` — read and edit any channel-room message.
- `message.read-interactions` — read and edit a channel-room message in a
  related thread. DM membership authorizes DM reads without either permission.
  Deletion remains independently authorized by authorship or `message.manage`.
- (No separate permission for editing/deleting one's own messages — that's gated by authorship and the edit window only.)
- Attachment and link-preview removal is author-only; `message.manage` does not grant cross-user removal for those partial message edits.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-011 (message body/event split), ADR-016 (OCC for message publishing), ADR-033 (event-sourced state), ADR-034 (single domain event stream), ADR-038 (room-owned thread state), ADR-076 (notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions), ADR-082 (derived thread interactions), ADR-087 (request-time authorization with aggregate OCC)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-006 (@Mentions), FDR-012 (Notifications), FDR-039 (Message Access & Interactions)
