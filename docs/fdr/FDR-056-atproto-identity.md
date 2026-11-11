# FDR-056: AT Protocol Identity

**Status:** Experimental
**Last reviewed:** 2026-11-11

## Overview

Users can sign in to Chatto with their AT Protocol identity, for example a
Bluesky account. Users can also publish this identity on their profile. Other
users then see the AT Protocol handle as a verified handle. The viewer's
client verifies the handle itself and does not trust the server. A client
that knows the user's AT Protocol identity offers it for sign-in to other
servers.

The feature has three parts:

1. Sign-in on the server (an external provider).
2. Verified handles on profiles.
3. The device's own AT Protocol session, which writes link records and gives
   one-click sign-in.

## Behavior

### Sign-in

- An operator enables the provider with the `atproto` provider type. No client
  registration or client secret is necessary. The default button label is
  "Bluesky".
- The login page shows a handle field with the provider button. The server
  resolves the handle and sends the user to the authorization page of the
  user's PDS. After approval, Chatto signs the user in or links the identity,
  as with other providers. Without a handle, the user enters it on
  `bsky.social`.
- The linked external identity is the DID. A handle change does not change the
  link.
- With `manage_usernames`, a new account gets the handle as its username.
- A handle that cannot be resolved returns the user to the login page with an
  explanation.

### Verified handles

- In account settings, a user enters a handle and selects **Connect**. The
  device signs in to the user's PDS in a popup window. It writes an account
  link record to the user's repository, and then publishes the DID on the
  Chatto profile. The user approves the write access on the PDS
  authorization page.
- The settings panel shows whether other clients can verify the link.
- Profile cards and profile views show the handle with a check mark only
  after the viewer's client verified the link. An unverified claim shows
  nothing. The handle links to the Bluesky profile.
- **Remove** deletes the DID from the profile and the record from the
  repository. The device must have, or sign in to, a session for the
  published DID. A sign-in with a different AT Protocol account is rejected.
- The server reports the published DID as part of the public user resource.
  The server does not report a verification result.

### One-click sign-in

- The device keeps its AT Protocol session in browser storage. The handle
  field on login pages shows the handle of that session.
- When the device signs in to another server that has an `atproto` provider,
  the sign-in screen offers **Continue as @handle**. The server's own provider
  flow then starts with that handle, so the user usually only approves the
  sign-in on the PDS.

## Design Decisions

### 1. The server gets identity only

**Decision:** The server requests only the `atproto` scope. It discards the
tokens after it verifies the DID.
**Why:** A server with write access can write a false link record. See
ADR-123.
**Tradeoff:** The server cannot publish the link for the user. The device must
have its own AT Protocol session.

### 2. The link is a record in the user's repository

**Decision:** The proof is a record in the user's repository. A modified
server cannot write it.
**Why:** Users do not hold their signing keys, so they cannot sign an
attestation directly. A repository record is the available proof of control.
**Tradeoff:** The proof is public, and the user's PDS is the trust root.

### 3. The record key hides the server

**Decision:** The records are in the `run.chatto.accountLink` collection. The
record key is a SHA-256 hash of the server origin and the Chatto user ID. The
record contains only its type and creation time.
**Why:** Repository records are public and the network distributes them. A
plain record tells everyone which servers the user uses, including private
servers.
**Tradeoff:** The user's repository does not show linked servers in readable
form. The Chatto client must show the links.

### 4. The server origin is the origin that the client uses

**Decision:** The record key uses the origin through which the client reaches
the server. Normally this is the configured public URL (`webserver.url`).
**Why:** A modified server could report another server's origin and copy that
server's user IDs. The client trusts only the origin that it connects to.
**Tradeoff:** A client that uses a host name alias cannot verify links that
were written for the public URL.

### 5. The DID is a user-set profile claim

**Decision:** The published DID is a separate profile field. The user sets it
from the device, independent of the `atproto` sign-in provider.
**Why:** The server's opinion does not matter for verification. Users on
servers without the provider can still publish their identity, and users who
sign in with AT Protocol do not publish it unless they choose to.
**Tradeoff:** The sign-in link and the published DID can differ.

### 6. The viewer's client verifies

**Decision:** The viewer's client resolves the DID, examines the handle in both
directions, and reads the record directly from the user's PDS.
**Why:** The server can be modified. Only the viewer's own client can be
trusted. See ADR-123.
**Tradeoff:** The guarantee requires a trusted client: Chatto Desktop, or a
frontend that a different server serves. Verification also contacts external
systems from the viewer's device.

### 7. Verification on demand, with a cache

**Decision:** The client verifies when it shows a profile card or profile
view, not for each message author. It caches results for each DID and account
for ten minutes. It does not cache results of unavailable lookups.
**Why:** The profile owner chooses the PDS and can see the viewer's IP address.
On-demand checks limit this exposure and the request count.
**Tradeoff:** Message lists do not show a verified state.

### 8. The device requests only the link collection

**Decision:** The device requests the `atproto` scope and
`repo:run.chatto.accountLink`. It never requests `transition:generic`.
**Why:** `transition:generic` grants write access to the complete repository.
Verified handles do not justify that access.
**Tradeoff:** Users of a PDS without granular permissions cannot publish
links. The reference PDS supports granular permissions.

### 9. One-click sign-in uses the normal provider flow

**Decision:** The client starts the server's `atproto` provider with the known
handle as `login_hint`. The server's OAuth page passes the hint to its
provider.
**Why:** This needs no new protocol. The PDS session usually reduces the
sign-in to one approval.
**Tradeoff:** The user still sees the PDS authorization page.

### 10. Libraries

**Decision:** The server uses the `atproto/auth/oauth` package of Bluesky's
`indigo` Go module. The frontend uses `@atproto/oauth-client-browser` and
loads it only when the user connects an identity or a stored session exists.
**Why:** Both implement PAR, PKCE, DPoP, identity resolution, and issuer
verification. Bluesky maintains them.
**Tradeoff:** `indigo` is not stable (v0), and its API can change. The browser
library adds a large lazy chunk.

## Gates

- **Provider configuration:** an operator configures a provider with type
  `atproto` for sign-in. Verified handles need no server configuration.
- **User consent:** a user approves the device's write access on the PDS
  authorization page before the device can publish a link.
- **HTTPS:** the frontend publishes its AT Protocol client metadata only on
  HTTPS origins. On `http://` development origins, it uses the AT Protocol
  loopback client, which works only when the frontend is open on `127.0.0.1`.

## Privacy

- **Sign-in:** the server contacts the handle domain (DNS or HTTPS), the PLC
  directory for `did:plc` identities, the user's PDS, and its authorization
  server. Without a handle, it contacts `bsky.social`.
- **Linking:** the device contacts the same systems for the user's own
  identity.
- **Verification:** the viewer's device contacts the PLC directory or the
  `did:web` host, Cloudflare DNS over HTTPS (`cloudflare-dns.com`) and the
  handle domain, and the profile owner's PDS. This reveals the viewer's IP
  address to systems that the profile owner chooses. The requests send no
  cookies or referrer.
- **Link records:** they are public. They reveal that a link exists, but not
  the server or the account.
- **Logs:** handles and DIDs identify users. The server does not log them.

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
- **Service-token sign-in:** a server with a `did:web` identity could accept a
  short-lived service token from the user's PDS and skip the authorization
  page. This needs a server DID document and cluster-wide replay protection.
- **Chatto Desktop and mobile:** the native hosts need their own AT Protocol
  client identity. Until then, linking works in browsers only.
