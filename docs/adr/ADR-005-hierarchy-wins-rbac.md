# ADR-005: Hierarchy-Wins RBAC Permission Resolution

**Date:** 2026-03-01

## Context

Chatto needs a permission system that supports per-room overrides (e.g., an announcements channel where only moderators can post) without requiring special-case code for each scenario. Users can have multiple roles, and roles can explicitly grant or deny permissions.

Common resolution strategies:

- **Any-grant-wins**: If any role grants the permission, the user has it. Simple but makes denial impossible — you can't create a "read-only for everyone" override because a higher role's grant would always win.
- **Any-deny-wins**: If any role denies, the user is denied. Safe but too restrictive — denying `message.post` on `everyone` would block admins too, requiring explicit grants on every higher role.
- **Hierarchy-wins**: Check roles in rank order (highest rank first). First explicit grant or deny found wins. Lower-ranked roles are never consulted if a higher-ranked role has an opinion.

## Decision

Use hierarchy-wins resolution. Roles have a `position` field (lower number = higher rank). When checking a permission for a user:

1. Get the user's roles, sorted by position (ascending = highest rank first)
2. For each role, check if it has an explicit grant or deny for the permission
3. The first explicit decision found wins
4. If no role has an opinion, the permission is denied (default-deny)

## Consequences

- **Announcements pattern works naturally**: Deny `message.post` on `everyone`, but `owner`/`admin`/`moderator` roles (higher rank, checked first) retain their grant. No special-case code needed.
- **Thread replies can be separated**: Deny `message.post` on `everyone` but grant `message.post-in-thread`, so regular users can discuss in threads but not post root messages.
- **Predictable resolution**: Given a user's roles and the role hierarchy, the permission outcome is deterministic and explainable.
- **Testing requires rank awareness**: Denying a permission on the `everyone` role does NOT block users with higher-rank roles. Tests must deny on the user's actual highest-rank role to verify denial.
- **Role ordering matters**: Changing a role's position changes permission outcomes. The position field is part of the security model, not just a display preference.
- **Config-owner bypass**: Config-designated owners (configured via `owners.emails`) are checked at a level above the role hierarchy, so they are never blocked by role-level denials.
