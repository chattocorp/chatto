# FDR-028: Bot Accounts

**Status:** Active
**Last reviewed:** 2026-06-07

## Overview

Bot accounts are first-class Chatto users for automation and integrations. They appear anywhere a normal user can appear, can receive ordinary permissions and roles, and can act through the GraphQL/API surface, but they never sign in through the browser UI, passwords, OAuth, or cookie sessions.

## Behavior

- A permitted human user can create bot accounts. The creator becomes the bot's owner.
- Bot accounts are visible in member directories, room member lists, mentions, message authorship, profile links, and admin member views like other users, with a bot badge where the UI needs to disambiguate them.
- A bot account has `kind: BOT`, `isBot: true`, and a nullable owner link. Human accounts have `kind: HUMAN`, `isBot: false`, and no bot owner.
- Bots do not own other bots in v1. Bot-management mutations reject bot actors, even if the bot otherwise has permissions.
- New bots receive only the implicit `everyone` permissions by default. Operators may assign roles or user-level grants to a bot when they intentionally trust that automation.
- Promoting a bot means trusting its owner. The owner can create and revoke that bot's API tokens, so any role assigned to the bot is also a delegated capability to the owner.
- Bot accounts authenticate only with named bot API tokens. Password login, OAuth login, cookie sessions, and human bearer-session issuance reject bot accounts.
- Each bot can have multiple named API tokens. The raw token secret is shown only once on creation and uses the distinct `cht_BT...` prefix.
- The token creator chooses expiry at issuance: 30 days, 90 days, 365 days, indefinite, or a custom expiration date. If the server has a maximum bot-token TTL, indefinite tokens are unavailable and all fixed/custom expirations must fit within that maximum.
- Token metadata shows the token name, creator, created time, last-used time, expiry, and revocation metadata. Raw token secrets are never shown after creation.
- Deleting a human user first deletes every bot account that human owns. If deleting any owned bot fails, the human deletion aborts before ownerless bots can be created.
- Deleting a bot uses the same account deletion and crypto-shredding behavior as deleting a human account: login reuse, profile removal, credential cleanup, asset cleanup, and authored-content tombstoning all apply.

## Design Decisions

### 1. Bots are visible users, not hidden integrations

**Decision:** Bot accounts are ordinary user records with a bot kind, badge, and owner link.
**Why:** Messages, mentions, reactions, membership, permissions, moderation, deletion, and crypto-shredding already revolve around users. A bot that posts a message should leave the same accountable identity trail as a human author. See ADR-041.
**Tradeoff:** User-facing surfaces need to handle bot badges and owner context rather than assuming every member is a human.

### 2. Ownership is human-only and not transferable in v1

**Decision:** A bot is owned by the human who created it. Ownership transfer, team-owned bots, and nested bot ownership are not part of v1.
**Why:** Single-owner bots are understandable, easy to cascade on account deletion, and avoid ambiguous token authority. Transfer and teams are useful but need separate lifecycle and audit rules.
**Tradeoff:** Teams that share an integration must coordinate through the current owner or use an admin with `bot.manage`.

### 3. Bot tokens are dedicated credentials

**Decision:** Bots use dedicated named API tokens rather than passwords, OAuth identities, browser cookies, or human bearer sessions.
**Why:** Bot credentials need names, explicit expiry, one-time secret display, revocation per token, and a recognizable prefix for secret scanning. Reusing human sessions would blur UI login with automation access. See ADR-041.
**Tradeoff:** The API has an additional credential family to maintain and document.

### 4. Token expiry is chosen when the token is issued

**Decision:** The token creator chooses 30/90/365 days, indefinite, or a custom expiration date, subject to the server's optional maximum TTL.
**Why:** Integrations vary: a short-lived CI token and a long-running bridge have different operational needs. The server max TTL gives operators a policy ceiling without forcing one expiry on every token.
**Tradeoff:** Operators must decide whether indefinite tokens are acceptable. When a max TTL is configured, creators may need to rotate long-lived integrations.

### 5. `bot.create` manages own bots; `bot.manage` manages others' bots

**Decision:** `bot.create` lets a human create self-owned bots and manage/delete bots they own. `bot.manage` lets a human manage other users' bot accounts and tokens.
**Why:** Owning an integration should not require full admin power, but operators need a direct permission that recovers or disables other users' bots. This follows the permission-only RBAC model in ADR-040 instead of treating role display positions as authorization ranks.
**Tradeoff:** `bot.manage` is a broad automation-administration capability and should be granted carefully.

### 6. Owner deletion cascades before human deletion

**Decision:** Human account deletion first hard-deletes owned bots, then deletes the human account. Bot cascade failure aborts the owner deletion.
**Why:** Ownerless bot accounts are ambiguous authority. Cascading first gives deletion a clear all-or-nothing boundary before irreversible crypto-shredding starts.
**Tradeoff:** A bot deletion bug or backend failure can temporarily block deleting the owning human account.

## Permissions

- `bot.create` — create self-owned bot accounts and manage/delete bots owned by the caller.
- `bot.manage` — manage/delete other users' bot accounts and tokens.

## Related

- **ADRs:** ADR-004 (authorization at API boundary), ADR-007 (per-user encryption with crypto-shredding), ADR-024 (opaque bearer tokens), ADR-036 (runtime state), ADR-041 (bot accounts as user accounts with dedicated API tokens)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-018 (Account Lifecycle), FDR-023 (Authentication & Sessions), FDR-025 (User Search & Member Directory)

## Open Questions

- Should bot ownership become transferable?
- Should teams or roles be able to own bots instead of individual humans?
- Should bot tokens gain per-token scopes or room restrictions?
- Should bot tokens support one-time rotation workflows with overlap windows?
- Should bot-created content display both bot identity and owner identity in message chrome, or is the profile/admin owner link enough?
