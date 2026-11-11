# FDR-056: AT Protocol Identity

**Status:** Experimental
**Last reviewed:** 2026-11-11

## Overview

Users can sign in to Chatto with their AT Protocol identity, for example a
Bluesky account. The identity that a user links for sign-in is also the
identity on their profile: other users see the AT Protocol handle as a
verified handle. The viewer's client verifies the handle itself and does not
trust the server. A device that is signed in to the user's PDS signs the user
in to every Chatto server that supports AT Protocol sign-in, usually without
a new approval.

## Behavior

### Sign-in

- An operator enables sign-in with the `atproto` provider type. No client
  registration or client secret is necessary. The default button label is
  "Internet Handle", because AT Protocol handles are not limited to Bluesky.
- The login page shows a handle field with the provider button. The device
  signs in to the user's PDS in a popup window, if it is not signed in yet.
  Then it asks the PDS for a short-lived sign-in token for this server and
  sends it to the server.
- The server verifies the token. A known identity signs the user in. A new
  identity continues with the account confirmation in the same window, when
  the provider creates accounts.
- The linked external identity is the DID. A handle change does not change the
  link. With `manage_usernames`, a new account gets the handle as its username.
- A handle that cannot be resolved shows an explanation on the login page.

### Verified handles

- The DID of the linked AT Protocol sign-in identity is the user's published
  DID. Linking it publishes it; disconnecting it removes it. There is no
  separate setting.
- After a sign-in, an account creation, or a link, the device writes an account
  link record to the user's repository. The record lets other clients verify
  the handle.
- Profile cards and profile views show the handle with a check mark only after
  the viewer's client verified the link. An unverified claim shows nothing.
  The handle links to the profile on Bluesky.
- In account settings, the linked identity shows its verification state. When
  other clients cannot verify it, for example for a link from another device,
  **Verify** writes the record.
- Disconnecting the identity also deletes the record, when the device has a
  session for that DID.

### Linking from settings

- **Link** for the `atproto` provider signs the device in to the PDS, checks
  recent authentication like other providers, and links the identity. No
  provider window opens for the Chatto server.

### One-click sign-in on other servers

- When the device signs in to another server that has an `atproto` provider,
  the sign-in screen offers **Continue as @handle**. The device asks the PDS
  for a token for that server and receives a session without a window.
- When no account on that server uses the identity yet, the server's own
  sign-in window opens, where the user can create one.

## Design Decisions

### 1. The device signs in; servers accept service tokens

**Decision:** The device signs in to the PDS with its own OAuth client. A
Chatto server never runs an OAuth flow and receives no PDS access. It accepts a
service token that the PDS signs with the account's key, addressed to the
server's `did:web` identity, for the method `run.chatto.signIn`.
**Why:** The server must not be able to write the user's link record (ADR-123).
One device session gives the device everything: sign-in on any server, link
records, and the identity for one-click sign-in.
**Tradeoff:** Clients other than the Chatto app, such as bots, need their own
PDS session to sign in with an AT Protocol identity.

### 2. The device asks for narrow permissions

**Decision:** The device requests `atproto`, `repo:run.chatto.accountLink`, and
`rpc:run.chatto.signIn?aud=*`. It never requests `transition:generic`.
**Why:** These permissions cover identity, link records, and sign-in tokens for
any Chatto server, and nothing else. `aud=*` is valid because the method is
named.
**Tradeoff:** Users of a PDS without granular permissions cannot use the
feature. The reference PDS supports granular permissions.

### 3. The client derives the token audience

**Decision:** The device derives the audience from the origin that it connects
to: `did:web:<host>#chatto` for HTTPS without a port, and
`did:web:localhost%3A<port>#chatto` for plain-HTTP local development. The
server derives the same value from its configured public URL.
**Why:** A server that could choose the audience could request a token for
another server and use it there.
**Tradeoff:** HTTPS servers with a port, and clients that use a host name
alias, cannot use AT Protocol sign-in. AT Protocol allows ports in `did:web`
only for `localhost`.

### 4. Tokens are short-lived and single-use

**Decision:** The device requests tokens that expire after 60 seconds. The
server rejects tokens that live longer than two minutes and records each token
ID across the cluster until it expires.
**Why:** A token that leaks, for example from a log, cannot be used again.
**Tradeoff:** One record in runtime state for each sign-in.

### 5. The published DID follows the sign-in link

**Decision:** The public `atproto_did` of a user is the DID of their linked
`atproto` sign-in identity.
**Why:** Two settings for one identity confuse users, and publishing another
identity than the sign-in identity has no use.
**Tradeoff:** A user cannot sign in with an AT Protocol identity without
publishing it on the server.

### 6. The link is a record in the user's repository

**Decision:** The proof is a record in the user's repository, which only the
DID's owner can write.
**Why:** Users do not hold their signing keys, so they cannot sign an
attestation directly. A repository record is the available proof of control.
**Tradeoff:** The proof is public, and the user's PDS is the trust root.

### 7. The record key hides the server

**Decision:** The records are in the `run.chatto.accountLink` collection. The
record key is a SHA-256 hash of the server origin that the client uses and the
Chatto user ID. The record contains only its type and creation time.
**Why:** Repository records are public. A plain record tells everyone which
servers the user uses, including private servers.
**Tradeoff:** A client that uses a host name alias cannot verify links that
were written for the public URL.

### 8. The viewer's client verifies, on demand

**Decision:** The viewer's client resolves the DID, examines the handle in both
directions, and reads the record directly from the user's PDS. It verifies
when it shows a profile card or profile view and caches results for ten
minutes.
**Why:** The server can be modified; only the viewer's own client can be
trusted. The profile owner chooses the PDS and can see viewers' IP addresses,
so verification stays rare.
**Tradeoff:** The guarantee requires a trusted client: Chatto Desktop, or a
frontend that a different server serves. Message lists show no verified state.

### 9. Libraries

**Decision:** The server verifies tokens with the `atproto/auth` and
`atproto/identity` packages of Bluesky's `indigo` Go module. The frontend uses
`@atproto/oauth-client-browser` and loads it only where AT Protocol sign-in is
offered or a session exists.
**Why:** Both implement the AT Protocol rules for keys, DIDs, and OAuth.
**Tradeoff:** `indigo` is not stable (v0).

## Gates

- **Provider configuration:** an operator configures a provider with type
  `atproto`. The server needs an HTTPS `webserver.url` without a port, or a
  plain-HTTP local development URL.
- **User consent:** a user approves the device's permissions on the PDS
  authorization page once per device.
- **HTTPS:** the frontend publishes its AT Protocol client metadata only on
  HTTPS origins. On `http://` development origins, it uses the AT Protocol
  loopback client. Its callbacks return to `127.0.0.1`, and the server
  forwards them to the configured origin. The repository's development
  configuration enables an `atproto` provider.

## Privacy

- **Sign-in:** the device contacts the handle domain, the PLC directory for
  `did:plc` identities, the user's PDS, and its authorization server. The
  server contacts only the PLC directory or the `did:web` host, to verify the
  token and the handle.
- **Verification:** the viewer's device contacts the PLC directory or the
  `did:web` host, Cloudflare DNS over HTTPS (`cloudflare-dns.com`) and the
  handle domain, and the profile owner's PDS. This reveals the viewer's IP
  address to systems that the profile owner chooses. The requests send no
  cookies or referrer.
- **Link records:** they are public. They reveal that a link exists, but not
  the server or the account.
- **Logs:** handles, DIDs, and tokens identify users. The server logs only
  rejection reasons.

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
- **Chatto Desktop and mobile:** the native hosts need their own AT Protocol
  client identity. Until then, AT Protocol sign-in works in browsers only.
