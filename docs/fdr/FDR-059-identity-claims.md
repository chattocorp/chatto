# FDR-059: Identity Claims

**Status:** Planned
**Last reviewed:** 2026-11-18

## Overview

Members can show which external identities their account controls, for
example their AT Protocol handle. Each identity is an identity claim. The
profile shows who vouches for each claim. At first, only the server vouches.
Chatto then does not show the claim as verified (ADR-124, ADR-125).

## Behavior

- The Profile Card and the profile view show an **Identities** section. It
  has one row for each visible claim. A row shows the icon and the label of
  the scheme, the display value, and a trust statement.
- For a claim that the server checked, the statement is, for example,
  "Checked by this server at sign-in · 12 Nov". The row has no check mark or
  other verification mark.
- The display value can open the external profile, for example the Bluesky
  profile of a handle. The link opens only when the viewer selects it. The
  UI shows that the link goes to an external site.
- When the login is the same as the display value of a claim, the profile does
  not show the value two times. It shows the trust statement below the login.
- The first scheme is the AT Protocol handle. The server makes or updates the
  claim at each AT Protocol sign-in. When the account holder removes the
  linked AT Protocol identity, the server removes the claim.
- A new claim is hidden. When the server makes a claim, the account holder can
  make it visible. Profile settings list all claims of the account and let the
  account holder show or hide each claim.
- Other members, bots, and API clients see only visible claims. The account
  holder always sees all of their own claims.
- A claim with a scheme that the client does not know shows as a plain row
  with its display value and link.
- Viewing a profile does not send requests to external systems.

## Design Decisions

### 1. Trust comes from the attester, not from the scheme

**Decision:** The trust statement depends only on who vouches for the claim.
**Why:** Members can compare claims of different schemes. A new scheme does
not need a new trust design (ADR-125).
**Tradeoff:** A scheme with a stronger proof method gets no stronger
statement until an attester exists for it.

### 2. No verification mark for server checks

**Decision:** Claims that the server checked have a statement, not a mark.
**Why:** An operator can change a server. A mark would suggest independent
verification (ADR-124).
**Tradeoff:** Members can see a claim but cannot be sure of it.

### 3. Hidden by default

**Decision:** The account holder makes each claim visible.
**Why:** A claim links the account to an identity outside the server. The
account holder must decide this.
**Tradeoff:** Fewer profiles show claims.

### 4. The display value only, with the time of the last check

**Decision:** Profiles show the display value and the time of the last check.
They do not show DIDs or other subject identifiers.
**Why:** A handle can change, and the server examines it again only at the
next check. The time shows how current the value is. A subject identifier
gives no information that members need (ADR-125).
**Tradeoff:** A display value can be out of date.

## Gates

- **Provider configuration:** AT Protocol claims need an `atproto` provider
  (FDR-058).
- **Account holder consent:** each claim is hidden until its holder makes it
  visible.

## Privacy

- **Profile views:** the server sends visible claims with the profile. The
  viewer's device does not contact external systems.
- **External links:** the external site learns the viewer's IP address only
  when the viewer opens the link.
- **Logs:** display values and subject identifiers identify users. The server
  does not log them.

## Related

- **ADRs:** ADR-111 (client state in `@chatto/client`), ADR-124 (verified
  AT Protocol handles need an independent issuer), ADR-125 (identity claims)
- **FDRs:** FDR-022 (User Profile), FDR-023 (Authentication & Sessions),
  FDR-058 (AT Protocol Sign-In)

## Open Questions

- **Client proofs:** the viewer's client examines claims itself, so that a
  modified server cannot fake them. This is the intended direction (ADR-125).
  It needs a design for the external systems that viewers' devices contact.
- **Issuer attestations:** #3604 defines the issuer, the attestation, and
  what it reveals to viewers.
- **More schemes:** domains (DNS or `.well-known`), fediverse accounts
  (`rel=me`), and sign-in providers with public handles, such as GitHub and
  GitLab.
- **Account managers:** examine whether account managers can see hidden
  claims.
- **Refresh:** examine whether the server examines claims again between
  sign-ins.
