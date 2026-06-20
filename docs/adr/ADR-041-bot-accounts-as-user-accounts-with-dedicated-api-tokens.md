# ADR-041: Bot Accounts as User Accounts with Dedicated API Tokens

**Date:** 2026-06-07

## Context

Chatto needs automation identities for integrations, bridges, CI workflows, and custom clients. These actors need to post messages, react, join rooms, and use the same permission model as human users. They also need credentials that are safe to copy into deployment systems and easy to revoke.

There are two main modeling choices:

- Add a separate actor type for bots, with separate membership, permission, message-author, mention, deletion, and profile rules.
- Model bots as a kind of user, while separating how they authenticate.

There are also two credential choices:

- Let bots use the existing human login/session paths.
- Give bots dedicated API tokens with bot-specific metadata, expiry, and revocation.

## Decision

Model bot accounts as first-class `User` records with `kind: BOT`, an owner user ID, normal RBAC role assignment, normal user-level permission overrides, and the same deletion/crypto-shredding lifecycle as human accounts.

Bots are deliberately excluded from human login surfaces. Password login, OAuth login, cookie sessions, and human bearer-session issuance only authenticate human users. Bots authenticate through named bot API tokens. The GraphQL HTTP path, GraphQL WebSocket token path, and API upload paths accept both human bearer tokens and bot API tokens through the shared API-token validation path; normal browser session routes reject bots.

Bot API tokens are opaque random secrets with a distinct `cht_BT...` prefix. Their `RUNTIME_STATE` records are keyed by an HMAC of the raw token and store only safe metadata: token name, bot user ID, creator, created time, last-used time, nullable expiry, and revocation metadata. The raw token is returned only once on creation and is never stored in EVT, logs, or GraphQL metadata.

Token expiry is fixed at issuance. Creators can choose 30 days, 90 days, 365 days, indefinite, or a custom timestamp. Operators can configure a maximum bot-token TTL; when set, indefinite tokens are disabled and every fixed/custom expiry must be within the ceiling.

## Consequences

- **Reuse of existing identity machinery.** Message authorship, mentions, room membership, profile display, role assignment, permission overrides, user search, member directories, deletion, and crypto-shredding work with bots without inventing parallel actor tables.
- **Clear visual accountability.** Bots appear as members and authors, but UI surfaces can badge them and link to their owner.
- **Existing RBAC remains the source of truth.** A bot can do whatever its roles and overrides allow, and nothing more. Bot-management actions are gated by explicit bot permissions, not by role position.
- **Credential families stay semantically separate.** Human sessions remain for people and browser clients. Bot tokens are named, fixed-expiry automation credentials.
- **Instant token revocation.** Revoking a bot token marks its runtime-state record revoked, causing validation to reject it immediately while retaining safe metadata for token lists.
- **Runtime state remains the credential boundary.** Bot token records are durable enough to survive restarts and restores with the same `[core].secret_key`, but backups do not contain raw token secrets.
- **Owner trust is explicit.** Giving powerful roles to a bot delegates those powers to the bot owner, because the owner controls the bot's tokens.
- **No anonymous automation actor.** Every bot has an owner. Human deletion cascades through owned bots before deleting the human to avoid ownerless automation accounts.
- **More UI/documentation surface.** Member views, profiles, docs, and API clients must handle `User.kind`, bot badges, owner links, and named-token management.
