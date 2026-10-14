# FDR-050: Provider Group Roles

**Status:** Active
**Last reviewed:** 2026-10-14

## Overview

Operators can let an OIDC login provider assign Chatto roles. Each provider
configuration can map provider groups to Chatto roles. When a user signs in
through that provider, Chatto gives the user the roles that their groups map
to. Chatto removes such a role only if the provider gave it. Roles that admins
assign stay under admin control. This lets organizations manage moderators,
admins, and other roles in their identity provider, without losing manual role
management in Chatto.

## Behavior

- **Configuration** — an OIDC provider entry can set `group_roles`. The value
  is a list of `group:role` pairs, separated by whitespace, for example
  `chat-mods:moderator it-ops:admin`. The same text works in `chatto.toml` and
  in the counted provider environment variable. Chatto splits each pair at its
  last colon, so a group name can contain colons. Group names with spaces are
  not supported; use the group ID or another claim value without spaces.
- **Combining pairs** — a role can be listed with more than one group. Any one
  of those groups gives the role. One group can give more than one role; each
  role needs its own pair.
- **Groups source** — Chatto reads the groups from the claim that
  `groups_claim` names (default `groups`), the same claim that `allowed_groups`
  uses ([FDR-023](FDR-023-authentication-and-sessions.md)). Chatto reads it
  from the ID token first and then from UserInfo.
- **When roles change** — Chatto applies the mapping when a user signs in
  through the provider, when a user links the provider to an account, and when
  a user creates an account through the provider. It applies the mapping after
  the `allowed_groups` check. When that check denies the sign-in of a linked
  account, Chatto removes the roles that the provider gave and gives no roles.
  A user who may not sign in through the provider thus keeps no authority from
  it.
- **Giving roles** — for each mapped role, if the user is in a matching group
  and does not have the role, Chatto assigns it. If the user already has the
  role, Chatto changes nothing.
- **Removing roles** — if the user is in no matching group, Chatto removes the
  role only when the current assignment came from this provider. An assignment
  that an admin made stays, also when the user later joins or leaves a mapped
  group.
- **Admin changes** — admins can still assign and remove every role, also
  roles that a provider maps. If an admin removes a role that a provider gave,
  the provider gives it again at the next sign-in while the user stays in a
  matching group. An admin who assigns a role that the user already has
  changes nothing, as for any other assignment.
- **Missing groups** — if the provider sends no readable groups claim, Chatto
  changes no roles for that sign-in. With `allowed_groups`, a missing claim
  denies the sign-in, so the rule for denied sign-ins applies instead. A groups
  claim that is present but empty is real data, so Chatto removes the roles
  that the provider gave.
- **Missing roles** — if a mapped role does not exist in Chatto, Chatto skips
  it and logs a warning with only the provider ID and the role name.
- **Configuration changes** — a changed mapping takes effect for each user at
  their next sign-in through the provider. When an operator removes a role from
  the mapping, sync no longer changes that role, and its current members keep
  it. Chatto still remembers which assignments the provider created. If the
  operator maps the role again, or the user's sign-in is denied, those
  assignments can still be removed. Assignments that existed before a role was
  mapped count as admin assignments, so the provider never removes them.
- **Owner of an assignment** — Chatto records the issuer of the provider as
  the owner of each assignment that the provider created, the same value that
  identity links use. A change of the provider `id` keeps the ownership. After
  a change of the issuer URL, the provider no longer removes the assignments of
  the old issuer; admins remove them by hand.
- **Unlinking** — when a user unlinks the last identity of a provider from
  their account, Chatto removes the roles that the provider gave, in the same
  change as the unlink. Otherwise the
  account could keep them after the identity moves to another account, and
  later group changes would never reach it. Admin assignments stay.
- **Revocation delay** — Chatto learns about group changes only at sign-in.
  When the identity provider removes a user from a group, the user keeps the
  role until their next sign-in through the provider. Existing sessions keep
  the role during that time.
- **Audit** — role assignment and role removal events record why they
  happened. A change by the provider records the provider ID and uses the
  system actor. An `owners.emails` owner assignment records that the server
  configuration caused it. A change without a reason is an admin change or an
  event from an older server version. The audit log shows the reason in the
  event details ([FDR-021](FDR-021-admin-dashboard.md)).

## Design Decisions

### 1. The mapping is server configuration

**Decision:** Operators define the mapping in the provider configuration, not
in the role settings UI or in RBAC state.
**Why:** The mapping gives authority from an external system. It belongs next
to the provider credentials and the `allowed_groups` restriction, under
operator control, and it can be reviewed with the rest of the deployment
configuration. A configuration-time feature needs no new management API, UI,
or durable binding state.
**Tradeoff:** Roles exist only at runtime, so the configuration check cannot
confirm that a mapped role exists. Chatto skips unknown roles at sign-in and
logs a warning. A change needs a restart, like other provider changes.

### 2. One text value of `group:role` pairs

**Decision:** `group_roles` is one string of whitespace-separated
`group:role` pairs, the same in TOML and in environment variables.
**Why:** One key per provider keeps the configuration short and works with the
existing counted provider environment variables without a second index level.
Grafana's `org_mapping` uses the same pair style, so many operators know it.
Splitting at the last colon lets group names contain colons, because role names
cannot.
**Tradeoff:** Group names with spaces cannot be expressed. Operators use group
IDs or provider claim values without spaces instead.

### 3. The creator of an assignment owns it

**Decision:** The provider removes only role assignments that it created. An
assignment that an admin created stays until an admin removes it.
**Why:** Operators keep full manual control of every role, and the provider
still removes the roles that it gave when a user leaves a group. The rule is
easy to explain: Chatto removes a role only if the provider gave it. It needs
no lock on mapped roles and no per-role mode.
**Tradeoff:** A role that both an admin and a group would give stays when the
user leaves the group, because the admin assignment came first. An admin who
removes a provider-given role sees it return at the next sign-in while the user
stays in the group. Assignments from before the mapping existed are never
removed by the provider.

### 4. Assignment events carry a typed reason

**Decision:** Role assignment and removal events get an optional reason. The
reason is one of a fixed set of kinds, and each kind holds only its own data:
the provider ID and issuer for a provider group change, and nothing for a server
configuration owner. Events without a reason are admin changes or older
events.
**Why:** The RBAC projection uses the reason of the event that created the
current assignment to decide ownership (decision 3), so no separate state is
needed. Ownership uses the issuer, because the provider ID can change and older
identity links can carry another provider ID. The audit log can explain why a role changed. The same reason lets
Chatto recognize `owners.emails` owner assignments and tell them apart from
manual ones (see [FDR-001](FDR-001-roles-and-permissions.md) decision 5). A typed reason holds no free text, so it cannot carry
personal data.
**Tradeoff:** Each new kind of automatic role change needs a new reason kind.
Older server versions ignore the reason, so during a rolling upgrade an older
replica treats provider assignments as admin assignments.

### 5. Provider changes are operator authority

**Decision:** Provider role changes use the system actor and skip the
delegated role-assignment limits that apply to admins.
**Why:** The operator configured the mapping, and the operator can already
assign every role through the server configuration. The delegated limits
protect against admins who give more authority than they hold
([FDR-001](FDR-001-roles-and-permissions.md) decision 6); no admin is acting
here.
**Tradeoff:** The configuration check must reject mappings to `owner` and
`everyone`. `owner` is never automatic, and `everyone` has no assignments.

### 6. A missing groups claim changes nothing

**Decision:** If the provider sends no readable groups claim, Chatto keeps the
current roles. An empty claim that is present removes provider-given roles. A
sign-in that `allowed_groups` denies always removes provider-given roles,
also when the claim is missing.
**Why:** A temporary UserInfo failure or a provider configuration error must
not remove the roles of every user who signs in. An empty list is a clear
statement from the provider. A user who may not sign in through the provider
must not keep authority from it.
**Tradeoff:** A provider that stops sending the claim leaves provider-given
roles in place until the operator corrects the provider configuration. With
`allowed_groups`, a temporary UserInfo failure denies the sign-in and removes
provider-given roles until the next successful sign-in.

## Gates

- `group_roles` is available only for `type = "oidc"` providers. The
  configuration check rejects it for other types, rejects malformed or
  duplicate pairs, rejects `owner` and `everyone` as roles, and rejects a role
  that the mappings of two providers name.
- Provider role changes do not need an admin permission; the operator
  configuration authorizes them.

## Related

- **ADRs:** ADR-033 (event-sourced state with projections), ADR-040
  (permission-only RBAC with owner override), ADR-052 (subject-specific RBAC
  with an everyone baseline), ADR-087 (request-time authorization with
  aggregate OCC)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-021 (Admin Dashboard & System
  Monitoring), FDR-023 (Authentication & Sessions)

## Open Questions

- Role member lists could show which assignments came from a provider. This
  needs an added read field in the role API and is not part of the first
  version.
- Faster revocation needs deprovisioning, for example OIDC back-channel logout
  or a provider check when a session renews. It is not part of this feature.
