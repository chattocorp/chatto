# FDR-038: Bot Accounts

**Status:** Experimental
**Last reviewed:** 2026-08-11

## Overview

Bot accounts give automations a visible, accountable identity without treating
them as human users. This first slice establishes bot ownership, lifecycle,
management, and credentials. Bot credentials deliberately cannot use Chatto's
ordinary API or realtime protocol until a separately reviewed capability layer
grants specific operations.

## Behavior

- A bot is always unmistakably identified as a bot. Its username must end in
  `_bot`, while human accounts may not use usernames with that suffix.
- User-facing representations show an explicit bot label or icon wherever
  mistaking the bot for a human could matter.
- Every bot has exactly one active human owner. Bot profiles and management
  surfaces identify that owner.
- A person's bot-management page lists only bots they own. A separate server
  administration surface lists bots an administrator may manage.
- Every bot has a required description explaining its purpose and relevant
  data handling.
- `bot.create` gates creating and managing one's own bots. `bot.manage` gates
  inspecting, revoking, and deleting bots owned by other users.
- A bot cannot own or create another bot, use interactive human login, change a
  password or verified email, or attach an external login identity.
- A bot has one API key with no automatic expiry. The raw secret is shown only
  when issued and only to the owner. Replacing it immediately invalidates the
  previous key. Administrators may revoke another owner's key but cannot issue
  or receive its replacement.
- A valid bot API key identifies the bot, but the server rejects its use of the
  ordinary ConnectRPC and realtime surfaces until an approved capability layer
  explicitly opens a particular operation.
- Bots do not receive roles, direct permission decisions, or the `everyone`
  baseline as a way to gain runtime authority. Bot runtime authority is not
  configured through the RBAC permission matrix.
- A bot cannot enumerate or self-join rooms. Room installation, DM
  participation, thread invitation, message posting, and read access belong to
  later capability and interaction slices.
- Deleting an owner deletes every bot they own. Each bot follows the normal
  account and authored-content deletion behavior.
- Ownership transfer is not part of the first version.

## Design Decisions

### 1. Bot identity is redundant and unmistakable

**Decision:** Represent bot status as account data, reserve the `_bot` username
suffix for bots, and show explicit bot treatment in the UI.

**Why:** People must be able to recognise automation before deciding how to
interact with it. Account data gives clients an authoritative signal, the
username survives plain-text references and limited clients, and UI treatment
makes the distinction accessible and prominent.

**Trade-off:** Username validation gains an account-kind rule, and every
surface that renders users must deliberately handle bot identity.

### 2. Every bot is accountable to one human owner

**Decision:** Require exactly one human owner, display that relationship, and
delete owned bots when the owner is deleted.

**Why:** A bot with no responsible person has ambiguous authority and no clear
contact for behaviour or data-handling concerns.

**Trade-off:** Integrations tied to a departing owner must be recreated until a
separate ownership-transfer feature exists.

### 3. Runtime authority fails closed

**Decision:** A bot credential may be validated as an identity credential, but
it cannot use general ConnectRPC or realtime operations in this slice. Bots do
not acquire authority from ordinary RBAC configuration.

**Why:** Identity and credential lifecycle can be reviewed independently from
the more consequential question of what data an automation may read or change.
Default denial prevents a newly created credential from inheriting broad
participant or administrative access accidentally.

**Trade-off:** The credential is operationally inert until a capability slice
is installed above this foundation.

### 4. Bot descriptions are mandatory disclosures

**Decision:** Require a bot description that explains its purpose and is the
place for owners to disclose relevant data handling.

**Why:** Recognising that an account is automated is necessary but
insufficient; people should understand what the automation does before they
interact with it.

**Trade-off:** Chatto can require a description but cannot initially verify
that it is complete or accurate.

### 5. One indefinite API key in v1

**Decision:** Give each bot one API key with no automatic expiry. Creation
issues the first show-once secret; replacement immediately invalidates the old
one. If initial key issuance fails, creation compensates by deleting the newly
created bot.

**Why:** Long-running integrations generally need a durable credential. One
replacement action covers the first-version leak and rotation workflow without
expiry policy, multiple-key administration, or overlapping credentials.

**Trade-off:** Replacement can briefly interrupt an integration, and owners
cannot stage a zero-downtime rotation.

### 6. Administrators can stop abusive bots

**Decision:** Authorised administrators can revoke credentials and delete any
bot without taking ownership of it.

**Why:** Server operators need an immediate abuse-response path even when the
owner is unavailable or malicious.

**Trade-off:** Bot owners cannot assume exclusive operational control.

## Permissions

- `bot.create` gates creating and managing one's own bot records and keys.
- `bot.manage` gates inspecting, revoking, and deleting bots owned by other
  users.
- Neither permission grants runtime authority to a bot actor.
- Bots are not managed through roles or direct per-user permission decisions.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-033
  (event-sourced state), ADR-042 (protobuf-first public API), ADR-046 (typed
  runtime credentials)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle), FDR-023
  (Authentication & Sessions), FDR-025 (User Search & Member Directory)

## Follow-up slices

- Define a shared application-capability vocabulary and server-enforced bot
  grants, bounded by the owner's live authority.
- Disclose granted capabilities on bot profiles and management surfaces.
- Permit explicitly capability-gated DM reads and message writes.
- Add explicit room installation, thread invitations, and write-only incoming
  webhooks without enabling passive room-wide reading.
- Consider per-user bot opt-out after the interaction model is established.
