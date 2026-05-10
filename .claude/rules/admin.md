# Admin Interface

## Server roles

After Phase 5 of #330, RBAC is a single flat tier of server roles in the
`SERVER_RBAC` bucket. The system roles are `owner`, `admin`, `moderator`,
and the virtual `everyone`. There is no longer an instance-vs-space tier
split, and the legacy `instance-` prefix on role names is gone.

- **`owner`** — full server control. Top of the hierarchy. Holders pass
  every permission check, can edit every user, and can never be
  demoted by an admin (rank-based hierarchy enforcement).
- **`admin`** — full administrative access. Can do everything an owner
  can except manage owner-rank users.
- **`moderator`** — moderation permissions without administrative
  reach.
- **`everyone`** — virtual role assigned to every authenticated user.
  Default-permission grants (e.g. "all members can post") attach here.

## Config-designated owner

`owners.emails` in `chatto.toml` declares email addresses that confer
ownership. The wiring is fully role-based — there is no longer a
config-owner short-circuit in the permission resolver:

- On email verification (registration / OAuth / admin-direct),
  `addVerifiedEmail` checks the new email against `owners.emails` and
  auto-assigns the `owner` role if it matches. This closes the
  chicken-and-egg case on a fresh deployment: the operator signs up,
  verifies their email, and immediately has owner permissions without
  needing a server restart.
- For existing deployments, run `chatto reset rbac` after upgrading.
  The command wipes `SERVER_RBAC`, re-seeds the system roles plus
  default permissions from code, and assigns the `owner` role to every
  user whose verified email matches `owners.emails`.

## Privacy Boundary

Owners and admins can see operational metadata but NOT user content:

| Can See                            | Cannot See       |
| ---------------------------------- | ---------------- |
| User list (login, email, avatar)   | Message content  |
| Room names and member counts       | Private messages |
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
    isAdmin, _ := r.isInstanceAdmin(ctx, user.Id)
    if !isAdmin {
        return nil, nil // Not owner or admin
    }
    // Return populated AdminQueries...
}
```

`r.isInstanceAdmin` is the unified role check — true for users with the
`owner` or `admin` role. All fields under `admin` (users, members,
systemInfo) don't need individual auth checks — the parent resolver
handles it.

## Configuration

```toml
[owners]
emails = ["owner@example.com", "ops@example.com"]
```

Users are granted owner status when one of their verified email
addresses matches an entry in this list. Only verified emails are
considered, never pending / unverified ones.
