---
name: chatto-rbac-deprecation
description: Retire, rename, split, or replace Chatto RBAC permissions, including upgrade handling for stored decisions.
---

# Chatto RBAC Deprecation

Follow [ADR-113](../../../docs/adr/ADR-113-grant-new-permissions-once-on-upgrade.md)
for upgrade grants. Pre-release permissions can exist in real server logs.

- **Define the transition.** Separate fresh-server defaults from upgrades.
  Do not automatically transfer grants when meaning, scope, or privileged-mode
  requirements change. Never reuse an identifier for a different capability.
  State what happens to existing resources and ownership.
- **Preserve operator intent.** Keep grants, denials, and clears intact unless
  the transition explicitly migrates them. Retain event history and any upgrade
  detection that depends on it. Migrations must respect destination decisions
  and remain safe across retries, restarts, and concurrent startup.
- **Handle stored decisions.** Catalog removal does not delete them. Choose
  inert retention or cleanup through RBAC events; check whether mutation APIs
  permit cleanup. Inspect role and direct-user decisions at historical scopes.
- **Keep delegation working.** Known retired permissions grant no authority and
  must not block otherwise authorized role assignment or revocation. Ignore
  only explicitly retired identifiers, never arbitrary unknown permissions.
- **Test upgrades.** Use historical events with allows, denials, and clears.
  Cover delegation and continued enforcement of active and unknown permissions.
  For migrations, also cover destination decisions, replay, and repeat or
  concurrent startup.
- **Finish the removal.** Remove obsolete catalog entries, UI uses, and
  translations. Update the feature FDR and required operator instructions;
  check API compatibility when exposed behavior changes.
