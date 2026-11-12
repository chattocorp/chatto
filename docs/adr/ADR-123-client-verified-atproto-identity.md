# ADR-123: Client-Verified AT Protocol Identity

**Status:** Accepted
**Date:** 2026-11-11

## Context

Chatto servers are self-hosted. An operator can change a server, so a client
cannot trust a server's claim that a user controls an external identity. For
[FDR-056](../fdr/FDR-056-atproto-identity.md), a client must show an AT
Protocol handle as verified only when the user controls that identity.

An AT Protocol identity is a DID. The DID document names the user's PDS and
the handle. The PDS stores public, signed records in the user's repository.
Users do not hold their signing keys. The PDS holds them. Thus a user cannot
sign a Chatto-specific attestation directly. A record in the repository is the
available proof of control.

If a server can write to a user's repository, a modified server can write a
false proof. For example, the server could link the DID to a different
account. The proof is valid only if the verified party cannot write it.

## Decision

1. **Each server signs in with its own client and the identity scope only.** A
   Chatto server signs a user in with its own AT Protocol OAuth client and
   requests only the `atproto` scope. It verifies the DID and discards the
   tokens. It never gets write access to a user's repository.
2. **No shared sign-in tokens.** Chatto does not accept AT Protocol service
   tokens for sign-in. A service token does not name the app that requested
   it. Any app with broad PDS access, or the web app of a modified Chatto
   server, could otherwise sign the user in to other Chatto servers.
3. **The server publishes the sign-in DID.** The public profile reports the
   DID of the user's linked AT Protocol sign-in identity. The server never
   supplies a verification result.
4. **The client writes the proof.** The user's device has its own AT Protocol
   OAuth session for each server's app. The session may write only that
   server's collection of account link records. It writes one **account link
   record** for each linked Chatto account.
5. **Each server has its own collection.** The collection is
   `run.chatto.link.h` and 26 base32 characters of a SHA-256 hash of the
   server origin that the client uses. The record key is a hash of that origin
   and the Chatto user ID. The record contains no server name or user ID.
   Anyone who knows both values can find the record.
6. **The viewer's client verifies.** The viewer's client resolves the DID,
   examines the handle in both directions, and reads the record from the PDS.
   It shows the handle as verified only if all examinations succeed.

## Consequences

- A modified server cannot make a client show a false verified handle. A
  modified server can only omit or hide a link.
- The trust is as strong as the verifying client. If a viewer uses the web
  bundle of the server that they verify, that server controls the verifier.
  The guarantee applies to Chatto Desktop, and to a frontend that a different,
  trusted server serves (see ADR-025 and ADR-074).
- A forged record cannot help another account: a server publishes only a DID
  that it verified itself through sign-in.
- Writing a record needs the device's own AT Protocol session, one approval per
  server and device. Viewing and verifying a handle needs no session.
- AT Protocol does not record which app wrote a record. Any app with write
  access to a collection can write link records in it. Because each server has
  its own collection, one approval can write links for one server only. An
  approval in the app of a modified server affects only that server, which can
  already show false data to its own users. A future AT Protocol scope for one
  record key, or permissioned spaces, could narrow this further.
- Anyone can test a known server origin against the collection names in a
  public repository. This shows which public servers a user links. The origins
  of private servers stay hidden, because nobody can guess them.
- The app of one server cannot write records for another server. To verify a
  link on another server, the user opens that server's app.
- Each server asks for its own sign-in approval. The PDS remembers it for
  servers with an HTTPS client ID.
- The user's PDS is the trust root. A malicious PDS can write false records.
  This is the same trust that the AT Protocol network has in the PDS.
- Verification sends requests from the viewer's device to the PLC directory,
  a DNS-over-HTTPS resolver, the handle domain, and the profile owner's PDS. The profile owner chooses
  the PDS. Thus a profile owner can learn the IP addresses of viewers. The
  client must limit when it verifies and cache the results.
- Account link records stay in the repository until the user deletes them.
  When a user removes a link, the client must delete the record.
- The server needs no long-term AT Protocol keys or stored tokens.
