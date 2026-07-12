# ADR-051: OIDC-Managed Role Sources

**Date:** 2026-07-12

## Context

OIDC providers can expose role claims that operators want to use for Chatto
authorization. A plain role assignment cannot distinguish a manual grant from
one derived from an identity provider, so reconciling a changed claim would
otherwise revoke administrator-made assignments or grants from another provider.

## Decision

Store OIDC-managed role grants as separate, additive RBAC events keyed by the
provider ID and role name. Effective membership is the union of manual and all
provider-managed sources. A provider may add roles only or reconcile only its
own sources at successful interactive OIDC authentication.

Raw OIDC claim values are transient callback data. They are never written to
EVT; only accepted role names and provider IDs are durable. Operators opt in
with an explicit role allowlist or `[*]`; the wildcard delegates every
assignable role, including roles added later and `owner`.

## Consequences

- Manual assignments and independent identity providers cannot clobber each
  other during reconciliation.
- An OIDC-only role cannot be revoked through ordinary role administration;
  operators change the identity provider or its Chatto configuration instead.
- Group-to-role mappings and background provider-token refresh remain outside
  this first version. Role changes take effect on the next OIDC authentication.
