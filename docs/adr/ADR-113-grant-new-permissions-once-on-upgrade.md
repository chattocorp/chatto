# ADR-113: Grant New Permissions Once on Upgrade

**Date:** 2026-09-28

## Status

Accepted. Amends [ADR-080](ADR-080-explicit-message-read-permissions.md).

## Context

Chatto applies its default permissions only when it bootstraps an empty RBAC
stream (FDR-001). An absent decision is operator state, so startup does not
reconcile existing servers with the current defaults.

Version 0.5 adds permissions that gate capabilities that 0.4 users already
had. ADR-080 gates channel messages with `message.read` and does not grant it
on upgrade. A 0.4 server that upgrades without manual grants has members who
cannot read messages. `room.remove-member` replaces `room.ban-member`, so
moderators lose room moderation. Admins lose invite links and bot management.
The 0.5 release notes listed these grants as manual steps. Operators who skip
the list get a broken server.

Call permissions already use a one-time upgrade grant. Each missing
server-level `everyone` call permission is initialized once. Any historical
grant, deny, or clear prevents it.

## Decision

When a release adds a permission that gates an existing capability, add a
one-time upgrade grant that keeps the previous behavior. Write the grant as
ordinary RBAC permission facts at startup, after default bootstrap. Guard the
batch with optimistic concurrency on the complete RBAC subject tail. Read the
complete RBAC history to make the decision, not the live projection.

Each upgrade grant has a gate that detects operator intent from history:

- **Per permission.** The grant applies only if the target permission never
  had a decision for that subject and scope. Calls use this gate. It is
  correct when every server lacks the permission before the upgrade.
- **Per release set.** The grants apply only if no permission in the set ever
  had a decision, at any scope and for any subject. The 0.5 set uses this gate.
  A decision for one permission in the set shows that 0.5 bootstrapped the log
  or that an operator reviewed its permissions. Chatto then leaves the whole
  set to the operator.

The 0.5 set contains `server.manage-neighbors`, `room.remove-member`,
`message.read`, `message.read-interactions`, `message.post-in-interactions`,
`user.invite`, `bot.create`, and `bot.manage`. When its gate is open, Chatto
grants at server scope:

- `message.read` and `bot.create` to `everyone`;
- `user.invite` and `bot.manage` to `admin`.

It also copies each current `room.ban-member` decision to `room.remove-member`
with the same scope, subject, and allow or deny. The copy replays role
deletion, clears, and legacy event shapes, so it uses the same current state as
the RBAC projection.

Do not use an upgrade grant for a change of meaning. `room.manage` for
delegated room-group managers, the admin allow in the `announcements` room,
thread-posting inclusion, and `everyone`-deny precedence remain manual review
items.

## Consequences

- A 0.4 server keeps message reads, room moderation, invites, and bot
  management after the upgrade without manual steps.
- Members of an upgraded server can create bots, as on a fresh 0.5 server.
  Operators who do not want this must remove the grant.
- A cleared or denied decision never returns after a restart, because each gate
  reads historical decisions.
- An operator who reviewed some 0.5 permissions on a pre-release keeps that
  state. Chatto does not add the rest of the set.
- A server that runs an upgrade grant writes one extra RBAC batch on first
  startup. Later startups stop the scan early when every gate is closed.
- Future permissions that gate an existing capability need an upgrade grant
  and a test for the pre-upgrade log.
