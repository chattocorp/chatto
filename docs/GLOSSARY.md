# Glossary

The canonical vocabulary for Chatto: UI surfaces, product concepts, authorization terms, and backend infrastructure. One line per entry (occasionally one short paragraph) — just enough to recognize the word and know where to read more.

This document is **also a naming surface**: when we need a name for a thing we're building, we add it here first. That's how vocabulary stays consistent across code, UI, docs, and conversation.

This is **not** a tutorial, design doc, or API reference. If a concept needs more than a paragraph, link to the relevant [FDR](fdr/INDEX.md), [ADR](adr/INDEX.md), `.claude/rules/*`, or [ARCHITECTURE.md](ARCHITECTURE.md) rather than inlining.

Entries within each section are ordered by **conceptual flow** — foundational terms first, derivatives after — not alphabetically. See [`.claude/skills/glossary/SKILL.md`](../.claude/skills/glossary/SKILL.md) for the maintenance workflow.

## UI

Names for visible surfaces and component groupings. When a name here disagrees with a file or component name in the codebase, the glossary wins — the file is the one that should rename.

**Server Gutter** — Narrow leftmost column listing the user's servers, with the add-server button at the bottom. Metaphor borrowed from the gutter in a text editor: a thin marginal strip. Implemented in `frontend/src/lib/ServerGutter.svelte`.

**Server Sidebar** — The wider sidebar to the right of the Server Gutter, scoped to a single server. Owns the per-server pane's chrome (positioning, mobile slide, resize, current-user bar pinned to bottom). The actual contents are passed in by `Chrome.svelte` — typically the server banner + header + room list, or the settings/admin nav while those modes are active. Implemented in `frontend/src/lib/components/ServerSidebar.svelte`.

**Room View** — The main central area showing the current room: message list plus the composer at the bottom. Not "the chat area" — *Room View* is the canonical name.

**Room Sidebar** — Right-hand pane scoped to the current room. Hosts room-specific extras such as the member list today and future surfaces like files or calls. Implemented in `frontend/src/routes/chat/[serverId]/[roomId]/RoomSidebar.svelte`.

**Composer** — The message input at the bottom of the Room View. Includes text input, attachment picker, emoji picker, mentions autocomplete.

**Pane Header** — The top bar of a content pane (Room View, settings page, admin page, etc.). Carries the title, optional subtitle, optional back arrow, and icon-only action buttons via the `actions` snippet. Chunky labelled buttons belong in the body, not here. See `.claude/rules/general.md`.

**Quick Switcher** — Cmd-K / Ctrl-K palette for jumping between rooms, DMs, servers, and admin pages. Distinct from the Server Gutter — both let you change server, but the Quick Switcher is keyboard-first and searchable. See [FDR-015](fdr/FDR-015-quick-switcher.md).

**Slideover** — A pane that slides in over existing content (e.g. settings, thread view on mobile). Distinct from a modal: dismissable by navigation, not by an explicit close.

**Hint** — Inline informational callout used in admin/settings panels to introduce or contextualise a control. Use instead of nesting an outer Panel around a self-contained matrix.

**Panel** — Bordered card used across instance-admin (`/chat/[serverId]/admin/*`) and per-server settings pages. Shared visual chrome for administrative interfaces. See `.claude/rules/admin.md`.

## Product

User-facing concepts. If a user might say the word, it goes here.

**Server** — Top-level Chatto deployment: one process, one NATS account, one membership boundary. Formerly called *Instance* in the codebase. See [ADR-029](adr/ADR-029-instance-to-server-rename.md).

**Space** — Legacy tier between server and room. Being consolidated into the server concept; in most deployments there is exactly one space per server (the *primary space*). See [ADR-027](adr/ADR-027-instance-space-server-consolidation.md).

**Primary Space** — Transitional config-designated "the one space that matters" within a server. Bridge construct used while Instance + Space collapse into Server. See [ADR-027](adr/ADR-027-instance-space-server-consolidation.md).

**Room** — A channel or DM. Where messages live. Identified by `(serverId, roomId)`.

**Room Group** — Named collection of rooms within a server, with its own per-group permission overrides. See [ADR-031](adr/ADR-031-room-group-centric-acl.md) and [FDR-017](fdr/FDR-017-room-groups-and-sidebar-layout.md).

**DM (Direct Message)** — Private conversation between users, modelled as a room with `kind: dm`. See [FDR-007](fdr/FDR-007-direct-messages.md).

**Message** — A user-posted entry in a room. Root messages live at the top level; thread replies hang off a root.

**Thread** — Reply chain rooted at a message. See [FDR-002](fdr/FDR-002-replies-and-threads.md).

**Echo** — Reposting a thread reply back to its parent channel so non-thread participants see it. Gated by `message.echo`. See [FDR-003](fdr/FDR-003-thread-reply-echo.md).

**Reaction** — Emoji attached to a message by a user. See [FDR-005](fdr/FDR-005-reactions.md).

**Mention** — `@handle` syntax in a message that notifies referenced users, pingable roles, or virtual room groups such as `@all` and `@here`. See [FDR-006](fdr/FDR-006-mentions.md).

**Attachment** — File (image, document, video) uploaded alongside a message. See [FDR-008](fdr/FDR-008-file-attachments-and-video.md).

**Link Preview** — Auto-generated preview card for URLs in messages. See [FDR-009](fdr/FDR-009-link-previews.md).

**Typing Indicator** — Ephemeral "X is typing…" signal. Published as a live event, never persisted. See [FDR-010](fdr/FDR-010-typing-indicators.md).

**Presence** — A user's online/away/offline state. See [FDR-011](fdr/FDR-011-user-presence.md).

**Voice Call** — Real-time audio call attached to a room. See [FDR-016](fdr/FDR-016-voice-calls.md).

**Jump to Present** — UI affordance that returns the Room View to the latest message after scrolling back through history. See [FDR-014](fdr/FDR-014-jump-to-present.md).

**Last-Room Memory** — The system that remembers which room a user was last in per-server. See [FDR-026](fdr/FDR-026-last-room-memory.md).

## Authorization

Chatto's RBAC model. Read top-to-bottom — terms build on each other.

**RBAC (Role-Based Access Control)** — The model: roles bundle permissions, users hold roles. See [ADR-005](adr/ADR-005-hierarchy-wins-rbac.md).

**Role** — Named bundle of permissions, assignable to users. System roles are seeded; custom roles can be created. Role names share the message-mention namespace with user logins, and each role can be marked pingable to allow `@role` pings.

**Permission** — Named capability gate, e.g. `message.post`, `role.assign`. Strings use hyphens, never underscores. The full list lives in `cli/internal/core/permissions.go`.

**Position** — Numeric rank of a role. Higher = more power. `everyone` = 0, `moderator` = 100, `admin` = 900, `owner` = 1000. Custom roles slot in the gaps.

**Rank** — Comparison between two users' highest role positions. Answers a hierarchy question ("does A outrank B?"), not a capability question.

**Outranking** — Hierarchy check: actor's highest role position must be strictly greater than the target's. Required *alongside* the relevant permission for any mutation targeting another user. See `.claude/rules/authorization.md`.

**Owner** — Top system role (position 1000). Conferred via `owners.emails` in `chatto.toml` or by another owner.

**Admin** — System role (position 900). Full administrative reach except over `owner`-rank users.

**Moderator** — System role (position 100). Moderation permissions, no administrative reach.

**Everyone** — Implicit virtual role (position 0) held by every authenticated user. Default-permission grants attach here.

**Scope** — Tier at which a permission is configured: `server`, `group`, or `room`. Resolution: room > group > server (first explicit decision wins). See `.claude/rules/authorization.md`.

**User-level override** — Permission grant or deny attached directly to a user, not via a role. Outranks every role grant. Used for suspensions and ad-hoc grants.

**DM Privacy Boundary** — Static set of permissions (`message.manage`, `message.echo`, `room.manage`, …) unconditionally denied inside DM rooms regardless of role grants. Owners can't moderate DM contents; DM read access comes from room membership, not a separate read permission. See [ADR-037](adr/ADR-037-dm-access-via-membership.md).

## Backend

Infrastructure jargon. If only contributors say the word, it goes here.

**ChattoCore** — Go package (`cli/internal/core`) that holds low-level domain logic and talks to NATS. Takes an `actorID` but performs no authorization — that lives in the GraphQL layer. See [ADR-004](adr/ADR-004-authorization-at-api-boundary.md).

**NATS** — Messaging system Chatto uses for pubsub and persistence. Runs embedded in the single binary by default.

**JetStream** — NATS's persistence layer (streams + KV buckets). Chatto's primary data store. See [ADR-001](adr/ADR-001-nats-jetstream-as-primary-data-store.md).

**Stream** — JetStream append-only log. Chatto's event-sourcing stream is `EVT`, which stores durable domain facts. See [ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) and [ARCHITECTURE.md](ARCHITECTURE.md#nats-resource-inventory).

**KV (Key-Value Bucket)** — JetStream-backed key/value store. Chatto uses several current buckets, especially `RUNTIME_STATE`, `MEMORY_CACHE`, and `ENCRYPTION_KEYS`; event-sourced domain state is sourced from `EVT`. See [ADR-033](adr/ADR-033-event-sourced-state-with-projections.md).

**Subject** — NATS message topic. Current durable facts use `evt.{aggregateType}.{aggregateId}.{eventType}`; transient sync uses `live.sync.…`; committed EVT facts are internally republished on `live.evt.…`. See `.claude/rules/nats-subjects.md` and [ARCHITECTURE.md](ARCHITECTURE.md#evt-subject-patterns).

**Event** — Durable domain fact stored on `EVT` using the `corev1.Event` wrapper. Contrast with *Live Event*.

**Projection** — In-memory read model rebuilt from `EVT` and owned independently by each Chatto process. Projections serve current-state and timeline reads while `EVT` remains the source of truth. See [ADR-033](adr/ADR-033-event-sourced-state-with-projections.md).

**Auth generation** — Per-user authentication epoch derived from durable user events. Cookie sessions, bearer tokens, and OAuth authorization codes are valid only when their stored generation matches the user's current generation. See [FDR-023](fdr/FDR-023-authentication-and-sessions.md).

**Live Event** — Transient `corev1.LiveEvent` published on `live.sync.>` (typing, notification sync, voice-call presence). Durable EVT facts reach live subscribers through the internal `live.evt.>` republish path after server-side projection readiness and authorization checks.

**Republish** — JetStream feature that mirrors accepted stream messages onto another NATS subject. Chatto uses it to expose committed EVT facts on `live.evt.>`; `myEvents` treats that as an internal feed, not a client contract. See `.claude/rules/nats-subjects.md`.

**OCC (Optimistic Concurrency Control)** — Publishing with an expected stream sequence so concurrent writers don't clobber each other. Used for message posting. See [ADR-016](adr/ADR-016-occ-for-message-publishing.md).

**Nanoid** — Short URL-safe unique ID format. All Chatto entities are prefixed (`usr_…`, `rm_…`, `srv_…`). See [ADR-022](adr/ADR-022-nanoid-with-entity-prefixes.md).

**Crypto-shredding** — Deleting a user's data by destroying the app-owned DEK refs and KMS wrapping-key refs that protect their encrypted content rather than mutating storage. See [ADR-007](adr/ADR-007-per-user-encryption-with-crypto-shredding.md).
