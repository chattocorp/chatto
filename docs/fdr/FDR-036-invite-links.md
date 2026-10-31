# FDR-036: Invitation Codes

**Status:** Active
**Last reviewed:** 2026-10-31

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
- **Create code** in the table header opens a confirmation dialog. It explains
  that a code expires after seven days, admits one account, and is used only
  after successful signup. Confirmation creates one code and closes the dialog.
  The server sets expiry to exactly seven days after creation for every caller.
  Neither the UI nor the creation API accepts an expiry. Existing records keep
  their recorded expiry. Active codes can be
  copied and revoked; inactive rows show their status without these actions.
- A creation-time note can identify the intended recipient or purpose. It is
  optional plain text, limited to 250 Unicode characters, and shown beneath the
  code. Surrounding whitespace is removed. Creators and invitation managers can read
  notes through the app or API; the event log omits them. A note does not restrict
  who can redeem the code. Existing codes have no note.
- **Server Admin → Registration**, directly below General, contains the admission
  setting. Radio options select **Open Registration** or
  **Invite Code Required** and save immediately. The page lists enabled signup
  methods: email/password registration and providers that can create accounts.
  Registration requires `server.manage`.
- **Server Admin → Invites** contains code management. Members with `user.invite`
  can list, read, create, copy, and revoke their own codes. Users with effective
  `user.manage-invites` can manage all codes. The table shows each creator's avatar
  and name in a separate column. The API applies the same ownership boundary,
  including pagination totals. Other users' codes return not found.
- Invites remains available when registration is open; signup then ignores codes.
  Codes can be retrieved again while the caller has the required permission.
  No code is stored in browser storage. Chatto does not send codes to recipients.
- Issuance has no automatic retry. Creators can retrieve a code after a lost response through
  the management API. Changing a creator's permissions or account state does
  not revoke an issued code; an authorized caller can revoke it explicitly.
- Expired, used, revoked, malformed, and unknown codes show the same public
  error. A code is checked again when the account is created.
- Changing the server secret invalidates distributed codes. Administrators
  can copy newly derived codes for records that remain active.

## Design Decisions

### 1. Each code admits one account

**Decision:** Remove reusable links and use limits. Every code is single-use.
The unused beta link system has no compatibility or migration support. Its
removed `max_uses` protobuf field number and name stay reserved.
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

### 4. Authorized users can recover codes

**Decision:** Creators with `user.invite` can copy their active codes again.
Holders of effective `user.manage-invites` can copy all active codes.
**Why:** Users choose their own distribution method and may need to
send a code again. ADR-070 avoids storing raw codes in durable history.
**Tradeoff:** Management permission grants access to all active codes.

## Gates

- `registration.admission = invite_only` in the admin server resource requires
  a code for new self-service accounts. A server manager changes this under
  **Server Admin → Registration → Account creation**. The default is open.
  `ServerRegistrationAdmissionChangedEvent` stores the change in EVT.
  Enabled signup methods remain separate settings.
- `server.manage` permits admission changes. Account creation checks the current
  policy under stream-wide EVT OCC, including after a retry. A pending signup
  has no exemption. When the policy is open, a supplied code is not consumed.
- `user.invite` permits creation and management of own codes without privileged mode.
- `user.manage-invites` permits all administrative code operations and requires
  privileged mode for human sessions. It does not include `user.invite`.
- Fresh bootstrap grants both permissions to admins and neither to everyone.
  On upgrade, current server-scope `user.invite` allows and denies are copied
  once to `user.manage-invites`. The original decisions remain. Any historical
  decision for the new permission closes this upgrade gate. An empty upgrade
  records an explicit clear for everyone to prevent future member grants from
  becoming management grants after restart. The copy uses whole-RBAC OCC.

## Related

- **ADRs:** ADR-033, ADR-036, ADR-040, ADR-045, ADR-070
- **FDRs:** FDR-001, FDR-018, FDR-020, FDR-023, FDR-028, FDR-031

## Deferred Work

Code handoff through registration URLs, email-less direct signup, email-domain
restrictions, and cross-replica rate limiting are separate changes.
