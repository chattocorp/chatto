# FDR-056: AT Protocol Sign-In

**Status:** Experimental
**Last reviewed:** 2026-11-12

## Overview

Users can sign in to Chatto with their AT Protocol identity, for example a
Bluesky account. Chatto does not show AT Protocol handles as verified. A
verified handle needs an independent issuer, which Chatto does not have yet
(ADR-123).

## Behavior

- An operator enables sign-in with the `atproto` provider type. No client
  registration or client secret is necessary. The default button label is
  "Internet Handle", because AT Protocol handles are not limited to Bluesky.
- The login page shows a handle field with the provider button. The server
  resolves the handle and sends the user to the authorization page of the
  user's PDS in a sign-in window. After approval, Chatto signs the user in or
  links the identity, as with other providers. A new account is confirmed in
  the window that started the sign-in. Without a handle, the user enters it on
  `bsky.social`.
- The linked external identity is the DID. A handle change does not change the
  link. With `manage_usernames`, a new account gets the handle as its username.
- A new account gets the display name and avatar of the user's public Bluesky
  profile (`app.bsky.actor.profile`) as suggestions, when the repository has
  one. Without it, the handle is the display name suggestion. A signed-in
  account without an avatar gets the profile avatar.
- A handle that cannot be resolved shows an explanation on the login page.
  Other failures show the general provider error.
- The device remembers the last handle that the user entered. The handle
  field suggests it. On another server that has an `atproto` provider, the
  sign-in screen offers **Continue as @handle**, which starts that server's
  sign-in with the handle filled in. An empty handle field removes the
  remembered handle.

## Design Decisions

### 1. Each server signs in with its own client and identity scope

**Decision:** A Chatto server signs users in with its own AT Protocol OAuth
client. It requests only the `atproto` scope and discards the tokens after it
verifies the DID.
**Why:** A sign-in succeeds only when the user approved this server's client
on the PDS. No other app or server can produce that proof. AT Protocol service
tokens do not name the app that requested them, so any app with broad PDS
access could use them to sign the user in to Chatto servers.
**Tradeoff:** Each server asks for approval once. Production servers have an
HTTPS client ID, so the PDS remembers the approval. Development servers use
the loopback client, for which the PDS asks every time.

### 2. No verified handles from repository records

**Decision:** Chatto does not publish the linked DID on profiles and does not
show handles as verified.
**Why:** A proof that the user writes to their own repository can also be
written by every app that has write access to that repository, for example
apps with app passwords. A modified server and one such app could link a
user's identity to a different account (ADR-123).
**Tradeoff:** Members cannot see which AT Protocol identity an account uses.

### 3. A remembered handle instead of a device session

**Decision:** The frontend remembers the last entered handle in browser
storage. It has no AT Protocol session of its own.
**Why:** **Continue as @handle** needs only a hint. The server's own sign-in
still proves the identity.
**Tradeoff:** A mistyped handle stays the suggestion until the user enters
another one.

### 4. Libraries

**Decision:** The server uses the `atproto/auth/oauth` package of Bluesky's
`indigo` Go module.
**Why:** It implements PAR, PKCE, DPoP, identity resolution, and issuer
verification.
**Tradeoff:** `indigo` is not stable (v0).

## Gates

- **Provider configuration:** an operator configures a provider with type
  `atproto`.
- **User consent:** a user approves each server's sign-in on the PDS.
- **Plain HTTP:** on `http://` development origins, the server uses the AT
  Protocol loopback client. Its callbacks return to `127.0.0.1`, and the server
  forwards them to the configured origin. The repository's development
  configuration enables an `atproto` provider.

## Privacy

- **Sign-in:** the server contacts the handle domain, the PLC directory for
  `did:plc` identities, the user's PDS, and its authorization server. From the
  PDS, it also reads the public profile record and the avatar image. Without a
  handle, it contacts `bsky.social`.
- **Remembered handle:** it stays in the browser's storage. The device sends
  it only to the server where the user signs in.
- **Logs:** handles, DIDs, and tokens identify users. The server does not log
  them.

## Related

- **ADRs:** ADR-071 (CIMD-identified OAuth clients), ADR-123 (verified AT
  Protocol handles need an independent issuer)
- **FDRs:** FDR-023 (Authentication & Sessions)

## Open Questions

- **Verified handles:** ADR-123 describes the requirements for an issuer.
- **Windowless sign-in:** AT Protocol service tokens do not name the app that
  requested them, so they are not safe for sign-in. A future protocol feature
  that binds a token to the server's OAuth client could remove the sign-in
  window.
- **Chatto Desktop and mobile:** examine whether the native hosts need their
  own handling of the sign-in window.
