# ADR-071: Bound Application Capabilities by Owner Authority and Resource Context

**Date:** 2026-08-11

## Context

Chatto needs to let external applications perform narrowly approved operations
without turning bot accounts into ordinary users that passively inherit RBAC
access. The same operation vocabulary may later describe OAuth grants, but
first-party user sessions must continue to receive their normal live authority
without requiring an exhaustive scope grant.

A capability grant alone is insufficient. A bot must never retain powers that
its responsible human owner has lost, and data access must remain limited to a
conversation or interaction that explicitly includes the bot.

## Decision

Chatto defines one server-owned, OAuth-scope-style application capability
vocabulary. Capability identifiers describe operation classes, are exposed
with human-readable disclosure metadata, and are suitable for bot grants and
future third-party OAuth grants. They are not a replacement for RBAC and do not
scope first-party client sessions.

Every application operation is authorised by the intersection of three gates:

1. the application has an explicit grant for the required capability;
2. the responsible human owner currently has the corresponding authority; and
3. the target resource has an explicit context relationship with the
   application, such as bot membership in a direct-message conversation or an
   invitation into a thread. Bot-authored messages cannot create invitations,
   including by mentioning the authoring bot itself.

Missing, unknown, or removed grants fail closed. Bot accounts do not receive
roles, direct RBAC decisions, or the `everyone` baseline. Bot API keys are
accepted only by dedicated capability-aware endpoints; the normal user API,
realtime socket, room enumeration, and self-join surfaces remain unavailable.

The initial capability slices define `dm.messages.read`,
`thread.messages.read`, and `messages.write`.
Direct-message reads require bot membership in the DM. Message writes require
an explicitly shared context and the owner's live message-posting authority.
For DM reads, the owner's account-level authority gate is active account
status: Chatto deliberately has no DM-read permission, and existing human DM
members retain read access when posting is denied (ADR-037). The bot's own DM
membership remains the separate resource-context gate.

Channel membership is an installation record, not a read grant. A bot must be
explicitly installed even in a Universal room. A direct mention in a message
then grants access only to that message's thread root and replies; role and
broadcast mention expansion cannot create this context. Leaving the room
revokes all of the bot's thread contexts there. The write-only incoming webhook
uses installation plus `messages.write` and the owner's current room posting
authority and membership, but has no corresponding channel-read operation.
Thread reads and writes likewise require the owner to remain an effective room
member.

Privacy-bearing bot reads capture the room and global authorization boundaries,
wait for the room-directory, thread, group-layout, RBAC, and account projections,
retain that boundary through protected response assembly, and discard and retry
the response if either boundary advances. Mutations authorised through live
RBAC, such as room-manager revocation of thread access, commit the domain event
and authorization-fence advance atomically.

## Consequences

Bot profiles can accurately disclose the maximum operation classes approved by
the server, while runtime checks remain stricter than that disclosure. Revoking
a grant or reducing owner authority takes effect without editing bot RBAC.

Operation models need dedicated bot paths and must recheck all three gates at
their consistency boundary. Capability-aware integrations cannot reuse the
ordinary user API wholesale, which adds API surface but prevents accidental
authority expansion.

Future OAuth work can reuse the vocabulary and disclosure metadata while
choosing a different grant lifecycle. Some capabilities may need to be split as
new contexts emerge; pre-1.0 identifiers therefore remain experimental.
