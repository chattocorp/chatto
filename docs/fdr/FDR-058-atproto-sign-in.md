# FDR-058: AT Protocol Sign-In

**Status:** Experimental
**Last reviewed:** 2026-11-12

## Overview

Users can sign in to Chatto with their AT Protocol identity, for example a
Bluesky account. The server reports the linked handle as a profile identity.
Independent verification by the viewer requires an issuer (ADR-124).

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
  A handle that is not a valid username, for example one longer than the
  configured username limit, then prevents account creation, as for other providers.
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

**Decision:** Chatto keeps the linked DID private. The profile shows a handle
that the server resolved from the linked sign-in identity.
**Why:** A proof that the user writes to their own repository can also be
written by every app that has write access to that repository, for example
apps with app passwords. A modified server and one such app could link a
user's identity to a different account (ADR-124).
**Profile display:** The profile card and full profile show the handles of linked
AT Protocol accounts to other server members. The card shows the pills between the identity header and the bio.
The profile sidebar uses a collapsible **Verified identities** section before **About** and remembers its state per server. Both views omit the
section when the account has no available identities. Each handle appears in a green
self-verification pill. A 1px gap separates the @ icon from the handle.
The pill stays on one line. If the handle is too long, an ellipsis marks
the omitted text. Hovering over the pill shows the full handle. The
label reflects the server operator’s verification, including links the operator
creates after checking account ownership (ADR-124). The server resolves each linked
DID when a client requests verified identities. The AT Protocol entry uses
the method key `atproto` and the handle as its public value. The directory caches successful lookups
for up to a day, so a renamed handle can take time to change. A missing handle
or failed lookup leaves that handle out. Disconnecting the account removes
its handle from subsequent server reads. The popup and sidebar share a session
cache. Reopening either view shows cached identities immediately. After 30 seconds,
reopening also refreshes them in the background. The cache drops unused results
after five minutes.
**Account settings:** The Single Sign-On panel shows the handle of a linked AT
Protocol account next to its **Linked** status. `ListExternalIdentities`
returns it as `account_label`, from the same directory lookup as the profile.
A missing handle or failed lookup leaves the label empty. Other provider types
store only an opaque subject, so their label stays empty.

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

- **Independent verification:** A future client can use evidence from a trusted
  third party to verify an account link. ADR-124 describes the issuer requirements.
- **Profile visibility:** A later version should let members opt in to showing
  a linked identity. This version displays eligible identities automatically.
- **Windowless sign-in:** AT Protocol service tokens do not name the app that
  requested them, so they are not safe for sign-in. A future protocol feature
  that binds a token to the server's OAuth client could remove the sign-in
  window.
- **Chatto Desktop and mobile:** examine whether the native hosts need their
  own handling of the sign-in window.

### Configured owners without password login

`owners.dids` grants the owner role to a matching DID from a configured AT Protocol provider. The stored issuer is the provider ID and the subject is the DID. Handles do not match. Like `owners.emails`, this policy grants roles at creation, linking, and startup; removal never revokes a role. Link grants use the durable notification-effects lane and wait for both its acknowledgement and the local RBAC projection.

When `auth.direct_login` is false and either owner list is non-empty, setup discovery returns false without changing durable setup state. Normal provider admission still applies, including `auto_provision` and invitation rules.

Automatic setup skipping does not add an owner welcome, activate privileged mode, or write setup-completion events during sign-in. The existing browser-wizard welcome remains unchanged. A separate feature can define onboarding for configured owners.
