# ADR-125: Identity Claims with Separate Schemes and Attesters

**Status:** Accepted
**Date:** 2026-11-18

## Context

Members want to see which external identities an account controls, for
example an AT Protocol handle, a domain, or a fediverse account. Chatto starts
with AT Protocol handles. The server already checks the DID and the handle at
each AT Protocol sign-in
([FDR-058](../fdr/FDR-058-atproto-sign-in.md)).

Each kind of identity has its own proof method. A sign-in with the provider,
a DNS record, a backlink on a profile page, and a signed proof are all
possible. If each kind of identity also defined its own trust statement, the
UI would show many different ideas of "verified". Members could not compare
them, and each new kind of identity would need a new security review of the
UI.

[ADR-124](ADR-124-atproto-handle-verification-needs-an-issuer.md) shows that a
server's claim alone is not verification, because an operator can change a
server. Only an independent issuer can make a verification that a modified
server cannot fake.

## Decision

1. **Identity claims.** An account has a list of identity claims. Each claim
   is one external identity that the account holder has shown they control.
   A claim has a scheme, a display value, an optional link, the time of the
   last check, and an attestation.
2. **Three separate axes.**
   - The **scheme** tells which kind of identity it is, for example `atproto`.
   - The **method** tells how the account holder proved control, for example
     a sign-in with the provider.
   - The **attester** tells who vouches for the claim.
3. **Only the attester controls trust.** Clients make the trust statement
   from the attestation only, never from the scheme. Two claims with the same
   attester get the same statement, whatever their schemes are.
4. **Open schemes, closed attestations.** Schemes are an open set of string
   identifiers. A client shows a claim with an unknown scheme as a plain row
   with its display value and link. Attestations are a closed set. A client
   shows a claim with an unknown attestation with the statement for a server
   check, because the server published it. A client never shows a
   verification mark for an attestation that it does not know.
5. **Server checks first.** The first attestation is a server check. Its
   statement says that this server checked the identity, and when. It has no
   verification mark. A later issuer attestation follows ADR-124 and #3604.
   Only that attestation can get a verification mark, and only when the
   viewer's client examines it against the viewer's own list of issuers.
6. **Client proofs later.** The intended direction is that the viewer's
   client examines claims itself. Then a modified server cannot fake a claim.
   Some schemes, for example DNS records and profile backlinks, permit this
   without an issuer. Clients do not do it in the first version. A client
   proof needs its own design for the external systems that each viewer's
   device contacts, and for the clients that can make the guarantee
   (ADR-124). The attestation set gets a client-proof kind for it. The
   decisions above must not prevent this addition.
7. **No published subject identifiers.** A claim contains the display value,
   not the stable subject identifier, such as a DID or an OAuth subject. The
   server keeps the subject identifier private. This replaces ADR-124
   decision 2: servers now publish the checked handle, but still not the DID.
8. **Opt-in visibility for each claim.** A new claim is hidden. The account
   holder selects which claims other people can see. The account holder
   always sees all of their own claims.
9. **Verifiers on the server.** Each scheme has a verifier on the server. The
   verifier records claim facts on the user aggregate: a claim was checked, a
   claim was removed, or the visibility of a claim changed. A claim changes
   only when its verifier runs again. When the account holder removes the
   linked identity, the server removes the claim.
10. **Shared client vocabulary.** `@chatto/client` contains the scheme
    registry (labels and icons) and the trust statements. The bundled
    frontend, bots, and other frontends then describe claims in the same way
    (ADR-111).

## Consequences

- A new scheme needs a verifier and a registry entry. It does not need a new
  trust design or a new UI.
- The trust in a server check is only the trust in the server. The wording
  must say this. The UI must not show a mark that suggests independent
  verification.
- A displayed value can be out of date until the verifier runs again. The UI
  shows the time of the last check.
- Members can see the AT Protocol handle of an account when its holder makes
  the claim visible.
- Sign-in providers with a public handle, for example GitHub and GitLab, can
  become server-checked schemes with little work. Providers that have only an
  opaque subject identifier cannot.
- The issuer attestation of #3604 must define what it reveals to viewers. A
  signed statement that contains the DID makes the DID public.

## Related

- [ADR-111: Move the Client State Layer into `@chatto/client`](ADR-111-move-client-state-into-chatto-client.md)
- [ADR-124: Verified AT Protocol Handles Need an Independent Issuer](ADR-124-atproto-handle-verification-needs-an-issuer.md)
- [FDR-058: AT Protocol Sign-In](../fdr/FDR-058-atproto-sign-in.md)
- [FDR-059: Identity Claims](../fdr/FDR-059-identity-claims.md)
