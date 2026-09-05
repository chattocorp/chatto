# ADR-091: Direct-Message Permission Scope and Threads

**Date:** 2026-09-04

## Context

DM membership identifies the fixed participants in a private conversation.
Membership alone cannot give operators a safe DM-only bot policy. A bot that
receives a server-level message permission also receives that permission in
channel rooms. The membership-only read rule also prevents an operator from
stopping message-content access without changing the durable participant set.

DMs and channel rooms already use the same message and thread event model.
The old prohibition on new DM threads is now an artificial difference. It also
prevents a DM-only bot from using a structured conversation without channel
access.

## Decision

Add one global `Direct messages` permission scope. It is a sibling of room
group and room scopes. It has no per-DM identity.

- A channel-room check uses Room, Room group, and Server in that order.
- A DM check uses Direct messages and Server in that order.
- All `message.*` permissions apply at the DM scope. No `room.*` permission
  applies at the DM scope.
- Membership remains necessary to discover or use an existing DM. Message
  content and message-derived state also require `message.read` or an allowed
  interaction thread through `message.read-interactions`.
- `message.manage` permits moderation of another participant's message.
  Membership prevents access to DMs where the manager is not a participant.
- `message.post` permits a human to start a DM and permits a participant to
  post a root. Bots cannot call `StartDM`.
- Bots inherit no DM decision. A bot needs an explicit direct-user allow, and
  its owner's current effective decision remains the ceiling.

Treat each DM as an Enabled-threading room. A root needs `message.post`. A
thread reply or explicit thread creation needs `message.post-in-thread`. An
echo to the main room timeline needs `message.echo` and `message.post`. The
public `also_send_to_channel` field keeps its wire name for compatibility.

Persist DM decisions on the singleton `evt.rbac.dm` aggregate lane. Use an
empty scope ID in the public API and `dm` as the permission-matrix column ID.
Full matrices, decision lists, followed-thread lists, and realtime DM thread
projection data use explicit client opt-ins. This prevents an old client from
receiving a new enum or projection shape that it can interpret incorrectly.

The Threads projection derives authored-root and direct-mention relationships
from DM message facts in the same way as channel-room facts. EVT replay remains
authoritative. The RBAC and Threads snapshot contracts use new versions.

This decision supersedes the membership-only read rule, the DM thread
prohibition, and the static DM message-permission deny list in ADR-037. It also
supersedes the channel-only relationship rule in ADR-082. The fixed membership
privacy boundary and the message-fact derivation model remain current.

## Consequences

- Operators can give a bot DM read and write access without channel access.
- A DM participant can know that a conversation exists while current read
  permissions hide its message content and message-derived state.
- Search, attachments, reactions, typing, notifications, push, unread state,
  follows, realtime delivery, and reconnect replay must use the same DM read
  decision as request reads.
- Existing servers receive no new stored decision. Human users inherit their
  current Server decisions until an operator adds a DM override.
- Mixed-version replicas are not supported for this change. Operators must
  replace all replicas before they depend on DM overrides or DM threads.
- Old public clients keep their previous matrix, followed-thread, and realtime
  shapes until they send the relevant opt-in.
