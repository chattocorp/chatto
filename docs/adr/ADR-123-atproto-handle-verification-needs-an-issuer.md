# ADR-123: Verified AT Protocol Handles Need an Independent Issuer

**Status:** Accepted
**Date:** 2026-11-12

## Context

Users can sign in with an AT Protocol identity
([FDR-056](../fdr/FDR-056-atproto-identity.md)). Members would like to see that
an account belongs to a known AT Protocol handle. Chatto servers are
self-hosted, and an operator can change a server. Thus a client must not
trust a server's claim that a user controls an identity.

We examined a proof that the user writes to their own AT Protocol repository:

1. The server publishes the DID of the linked sign-in identity.
2. The user's device writes an account link record to the user's repository.
   The record key is a hash of the server origin and the Chatto user ID.
3. The viewer's client resolves the DID, examines the handle in both
   directions, and reads the record from the user's PDS.

A modified server cannot write to the user's repository, so it cannot write
the record itself. But AT Protocol does not record which app wrote a record.
Every app with write access to the repository can write the record. This
includes apps that the user approved for broad access, and apps that use app
passwords. Many users approve apps without reading the consent screen. A
modified server and one such app can link a user's identity to a different
account, and every client then shows that link as verified.

Narrower designs do not remove this:

- A collection for each server limits an honest app's session. But each app
  chooses its own scopes, so the app of a modified server can ask for any
  collection.
- AT Protocol scopes cannot name a single record key, and they cannot name a
  collection prefix.
- Permissioned spaces (proposal 0016) can limit a scope to one space. But they
  give read access only to members, and verification needs public reads. They
  are also not ready for production.

## Decision

1. **No verified handles from repository records.** Chatto does not show an
   AT Protocol handle as verified based on records in the user's repository.
2. **No published DID for now.** Servers do not publish the DID of the linked
   sign-in identity. A future verification design defines what a server
   publishes.
3. **Verification needs an independent issuer.** A future design must use an
   attestation that only a trusted issuer can make, for example a signature
   with the issuer's key. The issuer makes it only after the DID's owner signs
   in to the issuer and confirms the account on the issuer's own page. The
   viewer's client accepts only issuers from its own list. A server must not
   add issuers to that list.

## Consequences

- AT Protocol sign-in works without a published identity. Members do not see
  which AT Protocol identity an account uses.
- A verified handle needs a service that Chatto or another trusted party
  operates. That service learns which accounts its users link.
- The trust in a verified handle also depends on the verifying client. A
  modified server controls the web app that it serves. A guarantee needs
  Chatto Desktop, or a frontend that a different, trusted server serves.
- Repository records can still be useful as a weaker signal, but a client must
  not present them as verification.

## Related

- [FDR-056: AT Protocol Sign-In](../fdr/FDR-056-atproto-identity.md)
- [ADR-025: Multi-Server Client Architecture](ADR-025-multi-instance-client-architecture.md)
- [ADR-071: Identify Open OAuth Clients through CIMD](ADR-071-cimd-identified-open-oauth-clients.md)
