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

1. **The server never gets PDS access.** A Chatto server runs no AT Protocol
   OAuth flow. The user's device signs in to the PDS with its own OAuth
   client. To sign in to a Chatto server, the device asks the PDS for a
   short-lived service token for the method `run.chatto.signIn`, addressed to
   the server's `did:web` identity with the `#chatto` service. The server
   verifies the signature against the DID document and accepts each token ID
   once across the cluster.
2. **The client chooses the audience.** The device derives the audience from
   the origin that it connects to, never from data that the server supplies.
   A server that could choose the audience could request a token for another
   server and use it there.
3. **The client writes the proof.** The device's OAuth session may write only
   the `run.chatto.accountLink` collection. After a sign-in or a link, it
   writes one **account link record** for the Chatto account.
4. **The record key identifies the account.** The record key is a hash of the
   server origin that the client uses and the Chatto user ID. The record
   contains no server name or user ID. Anyone who knows both values can find
   the record. Other readers of the public repository cannot learn which
   servers the user uses.
5. **The server publishes the sign-in DID.** The public profile reports the
   DID of the user's linked AT Protocol sign-in identity. The server never
   supplies a verification result.
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
- One device session signs the user in to every Chatto server and keeps the
  link records current. Viewing and verifying a handle needs no session.
- Clients without their own PDS session, such as bots, cannot sign in with an
  AT Protocol identity.
- AT Protocol allows ports in `did:web` only for `localhost`. HTTPS servers
  with a port cannot accept AT Protocol sign-in.
- The user's PDS is the trust root. A malicious PDS can write false records.
  This is the same trust that the AT Protocol network has in the PDS.
- Verification sends requests from the viewer's device to the PLC directory,
  a DNS-over-HTTPS resolver, the handle domain, and the profile owner's PDS. The profile owner chooses
  the PDS. Thus a profile owner can learn the IP addresses of viewers. The
  client must limit when it verifies and cache the results.
- Account link records stay in the repository until the user deletes them.
  When a user removes a link, the client must delete the record.
- The server needs no AT Protocol keys or stored OAuth tokens. It needs a
  `did:web` document and short-lived replay records in runtime state.
