# FDR-047: First-Run Setup

**Status:** Active
**Last reviewed:** 2026-09-10

## Summary

A new server opens a web setup wizard before normal registration. The installer
chooses a server name, an optional description, and a local owner account with
a username, display name, and password. Email delivery is not required. Existing
default rooms remain in place. After setup, the installer signs in normally.

## Decisions

### Setup is enabled by default

The first visitor can complete setup without a session or setup secret. The
operator must control access to a new deployment until setup is complete.
`core.skip_setup_wizard = true` suppresses the wizard for automated installations.
It does not change durable initialization state. The same value must be used on
all replicas. Direct password login must be enabled to create the local owner.

### Completion is permanent and atomic

The initial account, owner role, server settings, and completion record commit
together. Concurrent submissions cannot create two initial owners. A lost
response does not reopen setup: the installer can use normal sign-in. Invalid
input leaves setup available. The wizard keeps a failed draft in memory only.

### Existing servers remain closed

Only an empty event history can become eligible for setup. Existing servers
receive a durable completion record on upgrade. Deleting users, restarting,
restoring snapshots, or changing the opt-out flag does not reopen completed
setup. Operator-created user history also prevents first-run ownership claims.
Development bootstrap records completion after it creates accounts; it remains
unavailable in release builds.

### Setup belongs to the origin server

The wizard collects server details and the owner account in one form. Password
confirmation must match before submission. Account validation errors appear
below the affected field; connection and setup-state errors apply to the form. A short
Chatto logo reveal welcomes the operator and respects reduced-motion settings.
The wizard uses the normal app header, frame, and server gutter. Selecting an
uninitialized origin server opens setup instead of sign-in. Users can still add
other servers and use their accounts on those servers. Setup does not block
client-wide navigation or require an account on the origin server.

### Keep the wizard small

SMTP, external login providers, branding, and room layout remain separate
configuration or management tasks. The initial account receives the owner role
because it must have full server control.

## Related records

- [FDR-001: Roles and Permissions](FDR-001-roles-and-permissions.md)
- [FDR-028: Operator API and CLI](FDR-028-operator-api-and-cli.md)
- [ADR-068: Selectable Event Mutation Consistency Boundaries](../adr/ADR-068-selectable-event-mutation-consistency-boundaries.md)
