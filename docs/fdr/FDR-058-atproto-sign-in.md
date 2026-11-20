# FDR-058: AT Protocol Sign-In

**Status:** Experimental
**Last reviewed:** 2026-11-12

## Overview

Users can sign in to Chatto with their AT Protocol identity, for example a
Bluesky account. Chatto does not show AT Protocol handles as verified. A
verified handle needs an independent issuer, which Chatto does not have yet
(ADR-124).

## Behavior

- An operator enables sign-in with the `atproto` provider type. No client
  registration or client secret is necessary. The default button label is
  "Internet Handle", because AT Protocol handles are not limited to Bluesky.
- The login page shows the provider like other providers. The sign-in window
  first asks for the handle, on Chatto's handle page. The server then resolves
  the handle and sends the user to the authorization page of the user's PDS.
  After approval, Chatto signs the user in or links the identity, as with
  other providers. A new account is confirmed in the window that started the
  sign-in.
- The linked external identity is the DID. A handle change does not change the
  link. With `manage_usernames`, a new account gets the handle as its username.
  A handle that is not a valid username, for example one longer than 32
  characters, then prevents account creation, as for other providers.
- A new account gets the display name and avatar of the user's public Bluesky
  profile (`app.bsky.actor.profile`) as suggestions, when the repository has
  one. Without it, the handle is the display name suggestion. A signed-in
  account without an avatar gets the profile avatar.
- A handle that cannot be resolved returns to the handle page with an
  explanation. Other failures show the general provider error.
- After a successful sign-in, the device remembers the handle. The handle
  page suggests it. On another server that has an `atproto` provider, the
  sign-in screen offers **Continue as @handle**, which starts that server's
  sign-in with the handle, without the handle page. **Use another handle**
  forgets it.

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
user's identity to a different account (ADR-124).
**Profile display:** The profile card and full profile show the handles of linked
AT Protocol accounts to other server members. The card lists them in **Verified identities**
between the identity header and the bio. The profile sidebar uses a collapsible
section before **About** and remembers its state per server. Both views omit the
section when the account has no available identities. Each handle appears in a green
self-verification pill. A 1px gap separates the @ icon from the handle.
The pill stays on one line. If the handle is too long, an ellipsis marks
the omitted text. Hovering over the pill shows the full handle. The
label identifies the server-linked account (ADR-124). The server resolves each linked
DID when a client requests handles. The directory caches successful lookups
for up to a day, so a renamed handle can take time to change. A missing handle
or failed lookup leaves that handle out. Disconnecting the account removes
its handle from subsequent reads.

### 3. A remembered handle instead of a device session

**Decision:** The frontend remembers the handle of the last successful
sign-in in browser storage. It has no AT Protocol session of its own.
**Why:** **Continue as @handle** needs only a hint. The server's own sign-in
still proves the identity. A failed sign-in does not replace the hint.
**Tradeoff:** The next person on a shared browser sees the handle until
someone selects **Use another handle**.

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
  PDS, it also reads the public profile record and the avatar image.
- **Profile reads:** On a cache miss, the server contacts the identity directory and handle domain. Those services see the server’s address. The viewer connects only to Chatto.
- **Remembered handle:** it stays in the browser's storage. The device sends
  it only to the server where the user signs in.
- **Logs:** handles, DIDs, and tokens identify users. The server does not log
  them.

## Related

- **ADRs:** ADR-071 (CIMD-identified OAuth clients), ADR-124 (verified AT
  Protocol handles need an independent issuer)
- **FDRs:** FDR-023 (Authentication & Sessions)

## Open Questions

- **Verified handles:** ADR-124 describes the requirements for an issuer.
- **Windowless sign-in:** AT Protocol service tokens do not name the app that
  requested them, so they are not safe for sign-in. A future protocol feature
  that binds a token to the server's OAuth client could remove the sign-in
  window.
- **Chatto Desktop and mobile:** examine whether the native hosts need their
  own handling of the sign-in window.
