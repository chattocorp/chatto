# ADR-037: DM Access via Membership, Not a Read Permission

**Date:** 2026-05-31

## Context

Direct messages used to carry two server-scope permissions:

- `dm.view` — access and read DMs.
- `dm.write` — start DMs and send messages.

That split made sense when DMs still had traces of the old hidden-space model: the system needed an answer to "can this user enter the DM space?" Now DMs are rooms with `kind: dm`, membership is an event-sourced room fact, and room membership is already the privacy boundary for live delivery and reads.

`dm.view` no longer describes a useful operator action. If a user is a participant in a private conversation, hiding that conversation from them is surprising and not a meaningful abuse-control tool. The real administrative need is to stop an abusive user from starting or continuing DM conversations.

## Decision

Remove `dm.view` as a product and authorization concept.

- Reading a DM is allowed by room membership alone.
- Listing DMs returns the DM rooms the caller participates in.
- Live DM events are filtered by room membership, the same as channel-room events.
- `dm.write` remains as the server-scope permission for starting DMs and sending messages in DM rooms.
- The DM privacy boundary remains: permissions such as `message.manage`, `room.manage`, `message.echo`, and channel-style `room.create` are denied inside DM rooms regardless of role grants.

This decision does not make DMs globally visible. It removes the redundant read gate; the participant set remains the access boundary.

## Consequences

- Operators can still stop DM abuse by revoking `dm.write`, suspending the user, or removing the account.
- Users do not lose read access to conversations they are already part of because an operator toggled a broad server permission.
- The authorization model becomes easier to explain: membership answers "can read this room?", while permissions answer "can perform this capability?"
- Subscription filtering and sidebar queries no longer need a second DM-specific read check on top of membership.
- GraphQL fields, frontend guards, tests, and permission seed data that existed only for `dm.view` have been removed.
