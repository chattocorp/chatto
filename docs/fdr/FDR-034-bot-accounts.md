# FDR-034: Bot Accounts

**Status:** Proposed
**Last reviewed:** 2026-07-22

## Overview

Bot accounts let people operate automations and integrations as visible,
accountable participants in a Chatto server. A bot has its own identity,
permissions, DM memberships, and API credential, but it is always owned by a
human account and can never exercise authority its owner does not currently
possess.

## Behavior

- A bot is always unmistakably identified as a bot. Its username must end in
  `_bot`, while human accounts may not use usernames with that suffix.
- Username validation preserves that distinction during bot creation and every
  later username change; changing an account's username cannot make a human
  appear to be a bot or a bot appear to be human.
- User-facing representations use explicit bot labels or icons wherever
  mistaking the bot for a human could matter. The username convention is an
  additional durable signal, not a substitute for accessible UI treatment.
- Every bot has exactly one human owner. Bot profiles and administration
  surfaces identify that owner.
- A person's bot-management page lists only bots they own, even when that
  person also has administrative authority. The separate server-administration
  surface lists all bots the administrator is authorized to manage.
- Every bot has a required description explaining what it does. Bot owners are
  expected to disclose relevant data handling in that description. The first
  version may present this as plain text before richer bot profiles exist.
- Creating a bot requires an explicit permission. The creator becomes its
  owner.
- A newly created bot receives the server's ordinary `everyone` permission
  baseline. It has no additional roles or direct permission decisions until
  they are deliberately configured.
- A bot's effective authority is the intersection of its own configured
  authority and its owner's current effective authority at the same resource
  and scope. A bot-specific restriction can narrow that authority; no bot
  setting can widen it beyond the owner.
- Explicit denies on an owner constrain their bots even when the owner has the
  built-in Server Owner role. The role's virtual allow cannot be used to
  delegate through a deny.
- Changes to the owner's roles, direct permission decisions, or account status
  constrain the bot immediately. Suspending the owner also suspends the bot.
- Bot administration presents a permission matrix that lets the owner narrow
  the bot's baseline or deliberately grant additional capabilities within the
  owner's current authority. It makes that ceiling clear and does not offer
  apparently selectable grants that the owner cannot delegate.
- Bot-kind invariants prohibit human authentication and security-identity
  operations regardless of RBAC. Bots cannot use interactive login, create or
  own bots, manage their own API credentials, or change passwords, verified
  email addresses, or external login identities. Bots may perform ordinary
  moderation or administration when both bot and owner have the required
  authority and the normal target/delegation rules pass.
- Authorized administrators can inspect bots owned by other users, reduce or
  revoke their permissions, revoke their API access, and delete them. This is
  an abuse-response and server-safety capability, not an ownership transfer.
- A bot can participate in DMs. DM membership determines which conversations
  the bot can read, exactly as it does for human participants; ordinary DM
  authorization rules determine which actions the bot may perform.
- A bot authenticates through one dedicated API key with no automatic expiry.
  The raw secret is shown only when issued and only to the bot's owner. Its
  owner can replace it with a new key, immediately invalidating the previous
  key. Administrators can revoke another owner's bot key for abuse response,
  but cannot issue or receive its replacement secret.
- Bot credentials authenticate the same general-purpose public ConnectRPC and
  realtime APIs used by other clients. Bot-kind invariants and ordinary
  authorization determine which operations a bot may perform; Chatto does not
  maintain a narrower frontend-shaped or parallel bot API.
- Deleting an owner deletes every bot that owner owns. Each bot follows the
  normal account and authored-content deletion behavior.
- Bot ownership transfer is not part of the first version. It is planned as a
  separate lifecycle feature.

## Design Decisions

### 1. Bot identity is redundant and unmistakable

**Decision:** Represent bot status as account data, reserve the `_bot` username
suffix for bots, and show explicit bot treatment in the UI.
**Why:** People must be able to recognize automation before deciding how to
interact with it. Account data gives clients an authoritative signal, the
username survives plain-text references and limited clients, and UI treatment
makes the distinction accessible and prominent.
**Tradeoff:** Username validation gains an account-kind rule, and every surface
that renders users must deliberately handle bot identity.

### 2. Every bot is accountable to one human owner

**Decision:** Require exactly one human owner, display that relationship, and
delete owned bots when the owner is deleted.
**Why:** A bot with no responsible person has ambiguous authority and no clear
contact for behavior or data-handling concerns. A single owner gives creation,
administration, and deletion a clear first-version lifecycle.
**Tradeoff:** Integrations tied to a departing owner must be recreated until a
separate ownership-transfer feature exists.

### 3. Bot authority is an owner-bounded intersection

**Decision:** Resolve each bot capability from both the bot and its owner at
authorization time. Both must be allowed for the bot to act. See ADR-056.
**Why:** Copying permissions at creation would become stale after role,
permission, or suspension changes. A dynamic ceiling makes it
impossible for an owner to delegate authority they no longer possess.
**Tradeoff:** Bot authorization is more expensive and permission explanations
must describe two subjects. Owner changes can immediately interrupt an
integration that was previously working.

### 4. Bots begin with the `everyone` baseline

**Decision:** Resolve bots through ordinary RBAC, including the implicit
`everyone` role. New bots receive no additional roles or direct decisions.
Their owner may narrow the baseline or deliberately configure more authority,
subject to the owner ceiling. Administrators may assign ordinary roles under
the existing role-assignment rules; bot owners may not.
**Why:** `everyone` expresses the operator's ordinary participant policy. Using
it for bots preserves existing server, group, and room inheritance without
introducing a speculative bot-wide policy layer. Direct decisions provide the
per-bot control owners need most often.
**Tradeoff:** A newly issued bot credential may immediately exercise ordinary
participant capabilities such as posting or reacting when `everyone` grants
them. Operators who eventually need one policy for every bot must configure
bots individually until a proven bulk-policy design is introduced.

### 5. Human-only operations are bot-kind invariants

**Decision:** Reject interactive login, bot ownership or creation, self-managed
API credentials, and human security-identity operations for bot actors
regardless of their resolved permissions. Permit ordinary moderation and
administration through the same owner-bounded RBAC rules as other actions.
**Why:** These operations are nonsensical or unsafe for automation identities.
Making them categorical invariants prevents an unrelated role or future
`everyone` default from accidentally enabling them.
**Tradeoff:** Bot actors do not receive every identity capability their
permission trace might otherwise suggest. Conversely, deliberately granting
administrative authority to both a bot and its owner can create powerful
moderation automation that operators must configure carefully.

### 6. DMs retain their existing membership boundary

**Decision:** Treat bots as ordinary DM participants. Membership controls read
access, while the owner-bounded permission intersection controls actions. The
owner does not need to be a participant and does not gain access through
ownership.
**Why:** DMs are a useful bot interface and Chatto already has a clear privacy
boundary for them. A parallel bot-only DM authorization model would create
inconsistent privacy semantics. See ADR-037.
**Tradeoff:** Inviting or messaging a bot gives that automation access to the
conversation under the same durable membership rules as any other participant;
the UI must make the bot's nature and owner obvious before that happens.

### 7. Bot descriptions are mandatory disclosures

**Decision:** Require a bot description that explains its purpose and is the
place for owners to disclose relevant data handling.
**Why:** Recognizing that an account is automated is necessary but insufficient;
people should also be able to understand what the automation does before they
interact with it.
**Tradeoff:** Chatto can require a description but cannot initially verify that
it is complete or accurate. Moderation and stronger disclosure structure may
be needed later.

### 8. Administrators can stop abusive bots

**Decision:** Give authorized administrators a direct way to restrict, revoke,
or delete any bot without taking ownership of it.
**Why:** Bots can act quickly and at scale. Server operators need an immediate
abuse-response path even when the owner is unavailable or malicious.
**Tradeoff:** Bot owners cannot assume exclusive operational control, and these
powerful administrative actions need clear permission gates and durable audit
facts.

### 9. One indefinite API key in v1

**Decision:** Give each bot one API key with no automatic expiry. Bot creation
issues the first show-once secret. Replacing it issues a new show-once secret
and immediately invalidates the old one.
**Why:** Long-running integrations generally need a durable credential. A
single replacement action covers the first-version leak and rotation workflow
without expiry policy, multiple-key administration, or overlapping credential
states.
**Tradeoff:** Replacement can briefly interrupt an integration while its
configuration is updated. Owners cannot stage a zero-downtime rotation or use
separate credentials for multiple installations of the same bot.

### 10. Bots use the general-purpose public API

**Decision:** Bot credentials authenticate the same public ConnectRPC and
realtime surfaces used by Chatto's bundled client and other clients.
**Why:** The public API is a product interface, not a private backend for the
bundled frontend. Reusing it prevents a bot-specific protocol from drifting and
continues the protobuf-first direction in ADR-042.
**Tradeoff:** API operations must express general resource behavior and reject
human-only actions through bot-kind invariants. Cleanups discovered during bot
implementation should be tracked separately unless required for safe bot use.

## Permissions

- `bot.create` gates creating and managing one's own bots.
- `bot.manage` gates inspecting, restricting, revoking, and deleting bots
  owned by other users. It does not bypass the owner's authority ceiling.
- A bot's ordinary capabilities use Chatto's existing permission catalog and
  scopes; bot accounts do not get a parallel capability vocabulary.
- Bot owners may configure direct permission decisions for their bots but may
  not assign roles. Role assignment remains an administrative operation gated
  by the existing role-assignment rules.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-033
  (event-sourced state), ADR-037 (DM access via membership), ADR-042
  (protobuf-first public API), ADR-046 (typed runtime credentials), ADR-052
  (subject-specific RBAC), ADR-056 (owner-bounded bot authorization)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-018
  (Account Lifecycle), FDR-023 (Authentication & Sessions), FDR-025 (User Search
  & Member Directory)

## Open Questions

- How can an administrator impose a bot restriction that its owner cannot later
  clear through ordinary bot-permission management?
- Which account-suspension mechanism is canonical, and how should the bot UI
  communicate that its owner currently disables it?
- What separate design governs transferring a bot to another human owner?
