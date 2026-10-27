# FDR-054: Room Notices

**Status:** Experimental
**Last reviewed:** 2026-10-25

## Overview

A channel room can publish one Markdown notice for its current members. It
gives important context near the room header and remains separate from messages,
pins, and the room description.

## Behavior

- Notices are room-owned information, like room descriptions. Event records
  retain their plaintext bodies, outside the per-user encrypted message-body
  lifecycle. Projection snapshots include the bodies under the normal snapshot
  encryption described in ADR-050. Deleting an author's account does
  not remove notice text. **Clear** removes the current notice, but does not
  erase earlier text from the event log. Account deletion must not silently
  remove room guidance that other members may have edited.

- A published notice appears near the room header. Members can dismiss it on
  the current device. Dismissal removes the banner without a replacement control
  or reserved space. Room information still shows the current notice.
- **Edit notice** opens the standard form dialog. On narrow touch screens,
  it uses the standard bottom sheet. **Update** and **Announce** are disabled
  until the editor changes the text.
  The field uses the viewer's preferred visual or Markdown composer, with
  formatting controls. Switching composers preserves the current draft.
  Typing `@` opens the shared member selector in either composer. Published
  notices and Room information render mentions with member display names and
  highlight mentions of the viewer, as chat messages do. Notice mentions do not
  create message notifications.
- **Update** changes the text without making a dismissed notice appear again.
  **Announce** publishes the text and makes it appear again. The empty editor
  also uses **Announce**. Publishing after a clear makes the notice appear again.
- **Clear** requires confirmation. The confirmation explains that this action
  immediately removes the active notice for all room members and does not
  publish unsaved edits.
- Dismissals belong to one viewer, server, room, and publication. They do not
  sync across devices. Device storage contains only the dismissal identifier.
- Current channel members can read notices, including members without message
  read permission. Non-members cannot read or manage notices. The room directory
  and join preview do not expose the notice text. DMs do not support notices.
- Members with `room.manage-notice` can publish, correct, republish, and clear
  the notice. This permission applies at server, room-group, and room scopes.
  New instances grant it only to the bootstrap admin role. Existing instances
  keep their permission grants. The normal effective-owner override applies.
  Explicit grants do not require privileged mode.
- Archived rooms retain readable notices but reject changes.
- Notices show their publication date and time in the viewer's time format
  and timezone. Corrections preserve this time; explicit republishing updates it.
- Notice source is limited to 4,000 Unicode code points. A publication cannot
  be blank. The current Markdown renderer handles the content; source HTML and
  image syntax do not load external content. Notice operations contact only
  the selected Chatto server, which receives the viewer's IP address and normal
  authentication data. Links contact their destinations only when opened.
- Concurrent editors cannot silently replace each other's changes. A stale
  save or clear returns a conflict. The editor keeps the draft and offers to
  load the current notice before another save.
- Notice changes arrive in real time and reconcile after reconnect. Loss of
  room membership clears the notice and invalidates pending reads and drafts.
- Servers without the notice API keep their normal room behavior; the client
  hides the optional notice controls.

## Design Decisions

### 1. Separate write versions from dismissal revisions

**Decision:** Every content change has a write version. Only publication and
explicit republishing create a dismissal revision.
**Why:** Editors can fix a typo without asking all members to read the notice
again. A stale editor still conflicts with that correction.
**Tradeoff:** Editors choose between correction and republish.

### 2. Keep dismissal on the device

**Decision:** Each device remembers its own dismissal, like pin-seen markers.
**Why:** Dismissal is a display preference, not a shared read receipt.
**Tradeoff:** A member may dismiss the same notice on several devices.

### 3. Separate notice management from room administration

**Decision:** Use `room.manage-notice`, denied by default, with membership
required for both reading and management. Only the admin role receives a grant
when a new instance is initialized.
**Why:** Operators can delegate notice editing without permitting room changes
or exposing content to non-members.
**Tradeoff:** Operators on existing instances must grant the new permission explicitly.

### 4. Keep only current notice state

**Decision:** Use existing event history for audit and recovery. Do not add a
second message history. Clearing content retains a write version. Each change
stores the Markdown body and whether it announces the notice. The projection
derives the write version from the event ID. An announcement also supplies the
publication ID and time from its event envelope; corrections retain them.
**Why:** A notice describes current room context. Retaining its version prevents
an old editor from replacing a newer notice after a clear.
**Tradeoff:** There is no notice-history browser or restore-previous button.

## Related

- [FDR-019: Room Lifecycle](FDR-019-room-lifecycle.md)
- [FDR-032: Message Formatting](FDR-032-message-formatting.md)
- [FDR-037: Pinned Messages](FDR-037-pinned-messages.md)
- [Issue #3024](https://github.com/chattocorp/chatto/issues/3024)
