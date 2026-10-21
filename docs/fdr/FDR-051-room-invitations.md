# FDR-051: Room Invitations

**Status:** Experimental
**Last reviewed:** 2026-10-21

## Overview

Room members with `room.invite` can invite existing human server members to a
channel room. The recipient accepts or declines in the room welcome screen.

## Behavior

- The invitation dialog names the room and explains the seven-day response
  period. Senders can select several members as removable pills before sending.
  Search results include existing room members as disabled choices with an
  explanation, so senders can see why they cannot select them.
  If some invitations fail, successful recipients leave the selection and
  failed recipients remain available for retry.
- Invitations expire after seven days. A room and recipient can have one
  pending invitation. Sending it again does not extend its expiry.
- The sender must be a room member with `room.invite`. Acceptance checks that
  the sender still meets both conditions. Suspensions prevent acceptance.
- Acceptance grants membership without `room.join`. It does not grant message,
  attachment, or call permissions. A repeated acceptance does not join again
  after the recipient leaves.
- Invited rooms appear in their normal sidebar groups. This can make a group
  visible for the first time. The group shows only visible or invited rooms.
- Overview marks rooms with an active invitation as Invited. The recipient
  can select the room to open its invitation screen without `room.join`.
- An invitation icon identifies the room. Opening it shows the inviter,
  expiry, and Accept invitation and Decline buttons. Protected room content
  stays unavailable until the recipient joins.
- The inviter's badge opens their user menu. Decline requires confirmation.
  Decline and leave confirmations explain whether the user can join without
  an invitation or needs a new invitation. If an older server does not report
  rejoin authority, the leave confirmation says a new invitation may be needed.
- Decline and expiry remove the invitation entry. Ordinary room visibility
  rules still apply. A group with no visible content disappears.
- Invitations apply to human accounts in active, non-universal channel rooms.
  They do not apply to DMs, bot accounts, the sender, or existing members.
- This version has no sender cancellation, reminder, or notification delivery.

## Design Decisions

### 1. Invitations belong to the room

**Decision:** Show invitations in the room sidebar and welcome screen.
**Why:** The recipient can find the invitation where the room belongs.
**Tradeoff:** A group can appear before the recipient joins any of its rooms.

### 2. Keep invitation history independent of attention

**Decision:** Record creation, acceptance, and decline as durable domain facts,
with the acting account and time. Acceptance and membership change are atomic.
**Why:** Invitation decisions must remain available for auditing and recovery.
**Tradeoff:** Future notifications need a separate connection to invitation state.
Deleting a future notification must not decline an invitation.

### 3. Preserve permission choices

**Decision:** Invitations bypass only the recipient's join permission.
**Why:** An invitation must not override an operator's message or call rules.
**Tradeoff:** A recipient can join a room without permission to read its messages.

## Gates

- `room.invite`: available at server, group, and room scope. The sender also
  needs membership. The permission does not require privileged mode. A temporary
  owner override does not replace the grant because the invitation outlives
  the sending session.
- Fresh servers grant it to the default admin role only. Existing servers need
  an explicit grant. `room.manage` does not include it.

## Related

- **ADRs:** ADR-033, ADR-087, ADR-111, ADR-113
- **FDRs:** FDR-001, FDR-012, FDR-017, FDR-019
- **Issue:** [#1013](https://github.com/chattocorp/chatto/issues/1013)

## Open Questions

Phase 2 adds invitation notifications. Notifications will link to the same
welcome screen and will not own invitation state.
