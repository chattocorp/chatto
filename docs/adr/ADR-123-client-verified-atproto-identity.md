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

1. **The server gets identity only.** A Chatto server that signs a user in
   with AT Protocol OAuth requests only the `atproto` scope. The server
   verifies the DID and discards the tokens. It never gets write access to a
   user's repository.
2. **The client writes the proof.** The user's Chatto client has its own AT
   Protocol OAuth session. It requests write access to the Chatto link
   collection only. It writes one **account link record** for each linked
   Chatto account in the `run.chatto.accountLink` collection.
3. **The record key identifies the account.** The record key is a hash of the
   server origin and the Chatto user ID. The server origin is the origin
   through which the client reaches the server, normally the configured public
   URL. The client does not accept an origin that the server reports. The
   record contains no
   server name or user ID. Anyone who knows both values can find the record.
   Other readers of the public repository cannot learn which servers the user
   uses.
4. **The viewer's client verifies.** The server reports only the DID. The
   viewer's client resolves the DID, examines the handle in both directions,
   and reads the record from the PDS. It shows the handle as verified only if
   all examinations succeed. The server never supplies a verification result.
5. **Service authentication can give one-click sign-in later.** A client with an AT
   Protocol session can get a short-lived service token from the user's PDS.
   The PDS signs the token with the account's signing key. A Chatto server
   that has a `did:web` identity accepts this token as proof of the DID. The
   server verifies the signature against the DID document. It records each
   token ID across the cluster to prevent replay.

## Consequences

- A modified server cannot make a client show a false verified handle. A
  modified server can only omit or hide a link.
- The trust is as strong as the verifying client. If a viewer uses the web
  bundle of the server that they verify, that server controls the verifier.
  The guarantee applies to Chatto Desktop, and to a frontend that a different,
  trusted server serves (see ADR-025 and ADR-074).
- Publishing a verified handle needs the device's own AT Protocol session, in
  addition to any server sign-in. Viewing and verifying a handle needs no
  session.
- The user's PDS is the trust root. A malicious PDS can write false records.
  This is the same trust that the AT Protocol network has in the PDS.
- Verification sends requests from the viewer's device to the PLC directory,
  a DNS-over-HTTPS resolver, the handle domain, and the profile owner's PDS. The profile owner chooses
  the PDS. Thus a profile owner can learn the IP addresses of viewers. The
  client must limit when it verifies and cache the results.
- Account link records stay in the repository until the user deletes them.
  When a user removes a link, the client must delete the record.
- The server needs no long-term AT Protocol keys or stored tokens for
  sign-in. Service-token sign-in adds a `did:web` document and replay state in
  NATS.
