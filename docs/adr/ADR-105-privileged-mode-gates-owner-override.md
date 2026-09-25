# ADR-105: Gate the Effective-Owner Override with Privileged Mode

**Date:** 2026-09-25

**Status:** Accepted

## Context

ADR-040 gives effective owners every known permission. ADR-096 then made
elevation-required permissions effective only in privileged mode. The owner
override itself stayed active. Thus an owner without privileged mode could
still see, join, and read every channel room, including rooms that deny these
permissions to `everyone`. These actions were not audited and did not need an
explicit activation.

Privileged mode keeps assigned authority off during ordinary chat use. Access
to rooms that the RBAC settings restrict is such authority.

## Decision

The effective-owner override is effective only where privileged mode is
active for the owner. Owners are still entitled to every permission.

Effective authorization for a human applies these rules:

- The authenticated owner with active privileged mode has the override.
- The authenticated owner without active privileged mode resolves through
  direct grants, named roles, and the `everyone` baseline like every other
  human. The `owner` role has no stored decisions, so it adds nothing.
- A request that checks a different human uses that human's inactive state.
  This rule already applied to elevation-required permissions. It also
  applies to bot API key requests.
- Internal work without a runtime credential keeps entitlement semantics.
  Bot API keys do not use privileged mode.

Every transport that authorizes a human must put the runtime credential in the
request context. Otherwise, core authorization applies internal-work
semantics. MCP tool calls and cookie or bearer asset reads carry the
credential.

Entitlement resolution keeps the override. Thus these functions do not
change: privileged-mode availability, bot owner ceilings, delegated role
assignment, and bot permission explanations.

Delivery paths that expose content outside a request use the state of the
receiving session:

- `MyEventsHub` keys shared room-visibility state by user and
  privileged-mode state. Sessions of one user with different states do not
  share visibility. Each fan-out decision uses the fixed state of its sessions,
  not the credential of the first subscriber and not an internal context.
- A privileged realtime connection does not write an event after its
  privilege deadline. The existing deadline close then reconnects the session.
  The periodic credential check also reconnects a privileged socket when
  another connection of the same session ends privileged mode.
- Notification decisions and alert revalidation are not bound to one session.
  They use the unprivileged view for owners.
- A call connection outlives its token request. The token's participant
  metadata stores the privileged-mode deadline of the requesting session.
  Call reconciliation evaluates the participant with that state, so the
  next reconciliation scan after the deadline applies ordinary RBAC. Other
  call participants can read this deadline.
- A signed asset access ticket is a bounded capability. Its read check does
  not have a runtime credential and keeps entitlement semantics until the
  ticket expires. Chatto issues a ticket only after an authorized read.

The permission explainer shows the owner override only when the override
applies to the inspected user in the request. Otherwise it explains the
ordinary decisions, which agree with effective authorization.

## Compatibility

This decision changes authorization behavior in Chatto 0.5. It does not change
protobuf messages, persisted events, or runtime-state records.

- An owner without privileged mode loses access to rooms that only the
  override made available. Explicit memberships stay, but read access follows
  RBAC.
- On a fresh server, the first owner has only the `owner` role. The
  announcements room denies `message.post` to `everyone` at room scope, so
  this owner must activate privileged mode to post root messages there. An
  operator can also give the owner the `admin` role or a direct room allow.
- MCP tools cannot activate privileged mode. An owner's MCP client therefore
  acts without the owner override.
- All replicas must run the new authorization code before an operator relies
  on the gate. An older replica still applies the override.

## Consequences

- Access to restricted rooms by an owner requires an explicit, audited
  activation.
- An owner's room list changes when privileged mode starts or ends. The client
  already reads rooms and room groups again after each change and expiry.
- An owner can be a room member without read access after privileged mode
  ends.
- The realtime hub can keep two visibility states for one owner.
- An asset URL that Chatto issued during privileged mode stays usable until
  its ticket expires.
- Owners and administrators must configure ordinary access for owners through
  roles and grants, as for other users.

## Related

- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md)
- [ADR-096](ADR-096-session-scoped-privileged-mode.md)
- [FDR-001](../fdr/FDR-001-roles-and-permissions.md)
- [FDR-046](../fdr/FDR-046-privileged-mode.md)
