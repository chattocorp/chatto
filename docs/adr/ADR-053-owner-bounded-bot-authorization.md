# ADR-053: Owner-Bounded Bot Authorization

**Date:** 2026-07-22

## Context

Bot accounts need independent identities and narrowly configured capabilities,
but they act under authority delegated by a responsible human owner. Treating a
bot exactly like an unrelated user would let its roles or direct grants outlive
the owner's authority. Copying the owner's current permissions into the bot at
creation or configuration time would have the same problem: later role,
permission, room-access, and account-status changes would leave stale grants.

Chatto resolves permission decisions per subject and scope under ADR-052. Some
access boundaries are deliberately not permissions: most importantly, DM read
access comes from room membership under ADR-037. Bot authorization must compose
with both kinds of rule without creating a parallel RBAC system or weakening a
room privacy boundary.

Server operators also need to stop abusive or compromised bots regardless of
the owner's cooperation. Conversely, bot owners must be able to make a bot less
powerful than themselves while retaining the existing RBAC inheritance model.

## Decision

Every bot account has exactly one human owner. A bot cannot own another bot and
cannot exist without an owner.

For every attempted action, Chatto evaluates the bot and its owner against the
same current permission and scope. The action is authorized only when:

1. The bot's own configured authority allows it.
2. The owner's current effective authority allows it.
3. The bot satisfies every applicable non-RBAC invariant for the operation.
4. The owner's account state permits owned bots to act.

The conjunction is evaluated at authorization time. Owner authority is never
copied into durable bot grants as a substitute for checking the owner. A bot
grant therefore means "this bot may use this capability while its owner may
also use it," not an independent source of authority.

Bots resolve through ordinary RBAC, including the implicit `everyone` baseline,
assigned roles, direct user decisions, and server, group, and room scopes. A new
bot has no roles or direct decisions, so its initial authority is the
intersection of `everyone` and its owner's current authority. Bot-specific
direct decisions can narrow that baseline or deliberately widen the bot's own
RBAC result, but the owner intersection remains an absolute ceiling.

Some operations are categorically unavailable to bot actors regardless of
RBAC. Bots cannot use interactive human login, create or own other bots, manage
their own API credentials, or perform human account-security operations. These
are account-kind invariants, not permission decisions.

Bot owners may configure direct permission decisions for their bots, subject to
the owner ceiling, but may not assign roles. Role assignment to bots remains an
administrative operation governed by the existing role-assignment permissions
and delegation bounds.

`bot.create` gates creation and management of one's own bots. `bot.manage`
gates management of bots owned by other users. Neither permission is an
ordinary bot capability: bot actors are categorically denied both management
operations even if an RBAC trace would otherwise allow them.

Non-permission access boundaries remain authoritative and are evaluated for the
bot, not inherited from or independently required of the owner. In a DM, the
bot must be a participant to read the conversation, just like a human account.
The owner does not need to be a participant. Bot membership does not grant the
owner access to that DM, and owner membership does not implicitly add the bot.
An action within the DM must additionally pass the bot-and-owner permission
conjunction and the existing DM privacy boundary.

An owner suspension or other account state that prevents the owner from acting
also prevents every owned bot from authenticating or acting. Deleting an owner
deletes all owned bots as part of the account-deletion operation. Ownership
transfer is a future lifecycle feature and does not weaken the invariant that a
bot always has exactly one current human owner.

Authorized administrators may independently restrict a bot, revoke its API
access, or delete it. Administrative intervention cannot grant the bot
authority beyond its owner or silently transfer ownership.

Permission explanations and administrative APIs must expose both halves of the
decision so operators can distinguish a bot restriction from an owner ceiling.
Authorization-sensitive owner, bot, membership, and account-state changes must
use Chatto's durable authorization fencing so another replica cannot authorize
from stale state after such a change commits.

## Consequences

- Removing an owner's permission or active-account status immediately
  constrains all of their bots without rewriting each bot.
- A leaked bot credential is bounded twice: by the bot's explicitly enabled
  capabilities and by the owner's current authority.
- Bot owners can follow least privilege without creating bot-specific
  permission names for ordinary actions.
- Bot ownership does not implicitly confer role-assignment authority. Owners
  use the dedicated bot-permission surface; administrators retain role
  administration through ordinary RBAC controls.
- Existing `everyone`, role, direct-user, and scoped inheritance semantics apply
  to bots without a parallel permission system.
- DM privacy remains membership-based. The owner relationship neither exposes
  the bot's DMs to its owner nor lets the bot inherit the owner's conversations.
- Permission resolution and explanation become more complex because a bot
  action requires two subject evaluations plus non-RBAC invariants.
- A new bot inherits ordinary `everyone` capabilities immediately. Bot creation
  and credential issuance must show that baseline clearly so the owner can
  narrow it before putting the credential into service.
- Authorization has a second class of denials for bot-kind invariants. Permission
  explanations must distinguish those categorical restrictions from RBAC and
  owner-ceiling results.
- Owner deletion becomes a multi-account lifecycle operation. It must not leave
  ownerless bots if a partial failure occurs.
- Cross-replica authorization must include owner-affecting changes in the same
  durable fencing discipline as other authorization-sensitive state.
- A future ownership-transfer feature must define an atomic transition,
  re-evaluate the bot against the new owner's authority, and avoid any interval
  with zero or multiple owners.
