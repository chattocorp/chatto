# FDR-010: OIDC Authorization Grants

**Status:** Experimental
**Last reviewed:** 2026-09-20

## Overview

Authling remembers an account's explicit authorization of an OIDC client.
Later authorization requests for the same client and an already granted scope
set can continue without showing the consent page again. A signed-in person can
review and revoke these durable relationships under **Authorized apps**.

These grants describe applications authorized through Authling. They are not
Authling browser sessions and do not enumerate the relying party's own login
sessions.

## Behavior

- The initial grant boundary is one exact OIDC `client_id`. Authling does not
  infer that clients with similar names, redirect hosts, or CIMD hostnames are
  the same relying party.
- Explicitly allowing a consent request creates a durable grant for the
  authenticated account, exact client, and granted scopes. The grant captures
  the validated client name and display host used by the account UI.
- A later request skips the consent page only when its exact client ID has an
  active grant containing every requested scope and the current consent
  disclosure version. Authentication is still
  required, and all ordinary client, redirect, request, and PKCE validation
  still runs.
- `prompt=consent` always displays consent. Allowing it renews the active grant
  and records a new authorization fact without changing the active grant ID.
- Consent disclosure version 2 lists the stable account ID (`sub`), names for
  `profile`, and the current verified address for `email`. It explains that
  access includes later changes to the requested profile and email information.
- Explicit approval replaces the grant with exactly the approved scopes.
  Subsets can reuse a covering grant; expanded requests require consent.
- Approval forms carry the disclosure version. The server rejects an outdated
  approval form and asks the person to reload it. Denial remains available.
- Denying a forced-consent request does not revoke an existing grant.
- The account page lists active grants with their client name, display host,
  recorded scopes, and latest explicit authorization time. Same-origin POST is required to
  revoke one.
- Revocation affects future authorization decisions immediately after the
  durable write. Existing relying-party sessions, authorization codes, ID
  tokens, and five-minute access tokens are not enumerated or terminated and
  remain subject to their own expiry and one-time-use rules.
- Re-authorizing a revoked client creates a fresh grant ID. This separates the
  new authorization generation from credentials that may later be bound to the
  revoked generation.

## Durable Model

`OIDCGrantAuthorizedEvent` and `OIDCGrantRevokedEvent` are account aggregate
facts in `AUTHLING_EVT`. Grants contain opaque account and grant IDs, a
deployment-keyed digest of the exact client ID, scopes, consent disclosure
version, encrypted display metadata, and opaque event correlations. Client
names and hosts can contain personal data, so events never store them in
plaintext. Raw client IDs, CIMD URLs, account emails, tokens, codes, redirect
URIs, and submitted request URLs are not added to these records.

Metadata envelope version 1 encrypts the name and display host as a JSON object
with XChaCha20-Poly1305. It uses the account's existing credential data key and
user-key hierarchy; authorization does not provision or delete keys. Associated
data binds the envelope to the event ID, account ID, grant ID, exact-client
digest, scopes, prior authorization event, disclosure version, and both key
references. The version-specific domain separates it from other encrypted
account data. Authorization requires an account with encryption keys.

Only metadata envelope version 1 is supported. The unused plaintext protobuf
fields retain their original tags but must be empty. Authling has not been
deployed; this implementation does not provide a migration from plaintext
grant history.

Disclosure versions 1 and 2 can replay and decrypt with their original
associated data. Only version 2 permits consent reuse. Version-1 grants remain
visible and revocable, but require fresh approval. Old approval forms fail.
Upgrade all replicas together. Older binaries cannot replay version-2 grants;
rollback to those binaries is not supported after a new grant is recorded.
No protobuf field or envelope shape changes are required.

The authorization projection consumes `authling.evt.account.*`, rebuilds the
active grant inventory in memory, and is disposable. Commands synchronize it
to the current account tail, publish with account-subject OCC, re-evaluate
after conflicts, and wait for the committed position before returning. Replay
rejects renewals or revocations that reference another active authorization,
grant IDs reused after revocation, grants for absent accounts, and protected
metadata that references a different account key hierarchy. The decoder
rejects unknown envelope or disclosure versions and mixed plaintext/encrypted
fields. The projection retains ciphertext for protected grants. The service
authenticates and decrypts it when returning display data or checking consent
reuse. Missing keys or invalid ciphertext fail closed. Revocation itself does
not require decryption.

The client metadata stored in a grant is a display snapshot, not the authority
for future protocol requests. Every authorization request continues to resolve
and validate the currently configured client or CIMD document. This avoids an
outbound metadata fetch from the account page and prevents transient client
resolution failures from hiding revocation controls.

## Security and Failure Behavior

- A stale replica cannot reuse a revoked grant: consent-reuse decisions first
  wait for the local projection to reach the authoritative account tail.
- The opaque grant ID in a revocation form is not sufficient authority. The
  server derives the account from the active Authling browser session and
  resolves the grant under that account.
- Cross-origin revocation is rejected. A missing, already revoked, or
  cross-account grant has the same account-facing unavailable result.
- If durable grant creation succeeds but the expiring authorization request
  cannot be updated, retrying the request can continue from the committed
  grant. Runtime protocol state never becomes the only record of consent.
- Logs and errors do not include account IDs, client IDs, grant IDs, tokens,
  authorization codes, or full request URLs.

## Extension Notes

### Refresh tokens

Refresh tokens are the intended next OIDC feature because the durable grant is
their revocation anchor. The implementation should:

- bind every opaque refresh-token family to the account, exact client ID,
  grant ID, and granted scopes;
- verify that the grant ID is still active on every refresh and fail closed
  when the authorization projection is unavailable;
- rotate refresh tokens with single-use OCC so concurrent reuse has at most one
  winner;
- keep refresh-token credentials in encrypted runtime state rather than the
  durable event log;
- make grant revocation invalidate every refresh token in that grant
  generation, while leaving already issued short-lived access and ID tokens to
  expire; and
- decide and document the `offline_access` request, consent, client-policy, and
  discovery semantics before advertising the refresh-token grant type.

Token revocation and RP-initiated logout remain separate protocol features.
They may provide narrower ways to end credentials or relying-party sessions,
but must not redefine account grant revocation implicitly.

### Relying-party grouping

A future relying-party identity may group multiple exact client IDs behind one
account-facing application. Grouping must use explicit, authenticated metadata
and a durable migration policy; matching names, redirect hosts, or hostname
suffixes is not sufficient. Existing exact-client grants must remain valid and
revocable during migration. The projection can later materialize grouped views
without changing historical events, while a new event type should record any
durable grouping or grant migration fact.

### Additional scopes and metadata changes

Incremental authorization compares requested scopes with the active set and
shows consent for new access. Explicit approval replaces the scope set. Client
display-metadata refresh must remain separate from protocol client
validation and must not let a changed document silently broaden a grant.

## Related

- **OIDC provider:** [FDR-004](FDR-004-openid-connect-provider.md)
- **Browser sessions:** [FDR-009](FDR-009-browser-session-management.md)
- **Architecture:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md), [ADR-004](../adr/ADR-004-cimd-native-openid-provider.md)
