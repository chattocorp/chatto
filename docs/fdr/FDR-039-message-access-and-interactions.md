# FDR-039: Message Access & Interactions

**Status:** Experimental
**Last reviewed:** 2026-08-27

## Overview

Message access controls which message content Chatto can give to a human or
bot. Channel rooms use explicit broad or interaction-scoped read permissions.
DM membership continues to authorize complete DM reads.

## Behavior

- Channel-room membership is always necessary for message access. It is not
  sufficient.
- `message.read` gives broad access to message content in a channel room and
  includes `message.read.interactions`.
- `message.read.interactions` gives access only to channel-room threads where
  the account has an interaction relationship.
- The same permissions and rules apply to human and bot accounts.
- A direct mention from another account creates an interaction relationship.
  The mention can be in a root message or a reply.
- Authoring a channel-room root message creates an interaction relationship
  with that thread.
- A self-mention, role mention, `@all`, `@here`, or authored reply does not
  create an interaction relationship.
- A relationship gives access to the complete thread. This includes the root,
  replies from before the relationship started, and future replies.
- A relationship is a post-time fact. Editing or retracting the message that
  caused it does not remove the relationship. Current edited content and
  retraction tombstones still apply.
- This slice has no action to end a relationship. Permission loss or room
  membership loss closes current access. Permission restoration or room
  re-entry opens an existing relationship again.
- A DM participant can read the complete DM. `message.read` and
  `message.read.interactions` decisions do not restrict DM reads.
- Message-read authority does not grant write authority. Each post, upload,
  reaction, edit, or moderation action needs its normal permission.
- A channel-room operation that reads or returns an existing message also
  needs access to that message's thread. Deletion remains independently
  authorized and does not return surrounding message state.
- The same access decision protects timelines, individual messages, thread
  reads, pins, search, attachments and file bytes, message-derived
  notifications, unread state, thread-follow state, typing indicators, and
  realtime delivery.
- A room timeline for an account with only interaction-scoped access contains
  the roots of threads that the account can read. The account can then read
  each complete thread through the thread API.
- Main-room typing indicators require broad access. A thread typing indicator
  is visible when the account can read that thread.
- The normal realtime protocol carries authorized updates for retained room
  timelines. A client that knows a thread root can use the thread API to get
  complete context.
- The normal realtime protocol also carries notification occurrence
  replacements. A bot can use direct-mention, direct-message, reply, and
  followed-thread occurrences to learn the message and thread IDs that it must
  load through the normal API.
- A delivered direct mention in a channel-room root or reply attempts to
  follow that thread when the recipient has no prior follow state. Notification
  policy Off suppresses the occurrence and this follow, but it does not remove
  the interaction relationship.
- The public API does not list or inspect interaction relationships. A thread
  read succeeds or fails after the server applies the current access rules.
- Fresh servers grant only `message.read` to `everyone` at server scope when
  they initialize an empty RBAC stream. That effective allow includes the
  interaction permission.
- Existing servers receive no automatic grant. Operators must review and
  update existing RBAC state.
- Bots do not inherit `everyone`. A bot needs an explicit grant for each read
  mode that it uses.
- A bot read grant is effective only while its owner has sufficient effective
  read authority at the applicable scope. `message.read` can satisfy the
  narrower requirement for the bot or its owner.
- Old replicas know only broad `message.read`. During a mixed rollout, they
  deny interaction-scoped reads instead of giving broad access. Operators must
  complete the rollout before they depend on interaction-scoped availability.

## Design Decisions

### 1. Broad and interaction-scoped access use separate permissions

**Decision:** Use `message.read` for broad channel-room access and
`message.read.interactions` for relationship-scoped channel-room access. Keep
membership as a separate required boundary. Make the broad permission
explicitly include the narrower permission. Do not infer other inclusions from
dotted names. The child does not include the parent. A child deny cannot
restrict an effective parent allow, and a parent deny cannot restrict a
separate child allow.
**Why:** Operators can inspect the difference between broad and narrow access.
An absent broad permission does not cause an implicit privacy mode.
**Tradeoff:** Each narrow read checks both RBAC and the requested thread
relationship. The resolver and inspection surfaces must explain the explicit
inclusion.

### 2. Direct mentions and authored roots create relationships

**Decision:** Create a relationship when another account directly mentions the
account or when the account authors a channel-room root. Do not use broad,
role, self, or authored-reply causes.
**Why:** These causes show an intentional interaction with one account or a
thread that the account started. Broadcast causes do not show the same intent.
**Tradeoff:** A bot that authors only a reply does not gain read access from
that reply.

### 3. One relationship gives the complete thread

**Decision:** Give access to the root and all replies when a relationship
exists.
**Why:** A mention without earlier context is often not useful. A full thread
is also a clear resource boundary for APIs and realtime filtering.
**Tradeoff:** A user can give the mentioned account access to earlier content
in that thread. The user interface and documentation must make this rule
clear.

### 4. Relationships are immutable post-time facts in this slice

**Decision:** Keep a relationship after an edit or retraction. Do not add an
end action in this slice.
**Why:** A mention already occurred, and source message facts keep typed
mention provenance. This rule also matches the post-time notification model.
**Tradeoff:** Permission restoration or room re-entry can open an old
relationship again. A later explicit end feature will need a durable end fact.

### 5. All message-derived surfaces use one thread boundary

**Decision:** Apply broad or interaction-scoped access to every surface that
can expose channel-room message content or message-specific metadata. Keep DM
versions membership-based.
**Why:** Search, notifications, files, typing, or realtime must not bypass the
primary timeline boundary.
**Tradeoff:** List and room-wide surfaces must filter their results instead of
using one room-level allow decision.

### 6. New-server defaults do not change existing RBAC

**Decision:** Grant only `message.read` to `everyone` during empty-RBAC
bootstrap. Its effective allow includes `message.read.interactions`. Do not
migrate, backfill, or reconcile an existing server.
**Why:** Existing RBAC state belongs to the operator. Startup must not replace
an absent decision with a code default.
**Tradeoff:** Operators must add the new grant during an upgrade when they want
interaction-scoped reads.

### 7. Bots use the existing owner ceiling

**Decision:** Use the same permissions for human and bot accounts. Apply the
explicit read inclusion independently to the bot allowlist and to the owner's
effective authority.
**Why:** This keeps one permission vocabulary and one delegation rule.
**Tradeoff:** Removing broad read access from an owner does not remove a bot's
narrow read mode when the owner still has the narrow permission. A bot still
needs its own broad or narrow grant.

### 8. Relationships are not public resources

**Decision:** Do not add public operations that list or inspect interaction
relationships. A client supplies a known thread root to the normal thread API,
which applies the current access rules.
**Why:** A relationship is an authorization input, not a user-managed resource.
This keeps internal cause metadata out of the public API.
**Tradeoff:** Clients cannot enumerate related threads. A bot learns the
relevant message and thread IDs from its normal notification occurrences.

## Permissions

- `message.read` — read all message content and message-specific metadata in a
  channel room at the configured scope. It includes
  `message.read.interactions`.
- `message.read.interactions` — read message content and message-specific
  metadata only in channel-room threads with a current interaction
  relationship.
- `message.post` — post root messages and send messages in an existing DM.
- `message.post-in-thread` — post replies in a channel-room thread.

DM membership, not a message-read permission, authorizes DM reads.

## Related

- **ADRs:** ADR-031 (room-group permission scopes), ADR-037 (DM access through
  membership), ADR-040 (permission-only RBAC with owner override), ADR-045
  (public API stability), ADR-051 (resumable client projection), ADR-080
  (`message.read`), ADR-082 (derived interaction relationships)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-004
  (Message Editing & Deletion), FDR-005 (Reactions), FDR-006 (@Mentions),
  FDR-007 (Direct Messages), FDR-008 (File Attachments & Video Processing),
  FDR-010 (Typing Indicators), FDR-012 (Notifications), FDR-033 (Message
  Search), FDR-037 (Pinned Messages), FDR-038 (Bot Accounts)

## Open Questions

- Define an explicit end action and the accounts that can use it.
- Define profile and administration views that show active relationships.
