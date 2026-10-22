# FDR-036: Invitation Codes

**Status:** Active
**Last reviewed:** 2026-10-19

## Overview

Invitation codes let administrators control who can create an account. Each
code permits one new account through any enabled signup method. Open servers
do not require codes. Administrators can distribute codes themselves or use
the API to integrate code issuance with an external system.

## Behavior

- Operators select `open` or `invite_only` account creation. The default is
  `open`. The setting applies only to new self-service accounts.
- On invite-only servers, Create Account asks for a code before showing email
  registration or eligible SSO providers. Validation does not reserve or
  consume a code.
- The browser remembers admission for 30 minutes across reloads and email
  retries. Started email and SSO flows keep their own invitation. A code
  entered in another tab does not replace that invitation.
- A new SSO identity that enters through Sign In must supply a code before
  account creation. Existing-account sign-in, account linking, and
  operator-created accounts require no code.
- Each code permits exactly one account. The account and code use succeed
  together. Failed signup does not consume the code. Concurrent signups with
  one code can create at most one account.
- Codes can expire. The admin screen defaults to seven days and also offers
  one day, 30 days, or no expiry. Codes can be revoked before use.
- Administrators can list, read, create, copy, and revoke codes. The API
  offers the same operations. Chatto does not send codes to recipients.
- Expired, used, revoked, malformed, and unknown codes show the same public
  error. A code is checked again when the account is created.
- Beta invite links are retired, including pending registrations that carry
  them. Administrators must issue new codes. Existing accounts and historical
  invitation records remain intact.
- Changing the server secret invalidates distributed codes. Administrators
  can copy newly derived codes for records that remain active.

## Design Decisions

### 1. Each code admits one account

**Decision:** Remove reusable links and use limits. Every code is single-use.
**Why:** Chatto supports open registration. Invitations serve controlled
account creation rather than general community discovery.
**Tradeoff:** Administrators must issue one code per intended new account.

### 2. Admission is separate from identity

**Decision:** Apply the same gate to email registration and SSO provisioning.
**Why:** A code grants admission. Existing email and provider identity checks
still apply.
**Tradeoff:** Email-less direct registration and email-domain policies need
separate work. They are not part of this change.

### 3. Validation does not consume a code

**Decision:** Consume the code only with successful account creation.
**Why:** Email delivery, provider callbacks, and account validation can fail.
Retries must remain possible. ADR-070 defines the atomic commit boundary.
**Tradeoff:** Validation does not guarantee admission if another person uses
the same code first.

### 4. Administrators can recover codes

**Decision:** Holders of `user.invite` can copy active codes again.
**Why:** Administrators choose their own distribution method and may need to
send a code again. ADR-070 avoids storing raw codes in durable history.
**Tradeoff:** This permission grants access to all active codes.

### 5. Retire beta links without conversion

**Decision:** Reject old links instead of converting their remaining uses.
**Why:** Reusable links have no single intended recipient. Conversion would
give the first visitor an arbitrary claim on a shared invitation.
**Tradeoff:** Administrators must replace invitations they already shared.

## Gates

- `auth.account_creation_policy = "invite_only"` requires a code for new
  self-service accounts. Enabled signup methods remain separate settings.
- `user.invite` permits code management. Existing RBAC grants remain in use.

## Related

- **ADRs:** ADR-033, ADR-036, ADR-040, ADR-045, ADR-070
- **FDRs:** FDR-001, FDR-018, FDR-020, FDR-023, FDR-028, FDR-031

## Deferred Work

Code handoff through registration URLs, email-less direct signup, email-domain
restrictions, and cross-replica rate limiting are separate changes.
