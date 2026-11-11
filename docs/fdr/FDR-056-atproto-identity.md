# FDR-056: AT Protocol Identity

**Status:** Experimental
**Last reviewed:** 2026-11-11

## Overview

Users can sign in to Chatto with their AT Protocol identity, for example a
Bluesky account. The identity that a user links for sign-in is also the
identity on their profile: other users see the AT Protocol handle as a
verified handle. The viewer's client verifies the handle itself and does not
trust the server.

## Behavior

### Sign-in

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
- On another server that has an `atproto` provider, the sign-in screen offers
  **Continue as @handle** for the handle of the device's AT Protocol session.
  It starts that server's sign-in with the handle filled in.

### Verified handles

- The DID of the linked AT Protocol sign-in identity is the user's published
  DID. Linking it publishes it; disconnecting it removes it. There is no
  separate setting.
- Profile cards, profile views, and the current-user card mark a verified
  identity only after the
  viewer's client verified the link. When the verified handle equals the
  username, the `@` before the username shows in the accent colour. Otherwise
  a separate line shows the handle, linked to the profile on
  Bluesky. An unverified claim shows nothing.
- In account settings, the linked identity shows its verification state. When
  other clients cannot verify it, **Verify** signs the device in to the PDS and
  writes the account link record. Each device needs this approval once.
- After an account creation, a device with a session for the same DID writes
  the record without a prompt.
- Disconnecting the identity also deletes the record, when the device has a
  session for that DID.

## Design Decisions

### 1. Each server signs in with its own client and identity scope

**Decision:** A Chatto server signs users in with its own AT Protocol OAuth
client. It requests only the `atproto` scope and discards the tokens after it
verifies the DID.
**Why:** A sign-in succeeds only when the user approved this server's client
on the PDS. No other app, server, or Chatto web app can produce that proof.
Shared sign-in tokens, for example AT Protocol service tokens, would let any
app with broad PDS access, or the web app of a modified server, sign the user
in to other Chatto servers. A server with write access could also write a
false link record (ADR-123).
**Tradeoff:** Each server asks for approval once. Production servers have an
HTTPS client ID, so the PDS remembers the approval; development servers use
the loopback client, for which the PDS asks every time.

### 2. The published DID follows the sign-in link

**Decision:** The public `atproto_did` of a user is the DID of their linked
`atproto` sign-in identity.
**Why:** Two settings for one identity confuse users. A server publishes only
a DID that it verified itself, so a record that another app wrote for another
account cannot make that account show the user's handle.
**Tradeoff:** A user cannot sign in with an AT Protocol identity without
publishing it on the server.

### 3. The device writes the proof, with a narrow scope

**Decision:** The device has its own AT Protocol session with the scopes
`atproto` and `repo:run.chatto.accountLink`. It uses the session only to write
and delete link records. It never requests `transition:generic`.
**Why:** Only the DID's owner can write the record, so a modified server
cannot forge it. The scope cannot change other data.
**Tradeoff:** Each device needs one approval before it can write records.
Users of a PDS without granular permissions cannot publish verified handles.
The reference PDS supports granular permissions.

### 4. The record key hides the server

**Decision:** The records are in the `run.chatto.accountLink` collection. The
record key is a SHA-256 hash of the server origin that the client uses and the
Chatto user ID. The record contains only its type and creation time.
**Why:** Repository records are public. A plain record tells everyone which
servers the user uses, including private servers. The client uses its own
origin, so a server cannot report another server's origin.
**Tradeoff:** A client that uses a host name alias cannot verify links that
were written for the public URL.

### 5. The viewer's client verifies, on demand

**Decision:** The viewer's client resolves the DID, examines the handle in both
directions, and reads the record directly from the user's PDS. It verifies
when it shows a profile card, a profile view, or the current-user card, and
caches results for ten minutes.
**Why:** The server can be modified; only the viewer's own client can be
trusted. The profile owner chooses the PDS and can see viewers' IP addresses,
so verification stays rare. The current-user card verifies on each app load,
but only the user's own identity.
**Tradeoff:** The guarantee requires a trusted client: Chatto Desktop, or a
frontend that a different server serves. Message lists show no verified state.

### 6. Libraries

**Decision:** The server uses the `atproto/auth/oauth` package of Bluesky's
`indigo` Go module. The frontend uses `@atproto/oauth-client-browser` and
loads it only when it shows the **Verify** action or a session exists.
**Why:** Both implement PAR, PKCE, DPoP, identity resolution, and issuer
verification.
**Tradeoff:** `indigo` is not stable (v0).

## Gates

- **Provider configuration:** an operator configures a provider with type
  `atproto`.
- **User consent:** a user approves the server's sign-in on the PDS, and the
  device's record access once per device.
- **Plain HTTP:** on `http://` development origins, the server and the
  frontend use the AT Protocol loopback client. Its callbacks return to
  `127.0.0.1`, and the server forwards them to the configured origin. The
  repository's development configuration enables an `atproto` provider.

## Privacy

- **Sign-in:** the server contacts the handle domain, the PLC directory for
  `did:plc` identities, the user's PDS, and its authorization server. From the
  PDS, it also reads the public profile record and the avatar image. Without a
  handle, it contacts `bsky.social`.
- **Verify:** the device contacts the same systems for the user's own identity.
- **Verification:** the viewer's device contacts the PLC directory or the
  `did:web` host, Cloudflare DNS over HTTPS (`cloudflare-dns.com`) and the
  handle domain, and the profile owner's PDS. This reveals the viewer's IP
  address to systems that the profile owner chooses. The requests send no
  cookies or referrer.
- **Link records:** they are public. They reveal that a link exists, but not
  the server or the account.
- **Logs:** handles, DIDs, and tokens identify users. The server does not log
  them.

## Related

- **ADRs:** ADR-025 (multi-server client), ADR-071 (CIMD-identified OAuth
  clients), ADR-074 (device-local server catalogue), ADR-101 (shared client
  user profiles), ADR-123 (client-verified AT Protocol identity)
- **FDRs:** FDR-022 (User Profile), FDR-023 (Authentication & Sessions),
  FDR-034 (Chatto Desktop)

## Open Questions

- **Other PDS implementations:** examine which PDS implementations other than
  the reference PDS grant granular permissions.
- **Viewer IP exposure:** Chatto has no home server for each user, so a viewer's
  client contacts the external systems directly. On-demand verification and the
  cache limit this exposure but do not remove it.
- **Windowless sign-in:** AT Protocol service tokens do not name the app that
  requested them, so they are not safe for sign-in. A future protocol feature
  that binds a token to the server's OAuth client could remove the sign-in
  window.
- **Chatto Desktop and mobile:** the native hosts need their own AT Protocol
  client identity before they can verify links.
