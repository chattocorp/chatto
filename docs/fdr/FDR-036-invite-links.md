# FDR-036: Invitation Codes

**Status:** Active
**Last reviewed:** 2026-10-30

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
  The API can set a different expiry or no expiry. Active codes can be
  copied and revoked; inactive rows show their status without these actions.
- A creation-time note can identify the intended recipient or purpose. It is
  optional plain text, limited to 250 Unicode characters, and shown beneath the
  code. Surrounding whitespace is removed. Only invitation managers can read
  notes through the app or API; the event log omits them. A note does not restrict
  who can redeem the code. Existing codes have no note.
- **Server Admin → Registration**, directly below General, contains the admission
  setting and code management. Radio options select **Open Registration** or
  **Invite Code Required** and save immediately. The page lists enabled signup
  methods: email/password registration and providers that can create accounts.
  Each section requires its own permission.
- While registration is open, the code panel stays visible but faded, with
  Create, Copy, and Revoke disabled. Saving Invite Code Required activates
  these controls. Existing codes remain unchanged.
- Administrators can list, read, create, copy, and revoke codes. The API
  offers the same operations. Chatto does not send codes to recipients.
- Members with `user.invite` select **Invite people** in the server sidebar
  when public discovery reports invite-only registration. The action is hidden
  for open registration and while the policy is unknown.
  A modal creates one code with a seven-day expiry and shows it with Copy.
  Closing the modal discards the code. Back, Forward, reload, permission loss,
  and account changes do not recover it. Members cannot list, retrieve, or
  revoke codes, including their own. No code is stored in browser storage.
- Administrators see member-created codes and their creators in Registration.
  Member issuance does not accept a note or custom expiry. The API can still
  create a code while registration is open; signup then ignores it.
  Issuance has no automatic retry: a lost response can leave a code visible
  only to managers. Changing the creator's permissions or account state does
  not revoke an issued code; managers can revoke it explicitly.
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

### 4. Administrators can recover codes

**Decision:** Holders of `user.manage-invites` can copy active codes again.
**Why:** Administrators choose their own distribution method and may need to
send a code again. ADR-070 avoids storing raw codes in durable history.
**Tradeoff:** This permission grants access to all active codes.

## Gates

- `registration.admission = invite_only` in the admin server resource requires
  a code for new self-service accounts. A server manager changes this under
  **Server Admin → Registration → Account creation**. The default is open.
  `ServerRegistrationAdmissionChangedEvent` stores the change in EVT.
  Enabled signup methods remain separate settings.
- `server.manage` permits admission changes. Account creation checks the current
  policy under stream-wide EVT OCC, including after a retry. A pending signup
  has no exemption. When the policy is open, a supplied code is not consumed.
- `user.invite` permits member issuance without privileged mode.
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
