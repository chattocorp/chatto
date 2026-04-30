# Admin Interface

## Config Owner vs Space Admin

Two separate authorization concepts:

- **Config-designated instance owner**: Configured via `owners.emails` in `chatto.toml`. Users with any matching verified email get full instance access (owner-level permissions), including `/admin` routes. Used by Chatto Cloud's control plane to designate the customer as their instance owner, and by self-hosters to designate themselves.
- **Space admin**: Per-space role (`RoleAdmin` in permissions.go). Can manage a specific space's settings, rooms, and members.

These are independent — a space admin is not automatically an instance owner and vice versa.

There's also an **RBAC instance admin role** (separate from owner) that grants admin-level permissions but not full ownership; `requireInstanceAdmin` accepts both config-owners and RBAC admins.

## Privacy Boundary

Instance owners and admins can see operational metadata but NOT user content:

| Can See                            | Cannot See       |
| ---------------------------------- | ---------------- |
| User list (login, email, avatar)   | Message content  |
| Space/room names and member counts | Private messages |
| NATS/JetStream metrics             | File contents    |
| System configuration               | User passwords   |

This boundary is intentional. If message visibility is needed for moderation, it should be a separate, auditable feature with explicit consent.

## Backend Authorization

Admin queries use a nested `admin` type pattern. The `Query.admin` resolver checks authorization once and returns `nil` for non-admins:

```go
func (r *queryResolver) Admin(ctx context.Context) (*model.AdminQueries, error) {
    user := auth.ForContext(ctx)
    if user == nil {
        return nil, nil // Not authenticated
    }
    if !isConfigOwner(ctx, r.core, r.ownersConfig, user.Id) {
        return nil, nil // Not an owner
    }
    // Return populated AdminQueries...
}
```

The `isConfigOwner` helper checks if any of the user's _verified_ emails match the `owners.emails` list. Unverified/pending emails are never matched. A match short-circuits all instance-permission checks (owner has all permissions).

All fields under `admin` (users, spaces, systemInfo) don't need individual auth checks — the parent resolver handles it.

## Configuration

```toml
[owners]
emails = ["owner@example.com", "ops@example.com"]
```

Users are granted instance-owner status if any of their verified email addresses matches an entry in this list. The `isConfigOwner` helper performs the matching — only verified emails are considered, never pending/unverified ones.
