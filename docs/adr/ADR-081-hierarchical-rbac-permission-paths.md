# ADR-081: Hierarchical RBAC Permission Paths

**Date:** 2026-08-26

## Context

Chatto permissions use names such as `message.post`. The previous catalog and
resolver treated each name as an object and a verb. This prevents an operator
from granting a registered permission family without listing every child.

The change must preserve existing RBAC events. An existing grant, denial, or
clear is a decision for its exact registered name.

## Decision

Register permissions as dot-separated paths. Every registered descendant must
also have every registered ancestor.

An allow for a path applies to that path and to its registered descendants. A
denial applies only to its exact path. It does not deny a descendant.

Resolve the server, room-group, and room scope for each registered path before
the resolver uses an ancestor allow. Existing named-subject, `everyone`,
direct-user, deny-wins, bot-owner ceiling, and delegated role-assignment rules
apply independently at each path.

Permission matrices and explanations show the effective result. An explanation
trace identifies the registered path that supplied its decision.

Do not add wildcard permissions or a general permission-expression language.
Do not migrate, backfill, or reconcile existing RBAC state.

## Consequences

Operators can grant a registered parent path, such as `message`, to grant its
registered message permissions. An exact child denial can still block one
child. A parent denial does not block child permissions.

The catalog now includes parent paths. New paths must include their ancestors
in the same catalog change. Existing RBAC events replay without conversion.

## Related

- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md)
- [FDR-001](../fdr/FDR-001-roles-and-permissions.md)
