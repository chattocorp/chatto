# ADR-080: Gate Message Content with `message.read`

**Date:** 2026-08-23

## Context

Chatto uses room membership as the first message-read boundary. ADR-037 makes
membership the complete read boundary for direct messages. Channel-room
message and thread reads previously used membership alone.

Bot accounts expose a weakness in this model. Bots use an explicit permission
list and do not inherit `everyone`, but room membership can still give them
message content. An operator cannot describe or remove that access through the
bot permission matrix.

Chatto also plans an interaction-scoped read mode. A human or bot could then
participate in one channel-room thread without access to every message in the
room. That model still has open life-cycle and inspection questions. It is not
necessary to introduce broad read authority.

## Decision

Add `message.read` for human and bot accounts. It authorizes broad message
access in channel rooms at the applicable server, room-group, or room scope.

Channel-room membership remains necessary. It is no longer sufficient for
channel-room message content. Read and write authority remain separate.
`message.read` does not grant posting, attachment upload, reaction, or
moderation authority.

Keep the ADR-037 membership rule for DMs. A DM participant can read the
complete DM. `message.read` grants and denials do not change DM access. This
rule is the same for human and bot participants.

Apply the permission to channel-room message and thread timelines,
pinned-message reads, search, attachment metadata and bytes, message-derived
notifications, thread-follow state, unread message state, typing indicators,
and realtime message delivery. Room metadata and non-message room events remain
membership-based. DM versions of these surfaces use membership.

Channel-room mutations that read or return existing message state also require
`message.read` in addition to their normal action permission. This includes
editing, reacting, creating a pin whose response hydrates the message, and
changing thread-follow state. Posting and deletion remain independently
authorized and do not expose surrounding message state.

Use the same channel-room permission for humans and bots. A bot needs an
explicit direct grant because bots do not inherit `everyone`. The grant remains
effective only while the bot's owner has `message.read` at the same applicable
scope. A bot in a DM reads it through membership, not through a delegated
permission.

Grant `message.read` to `everyone` at server scope only when Chatto bootstraps
an empty RBAC stream. Do not add a migration or startup reconciliation for an
existing server. Existing RBAC state belongs to the operator, including an
absent decision.

Operators must grant `message.read` to the accounts or roles that need it when
they upgrade an existing server. They must replace all old replicas before
they rely on a deny or absent decision as a security boundary. Old replicas
continue to use membership-only reads.

ADR-082 adds interaction-scoped message access with
`message.read-interactions`. It derives thread relationships from durable
message facts. An effective `message.read` allow includes that narrower
permission.

## Consequences

- Humans and bots use the same broad channel-room message-read permission.
- DM membership remains a sufficient read boundary for human and bot
  participants.
- Fresh servers preserve current human behavior through the `everyone` grant.
- Existing servers receive no automatic RBAC changes.
- Operators can configure read-only and write-only channel-room accounts.
- Bots remain deny-by-default. The bot allowlist and the owner's effective
  authority are each evaluated with explicit permission-catalog inclusion.
- APIs and clients can distinguish room visibility from message-content
  visibility without treating missing permission as an implicit privacy mode.
- The authorization change is breaking experimental behavior under ADR-045.
- Interaction-scoped channel-room access builds on this boundary through
  ADR-082. It does not add a DM access cause.
