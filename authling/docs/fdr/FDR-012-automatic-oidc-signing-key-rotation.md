# FDR-012: Automatic OIDC Signing-Key Rotation

**Status:** Experimental
**Last reviewed:** 2026-08-21

## Overview

Authling automatically rolls over the RS256 key used to sign OpenID Connect ID
tokens. Relying parties discover prepared, active, and recently decommissioned
public keys through the existing JWKS endpoint. Rotation requires no Authling
account interaction and does not change the issuer or account `sub`.

## Behavior

- A signing key becomes eligible for rotation after 90 days by default.
  `oidc.signing_key_rotation_interval_days` configures an interval from one to
  3,650 days. Omitting it preserves the 90-day default.
- A replacement public key appears in JWKS for ten minutes before Authling
  signs an ID token with it. JWKS responses advertise a five-minute shared
  cache lifetime.
- Activation makes the replacement the sole signing key for new ID tokens.
  The preceding public key remains in JWKS for 15 minutes: five minutes for
  the maximum ID-token lifetime, five minutes for verifier clock skew, and
  five minutes for reconciliation and operational margin.
- After retirement, the preceding key disappears from JWKS and its private
  material is purged. Repeating interrupted preparation or destruction work is
  safe.
- The issuer URL, OIDC `sub`, browser sessions, authorization grants,
  authorization codes, and relying-party sessions do not change during
  rotation.
- Opaque access tokens issued before rotation remain usable through their
  ordinary five-minute lifetime because their encryption key is independent
  of ID-token signing keys.

## Durable Model

Five additive event variants record rotation request, key preparation,
activation, retirement request, and retirement completion on the singleton
`authling.evt.issuer` aggregate. The issuer projection materializes exactly one
active key plus at most one prepared successor and one retiring predecessor.
It rejects missing, duplicate, early, mismatched, or out-of-order transitions
during live consumption and cold replay.

A rotation request records a new opaque key reference before an outcome writes
private material. This makes provisioning recoverable after a crash rather
than leaving an unreferenced key whose ownership must be inferred from age.
Retirement likewise commits before private material is purged, then records
completion after successful destruction.

All lifecycle appends use OCC against the issuer subject. Concurrent replicas
may generate proposed event IDs or attempt the same idempotent key operation,
but only the transition matching the authoritative tail commits.

`AUTHLING_KEYS` retains the private RSA records and a separate stable symmetric
key record for opaque access-token envelopes. The first upgraded runtime seeds
that record with the legacy signing-derived value for rolling token-format
compatibility; signing rotation never changes it afterward. Event payloads
contain no private or symmetric key bytes. `AUTHLING_RUNTIME_STATE` and its key
scheme are unchanged.

## Security and Failure Behavior

- Token signing and JWKS fail closed if required key material is absent,
  malformed, substituted beneath another reference, or inconsistent with the
  durable `kid`.
- A prepared key cannot sign before its activation time. An active key cannot
  be retired before its predecessor overlap expires.
- A replica that is too stale to resolve a key after retirement fails closed;
  it does not silently select another key or mint an unverifiable token.
- The key identifier is the existing SHA-256 fingerprint of the public key.
  Distinct keys therefore have distinct `kid` values without exposing private
  material.
- Events and logs contain no account identifiers, client identifiers, tokens,
  authorization codes, emails, IP addresses, or private key bytes.

## Compatibility

Historical issuers with only `IssuerEstablishedEvent` replay with their
original key as the active key. The access-token encryption migration accepts
both legacy signing-derived envelopes and the stable-key envelope. Seeding the
stable record from the legacy value also lets an older replica decrypt a token
written by an upgraded replica before signing rotation begins.

Binaries predating FDR-012 reject its new event variants. Upgrade Authling
replicas together before allowing automatic rotation to write its first event.
After that event exists, do not roll back to a binary that predates FDR-012.

## Limitations

- Authling does not yet provide a manual emergency-rotation control or a
  compromise-specific JWKS withdrawal procedure.
- Routine rotation intervals are configured in whole days. The ten-minute
  publication lead and 15-minute retirement overlap are fixed safety policy.
- Automatic rotation does not revoke tokens already accepted by a relying
  party or force relying-party sessions to end.

## Related

- **Architecture:** [ADR-008](../adr/ADR-008-automatic-oidc-signing-key-rotation.md)
- **OIDC provider:** [FDR-004](FDR-004-openid-connect-provider.md)
- **Authorization grants:** [FDR-010](FDR-010-oidc-authorization-grants.md)
