# ADR-115: Role Hierarchy for Administration

**Date:** 2026-10-07

## Status

Accepted. Partially supersedes [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
and amends [ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md):
role position is now an administrative rank. Permission resolution does not
change.

## Context

ADR-040 removed role rank from permission resolution and from targeted
operations. Every role and every account became equal for administration:
a permission alone decided whether an actor could act on another account or
role. Two problems followed:

- **Escalation.** A holder of `role.manage` or `user.manage-permissions` could
  grant every permission, also to themselves. This made the bounded
  `role.assign` rule ineffective.
- **No target protection.** An actor with `user.manage-accounts`,
  `user.delete-any`, `room.remove-member`, or `user.manage-permissions` could
  act on any account. A help-desk role could set an owner's password, and an
  admin could delete an owner or another admin.

A rule "you can grant only what you have" stops escalation. It does not
protect targets, and it does not stop an actor who gives their authority to a
second account. Chat platforms such as Discord, Matrix, and Zulip solve
delegated administration with an order of roles or power levels. Features
that show a user's highest role also need such an order.

## Decision

Use role order as an administrative rank. Keep permission resolution
unchanged (ADR-052): the rank never decides whether a permission is allowed.

- **One order.** `owner` is fixed at the top and `everyone` at the bottom.
  `admin`, `moderator`, and custom roles share one order that role managers
  can change. A new role starts lowest.
- **Rank.** An account ranks at the position of its highest role. An account
  without roles ranks with `everyone`. Direct user permission decisions do not
  affect rank. Effective owners rank above every role and are exempt from the
  hierarchy, so an owner can always recover the server.
- **Accounts.** A non-owner may act on another account only when they rank
  strictly above it. This applies to password changes, profile and avatar
  edits, login cooldown resets, account deletion, role assignment and
  revocation, direct permission edits, room removal, suspension lifts,
  membership removal, and bot management and reassignment. Message moderation
  and adding a member to a room are not covered: they do not act against the
  account.
- **Roles.** A non-owner may assign or revoke only roles that rank strictly
  below their own highest role. A holder of `role.manage` may edit, delete,
  and move every role except `owner`, which stays owner-only. Other actors
  who edit a role's decisions, such as room managers, may edit only roles
  below their own highest role. `everyone` ranks below every account, so
  room managers can edit its decisions.
- **Authority bound.** A non-owner may change one role or direct decision only
  for a permission that they effectively hold at that scope. Assigning,
  revoking, or deleting a role requires every permission that the role
  allows. Roles cannot deny (ADR-116).
- **Bots.** Bots cannot hold roles. A bot ranks like its owner, both when it
  acts and when someone acts on it. Managing a bot therefore requires
  outranking its owner, unless the actor owns the bot.
- **Moves, not complete orders.** `MoveRole` and `RbacRoleMovedEvent` place
  one role directly above another role, or lowest. This matches the room
  layout's `before` moves. A role created with `place_lowest` starts lowest.
  For both, readers renumber every role except `owner` and `everyone` upward
  from 1 and place `owner` directly above them, so the order has no fixed
  limit. Older `RbacRolesReorderedEvent` events list custom roles only and
  replay with the previous fixed system positions.
- **Move rules.** A holder of `role.manage` can move every role except
  `owner` and `everyone`, also above their own highest role. `owner` and
  `everyone` cannot move or serve as the anchor. `role.manage` is an
  administrator permission: a holder can give themselves a higher rank.
- **Public API.** Clients get the role order from role lists, which list
  roles highest first. `ListRolesResponse.viewer_highest_role` names the role
  at which the caller ranks (`owner` for owners). Clients compare the two to
  show which actions can succeed. Role positions are internal and not part
  of the public API.

Rank checks run inside the command's OCC retry with the stable authorization
inputs that permission checks use (ADR-087).

## Consequences

- A delegated account, permission, or role-assignment manager can no longer
  give themselves more authority or act on accounts at or above their rank.
  A holder of `role.manage` can change every role except `owner` and move
  their own role higher, so give `role.manage` only to administrators.
- Two accounts with the same highest role cannot act on each other. Only an
  owner can act on an account whose highest role is the top role, which is
  `admin` by default.
- Rank comes only from roles. A user with administrative direct permissions
  but no matching role has a low rank, can act on few accounts, and has little
  protection. Give administrators a role, not direct permissions.
- A holder of `role.manage` can restrict higher-ranked accounts through
  `everyone`, within the permissions that they hold, and a room manager can
  do so in their rooms. Roles only grant (ADR-116), so no other role can
  restrict anyone. Give role management only to trusted roles.
- An actor can still give their authority to a second account that ranks
  below them. The event log records the actor of every change, so an operator
  can find and undo such grants.
- A move names only two roles. A role that another manager creates or
  deletes at the same time does not invalidate it; the OCC retry applies it
  to the new order. Each event stays small, also on servers with many roles.
- During a rolling upgrade, older replicas apply the earlier rules and show
  the earlier display order. They list roles lowest first and do not send
  `viewer_highest_role`. They ignore `RbacRoleMovedEvent` and
  `place_lowest`. A legacy reorder or role creation from an older replica
  after a move can give two roles the same position. Equal positions rank
  equally until the next move or role creation, which orders them by name. The RBAC projection snapshot contract
  changes, so old and new replicas never share snapshots.
- `everyone` stays at position 0 and `owner` stays highest. `admin` and
  `moderator` lose their fixed positions at the first move or role creation.
