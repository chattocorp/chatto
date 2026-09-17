# ADR-099: Store Partial Message Body Updates

**Date:** 2026-09-17

**Status:** Accepted

## Context

Each message edit previously replaced the complete encrypted body. An edit to
text also decrypted and encrypted all attachment descriptions. A writer that
did not understand a new body field could omit it from the replacement.

Partial updates must preserve encryption context and secure deletion. Erasing
an old payload must not erase the instruction that cleared a field. Otherwise,
cold replay could restore an old value from a payload kept for another field.
ADR-090's single current body reference cannot represent these dependencies.

## Decision

Posts and edits use the existing private `MessageBodyEvent` and `message_body`
subject. Edits append a masked body immediately followed by `MessageEditedEvent`
in one room-OCC atomic batch. Authorization and mutation run again after a
conflict, as required by ADR-087.

The body event has a `google.protobuf.FieldMask update_mask`, with paths relative
to `MessageBody`. An absent mask means a complete replacement, as used by
historical events and new posts. A present empty mask changes no fields. A
selected field replaces its value, including clearing it with an empty value.
Unselected fields keep their values. Unknown paths stop replay.

The text ciphertext, nonce, encryption version, and key epoch must be selected
together. The legacy attachments and current asset IDs must also be selected
together. Link preview and attachment descriptions are separate paths.
Repeated fields are replaced as complete lists. A description-list update copies
unchanged ciphertext with its original source event ID and encrypts only changed
descriptions. It does not use per-asset mask paths or a separate patch event.

The projection derives the latest source for each of four field groups: text,
attachments, preview, and descriptions. Complete bodies replace all four sources.
Masked bodies replace only selected sources. An empty value retains its source
event so replay cannot restore a value from an older retained payload.
`MessageEditedEvent` remains a bodyless semantic signal. No source references or
sequence conventions enter the stored event protocol.

The current body can use several source events. Text keeps its original body
encryption context. Each hydrated description identifies its own source body
event. Only changed text or descriptions require new ciphertext. Key epochs
can differ across current fields.

Room Timeline retains four source positions and payload sequence history, not
content. Hydration batches distinct source reads directly.
Existing read-plan checks detect concurrent edits, retractions, and shredding.
Room files and echoes use the same canonical-body path.

Secure deletion removes only payloads with no current field references. The
latest body event also stays as the source of edit timestamps. Retraction makes
all of a message's payloads obsolete. The existing boot cleanup repeats failed
or interrupted erasure. Unknown masks and body fields stop replay before a
reader makes incomplete retention decisions.

Writers repeat explicit clears for fields that are already empty. This moves
their sources to the new payload and permits cleanup of old payloads. It does
not change the visible value. Clear events remain necessary even when deletion
of an older payload failed; cleanup order must not affect replay.

The search projection applies text changes without clearing text on metadata
updates. Body masks update its current revision and attachment filter,
including on replay after obsolete payload erasure. Its checkpoint contract
changes, as does the Room Timeline snapshot contract.

## Consequences

- A text edit preserves attachment-description ciphertext. A description edit
  preserves text ciphertext. New fields can evolve without forcing every edit
  to write a complete replacement, but still need compatibility review.
- Reads can fetch several payloads. Work depends on current fields, not the
  number of historical edits. Source tracking stays in the projection and its
  disposable snapshot.
- Values grouped in one payload share an erasure boundary. A removed value's
  ciphertext can remain while another current field still uses that payload.
  A clear also keeps its payload until a later clear or value replaces it.
  Old values are not returned by current reads. Message deletion removes all payloads.
  This also applies to complete bodies written before this change.
- All server and search-provider readers must be upgraded before patch writers
  run. Complete-body writers and old cleanup implementations cannot safely
  coexist with patch writers. Rollback after patch writes is unsupported.
  The coordinated 0.4-to-0.5 server replacement requirement remains in force.
- Old complete-body EVT remains readable. Old projection snapshots are ignored.
  The existing search-provider checkpoint policy requires an operator to move
  or remove its disposable old index before rebuilding it.
- Public message-edit and realtime payloads are unchanged. Internal field-source
  metadata does not enter the public realtime union.

Related: [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md),
[ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md),
[ADR-090](ADR-090-hydrate-room-timeline-payloads-from-evt.md).
