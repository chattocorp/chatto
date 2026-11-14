# FDR-010: OIDC Authorization Grants

**Status:** Experimental
**Last reviewed:** 2026-11-13

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
  authenticated account, exact client, and granted scopes and resource. The grant captures
  the validated client name and display host used by the account UI.
- A later request skips the consent page only when its exact client ID has an
  active grant containing every requested scope and the current consent
  disclosure version and the same resource. Authentication is still
  required, and all ordinary client, redirect, request, and PKCE validation
  still runs.
- `prompt=consent` always displays consent. Allowing it renews the active grant
  and records a new authorization fact without changing the active grant ID.
- Consent disclosure version 3 lists the stable account ID (`sub`), names for
  `profile`, and the current verified address for `email`. It explains that
  access includes later changes to the requested profile and email information.
  The consent page groups this information by permission. The account ID is
  available under **Your account ID**. Approval and denial are adjacent actions.
  The page also identifies the selected API and explains offline access when requested.
- Explicit approval adds the approved scopes for the selected resource.
  Subsets can reuse a covering grant; expanded requests require consent.
- Approval forms carry the disclosure version. The server rejects an outdated
  approval form and asks the person to reload it. Denial remains available.
- Denying a forced-consent request does not revoke an existing grant.
- The Connected apps tab at `/account/apps` lists active grants with their client name, display host,
  recorded scopes, and latest explicit authorization time. Same-origin POST is required to
  revoke one.
- Revocation prevents further refreshes and affects future authorization decisions after the
  durable write. Existing relying-party sessions, authorization codes, ID
  tokens, and five-minute access tokens are not enumerated or terminated and
  remain subject to their own expiry and one-time-use rules. Codes that request
  offline access also require their bound grant to remain active at exchange.
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

Metadata envelope version 1 encrypts the name, display host, and resource as a JSON object
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

Disclosure versions 1, 2, and 3 replay and decrypt with their original
associated data. Only version 3 permits consent reuse. Older grants remain
visible and revocable but require fresh approval. Version 3 stores the resource
inside encrypted metadata. No raw resource URL enters durable plaintext.
Upgrade all replicas together. Older binaries reject version-3 grants, so
rollback is not supported after the first new grant. No protobuf tag changes
are required.

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

Refresh tokens require `offline_access`, consented once per grant.
They are available to configured confidential and public clients and to CIMD
public clients. Public clients must use S256 PKCE. A `prompt=none` request is
auto-approved when the grant already covers its scopes for that resource.
CIMD metadata must declare the `refresh_token` grant. Configured clients can
disable renewal with `allow_refresh_tokens = false`.

Each encrypted refresh family is bound to the account, exact client, grant
generation, scopes, and resource. Each use checks current client admission,
resource policy, active account state, and the authoritative grant. A refresh
does not fetch CIMD metadata: the family proves that the CIMD client was
admitted with the `refresh_token` grant. A refresh uses the cached CIMD
document when one is present. Otherwise, it uses a public client with refresh
permission. CIMD admission and resource policy remain current operator policy.
A failed client authentication returns `invalid_client` with HTTP 401. Additional consent adds permissions per resource. It never removes earlier
scopes or resources and does not transfer scopes between resources. A family
remains valid while the grant covers its original resource and scopes.

A family expires after 30 days or seven days without successful use. Operators
can configure both limits from one through 365 days; the idle limit must not
exceed the total limit. Rotation never extends the total limit. An explicit
scope subset limits only the response. A replacement refresh token retains
the original scope, also when the subset omits `offline_access` (RFC 6749
section 6, RFC 8707 section 2.2). Issue an ID token only with effective
`openid`. Refreshes use current account claims and retain the original
authentication time.

A refresh token rotates once. For a reuse grace period after a
rotation, the previous token can repeat that rotation one time. A repeat
returns a new access token and the current refresh token again. It does not
rotate the family; an OCC write records that the repeat is used. Two concurrent
uses of one token therefore converge on one replacement. After an OCC
conflict, Authling reads the family again; a third conflict revokes the
family. A second repeat, use of the previous token after the grace period, or
use of an older token at any time is reuse. Reuse invalidates the whole
family, including its latest token. Invalid credentials or another client
cannot revoke a family. A revoked or expired family has no grace period.
The grace period defaults to 10 seconds. Operators can set it from 0 to 60
seconds; 0 disables repeats. Okta (30 seconds by default), Auth0 and Ory Hydra
(both disabled by default) use comparable settings.
A failed or uncertain rotation returns no credentials and can require new
browser authorization.

The grace period trades a small exposure for reliability. An attacker who has
a stolen previous token and uses the one repeat within the grace period of the
rotation gets the current token. If the client already used the repeat, the
attacker's attempt revokes the family. If the attacker was first, the client's
own retry revokes it. Without the grace period, a lost response or two browser
tabs that refresh at the same time end the session. Three or more simultaneous
uses of one token revoke the family. Family state survives restart and expires automatically.

Revoking an app invalidates all families in that grant generation. Reauthorizing
the app creates a new generation and cannot restore old refresh tokens.
Account deletion also blocks refreshes. Password reset and signed-in password
change invalidate all account families through the durable password-event
version. Consent grants remain active. Email change and browser logout preserve
families. Existing access tokens
retain their issuance-time claims until expiry; UserInfo returns current data.
Relying-party sessions remain under the relying party's control.

Token revocation, introspection, RP-initiated logout, and first-party consent
bypass are not implemented and are not required for this flow.

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
shows consent for new access. Explicit approval adds to the resource scope set. Client
display-metadata refresh must remain separate from protocol client
validation and must not let a changed document silently broaden a grant.

## Related

- **Resource and refresh architecture:** [ADR-010](../adr/ADR-010-resource-access-and-refresh-tokens.md)
- **OIDC provider:** [FDR-004](FDR-004-openid-connect-provider.md)
- **Browser sessions:** [FDR-009](FDR-009-browser-session-management.md)
- **Architecture:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md), [ADR-004](../adr/ADR-004-cimd-native-openid-provider.md)
