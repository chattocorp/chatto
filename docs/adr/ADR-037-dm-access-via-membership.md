# ADR-037: DM Access via Membership, Not a Read Permission

**Date:** 2026-05-31

## Context

Direct messages used to carry two server-scope permissions:

- `dm.view` — access and read DMs.
- `dm.write` — start DMs and send messages.

That split made sense when DMs still had traces of the old hidden-space model: the system needed an answer to "can this user access DMs?" Now DMs are rooms with `kind: dm`, membership is an event-sourced room fact, and room membership is already the privacy boundary for live delivery and reads.

`dm.view` no longer describes a useful operator action. If a user is a participant in a private conversation, hiding that conversation from them is surprising and not a meaningful abuse-control tool. `dm.write` also became awkward once it was the only remaining `dm.*` permission: Chatto already has `message.post` for "may send messages", and keeping a separate DM send gate makes DMs look more special than they are.

## Decision

Remove both DM-specific permission strings as product and authorization concepts.

- Reading a DM is allowed by room membership alone.
- Listing DMs returns the DM rooms the caller participates in.
- Live DM events are filtered by room membership, the same as channel-room events.
- Starting a DM and sending root messages in DM rooms are gated by `message.post`.
- Thread replies in DM rooms are gated by `message.post-in-thread`.
- The DM privacy boundary remains: permissions such as `message.manage`, `room.manage`, `message.echo`, and channel-style `room.create` are denied inside DM rooms regardless of role grants.

This decision does not make DMs globally visible. It removes the redundant read gate; the participant set remains the access boundary.

## Consequences

- Operators can still stop DM abuse by revoking `message.post`, suspending the user, or removing the account.
- Users do not lose read access to conversations they are already part of because an operator toggled a broad server permission.
- The authorization model becomes easier to explain: membership answers "can read this room?", while `message.*` permissions answer "can perform this messaging capability?"
- Subscription filtering and sidebar queries no longer need a second DM-specific read check on top of membership.
- GraphQL fields, frontend guards, tests, and permission seed data that existed only for `dm.view` / `dm.write` have been removed.
