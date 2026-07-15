# FDR-031: Sign in with AT Protocol

**Status:** Active
**Last reviewed:** 2026-07-06

## Overview

Users can sign in to Chatto using their AT Protocol identity — a DID issued by their PDS (typically `bsky.social`, but any compliant PDS works). The sign-in flow is offered alongside password and configured external providers on the standard login page; once authentication completes, the resulting Chatto account behaves identically to one created by any other path. The intent is "unified identity": the same person signing in to two Chatto servers with the same ATProto handle is recognizable as the same person, without any federation between the servers.

## Behavior

- The login page shows a **Sign in with AT Protocol** form whenever the server has a `[[auth.providers]]` entry with `id = "atproto"` and `type = "atproto"`. The form takes a single input — an ATProto handle (e.g. `alice.bsky.social`, `hendrik.mans.de`) — and a Continue button. A leading `@` is tolerated.
- Submitting the form redirects the browser to the user's PDS for the standard OAuth approval screen. After approval, the PDS redirects back to Chatto's callback endpoint.
- **First sign-in creates a pending external-identity account flow.** The user's DID is recorded as the stable identifier, and Chatto asks the user to confirm the account login through the shared SSO confirmation screen before creating the account. The handle is used as the login hint (so `alice.bsky.social` is proposed as login `alice.bsky.social`); shared account-creation validation handles collision feedback.
- **First sign-in also seeds profile data.** Chatto reads the `app.bsky.actor.profile` record from the user's PDS and passes its display name and avatar as account-creation hints. The profile data is owned locally thereafter — Chatto does not re-sync from the PDS on later sign-ins.
- **Users without a Bluesky profile degrade gracefully.** A handle that has never used the `app.bsky.actor.profile` lexicon (e.g. a Frontpage-only user) signs in successfully; their Chatto display name defaults to their handle and they get the standard initial-letter avatar placeholder.
- **Subsequent sign-ins match by DID, not handle.** Handles can change at the protocol level; the DID is stable. The user's handle is re-resolved on each sign-in for display purposes but isn't used to identify them.
- **Email is requested only when it can seed a new account.** First-time account creation asks for the `account:email` scope alongside the base `atproto` scope. If the user grants it on the PDS consent screen and their PDS-side email is confirmed, the address is added to the Chatto account as a verified email — which also triggers `owners.emails` owner auto-promotion through the shared verified-email hook. Existing-account sign-ins and account-linking flows request only `atproto`, because Chatto does not use PDS email in those paths.
- **No ATProto credentials are retained.** Once the callback has identified the user and fetched optional profile/email hints, Chatto deletes its local OAuth session copy. Chatto does not store ATProto tokens and cannot act on the user's behalf against their PDS. The PDS-side authorization grant is left active, but public/loopback clients may still be asked for consent on every sign-in by the PDS.
- **The resulting Chatto session is identical to any other.** Cookie + bearer token issued per FDR-023; from this point on the sign-in path is invisible to the rest of the application.

## Design Decisions

### 1. DID as the stable foreign key, handle as cached display

**Decision:** Store the user's DID as the primary link between an ATProto identity and a Chatto account. Re-resolve the handle on every sign-in for display purposes; don't persist it as an identity.
**Why:** Handles in ATProto can change (a user can move from `alice.bsky.social` to `alice.example.com` without losing their identity). The DID is the protocol's stable identifier. Persisting the handle as identity would silently break re-sign-in after a handle change.
**Tradeoff:** A handle-change between sign-ins means the user's Chatto login is stale until they update it through the normal rename flow. Acceptable — the alternative (renaming Chatto logins automatically on each sign-in) would be surprising and could cause its own confusion.

### 2. Handle-derived login with collision suffixing

**Decision:** On first sign-in, the shared external-identity confirmation screen proposes a Chatto login derived from the ATProto handle. The normal account-creation validation handles collisions and lets the user choose a different login before creation.
**Why:** Handles are already public, unique-within-PDS identifiers — perfect login material. Forcing the user to pick a fresh Chatto login at signup would be extra friction for no gain.
**Tradeoff:** The user sees one explicit confirmation step before landing in Chatto. This matches the rest of the external-provider model and avoids silent account creation or surprise login collisions.

### 3. Request `account:email` and gracefully handle denial

**Decision:** First-time account creation requests both `atproto` and `account:email`; existing-account sign-ins and account-linking flows request only `atproto`. On the PDS consent screen the user can grant or deny the email scope independently. If `account:email` is granted and the PDS-side email is confirmed, Chatto seeds the new account's verified-email list with it. If it's denied (or the email is unconfirmed, or the fetch fails), sign-in still succeeds — the user simply has no email on their Chatto account.
**Why:** Email is what makes `owners.emails` auto-promotion work uniformly across sign-in paths (password, OIDC/Goth providers, ATProto), but Chatto only consumes PDS email while creating a new account. Avoiding the email scope on later logins keeps the PDS consent screen from asking for an optional permission that cannot change the login result.
**Tradeoff:** Users who decline the email scope during account creation land with no `owners.emails` eligibility and no email-based notifications until they add one manually; that's their explicit choice, not Chatto's omission. If an existing ATProto-only user later wants to seed email from their PDS, they need a future explicit email-link flow rather than an ordinary login.

### 4. Profile mirroring on first sign-in only

**Decision:** Read `app.bsky.actor.profile` from the user's PDS once, on first sign-in, and copy display name + avatar into the Chatto profile. Don't re-sync on later sign-ins. Failures are logged and swallowed.
**Why:** Users expect their Chatto profile edits to stick — silently overwriting them on the next sign-in would be hostile. The first-sign-in copy gives ATProto users a head start (recognizable avatar, real name) instead of a blank-slate account; further changes are theirs to manage. Failure-as-silence handles users on PDSes without a bsky profile record (Frontpage, WhiteWind, …) without breaking sign-in.
**Tradeoff:** A user who changes their Bluesky display name won't see it propagate to Chatto. Considered correct — Chatto isn't a Bluesky mirror.

### 5. No ATProto tokens persist past sign-in

**Decision:** Delete Chatto's local OAuth session copy at the end of the callback, but do not revoke the PDS-side authorization grant.
**Why:** Phase 1 has no "act on the user's PDS" feature, so retaining access tokens, refresh tokens, or DPoP key material is pure attack surface. Not revoking the provider grant avoids needless destructive cleanup, but the ATProto authorization server still controls whether a public/loopback client must show consent again on later sign-ins. See ADR-051 for the general principle covering external identity providers.
**Tradeoff:** Disconnecting the Chatto identity link is local to Chatto; it does not revoke the OAuth grant listed at the user's PDS. Users who want to fully reset provider consent must revoke Chatto from their PDS account settings. Public/loopback clients may still show the PDS consent screen on every sign-in; avoiding that requires a separately designed confidential-client or retained-provider-session model. Any future feature that needs to write to a user's PDS will have to do its own separate OAuth flow with its own scopes.

### 6. ATProto sign-in only works at publicly-reachable or loopback URLs

**Decision:** The OAuth client uses `webserver.url` directly. When that URL is `127.0.0.1`/`localhost` (any port), the loopback dev-mode escape hatch kicks in; otherwise the deployment URL must be publicly reachable so the PDS can fetch the client metadata document. There is no third option.
**Why:** The constraint is baked into the ATProto OAuth spec. Encoding it directly in `webserver.url` matches how the rest of the codebase derives URLs and keeps the configuration surface minimal.
**Tradeoff:** Dev deployments on `*.orb.local`-style local-only hostnames can't use ATProto sign-in; operators have to either deploy publicly or develop against a loopback URL. The current `mise dev` setup uses loopback by default, so this is invisible to most local development.

## Related

- **ADRs:** ADR-051 (External Identity Integration Boundaries)
- **FDRs:** FDR-023 (Authentication & Sessions), FDR-022 (User Profile), FDR-018 (Account Lifecycle)

## Open Questions

- Whether to define a `run.chatto.*` lexicon for Chatto-specific profile data on the user's PDS (cross-server discovery, "find me on these other Chatto servers"). Out of scope for Phase 1.
- Account linking: the backend supports the same pending external-identity link flow as other providers, but the account-settings UI does not expose an ATProto handle-first link control yet.
