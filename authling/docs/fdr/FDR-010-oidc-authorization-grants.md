# FDR-010: OIDC Authorization Grants

**Status:** Experimental
**Last reviewed:** 2026-08-21

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
  active grant containing every requested scope. Authentication is still
  required, and all ordinary client, redirect, request, and PKCE validation
  still runs.
- `prompt=consent` always displays consent. Allowing it renews the active grant
  and records a new authorization fact without changing the active grant ID.
- Denying a forced-consent request does not revoke an existing grant.
- The account page lists active grants with their client name, display host,
  and latest explicit authorization time. Same-origin POST is required to
  revoke one.
- Revocation affects future authorization decisions immediately after the
  durable write. Existing relying-party sessions, authorization codes, ID
  tokens, and five-minute access tokens are not enumerated or terminated and
  remain subject to their own expiry and one-time-use rules.
- Re-authorizing a revoked client creates a fresh grant ID. This separates the
  new authorization generation from credentials that may later be bound to the
  revoked generation.

## Durable Model

`OIDCGrantAuthorizedEvent` and `OIDCGrantRevokedEvent` are PII-free account
aggregate facts in `AUTHLING_EVT`. They contain opaque account and grant IDs,
a deployment-keyed digest of the exact client ID, the client metadata snapshot,
scopes, and opaque event correlations. They contain no raw configured client
ID or CIMD URL, account email, browser metadata, token, code, redirect URI, or
submitted request URL.

The authorization projection consumes `authling.evt.account.*`, rebuilds the
active grant inventory in memory, and is disposable. Commands synchronize it
to the current account tail, publish with account-subject OCC, re-evaluate
after conflicts, and wait for the committed position before returning. Replay
rejects renewals or revocations that reference another active authorization,
grant IDs reused after revocation, and grants for absent accounts.

The client metadata stored in a grant is a display snapshot, not the authority
for future protocol requests. Every authorization request continues to resolve
and validate the currently configured client or CIMD document. This avoids an
outbound metadata fetch from the account page and prevents transient client
resolution failures from hiding revocation controls.

The new protobuf variants are additive storage fields, but binaries predating
them reject the unknown Authling event payload during replay. Deploy this
experimental feature as a coordinated Authling upgrade. Once either event has
been written, do not roll a replica back to a version that predates FDR-010.

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

Future incremental authorization should compare requested scopes with the
active set and show consent for newly requested access. It should define
whether an explicit authorization replaces or unions scope sets. Client
display-metadata refresh should remain separate from protocol client
validation and must not let a changed document silently broaden a grant.

## Related

- **OIDC provider:** [FDR-004](FDR-004-openid-connect-provider.md)
- **Browser sessions:** [FDR-009](FDR-009-browser-session-management.md)
- **Architecture:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md), [ADR-004](../adr/ADR-004-cimd-native-openid-provider.md)
