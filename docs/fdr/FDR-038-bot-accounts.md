# FDR-038: Bot Accounts

**Status:** Experimental
**Last reviewed:** 2026-08-27

## Overview

Bot accounts let people run integrations as clearly identified Chatto users
without sharing a human login credential. A bot can use the normal public API
within an explicit permission allowlist, but it cannot sign in as a person or
exercise more authority than its human owner currently possesses.

## Behavior

- A human user with `bot.create` can create a bot account and becomes its
  owner.
- Server Admin's Bots page lists the bots visible to the caller and creates new
  bots. Selecting a bot opens its own detail page for login and display-name
  editing, key rotation, deletion, metadata, and permissions. Bot avatar,
  custom-status, and personal-settings management are not supported in this
  slice.
- On a fresh RBAC bootstrap, `everyone` receives `bot.create`, while `admin`
  and `owner` have `bot.manage`. The owner grant follows Chatto's normal
  effective-owner override rather than being stored as an editable permission
  row.
- Bot status and ownership are explicit, durable account properties. A login
  suffix is a naming rule, not the source of truth for whether an account is a
  bot.
- Bot logins must end in `_bot`, matched case-insensitively. New human accounts
  and human login changes cannot claim that suffix.
- Existing human accounts that already use an `_bot` login remain human
  accounts. They are not silently converted into bots, but other human
  accounts cannot newly claim or rename into the reserved suffix.
- Bot accounts are visible wherever ordinary users are visible, including
  messages, profiles, directories, mentions, direct messages, and member
  management. User identity displays mark them as bots with an accessible
  visual indicator.
- A bot has one active API key. The key is returned only when the bot is
  created or the key is rotated; it cannot be retrieved later.
- The bundled frontend stops in-app navigation while it requests a show-once
  bot credential and while it shows that credential. It asks for confirmation
  before the browser unloads the page. Navigation becomes available after the
  manager acknowledges the credential.
- A bot can have at most 20 active, named incoming webhooks. Creating one
  webhook shows its complete URL once. A manager can create a replacement
  before the manager revokes an old webhook.
- The bot detail page shows when each incoming webhook was created and
  approximately when Chatto recorded its last use. Chatto attempts to record a
  successful credential authentication even if the request subsequently
  fails. If Chatto has no observation, the page shows "No use recorded." This
  state does not prove that the credential was not used. If Chatto cannot read
  this optional telemetry, the page shows that it is temporarily unavailable.
- An incoming webhook can post plain-text messages as the bot. It accepts
  Slack-compatible `text` and `channel` fields, Chatto `body` and `room_id`
  aliases, an optional `room_id` query parameter, and the Chatto
  `create_thread` extension. All specified destinations and bodies must agree.
- An incoming webhook uses stable room IDs. It can select any channel room
  where the bot is a member and has the normal posting permissions. It can
  select an existing human-started DM that contains the bot. It cannot create
  or find a DM, and it cannot create a thread in a DM.
- Newly issued keys use a 128-bit random secret to remain compact enough for
  copy-and-paste workflows. Previously issued 256-bit keys remain valid until
  they are rotated.
- API keys do not expire through inactivity. Rotating a key immediately
  invalidates the previous key and terminates realtime connections established
  with it. Deleting the bot or its owner also invalidates the key.
- A bot API key authenticates normal public API and realtime requests as that
  bot. The bot can otherwise participate like a user wherever its explicit
  permissions allow.
- A bot uses the normal `subscribe_events` realtime subscription. Chatto sends
  its visible notification occurrences through
  `notification_occurrences_replace`; there is no bot-only realtime channel.
- Direct-message, direct-mention, reply, and followed-thread occurrences are
  the supported activation causes for bot integrations. The bot uses the
  message reference in the occurrence to fetch context through the normal API.
  Other notification causes can be present, so the integration must filter by
  cause.
- A delivered direct mention in a channel-room root or reply attempts to
  follow that thread if the bot has no prior follow state. Later replies can
  then create followed-thread occurrences for the bot.
- A direct-mention policy of Off creates no occurrence and no mention-driven
  follow. It does not remove the interaction relationship created by the
  durable mention fact.
- Bots do not inherit the implicit `everyone` role, named-role permissions, or
  any other baseline grants. An absent bot permission is denied.
- Channel-room membership does not give a bot message content. The bot needs
  an explicit `message.read` grant for broad access or an explicit
  `message.read-interactions` grant for related threads. The broad grant
  includes the narrow permission. Each grant is bounded by sufficient
  effective authority on its owner. DM membership authorizes the bot to read
  that DM.
- A bot cannot start or fetch a DM through `RoomService.StartDM`, even if it
  has `message.post` or the DM already exists. A human must start a DM that
  includes the bot. The bot can then interact in that DM through its normal
  message permissions.
- Bot permissions are granted explicitly at their applicable server, room
  group, or room scope. The bot's effective permission is allowed only when
  both the bot's allowlist and its owner's current effective permissions allow
  it at that scope.
- Bot permission mutations accept only allow or clear; explicit denials are
  rejected. The editor therefore presents each applicable permission as
  enabled or disabled instead of exposing RBAC's general three-state control.
  A direct grant can be cleared. An inherited grant is read-only at the
  narrower scope and must be changed where the broader grant was configured.
- Losing the owner's sufficient authority immediately removes the corresponding
  effective permission from every bot they own. A stored bot grant can become
  effective again if the owner later regains the required permission.
- The bot editor marks permissions the owner cannot grant as locked. A stored
  bot grant that is currently ineffective because of the owner's permission
  ceiling remains visibly enabled, but is shown as unavailable and distinct
  from both active grants and unconfigured permissions.
- A bot owner can view, update, rotate the key for, configure permissions for,
  and delete their own bots. Losing `bot.create` does not remove management of
  bots they already own.
- The permission matrix exposes room scopes only when they are visible through
  the normal room-directory policy to both the bot owner and the managing
  caller. It exposes the complete directory group layout, including empty
  groups, so group-scoped permissions such as `room.create` remain usable.
- A human user with `bot.manage` can manage any bot. Changes made by a global
  bot manager remain bounded by that bot's owner's permission ceiling.
- A human user with `bot.manage` can reassign a bot to another active human
  account. Ownership alone does not authorize reassignment, and the recipient
  does not need to accept it or hold `bot.create`.
- Reassignment preserves the bot's configured permission allowlist and active
  API key. Effective permissions immediately use the new owner's permission
  ceiling. The previous owner loses owner-derived management access, while the
  new owner gains it; either person may still have independent `bot.manage`.
- Bot accounts cannot create, own, or manage other bots, even if a
  bot-management permission appears in their stored allowlist.
- Bots cannot have passwords, verified emails, external identities, browser
  sessions, OAuth access tokens, password-reset flows, or other human sign-in
  methods. A bot API key can update its own public profile through
  `MyAccountService.UpdateProfile`, but cannot change ownership, permissions,
  or API keys.
- Bots cannot request their own deletion. Only their owner or a human user with
  `bot.manage` can delete them through `BotService`.
- Deleting a bot uses the normal account-deletion and crypto-shredding
  behavior. Deleting a human owner also deletes every bot they own at deletion
  time and revokes those bots' API keys. Bots reassigned beforehand remain
  active.
- Bot accounts count toward the server's user limit.

## Design Decisions

### 1. Explicit account kind, independent of the login suffix

**Decision:** Bot status is an immutable account kind. The `_bot` suffix is a
separate validation rule for current bot and human logins.
**Why:** Clients and authorization rules need a stable way to distinguish bots
from people. Inferring identity from a name would make existing accounts
ambiguous and would prevent the suffix rule from changing or becoming
operator-configurable later.
**Tradeoff:** Account creation, profile projection, public user shapes, and
identity rendering all need to carry the account kind explicitly.

### 2. User identity with non-human authentication

**Decision:** A bot is a normal user identity for API resources, authorship,
membership, and public rendering, but has a separate API-key authentication
path and cannot enrol a human sign-in method.
**Why:** Integrations should use the same rooms, messages, mentions, and public
APIs as people without requiring parallel bot-only resource models. Separating
authentication keeps an API credential from becoming an interactive login.
**Tradeoff:** Account-security and credential-enrolment operations must enforce
the account-kind boundary rather than treating every passwordless account as
eligible for a password or external identity. Self-profile operations can stay
shared because they always target the authenticated identity.

### 3. Explicit allowlist instead of normal role inheritance

**Decision:** Bots receive permissions only from explicit decisions in the
canonical user permission matrix. They do not receive the implicit `everyone`
baseline or named-role grants, and absence means deny.
**Why:** API keys are long-lived automation credentials and should start with
no ambient authority. Owners should be able to explain a bot's access from one
explicit matrix rather than by combining roles and server defaults.
**Tradeoff:** Owners must grant even ordinary member capabilities before a new
bot can do useful work, and newly introduced permissions do not automatically
become available to existing bots. Owners cannot carve out a denied narrower
scope beneath a broader bot grant; they must clear the broader grant and add
only the narrower grants the bot should retain.

### 4. The owner's current authority is a dynamic ceiling

**Decision:** A bot permission is effective only while the bot's human owner
also has that permission at the relevant scope. This ceiling applies during
server-side authorization, not only when the owner edits the matrix.
**Why:** A one-time grant check would let a bot retain authority after its owner
lost it. A dynamic ceiling makes bot delegation attenuating and keeps owner
revocation meaningful.
**Tradeoff:** Bot authorization depends on both bot and owner state. A stored
grant may appear configured but temporarily ineffective, so management UIs
must show the owner's ceiling and explain unavailable cells.

### 5. Bot creation is broadly available; global management is administrative

**Decision:** `bot.create` lets a human create bots, ownership lets them manage
their existing bots, and `bot.manage` lets a human manage any bot. Fresh RBAC
state grants `bot.create` to `everyone` and `bot.manage` to `admin`; effective
owners receive `bot.manage` through the normal virtual owner override. Bots do
not inherit `everyone` and cannot exercise either capability themselves.
**Why:** Every human member can create automation for their own use, while
global recovery and moderation remain administrative. Creators must not lose
access to rotate or delete an existing bot just because their ability to create
more bots is revoked.
**Tradeoff:** Servers that want bot creation to be restricted must change the
fresh default, and existing servers are not backfilled when defaults change.
Bot-management authorization also has an ownership path alongside the
permission path used by global managers.

### 6. One long-lived, show-once API key

**Decision:** Each bot has one non-expiring API key. Chatto shows the raw key
only at creation or rotation, and rotation immediately replaces the prior key.
New keys contain 128 bits of random secret; the verifier accepts the earlier
256-bit format so shortening issuance does not revoke existing integrations.
**Why:** A stable bearer credential makes unattended API integrations simple,
while a single active key gives owners a clear revocation and recovery model.
Not retaining a retrievable raw key reduces secret exposure.
**Tradeoff:** Losing the key requires rotation, and rotation requires every
consumer of that bot to update at once. Multiple independently rotatable keys
per bot are deferred. Established realtime connections retain only a
non-secret verifier generation and close when the durable rotation reaches the
local authentication projection.

### 7. Administrative reassignment preserves running integrations

**Decision:** A human with `bot.manage` can reassign a bot directly to another
active human account. Reassignment keeps the configured permission allowlist
and active API key, while immediately applying the new owner's permission
ceiling. Deleting a human still cascades to bots they own at deletion time.
**Why:** Operational handoffs need a recovery path before an owner leaves, but
do not require a two-party invitation protocol. Keeping credential rotation a
separate explicit action avoids unnecessary integration downtime; an operator
can still rotate immediately when credential custody is in doubt.
**Tradeoff:** Chatto relies on the administrator to verify that the recipient
requested or accepts the handoff. A permission may become active or inactive
as soon as the owner changes, so the UI warns about the new ceiling. Existing
credential holders retain access until an administrator separately rotates the
key.

### 8. Bot identity is visible on canonical user surfaces

**Decision:** Public user representations identify bot accounts, and clients
render an accessible bot marker anywhere user identity is presented.
**Why:** People should know when messages or other actions come from automation.
Using the canonical user representation keeps the distinction consistent
across existing and future surfaces.
**Tradeoff:** Older clients that do not understand the additive bot marker will
render a bot like an ordinary user until they are upgraded.

### 9. Bots cannot start DMs

**Decision:** Bot account kind always denies `RoomService.StartDM`. This rule
applies before RBAC and has no owner override. A human can start a DM with a
bot, after which DM membership and normal message permissions apply.
**Why:** An automation credential must not create an unsolicited private
conversation. The account-kind rule is visible and cannot be enabled by an
incorrect permission grant.
**Tradeoff:** A bot cannot use the idempotent start operation to get an
existing DM. It must use the room state that Chatto sends after a human starts
the DM.

### 10. Notification occurrences are the bot activation contract

**Decision:** Bots receive the same exact notification occurrences as human
accounts through `NotificationService` and the normal realtime projection.
Integrations use direct messages, direct mentions, replies, and
followed-thread activity as activation causes. A future webhook transport must
deliver these same occurrences instead of introducing separate bot events.

**Why:** Notification occurrences already own recipient selection, user policy,
current visibility, stable identity, and bounded recovery. Reusing them keeps
activation semantics independent of transport and avoids a second event model
that can disagree with the notification model.

**Tradeoff:** Realtime replacements can repeat occurrences and can contain
more than one cause for the same message. Integrations must checkpoint
occurrence IDs and can deduplicate by the referenced message event ID when they
want one action for each source message. The current realtime replacement
contains only the newest finite page; longer recovery uses the paginated
notification API.

### 11. Incoming webhooks use a separate action credential

**Decision:** A bot can have at most 20 active, named incoming webhook
credentials. Each credential can call only the incoming webhook HTTP endpoint.
The endpoint posts through the normal message operation as the bot. Each
credential can select a room through the request URL or JSON payload. Chatto
shows each raw URL only when it creates the selected credential. A manager
creates a replacement before the manager moves a caller and revokes the old
credential. The bot detail page shows creation metadata and an approximate
last-use time for each credential. The bundled frontend stops in-app
navigation until it shows the raw credential and the manager acknowledges it.

**Why:** An external system can post a message without receiving the bot's
complete API authority. Dynamic room selection keeps one automation usable
across the rooms that the bot can already access.

**Tradeoff:** The credential is in the webhook URL and needs the same secret
handling as an API key. The first version has no idempotency key. A retry after
a lost response can create a duplicate message. Last-use telemetry is
best-effort and can be delayed, unavailable, or missing after a process or
storage failure. Rich Slack payloads and replies to existing threads are
deferred.

## Permissions

- `bot.create` — create bot accounts and become their owner.
- `bot.manage` — view and manage every bot on the server, including reassigning
  its owner, while preserving the current owner's permission ceiling.
- `message.read` — give the bot broad message access in configured channel
  rooms, subject to membership and the owner's effective broad-read authority.
  This grant includes `message.read-interactions`.
- `message.read-interactions` — give the bot complete access to a
  channel-room thread that it started or where another account directly
  mentioned it, subject to membership and the owner's effective broad or
  narrow read authority.

Notification delivery modes are user preferences, not permissions. A bot can
change its own notification policy through the normal notification policy API
when it has access to the selected scope.

Fresh RBAC bootstrap grants `bot.create` to `everyone` and `bot.manage` to
`admin`. Effective owners have `bot.manage` through the virtual owner override,
so no editable owner permission row is seeded.

Bot ownership authorizes management of the owner's existing bots without a
separate permission. Effective owners retain their normal all-permissions
override, but bots themselves cannot exercise bot-management operations.

## API Compatibility

Bot identity, management, reassignment, permission ceilings, and credentials
are additive in Chatto 0.5. Historical users remain human accounts when bot
identity is absent. Older clients can still render bot accounts as ordinary
users, and the bundled client does not call bot-management operations on an
older server.

Operators must replace all replicas before they create or rotate bot
credentials, reassign bot ownership, or depend on the rule that bots cannot
start DMs. An older replica can omit newer bot facts or accept the earlier DM
behavior. Updated servers continue to accept the first longer bot-key format
while issuing the current shorter format. Exact field, method, event, and
version-gate details belong in the public schema and API compatibility guide.

Incoming webhook management methods, metadata, and lifecycle facts are
additive. The create response shows the URL one time. Older clients ignore the
metadata and do not call the new methods. Replace all replicas before you
create or revoke a webhook. An older replica cannot project multiple webhook
credentials correctly after replay and cannot authenticate the new URL format.
Current servers read the rotation fact from the unreleased implementation, but
they do not write it.

## Related

- **ADRs:** ADR-007 (per-user encryption and crypto-shredding), ADR-033
  (event-sourced state), ADR-036 (runtime state), ADR-040 (permission-only RBAC
  with owner override), ADR-045 (public API stability tiers), ADR-046 (typed
  runtime credentials), ADR-051 (resumable client projection), ADR-052
  (subject-specific RBAC), ADR-076 (deterministic notification occurrences),
  ADR-077 (persistent notification list), ADR-080 (explicit message-read
  permissions), ADR-083 (action-limited bot incoming webhooks)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-006
  (@Mentions), FDR-007 (Direct Messages), FDR-012 (Notifications), FDR-018
  (Account Lifecycle), FDR-022 (User Profile), FDR-023 (Authentication &
  Sessions), FDR-025 (User Search & Member Directory), FDR-039 (Message Access
  & Interactions)

## Open Questions

- Multiple named bot API keys, independent revocation, last-use telemetry, and
  key expiry are deferred. A future design should
  reuse the incoming-webhook credential lifecycle and usage-recording patterns
  where they apply. It must also define compatibility for the current
  two-part bot API key format.
- Define durable webhook registration, signing, retry, and delivery status for
  the same bot activation occurrences.
