# ADR-028: Event-ID-Keyed Read State

**Date:** 2026-05-06

**Status:** Accepted

**Tracking issue:** [#330](https://github.com/chattocorp/chatto/issues/330)

**Related:** [ADR-026](ADR-026-event-identity-via-nanoid.md), [ADR-027](ADR-027-instance-space-server-consolidation.md), [ADR-029](ADR-029-instance-to-server-rename.md), [ADR-030](ADR-030-space-tier-retirement.md)

**Naming note:** This ADR was written during the consolidation work and refers to subjects like `space.{s}.room.{r}.msg.*` and the legacy `SPACE_{id}_RUNTIME` KV bucket. Post-ADR-029/030, the subject became `server.room.{kind}.{roomId}.msg.*`; post-#596 read markers live in `RUNTIME_STATE` as `read.room.{userId}.{roomId}`. The core event-ID keying decision is unchanged.

## Context

Each user's per-room read marker (`room_read_status.{userId}.{roomId}` in the per-space RUNTIME KV) used to be the absolute JetStream sequence number of the last-read root message, encoded as 8 BigEndian bytes. `HasUnread` compared that stored seq to the room's current last-root seq.

ADR-026 already removed JetStream sequences from the public API and the event model — clients see only event IDs and timestamps. But the *persisted* read-state value remained sequence-keyed, which becomes a problem for the ADR-027 ("Server consolidation") migration: Phase 4 of #330 will copy / renumber JetStream streams. After renumbering, every stored uint64 sequence is either silently wrong (it now points to a different message) or out of range. We want Phase 4 to be a clean stream copy with no read-state translation logic in the migration script.

## Decision

Persist the **stable event ID** (14-char NanoID, see ADR-022/ADR-026) as the read marker instead of the volatile JetStream sequence number. Use a **new key prefix** (`room_read_event.*` instead of `room_read_status.*`) so the legacy 8-byte uint64 entries are simply orphaned, not translated.

**Comparison logic in `HasUnread`:**

1. Fetch the room's current last-root event via `GetLastMsgForSubject("space.{s}.room.{r}.msg.*")`. Extract the event ID from the returned subject and the timestamp from the message.
2. Fetch the user's stored event ID. If it equals the room's last-root ID, they're caught up (fast path — no further lookups).
3. Otherwise, resolve the stored event ID's timestamp via `GetLastMsgForSubject(SpaceRoomMessage(s, r, storedID))` and compare to the room's last-root timestamp.

A missing stored event (deleted message) is treated as "unread"; the user re-marks and state self-corrects.

### Lazy "caught up at first read post-deploy"

Members always have a marker — `JoinRoom` and the DM `joinDMRoom` write either the room's current last event ID (room had messages) or an empty-string sentinel (room was empty). The empty string is a real value that means "member with nothing specific read yet"; `HasUnread` treats it as unread once any messages exist, so a brand-new member of an empty room correctly sees later posts as unread.

A *missing* `room_read_event` key, by contrast, only happens for users who were members **before this PR shipped**. On their first `GetLastReadEventID` call post-deploy, the marker is lazy-initialized to the room's current last event ID and persisted, so they're treated as caught up. The legacy `room_read_status` key is never consulted.

The honest semantic is "caught up at first read post-deploy", not strictly "at deploy time": if a deploy-era user's first post-deploy interaction with a room comes after new messages have arrived, those messages are silently swallowed into the lazy-init. For active users this window is small (next page load); for inactive users it's the price of avoiding a per-instance migration step. We accept that trade given Chatto's alpha posture and the fact that read state is the most disposable data class in a chat app.

A related consequence: on a deploy-era user's *first* `markRoomAsRead` call, the GraphQL response's `previousLastReadAt` is null (because lazy-init makes the previous and new markers identical). The frontend's "messages since last read" highlight window is therefore empty for that one call. From the next mark-read onwards it works normally.

Concurrency safety: lazy-init uses `bucket.Create` (atomic insert), not `Put`. If another writer (`MarkRoomAsRead`, `JoinRoom`, `PostMessage` auto-mark) wrote a real marker between our not-found read and our write, `Create` returns `ErrKeyExists` and we re-read instead of clobbering. This follows the project convention spelled out in `cli/AGENTS.md`.

## Consequences

- **Phase 4 of #330 doesn't have to translate read state.** Event IDs are stream-renumber-proof; the bulk stream copy doesn't touch the read-state bucket. Legacy `room_read_status.*` entries can be deleted at any time (or never — they're tiny). Caveat: if Phase 4 ends up dropping and recreating the per-space RUNTIME KV bucket (rather than just renumbering the stream), all `room_read_event.*` markers go with it and every user becomes a deploy-era user under the lazy-init semantics above. That's likely tolerable but worth noting in the Phase 4 plan.
- **`HasUnread` does up to one extra `GetLastMsgForSubject` call** in the "user has unread" case (the rare case where their marker is older than the latest root and not equal). Still O(1) per call.
- **`GetSequenceTimestamp` is gone.** It only existed to resolve the old read-state seqs to timestamps for `markRoomAsRead`'s response. Replaced by `GetEventTimestamp(spaceID, roomID, eventID)`, which uses subject-based O(1) lookup. (`GetEventSequence` remains — it serves a different use case: deriving a JetStream consumer start position from an event ID.)
- **`JoinRoom` / `joinDMRoom` always write a marker**, even for empty rooms. The empty-string sentinel is what lets `GetLastReadEventID` distinguish "fresh member, nothing read" from "deploy-era user, no marker at all".
- **Auto-mark on `PostMessage` for thread replies looks up the room's last root event** (one extra subject lookup per thread-reply auto-mark) so the marker always points to a real root event ID. Previously this worked by accident because seqs are linear across root and thread events; with event IDs, we have to be explicit. Whether thread replies should dismiss room-level unread *at all* is a separate question for a future ADR.
- **Lazy init can silently swallow messages that arrived between deploy and a user's first post-deploy read.** Documented above; accepted.
- **GraphQL contract is unchanged.** `MarkRoomAsReadResult` still returns `lastReadAt` and `previousLastReadAt` Times — the same shape the frontend already consumed under ADR-026.
