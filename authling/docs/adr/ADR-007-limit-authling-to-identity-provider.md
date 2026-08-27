# ADR-007: Limit Authling to Identity-Provider State

**Status:** Accepted

**Date:** 2026-08-14

## Context

Authling added a global account-data scope, encrypted user-data bucket, and
TinyBase synchronization endpoint while its identity-provider surface was
still taking shape. That made one deployment both an identity authority and an
application-data host. The combined role broadened consent, storage,
availability, quota, portability, and deletion responsibilities without being
required for OpenID Connect.

Chatto used the service only to synchronize a browser's registered-server
catalogue. That catalogue can remain device-local without weakening Authling's
identity role or a Chatto server operator's explicit choice of trusted issuer.

## Decision

Authling is an identity provider. It persists only state needed to establish,
authenticate, and represent identities or to execute identity protocols:

- account and credential facts;
- browser sessions and short-lived authentication flows;
- issuer and signing-key material; and
- OIDC requests, codes, access tokens, consent, and abuse-control state.

Authling does not store application preferences, server catalogues, generic
documents, or synchronized personal data. It exposes no application-data OAuth
scope or synchronization endpoint. The `account_data` scope, `/data/sync`
WebSocket, TinyBase transport, `AUTHLING_USER_DATA` bucket creation, and
purpose-scoped application data keys are removed. The
`http.trusted_proxy_cidrs` setting is also removed because it existed only for
per-source synchronization-handshake admission.

Previously created `AUTHLING_USER_DATA` buckets are not read or deleted during
upgrade. Operators may remove that retired experimental data after taking any
backup they require. Authling must not make automatic destruction of an
existing bucket part of ordinary startup.

A future personal-data product would require its own product decision, API,
authorization model, operational limits, and trust boundary. It must not be
added to Authling merely as another identity-provider scope.

## Consequences

- Authling has a smaller security, persistence, and compatibility surface.
- OIDC authorization accepts exactly `openid`; clients requesting
  `account_data` fail closed as unsupported.
- Chatto server registrations and credentials are device-local. A Chatto
  frontend upgrades old synchronized registrations into ordinary local
  registrations and clears the retired authorization and TinyBase caches.
- Cross-device server-catalogue synchronization is no longer provided.
- ADR-005 and ADR-006 are superseded, and FDR-005 is retired.

## Related

- [ADR-004: Provide OpenID Connect with CIMD-Native Client Discovery](ADR-004-cimd-native-openid-provider.md)
- [ADR-005: Synchronize Account Data with a Durable TinyBase Peer](ADR-005-tinybase-account-data-sync.md)
- [ADR-006: Authorize Global Account Data Through OpenID Connect](ADR-006-oidc-authorized-account-data.md)
- [FDR-004: OpenID Connect Provider](../fdr/FDR-004-openid-connect-provider.md)
- [FDR-005: Account Data Synchronization](../fdr/FDR-005-account-data-sync.md)
