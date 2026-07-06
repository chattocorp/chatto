# ADR-005: Hierarchy-Wins RBAC Permission Resolution

**Date:** 2026-03-01

**Status:** Superseded by [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md).

## Context

Chatto needs a permission system that supports per-room overrides (e.g., an announcements channel where only moderators can post) without requiring special-case code for each scenario. Users can have multiple roles, and roles can explicitly grant or deny permissions.

Common resolution strategies:

- **Any-grant-wins**: If any role grants the permission, the user has it. Simple but makes denial impossible — you can't create a "read-only for everyone" override because a higher role's grant would always win.
- **Any-deny-wins**: If any role denies, the user is denied. Safe but too restrictive — denying `message.post` on `everyone` would block admins too, requiring explicit grants on every higher role.
- **Hierarchy-wins**: Check roles in rank order (highest rank first). First explicit grant or deny found wins. Lower-ranked roles are never consulted if a higher-ranked role has an opinion.

## Decision

Use hierarchy-wins resolution. Roles have a `position` field (higher number = higher rank). When checking a permission for a user:

1. Get the user's roles, sorted by position (descending = highest rank first)
2. For each role, check if it has an explicit grant or deny for the permission
3. The first explicit decision found wins
4. If no role has an opinion, the permission is denied (default-deny)

## Consequences

- **Announcements pattern works naturally**: Deny `message.post` on `everyone`, but `owner`/`admin`/`moderator` roles (higher rank, checked first) retain their grant. No special-case code needed.
- **Thread replies can be separated**: Deny `message.post` on `everyone` but grant `message.post-in-thread`, so regular users can discuss in threads but not post root messages.
- **Predictable resolution**: Given a user's roles and the role hierarchy, the permission outcome is deterministic and explainable.
- **Testing requires rank awareness**: Denying a permission on the `everyone` role does NOT block users with higher-rank roles. Tests must deny on the user's actual highest-rank role to verify denial.
- **Role ordering matters**: Changing a role's position changes permission outcomes. The position field is part of the security model, not just a display preference.
- **Config-owner is just an `owner` assignment**: Config-designated owners (configured via `owners.emails`) used to bypass role-hierarchy resolution as a special case. After Phase 5 of #330 they're materialised as a real `owner` role assignment — at email-verification time for new accounts and at server boot for already-verified matching users after config changes — so they flow through the same hierarchy walk as everyone else. No special case in the resolver.
