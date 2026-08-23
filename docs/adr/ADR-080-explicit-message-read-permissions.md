# ADR-080: Gate Message Content with `message.read`

**Date:** 2026-08-23

## Context

Chatto uses room membership as the message-read boundary. ADR-037 applies this
rule to direct messages. Channel-room message and thread reads use the same
basic rule.

Bot accounts expose a weakness in this model. Bots use an explicit permission
list and do not inherit `everyone`, but room membership can still give them
message content. An operator cannot describe or remove that access through the
bot permission matrix.

Chatto also plans an interaction-scoped read mode. A human or bot could then
participate in one thread or direct message without access to every message in
the room. That model still has open life-cycle and inspection questions. It is
not necessary to introduce broad read authority.

## Decision

Add `message.read` for human and bot accounts. It authorizes broad message
access at the applicable server, room-group, or room scope.

Room membership remains necessary. It is no longer sufficient for message
content. Read and write authority remain separate. `message.read` does not
grant posting, attachment upload, reaction, or moderation authority.

Apply the permission to message and thread timelines, pinned-message reads,
search, attachment metadata and bytes, message-derived notifications,
thread-follow state, unread message state, typing indicators, and
realtime message delivery. Room metadata and non-message room events remain
membership-based.

Mutations that read or return existing message state also require
`message.read` in addition to their normal action permission. This includes
editing, reacting, creating a pin whose response hydrates the message, and
changing thread-follow state. Posting and deletion remain independently
authorized and do not expose surrounding message state.

Use the same permission for humans and bots. A bot needs an explicit direct
grant because bots do not inherit `everyone`. The grant remains effective only
while the bot's owner has `message.read` at the same applicable scope.

Grant `message.read` to `everyone` at server scope only when Chatto bootstraps
an empty RBAC stream. Do not add a migration or startup reconciliation for an
existing server. Existing RBAC state belongs to the operator, including an
absent decision.

Operators must grant `message.read` to the accounts or roles that need it when
they upgrade an existing server. They must replace all old replicas before
they rely on a deny or absent decision as a security boundary. Old replicas
continue to use membership-only reads.

This decision partially supersedes ADR-037. DM membership remains a required
privacy boundary. ADR-080 replaces the rule that membership alone authorizes a
DM read. ADR-037 still governs DM identity, membership, posting, thread
prohibition, and the DM moderation deny-list.

Interaction-scoped message access remains a later feature slice. This ADR does
not select its permission name, access causes, durable representation,
revocation rules, inspection UI, or reconnect behavior.

## Consequences

- Humans and bots use the same broad message-read permission.
- Fresh servers preserve current human behavior through the `everyone` grant.
- Existing servers receive no automatic RBAC changes.
- Operators can configure read-only and write-only accounts.
- Bots remain deny-by-default, and bot-owner permission intersection stays
  exact.
- APIs and clients can distinguish room visibility from message-content
  visibility without treating missing permission as an implicit privacy mode.
- The authorization change is breaking experimental behavior under ADR-045.
- Interaction-scoped access can build on this boundary after its security
  model is complete.
