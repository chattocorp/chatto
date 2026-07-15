# ADR-051: OIDC-Managed Role Sources

**Date:** 2026-07-12

## Context

OIDC providers can expose role claims that operators want to use for Chatto
authorization. A plain role assignment cannot distinguish a manual grant from
one derived from an identity provider, so reconciling a changed claim would
otherwise revoke administrator-made assignments or grants from another provider.

## Decision

Record the source on the existing RBAC role-assignment and role-revocation
facts. A source is either manual or OIDC; OIDC sources also carry the configured
provider ID and canonical verified issuer. Effective membership is the union of assignment sources. A
provider may add roles only or reconcile only its own sources at successful
interactive OIDC authentication.

Raw OIDC claim values are transient callback data. They are never written to
EVT; only accepted role names, provider IDs, and issuers are durable. Operators opt in
with an explicit role allowlist or `[*]`; the wildcard delegates every
assignable role, including roles added later and `owner`.

## Consequences

- Manual assignments and independent identity providers cannot clobber each
  other during reconciliation.
- A pending confirmation is rejected if its provider type or OIDC issuer no
  longer matches configuration. Reusing a provider ID for another issuer
  revokes sources from the prior issuer at boot.
- Ordinary administrator role changes affect manual assignments only. A solely
  IdP-managed role reports that it is managed by the identity provider;
  disconnecting the identity, disabling the role claim, or resetting RBAC can
  remove the OIDC source.
- Group-to-role mappings and background provider-token refresh remain outside
  this first version. Role changes take effect on the next OIDC authentication.
