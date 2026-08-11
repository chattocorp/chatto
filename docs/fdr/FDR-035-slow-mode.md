# FDR-035: Slow Mode

**Status:** Active
**Last reviewed:** 2026-08-11

## Overview

Slow Mode limits how frequently one member can create messages in a channel
room. Room managers choose a visible interval from Off through six hours. The
server remains authoritative while every active client receives both room
configuration changes and the current viewer's next eligible posting time.

## Behavior

- Slow Mode is configured per channel room. `0` disables it; valid values are
  `0–21,600` seconds. The bundled client offers Off, 5s, 10s, 15s, 30s, 1m,
  2m, 5m, 10m, 15m, 30m, 1h, 2h, and 6h presets.
- One member-and-room timer covers root messages, thread replies, and
  attachment-only messages. A thread does not create a separate allowance.
- A channel echo is a projection of its original thread reply and does not
  start a second timer.
- Editing, deleting, reacting, typing, and staging attachments neither start
  nor reset the timer. Drafting and editing remain available while posting is
  blocked.
- Members with effective `room.manage` or `message.manage` permission bypass
  enforcement. The composer still identifies Slow Mode and says that the
  viewer is exempt.
- Enabling, increasing, decreasing, or disabling Slow Mode immediately
  recalculates eligibility from the member's latest successful original post,
  including posts made while Slow Mode was disabled or while the member was
  exempt. Losing a bypass permission therefore applies the latest post.
- Posting becomes legal at the exact boundary
  `now >= latest_post_at + slow_mode_seconds`.
- A rejected post preserves its draft. New clients show the authoritative
  countdown; older clients receive server enforcement and their existing
  generic send failure.

## Design Decisions

### 1. Store configuration as a room fact

**Decision:** `RoomSlowModeChangedEvent` carries the complete interval and the
room catalog projects it into authoritative room state.
**Why:** Slow Mode is durable room configuration, so replay, audit, and
realtime delivery should use the same event-sourced path as other room
settings.
**Tradeoff:** Mixed-version clusters must finish upgrading every replica before
operators enable Slow Mode because an older writer cannot enforce the new
fact.

### 2. Derive cooldowns from successful posts

**Decision:** Room Timeline keeps an O(1) latest-original-post timestamp for
each room and author and rebuilds it from retained timeline entries on snapshot
restore.
**Why:** The latest successful message is already the durable source of truth.
Derivation gives configuration changes immediate historical semantics without
creating per-user timer records or cleanup work.
**Tradeoff:** The timeline snapshot contract changes when index restore
semantics change, even though the serialized payload does not add an index.

### 3. Enforce during preflight and commit authorization

**Decision:** Message posting checks Slow Mode before work begins and again in
the room-wide OCC commit callback.
**Why:** Preflight gives quick feedback; the commit check and full-room fence
ensure concurrent posts on separate replicas cannot both succeed.
**Tradeoff:** Slow Mode shares the room aggregate contention boundary used by
message posting rather than adding a narrower author-specific write lane.

### 4. Keep edits outside the timer

**Decision:** Slow Mode applies only to new message creation.
**Why:** Its purpose is pacing conversation volume, not preventing correction
or moderation. Blocking edits would encourage leaving mistakes visible and
would make moderation less responsive.
**Tradeoff:** A member may continue revising the latest message during the
cooldown.

### 5. Reuse room realtime operations

**Decision:** Configuration changes emit `room_upsert`; message posts emit the
existing per-viewer `room_viewer_state_replace` with an optional next-post
timestamp.
**Why:** Existing room projection operations already provide ordered
multi-session convergence and reconnect bootstrap. No Slow-Mode-specific
realtime operation is needed.
**Tradeoff:** A room configuration change sends the full projected room row.

## Related

- **ADRs:** ADR-016 (OCC for message publishing), ADR-033 (event-sourced state), ADR-045 (public API stability), ADR-051 (resumable client projection)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-004 (Message Editing & Deletion), FDR-008 (File Attachments & Video Processing), FDR-019 (Room Lifecycle)
- **Issue:** [#999](https://github.com/chattocorp/chatto/issues/999)

## Open Questions

- Whether future moderation tools should expose the most recent post time or
  remaining cooldown for another member. The current API exposes only the
  authenticated viewer's deadline.
