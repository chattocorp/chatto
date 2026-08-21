# ADR-008: Automatically Rotate OIDC Signing Keys

**Status:** Accepted

**Date:** 2026-08-21

## Context

Authling originally generated one persistent RS256 signing key when an issuer
was established. The key had no rollover or retirement lifecycle. Long-lived
deployments therefore had to keep using the same private key, while replacing
it manually would invalidate the durable issuer identity or make relying
parties unable to verify tokens during the change.

OpenID Connect supports signing-key rollover by publishing multiple keys at
`jwks_uri`, naming the signing key with `kid`, and retaining recently
decommissioned public keys during the transition. Authling must also support
its embedded-NATS deployment, where a separate administration command cannot
connect to the private in-process server.

The OpenID Provider library previously derived opaque access-token encryption
from the active RSA private key. That incidental coupling would invalidate
live access tokens whenever the signing key changed.

## Decision

Authling automatically rotates its OIDC signing key inside the running
process. An active key is eligible for rotation after 90 days by default;
operators may configure the interval in whole days.

The issuer aggregate records a recoverable lifecycle:

1. A rotation request commits an opaque future key reference before key
   generation begins.
2. An idempotent outcome creates the private key at that reference. A prepared
   event publishes its public key in JWKS for ten minutes before activation.
3. Activation switches all new ID-token signatures to the prepared key. The
   preceding public key remains in JWKS for 15 minutes: five minutes for the
   maximum ID-token lifetime, five minutes for verifier clock skew, and five
   minutes for reconciliation and operational margin.
4. A retirement request removes the preceding key from the published set.
   The outcome purges its private material and records retirement completion.

Every issuer transition uses subject-level optimistic concurrency control.
All replicas may reconcile the lifecycle; one event append wins and the others
refresh from the authoritative projection. Missing or mismatched key material
fails token issuance and JWKS resolution closed.

The OpenID Provider's opaque access-token envelope uses a separate stable
symmetric-key record in `AUTHLING_KEYS`. Its first value is seeded from the
legacy signing-derived key, allowing old and new replicas to read each other's
five-minute tokens during the coordinated upgrade. The provider also accepts
both legacy envelope formats. Once persisted, the symmetric record remains
unchanged when RSA signing keys rotate.

Private keys remain exclusively in `AUTHLING_KEYS`. `AUTHLING_EVT` contains
only opaque references, public key identifiers, transition times, and event
correlations. No new stream, bucket, snapshot, or persisted index is added.

## Consequences

Routine rotation works in embedded and external NATS deployments without an
operator control connection. Restarting during any transition resumes from
the durable issuer history. JWKS can contain the active key, a pre-published
successor, and an unexpired predecessor at the same time.

The configured interval controls when rotation begins, not the fixed safety
windows. Keeping publication and retirement overlap under Authling's control
prevents an unsafe operator setting from making newly signed tokens
unverifiable or removing a still-needed key. The 15-minute retirement overlap
also avoids extending trust in a potentially compromised predecessor without
a concrete compatibility benefit.

Older Authling binaries reject the additive lifecycle event variants. The
release introducing this decision requires a coordinated Authling upgrade,
and rollback to an older binary is unsafe after the first rotation event has
been written. Existing `IssuerEstablishedEvent` records remain valid and
become the initial active-key state without rewriting history.

Automatic routine rollover is not emergency revocation. A relying party may
continue trusting a compromised key from its JWKS cache, and Authling does not
yet expose an authenticated manual rotation or compromise-response control.
The five-minute ID-token lifetime limits ordinary exposure but does not replace
an incident procedure.

## Related

- [ADR-001: Build Authling on an Event-Sourced NATS Architecture](ADR-001-event-sourced-nats-architecture.md)
- [ADR-002: Protect User Data with Hierarchical Keys and Cryptographic Erasure](ADR-002-hierarchical-keys-and-cryptographic-erasure.md)
- [ADR-004: Provide OpenID Connect with CIMD-Native Client Discovery](ADR-004-cimd-native-openid-provider.md)
- [FDR-012: Automatic OIDC Signing-Key Rotation](../fdr/FDR-012-automatic-oidc-signing-key-rotation.md)
