# ADR-032: External Identity Integration Boundaries

**Date:** 2026-05-19

## Context

Chatto supports authentication through external identity providers — OIDC today, AT Protocol now joining it, and likely more in the future. Each integration raises two recurring questions:

1. **Credential lifetime.** The OAuth flow yields tokens that could let Chatto act on the user's behalf against the provider (post records to their PDS, read their OIDC userinfo on demand, etc.). Do we keep those tokens to enable such features, or discard them once we've identified the user?
2. **Data source.** Providers often expose user data through both a vendor-hosted convenience API (Bluesky's AppView, Google's userinfo endpoint, a CDN for profile pictures) and a more decentralized protocol-level surface (a user's PDS, the OIDC ID token claims). When both exist, which does Chatto prefer?

Both questions touched the design of the OIDC integration when it was built; both came up again, sharper, when adding ATProto sign-in. Settling them as principles avoids re-deciding each time a new provider arrives.

## Decision

**Two related principles, captured as one ADR because they're both expressions of "minimal coupling to external identity providers":**

### 1. External identity providers are authentication-only

Chatto uses external providers to *identify* a user — establish that "this DID/subject/email belongs to this person and we can trust the assertion." Once identification is complete, the provider's credentials are dropped:

- OIDC: the access token, refresh token, and ID token are consumed for claims, then discarded. No long-lived storage.
- ATProto: the OAuth session created during sign-in is explicitly revoked (`oauth.Logout`) at the end of the callback handler. Tokens are not persisted.

Chatto does not act on the user's behalf against the external provider. Any future "post to your PDS" or "sync your OIDC profile" feature would be a deliberate, separately-considered addition — not a side-effect of having tokens lying around from the sign-in flow.

### 2. Prefer protocol-level data sources over vendor-hosted services

When a provider offers both a protocol-level endpoint and a vendor-hosted convenience API for the same data, Chatto integrates with the protocol-level one even when it costs us a small amount of code.

- ATProto profile data is fetched from the user's own PDS (`com.atproto.repo.getRecord` + `com.atproto.sync.getBlob`), not from `public.api.bsky.app` or `cdn.bsky.app`. Users who migrate their PDS continue to work; Chatto inherits no dependency on a Bluesky-Inc service.
- OIDC `picture` claims, when present, are fetched from whatever URL the IdP returns — we don't substitute a hosted thumbnail proxy.

### Options considered

**Keep provider tokens to enable "act on behalf" features later.** Rejected: it bakes a security and storage cost into every sign-in for a hypothetical feature, and forces every integration to think about token refresh + secure storage. Easier to add tokens later (deliberately, for a specific feature) than to remove them once stored.

**Use vendor-hosted APIs because they're easier.** Rejected on principle. Chatto's pitch includes "self-hostable, no required external dependencies"; sneaking in a hard dependency on `public.api.bsky.app` for sign-in to *work* contradicts that. The protocol-level path is usually one or two extra HTTP calls, which is a small cost paid once at sign-in.

## Consequences

- **Smaller attack surface.** No long-lived provider credentials means no key rotation, no refresh-token leakage, no "what's in our KV that an attacker could resell."
- **Provider integrations are interchangeable.** Adding a new identity provider only requires that it can produce a verified identifier. No need to design around "and we'll also hold a write token."
- **"Act on behalf" features become explicit projects.** If someday Chatto wants to post a status to a user's Bluesky feed when they go on-call, that requires a separate consent flow, separate scoped storage, and a separate ADR. The default sign-in doesn't quietly accumulate the necessary tokens.
- **One or two extra HTTP calls per sign-in.** Fetching the bsky profile from a user's PDS instead of bsky's AppView is one GET; fetching the avatar blob is another. Negligible cost, paid once per user per sign-in.
- **No vendor-CDN amortization for assets.** We re-host avatars in our own asset pipeline rather than hot-linking to `cdn.bsky.app`. The asset pipeline already handles this for every other avatar source; no new infrastructure needed.
- **Federation-shaped features stay accessible.** A non-Bluesky ATProto user (or a user who migrated their PDS) Just Works because we never assumed Bluesky's hosted services were the source.
