# ADR-091: Require Session-Scoped Privileged Mode

**Date:** 2026-09-03

**Status:** Accepted

## Context

A user can have ordinary permissions and administrative permissions at the
same time. Before this decision, all assigned permissions were always active.
The bundled client therefore showed edit, delete, and administration actions
during ordinary chat use. A user could select one of these actions by mistake.

Roles are reusable permission containers. A custom role or a direct user grant
can give the same authority as a system role. A solution that activates role
names would not cover these cases.

Chatto has one server session for each connected server. A global client state
cannot safely represent authority on several independent servers.

## Decision

The permission catalog identifies permissions that require privileged mode.
Chatto requires an authenticated human to activate privileged mode before
these assigned permissions become effective. Roles and direct grants continue
to define entitlement. Activation does not change RBAC state.

One activation applies to all elevation-required permissions that the user is
currently entitled to use on that server. It does not grant a new permission.
A later role removal or deny takes effect during an active window.

The first activation flow uses an explicit user confirmation. An activation
has a fixed 15-minute deadline. Permission use does not extend the deadline.
The user can deactivate it at any time.

Activation belongs to one runtime credential session:

- A same-origin cookie session stores the deadline in its mutable
  `session.{hmac}` record.
- A bearer login stores the deadline in its stable
  `renewable_session.{hmac}` record. Access-token rotation reads the current
  deadline from that record.
- Logout, session revocation, expiry, or deletion removes the activation with
  the session.
- Bot API keys do not require and cannot activate privileged mode. The direct
  bot permission and owner-ceiling rules do not change.

The permission resolver keeps two concepts separate. Entitlement resolution
describes assigned authority for administration, delegation ceilings, and UI
discovery. Effective authorization applies the privileged-mode gate when the
checked user is the human authenticated by the current request. Internal work
without a public runtime credential keeps entitlement semantics.

`ViewerService` reports whether privileged mode is available, whether it is
active, and its deadline. It also provides activate and deactivate RPCs. The
effective permission grants in viewer and room state describe permissions that
can be used now. The bundled client puts the control in the current-user area
for the selected server. It reconnects its realtime projection after a state
change so all effective room and server permissions use the new session state.

Privileged-mode state is runtime state. Chatto does not write an EVT event for
activation or deactivation. Privileged operations continue to write their
normal durable audit facts.

## Compatibility

The protobuf additions use new fields and RPCs. An older server ignores the
new client feature because its viewer response cannot report availability. An
older client on a newer server cannot activate elevated permissions. This is
an intentional Chatto 0.5 authorization behavior change.

A deployment must update all replicas before it relies on this boundary. An
old replica does not apply the new request-time gate.

## Consequences

- Ordinary chat use does not expose assigned administrative authority.
- The rule follows permissions, including custom roles and direct grants.
- Session revocation also revokes active elevated authority.
- The runtime-state records have one more mutable deadline.
- Clients must refresh effective permission state after activation,
  deactivation, or expiry.
- A future MFA step can be added to the activation RPC without changing the
  RBAC model.

## Related

- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md)
- [ADR-079](ADR-079-renewable-bearer-sessions.md)
- [ADR-081](ADR-081-explicit-expiry-for-mutable-runtime-credentials.md)
- [ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md)
- [FDR-045](../fdr/FDR-045-privileged-mode.md)
