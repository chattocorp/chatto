# FDR-038: Bot Accounts

**Status:** Experimental
**Last reviewed:** 2026-08-11

## Overview

Bot accounts give automations a visible, accountable identity without treating
them as human users. Owners approve explicit application capabilities, users
can inspect those capabilities before interacting, and the server admits bot
credentials only through narrow capability-aware operations.

## Behavior

- A bot is visibly identified as a bot. Its username ends in `_bot`; human
  accounts may not use that suffix.
- Every bot has exactly one active human owner and a required description of
  its purpose and relevant data handling.
- `bot.create` gates creating and managing one's own bots. `bot.manage` gates
  inspecting, revoking, and deleting bots owned by other users.
- A bot cannot own another bot, use interactive human login, manage human
  authentication data, receive roles, or receive direct RBAC decisions.
- A bot has one indefinite API key. The raw secret is shown only to its owner
  when issued; replacement invalidates the previous key immediately.
  Administrators may revoke another owner's key but cannot issue its replacement.
- The ordinary user API, realtime protocol, protected assets, room enumeration,
  and self-join surfaces reject bot API keys.
- Owners and authorised administrators choose grants from a server-defined
  capability catalogue. Unknown and absent capabilities fail closed.
- Bot management pages and public profile cards show the bot description and
  server-authored names and explanations for its approved capabilities.
- A user may start a DM with a bot only while it has `dm.messages.read`. The bot
  can list and read only DMs in which it is an explicit participant.
- A bot with `messages.write` may post text messages only in an explicit DM in
  which it is a participant, an invited channel thread, or an explicitly
  installed channel through its incoming webhook.
- `room.manage` holders explicitly install and remove bots through the room
  member controls. Universal rooms still require explicit bot installation;
  bots never gain implicit Universal membership or room-wide read access.
- A human directly mentioning an installed bot in a root message or thread
  reply grants `thread.messages.read` access to that root and its replies. Role
  and broadcast mentions do not invite bots. The inviter, bot owner, or a
  current room manager may revoke the invitation.
- Removing a bot from a room clears every thread invitation in that room.
  Reinstalling it does not revive old access.
- The incoming webhook accepts only root text-message writes for one explicitly
  installed channel. It exposes no read operation. Delivery is at least once:
  callers that retry an uncertain request may create duplicate messages.
- Every bot operation is bounded by the owner's current authority. A bot cannot
  keep posting after policy prevents its owner from posting.
- A bot cannot enumerate or self-join rooms, including Universal rooms.
- Deleting an owner deletes every bot they own. Each bot follows normal account
  and authored-content deletion behavior. Ownership transfer is not in v1.

## Design Decisions

### 1. Bot identity is redundant and unmistakable

**Decision:** Represent bot status as account data, reserve the `_bot` username
suffix, and show explicit bot treatment in the UI.

**Why:** People must recognise automation before deciding how to interact with
it. Account data is authoritative, while the suffix survives limited clients.

**Trade-off:** Every user-rendering surface must deliberately handle bot identity.

### 2. Every bot is accountable to one human owner

**Decision:** Require one human owner, disclose the relationship, and delete
owned bots when the owner is deleted.

**Why:** A bot needs a responsible person whose authority limits its actions.

**Trade-off:** Integrations must be recreated when an owner leaves until
ownership transfer exists.

### 3. Runtime authority is an intersection, not inherited RBAC

**Decision:** Require an approved application capability, the owner's live
authority, and an explicit resource context for every bot operation. See ADR-071.

**Why:** A capability says what may be attempted, the owner ceiling preserves
accountability, and context prevents passive access. No single gate grants full
user authority.

**Trade-off:** Bot integrations use dedicated APIs and operation models instead
of the full user API.

### 4. Capabilities are shared application vocabulary

**Decision:** Use OAuth-scope-style identifiers and server-owned disclosure
metadata that future third-party OAuth grants can reuse. First-party user
clients are not scoped this way. See ADR-071.

**Why:** Bot and OAuth integrations need one understandable language for
operation classes without weakening live first-party RBAC behavior.

**Trade-off:** Bot and OAuth grant lifecycles remain separate even when their
identifiers are shared.

### 5. Profiles are mandatory disclosure surfaces

**Decision:** Require a bot description and expose approved capability names
and explanations on management and public profile surfaces.

**Why:** A bot label alone does not tell people what data an automation can use.

**Trade-off:** Chatto cannot verify that an owner-written description is complete.

### 6. One indefinite API key in v1

**Decision:** Give each bot one show-once API key with no automatic expiry.

**Why:** One replacement action covers the first-version long-running
integration workflow without multi-key administration.

**Trade-off:** Owners cannot stage zero-downtime rotation.

### 7. Administrators can stop abusive bots

**Decision:** Authorised administrators can revoke credentials and delete any
bot without taking ownership.

**Why:** Operators need an immediate abuse-response path.

**Trade-off:** Owners do not have exclusive operational control.

## Permissions

- `bot.create` gates creating and managing one's own bot records and keys.
- `bot.manage` gates inspecting, revoking, and deleting other owners' bots.
- `dm.messages.read`, `thread.messages.read`, and `messages.write` are
  application capabilities, not RBAC permissions.
- Neither management permission grants bot runtime authority.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-033
  (event-sourced state), ADR-042 (protobuf-first public API), ADR-046 (typed
  runtime credentials), ADR-071 (owner-bounded application capabilities)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-018
  (Account Lifecycle), FDR-023 (Authentication & Sessions), FDR-025 (User
  Search & Member Directory)

## Follow-up slices

- Consider per-user bot opt-out after the interaction model is established.
- Add idempotency keys and rate limiting to incoming webhooks if operational
  use demonstrates the need.
