# FDR-039: Message Access & Interactions

**Status:** Experimental
**Last reviewed:** 2026-08-25

> Slice 1 implements broad access through `message.read`. Interaction-scoped
> access remains design work in
> [#2089](https://github.com/chattocorp/chatto/issues/2089).

## Overview

Message access controls which channel-room message content Chatto can give to
a human or bot. The first slice makes broad channel-room access explicit. DM
membership continues to authorize complete DM reads. A later slice can add
narrow channel-room access for direct interactions after its life cycle and
inspection model are complete.

## Behavior

- Channel-room membership remains necessary for message access. It is not
  sufficient.
- Every human and bot needs `message.read` to read channel-room message content
  in the first slice.
- `message.read` applies at server, room-group, and room scope.
- A DM participant can read the complete DM. `message.read` decisions do not
  restrict DM reads.
- Fresh servers grant `message.read` to `everyone` at server scope when they
  bootstrap an empty RBAC stream.
- Existing servers receive no automatic grant. Operators decide how to update
  their RBAC state.
- Bots do not inherit `everyone`. A bot needs an explicit `message.read` grant
  for channel-room reads.
- A bot's grant is effective only while its owner has `message.read` at the
  same applicable scope.
- Message-read authority does not grant write authority. An account needs the
  normal permission for each post, upload, reaction, or moderation action.
- Channel-room operations that read a current message or aggregate message
  state while they mutate it, such as editing, reacting, creating a hydrated
  pin, and changing thread-follow state, also require `message.read`. Posting
  and deletion remain independently authorized and do not return surrounding
  message state.
- A denied account can still see channel-room metadata that normal
  room-visibility rules allow. It cannot receive channel-room message bodies or
  message-specific metadata.
- The same channel-room decision applies to public timeline and thread APIs,
  pinned-message reads, search, attachment metadata and bytes, message-derived
  notifications, followed-thread state, unread message state, typing
  indicators, and realtime message delivery. DM versions of these surfaces use
  membership.
- The normal realtime protocol carries authorized message updates for human
  and bot clients.
- Old server replicas do not enforce `message.read` in channel rooms. Operators
  must complete the rollout before they rely on denial.
- A new client connected to an old server treats the absent room-permission row
  as unsupported and keeps the former membership-based navigation behavior.

## Design Decisions

### 1. All accounts use one explicit broad-read permission

**Decision:** Require `message.read` for broad channel-room message access by
humans and bots. Keep channel-room membership as a separate required boundary.
Keep DM reads membership-based under ADR-037.
**Why:** Operators can inspect and remove message access directly. Humans and
bots use the same permission vocabulary.
**Tradeoff:** Each channel-room message-content boundary needs one more
authorization decision. Operators cannot use this permission to hide a DM from
one of its participants.

### 2. Bootstrap defaults do not change existing RBAC

**Decision:** Grant `message.read` to `everyone` only when Chatto initializes an
empty RBAC stream. Do not migrate, backfill, or reconcile existing servers.
**Why:** An absent permission decision is operator-owned state. Startup must
not replace it with a new code default.
**Tradeoff:** Operators must grant `message.read` during an upgrade if they
want to preserve existing channel-room access.

### 3. Bot delegation uses the existing exact owner ceiling

**Decision:** A bot's direct `message.read` grant for channel-room access is
effective only when its owner has the same permission at the applicable scope.
**Why:** This uses the existing bot security model. It does not need special
intersection rules.
**Tradeoff:** Removing `message.read` from an owner also removes channel-room
read access from bots that the owner controls.

### 4. Read and write permissions remain separate

**Decision:** `message.read` does not grant any message action. A channel-room
mutation that reads or returns existing message state also needs `message.read`
in addition to its normal write authority. DM mutations use membership and the
normal action permission.
**Why:** Read-only accounts and posting-only automation accounts are valid
configurations. Mutation responses must not bypass the content boundary.
**Tradeoff:** Editing, reacting, creating hydrated pins, and changing
thread-follow state need both read and write authority.

### 5. Message-derived surfaces share the same boundary

**Decision:** Enforce the current `message.read` decision across channel-room
request reads, search, attachment delivery, notifications, unread state,
thread-follow state, typing indicators, and realtime delivery. Use membership
for the same DM surfaces.
**Why:** A secondary surface must not reveal content or metadata that the
primary timeline rejects.
**Tradeoff:** Permission changes can remove notification, unread, and followed
thread presentation while room metadata remains visible.

### 6. Interaction-scoped access is a separate slice

**Decision:** Do not add a narrow read permission or implicit exception in
Slice 1.
**Why:** Mentions and authored roots are useful channel-room access causes, but
their end conditions, durable state, inspection, and removal need a complete
design. DM membership already authorizes DM access.
**Tradeoff:** A bot that only needs one channel-room interaction still needs
broad `message.read` at the applicable scope for now.

### 7. Bots use the normal realtime protocol

**Decision:** Deliver authorized bot message events through the same
authenticated realtime protocol that human clients use.
**Why:** One transport keeps the existing authentication, resume, reset, and
cursor rules.
**Tradeoff:** A small bot can receive more non-message projection state than it
needs.

## Permissions

- `message.read` — read channel-room message content and message-specific
  metadata that normal room visibility makes available at the configured
  scope.
- `message.post` — post root messages and send messages in an existing direct
  message.
- `message.post-in-thread` — post replies in an authorized channel-room
  thread.

Fresh servers grant `message.read` to `everyone` at server scope. Existing
servers are not changed. Bots do not inherit the fresh-server grant.

## Related

- **ADRs:** ADR-031 (room-group permission scopes), ADR-037 (DM access through
  membership), ADR-040 (permission-only RBAC with owner override), ADR-045
  (public API stability), ADR-051 (resumable client projection), ADR-080
  (`message.read`)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-004
  (Message Editing & Deletion), FDR-005 (Reactions), FDR-006 (@Mentions),
  FDR-007 (Direct Messages), FDR-008 (File Attachments & Video Processing),
  FDR-010 (Typing Indicators), FDR-012 (Notifications), FDR-033 (Message
  Search), FDR-037 (Pinned Messages), FDR-038 (Bot Accounts)

## Open Questions

- Select the permission name for interaction-scoped access.
- Define which actions start access. Candidates are a direct mention and an
  authored thread root.
- Define when interaction access ends and who can end it.
- Define a durable boundary that prevents removed access from returning after
  permission restoration or room re-entry.
- Define the result when an author edits or deletes an access-causing mention.
- Define how attachments, reactions, deleted-message placeholders, previews,
  search results, and referenced messages follow the interaction boundary.
- Define how profiles and threads show active access and its cause.
- Define realtime resume, reset, pagination, duplicate removal, and
  acknowledgment behavior for unattended bots.
