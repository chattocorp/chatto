# Glossary

The canonical vocabulary for Chatto: UI surfaces, product concepts, authorization terms, and backend infrastructure. One line per entry (occasionally one short paragraph) — just enough to recognize the word and know where to read more.

This document is **also a naming surface**: when we need a name for a thing we're building, we add it here first. That's how vocabulary stays consistent across code, UI, docs, and conversation.

This is **not** a tutorial, design doc, or API reference. If a concept needs more than a paragraph, link to the relevant [FDR](fdr/INDEX.md), [ADR](adr/INDEX.md), [`AGENTS.md`](../AGENTS.md) and directory-specific `AGENTS.md` files, or [architecture inventory](architecture/INDEX.md) rather than inlining.

Entries within each section are ordered by **conceptual flow** — foundational terms first, derivatives after — not alphabetically. See [`.agents/skills/glossary/SKILL.md`](../.agents/skills/glossary/SKILL.md) for the maintenance workflow.

## UI

Names for visible surfaces and component groupings. When a name here disagrees with a file or component name in the codebase, the glossary wins — the file is the one that should rename.

**Application Header** — Global bar across the top of the client. Client-wide navigation, notifications, and meta controls live on the left; the active server's message of the day occupies the centre; version and session controls live on the right. Implemented in `apps/frontend/src/lib/ui/AppHeader.svelte`.

**App Preferences** — User choices that the current app applies to every registered server. They include appearance, thread presentation, language, message editor, and send-key behavior. The Application Header gear opens Appearance in unified Settings for the active authenticated server. The App preferences group also contains Language and Composer. A separate App Preferences sidebar remains available when there is no authenticated server. App Preferences do not sync to another browser or device. See [FDR-002](fdr/FDR-002-replies-and-threads.md), [FDR-022](fdr/FDR-022-user-profile.md), and [FDR-032](fdr/FDR-032-message-formatting.md).

**User Preferences** — A user's choices for one server. They include time and region settings and notification behavior. The server can sync a preference, or the app can store it with a key for each server. The term identifies the user and server scope, not the storage method. The unified Settings sidebar puts these pages in the Your account group. See [FDR-012](fdr/FDR-012-notifications.md) and [FDR-022](fdr/FDR-022-user-profile.md).

**Server Configuration** — Permission-gated settings that change a server or its managed resources. Server Configuration is different from one user's User Preferences. The unified Settings sidebar puts these pages in the Server configuration group. See [FDR-020](fdr/FDR-020-server-branding-and-configuration.md) and [FDR-021](fdr/FDR-021-admin-dashboard.md).

**Server Gutter** — Narrow leftmost column listing the user's servers, with the add-server button at the bottom. Metaphor borrowed from the gutter in a text editor: a thin marginal strip. Implemented in `apps/frontend/src/lib/ServerGutter.svelte`.

**Server Sidebar** — The wider sidebar to the right of the Server Gutter. It controls the position, mobile slide, size, and current-user footer of a server pane. App Preferences uses the same shell without the server footer. The sidebar contains the server banner and room list, the unified Settings navigation, or the App Preferences navigation. Implemented in `apps/frontend/src/lib/components/ServerSidebar.svelte`.

**Room View** — The main central area showing the current room: message list plus the composer at the bottom. Not "the chat area" — *Room View* is the canonical name.

**Message Header** — Line above a non-compact message body that contains the author identity, time, and other message metadata.

**Message Meta Bar** — Compact row beneath a message showing state and secondary actions such as thread status, reactions, and pin status.

**Room Sidebar** — Right-hand pane scoped to the current room. Hosts room-specific extras such as the member list today and future surfaces like files or calls. Implemented in `apps/frontend/src/routes/chat/[serverId]/[roomId]/RoomSidebar.svelte`.

**Member List** — Room Sidebar panel that lists and searches the members of the current room.

**Profile View** — Complete public user profile shown in the Room Sidebar of a one-to-one DM. See [FDR-022](fdr/FDR-022-user-profile.md).

**Composer** — The message input at the bottom of the Room View. Includes text input, attachment picker, emoji picker, mentions autocomplete.

**Pane Header** — The top bar of a content pane (Room View, settings page, admin page, etc.). Carries the title, optional subtitle, optional back arrow, and icon-only action buttons via the `actions` snippet. Chunky labelled buttons belong in the body, not here. See [`AGENTS.md`](../AGENTS.md).

**Quick Switcher** — Cmd-K / Ctrl-K palette for jumping between rooms, DMs, servers, and admin pages. Distinct from the Server Gutter — both let you change server, but the Quick Switcher is keyboard-first and searchable. See [FDR-015](fdr/FDR-015-quick-switcher.md).

**Profile Card** — Compact user profile shown in a popover on pointer devices or a bottom sheet on touch devices. It contains public identity details and relevant actions. See [FDR-022](fdr/FDR-022-user-profile.md).

**Role Badge** — Compact label that identifies one of a user's assigned roles. A server operator selects which roles provide badges. See [FDR-001](fdr/FDR-001-roles-and-permissions.md) and [FDR-022](fdr/FDR-022-user-profile.md).

**Slideover** — A pane that slides in over existing content (e.g. settings, thread view on mobile). Distinct from a modal: dismissable by navigation, not by an explicit close.

**Hint** — Inline informational callout used in admin/settings panels to introduce or contextualise a control. Use instead of nesting an outer Panel around a self-contained matrix.

**Panel** — Bordered card used across instance-admin (`/chat/[serverId]/admin/*`) and per-server settings pages. Shared visual chrome for administrative interfaces. See [`cli/AGENTS.md`](../cli/AGENTS.md).

## Product

User-facing concepts. If a user might say the word, it goes here.

**Server** — Top-level Chatto deployment: one process, one NATS account, one membership boundary. Formerly called *Instance* in the codebase. See [ADR-029](adr/ADR-029-instance-to-server-rename.md).

**Neighbor** — Chatto server that another server advertises in its public directory. A Neighbor has a canonical origin. It is a recommendation, not a trust or reciprocal relationship. See [FDR-042](fdr/FDR-042-chatto-neighbors.md).

**Server Directory** — Client page that shows direct Neighbor recommendations from registered servers and follows bounded mutual recommendations recursively. It adds a direct server after its public profile loads, expands a remote server only after mutuality is observed, keeps registered results visible as joined, shows recommendation-source attribution in a tapestry layout, and also accepts a direct server address. It does not rank its results. See [FDR-042](fdr/FDR-042-chatto-neighbors.md).

**Client application** — Browser, desktop, mobile app, or integration that a user authorizes to access a Chatto server; its stable identity comes from CIMD or a built-in registration. A client appears in server administration after completing at least one user-approved authorization. Administrators may label it trusted or block it, but trust never replaces user consent. See [ADR-071](adr/ADR-071-cimd-identified-open-oauth-clients.md) and [FDR-023](fdr/FDR-023-authentication-and-sessions.md).

**MCP server** — Optional Chatto public HTTP integration that lets an agent host discover and call bounded user-scoped tools through the Model Context Protocol. It has no Operator API authority. See [FDR-043](fdr/FDR-043-model-context-protocol-integration.md) and [ADR-085](adr/ADR-085-agent-integration-through-mcp.md).

**MCP scope** — OAuth grant ceiling for one class of MCP tools, such as `chatto:rooms:read` or `chatto:messages:write`. It limits delegated agent authority in addition to normal Chatto permissions and resource visibility. It is not an RBAC permission.

**Bot account** — Passwordless user identity for an integration, explicitly owned by a human and marked as a bot. It receives only directly configured permissions, capped by the owner's current authority. See [FDR-038](fdr/FDR-038-bot-accounts.md).

**Bot API key** — One of a bot account's named, non-expiring bearer credentials. Chatto shows the raw key only when it creates the key and stores only its durable verifier. Each key can be revoked independently. See [FDR-038](fdr/FDR-038-bot-accounts.md).

**Bot incoming webhook** — Named HTTP credential that allows an external system to post a message as a bot. A bot can have multiple incoming webhooks. Chatto shows each action-limited URL only when it creates that webhook. A manager replaces a webhook when the manager creates a new one, moves the caller, and revokes the old one. Each webhook has independent revocation and last-use metadata. It cannot authenticate the normal API or realtime connection. See [FDR-038](fdr/FDR-038-bot-accounts.md) and [ADR-083](adr/ADR-083-action-limited-bot-incoming-webhooks.md).

**Invite Link** — Shareable, revocable link that admits one or more new accounts when a server uses invite-only account creation; it may have a use limit or expiry. See [FDR-036](fdr/FDR-036-invite-links.md).

**Space** — Legacy tier between server and room. Being consolidated into the server concept; in most deployments there is exactly one space per server (the *primary space*). See [ADR-027](adr/ADR-027-instance-space-server-consolidation.md).

**Primary Space** — Transitional config-designated "the one space that matters" within a server. Bridge construct used while Instance + Space collapse into Server. See [ADR-027](adr/ADR-027-instance-space-server-consolidation.md).

**Room** — A channel or DM. Where messages live. Identified by `(serverId, roomId)`.

**Universal room** — Channel room that behaves as joined for every server member currently eligible to join it, without writing per-user membership events. See [FDR-019](fdr/FDR-019-room-lifecycle.md).

**Room Group** — Named collection of rooms within a server, with its own per-group permission overrides. See [ADR-031](adr/ADR-031-room-group-centric-acl.md) and [FDR-017](fdr/FDR-017-room-groups-and-sidebar-layout.md).

**Sidebar Link** — Operator-managed link shown in the Server Sidebar inside a Room Group, ordered alongside rooms and stored as a durable group aggregate fact. See [FDR-017](fdr/FDR-017-room-groups-and-sidebar-layout.md).

**DM (Direct Message)** — Private conversation between users, modelled as a room with `kind: dm`. See [FDR-007](fdr/FDR-007-direct-messages.md).

**Message** — A user-posted entry in a room. Root messages live at the top level; thread replies hang off a root.

**Slow Mode** — Per-channel pacing rule that limits each non-exempt member to one new message per configured interval across roots and threads. `room.manage` and `message.manage` bypass it; edits and other message interactions do not affect its timer. See [FDR-035](fdr/FDR-035-slow-mode.md).

**Thread** — Reply chain rooted at a message. See [FDR-002](fdr/FDR-002-replies-and-threads.md).

**Threading Mode** — Per-channel policy for creating threads and placing replies: Required, Encouraged, Enabled, or Disabled. It governs new writes without hiding or rewriting historical threads. See [FDR-002](fdr/FDR-002-replies-and-threads.md).

**Echo** — Reposting a thread reply back to its parent channel so non-thread participants see it. Gated by `message.echo`. See [FDR-003](fdr/FDR-003-thread-reply-echo.md).

**Reaction** — Emoji attached to a message by a user. See [FDR-005](fdr/FDR-005-reactions.md).

**Mention** — `@handle` syntax in a message that notifies referenced users, pingable roles, or virtual room groups such as `@all` and `@here`. See [FDR-006](fdr/FDR-006-mentions.md).

**Notification** — Persistent user-scoped attention created for activity such as a DM, root room message, reply, mention, followed conversation, or reaction. Unread occurrences carry an independent Ambient or Important visual level; notifications remain visible after being read and can be deleted independently of room read state. See [FDR-012](fdr/FDR-012-notifications.md).

**Notification Group** — Client-side presentation row that combines related notification occurrences by conversation or target while retaining their exact underlying activity and jump targets. It is not a server-side resource. See [ADR-077](adr/ADR-077-persistent-notification-list.md).

**Notification Delivery Mode** — Per-cause notification preference with one of four effective values: Off, Badge, Notification, or Push notification. Badge adds only a neutral unread dot. Notification creates an in-app item and can play the configured local sound. Push notification also permits push delivery. See [FDR-012](fdr/FDR-012-notifications.md).

**Message Read Cursor** — Per-user position of the last root message read in a room. It places the New messages separator. It does not create a room dot; notification policy controls room attention separately. See [FDR-012](fdr/FDR-012-notifications.md).

**Asset** — An uploaded or generated file stored by Chatto; it may exist before or independently of a message. See [FDR-008](fdr/FDR-008-file-attachments-and-video.md).

**Attachment** — An asset attached to one message. See [FDR-008](fdr/FDR-008-file-attachments-and-video.md).

**Link Preview** — Auto-generated preview card for URLs in messages. See [FDR-009](fdr/FDR-009-link-previews.md).

**Typing Indicator** — Ephemeral "X is typing…" signal. Published as a live event, never persisted. See [FDR-010](fdr/FDR-010-typing-indicators.md).

**Presence** — A user's online/away/offline state. See [FDR-011](fdr/FDR-011-user-presence.md).

**Voice Call** — Real-time audio call attached to a room. See [FDR-016](fdr/FDR-016-voice-calls.md).

**Jump to Present** — UI affordance that returns the Room View to the latest message after scrolling back through history. See [FDR-014](fdr/FDR-014-jump-to-present.md).

**Last-Room Memory** — The system that remembers which room a user was last in per-server. See [FDR-026](fdr/FDR-026-last-room-memory.md).

## Authorization

Chatto's RBAC model. Read top-to-bottom — terms build on each other.

**RBAC (Role-Based Access Control)** — The model: roles bundle permissions, users hold roles, and direct user decisions can grant or deny exceptions. See [ADR-040](adr/ADR-040-permission-only-rbac-with-owner-override.md) and [ADR-052](adr/ADR-052-subject-specific-rbac-with-everyone-baseline.md).

**Role** — Named bundle of permissions, assignable to users. System roles are seeded; custom roles can be created. Role names share the message-mention namespace with user logins, and each role can be marked pingable to allow `@role` pings.

**Permission** — Capability gate with an opaque, stable identifier, for example `message.post` or `role.assign`. Punctuation does not define authority. The catalog in `cli/internal/core/permission.go` defines scope and explicit inclusion.

**Position** — Numeric display/order value for a role. `everyone` = 0, `moderator` = 100, `admin` = 900, `owner` = 1000. Custom roles slot in the gaps. Position is not an authorization rank.

**Effective owner** — A user with the durable `owner` role. A verified email listed in `owners.emails` causes Chatto to materialize this role. Effective owners receive every known RBAC permission virtually. DM contents remain protected by participation checks at the API boundary.

**Owner** — Top system role (position 1000). Conferred through role assignment or through verified `owners.emails` configuration.

**Admin** — System role (position 900). Broad administrative defaults, still subject to explicit RBAC decisions unless the user is also an effective owner.

**Moderator** — System role (position 100). Moderation permissions, no administrative reach.

**Everyone** — Implicit virtual role (position 0) held by every authenticated user. Its nearest decision is the scoped permission baseline. A direct-user or named-role allow overrides an `everyone` deny only at the same or a nearer scope; a named/direct deny always wins.

**Scope** — Tier at which a permission is configured: `server`, `group`, or `room`. Each direct user or named role contributes only its nearest explicit decision (room, then group, then server). Denies win across those subject decisions; an allow must be at least as specific as an `everyone` deny to override the baseline. See [`cli/AGENTS.md`](../cli/AGENTS.md).

**Request-time authorization** — Command authorization decision that becomes final after Chatto confirms that its projected RBAC, room-group, user, and other declared inputs did not change during evaluation. A later concurrent authorization change does not cancel the command; domain invariants use OCC separately. See [ADR-087](adr/ADR-087-request-time-authorization-with-aggregate-occ.md).

**Interaction relationship** — Derived account-to-thread authorization relationship created when the account authors a channel-room root or another account directly mentions it. With room membership and `message.read-interactions`, it permits the complete thread. See [FDR-039](fdr/FDR-039-message-access-and-interactions.md) and [ADR-082](adr/ADR-082-derive-thread-interactions-from-message-facts.md).

**User-level decision** — Permission grant or deny attached directly to a user, not via a role. It participates alongside named-role decisions, so a user deny blocks named-role grants while a named-role deny blocks a user grant. Used for suspensions and ad-hoc grants.

**DM Privacy Boundary** — Static set of channel-style permissions (`message.manage`, `message.echo`, `room.manage`, …) denied to non-owners inside DM rooms regardless of role grants. DM read access comes from room membership, not a separate read permission, so ownership does not grant access to other people's DM contents. See [ADR-037](adr/ADR-037-dm-access-via-membership.md).

## Backend

Infrastructure jargon. If only contributors say the word, it goes here.

**ChattoCore** — Go package (`cli/internal/core`) that owns domain models, projections, and NATS access. Low-level helpers are not public transport entry points and may assume their caller has already authorized the operation; public ConnectRPC paths should delegate to core operation models that own authorization before domain state changes. See [ADR-044](adr/ADR-044-connectrpc-service-conventions.md).

**System actor** — Synthetic actor ID used when Chatto itself, bootstrap code, or trusted operator automation performs a domain write. It is not a login-capable user account.

**Admin API** — Public ConnectRPC administrative surface in `chatto.admin.v1`. On the public web listener it uses normal user authentication and RBAC. It is separate from the local Operator API. See [FDR-028](fdr/FDR-028-operator-api-and-cli.md).

**Operator API** — Root-equivalent local ConnectRPC surface in `chatto.operator.v1`, served only on the configured Unix socket. Socket filesystem permissions are the access boundary; anyone who can connect to the socket can perform operator actions as the system actor. See [FDR-028](fdr/FDR-028-operator-api-and-cli.md).

**Operator socket** — Unix socket configured by `[operator_api].socket_path` / `CHATTO_OPERATOR_API_SOCKET_PATH`. `chatto operator ...` uses it to send root-equivalent commands to the already-running Chatto process without opening a second store writer.

**NATS** — Messaging system Chatto uses for pubsub and persistence. Runs embedded in the single binary by default.

**JetStream** — NATS's persistence layer (streams + KV buckets). Chatto's primary data store. See [ADR-001](adr/ADR-001-nats-jetstream-as-primary-data-store.md).

**Loom Architecture** — Repository-wide event-sourced architecture used by Chatto, built around one authoritative event log, disposable materializations, and durable outcomes. See [ADR-073](adr/ADR-073-define-the-loom-architecture.md).

**Stream** — JetStream append-only log. Chatto's event-sourcing stream is `EVT`, which stores durable domain facts. See [ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) and the [NATS resource inventory](architecture/nats-resources.md).

**KV (Key-Value Bucket)** — JetStream-backed key/value store. Chatto uses several current buckets, especially `RUNTIME_STATE`, `MEMORY_CACHE`, and `ENCRYPTION_KEYS`; event-sourced domain state is sourced from `EVT`. See [ADR-033](adr/ADR-033-event-sourced-state-with-projections.md).

**Subject** — NATS message topic. Current durable facts use `evt.{aggregateType}.{aggregateId}.{eventType}`; transient sync uses `live.sync.…`; committed EVT facts are internally republished on `live.evt.…`. See [`cli/AGENTS.md`](../cli/AGENTS.md) and the [subject and event inventory](architecture/subjects-and-events.md#evt-subject-patterns).

**Event** — Durable domain fact stored on `EVT` using the `evtv1.Event` wrapper. Contrast with *Live Event*.

**Projection** — Derived read model rebuilt from `EVT` and owned independently by each consuming process. Persistence is optional: a projection may cold-replay every time, use an encrypted snapshot, or checkpoint a disposable local index and EVT cutoff for tail replay. `EVT` remains the source of truth. See [ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) and [ADR-054](adr/ADR-054-optional-projection-persistence.md).

**Materialization** — Loom term for disposable state derived from the event log; Chatto projections are materializations and may live in RAM, NATS, local storage, or an external store. See [ADR-073](adr/ADR-073-define-the-loom-architecture.md).

**Outcome** — Loom term for reliable asynchronous work caused by a committed event and performed by a durable worker, such as sending an email or updating another system. See [ADR-073](adr/ADR-073-define-the-loom-architecture.md).

**Notification Occurrence** — Projected current state of one exact recipient-specific notification signal. It is Unread or Read until deletion or expiry removes it; minimal lifecycle facts prevent dismissed activity from being recreated. Identity is deterministic per recipient, source event, and signal kind. See [ADR-076](adr/ADR-076-deterministic-notification-occurrences.md).

**Notification Signal** — Immutable event-shaped notification cause whose protobuf variant owns its exact destination and cause-specific data. Signals live in the bounded `NOTIFICATIONS` event stream rather than permanent `EVT`. See [ADR-076](adr/ADR-076-deterministic-notification-occurrences.md).

**Renewable session** — One human bearer login with short-lived access tokens, a single-use rotating refresh credential, and a session window that advances automatically while the client is active. Its stable `RUNTIME_STATE` record is the revocation authority for every access generation; it is not called a token family in Chatto vocabulary. See [ADR-079](adr/ADR-079-renewable-bearer-sessions.md) and [FDR-023](fdr/FDR-023-authentication-and-sessions.md).

**Auth generation** — Per-user authentication epoch derived from durable user events. Cookie sessions, bearer tokens, and OAuth authorization codes are valid only when their stored generation matches the user's current generation. See [FDR-023](fdr/FDR-023-authentication-and-sessions.md).

**External identity** — Provider-issued account identity linked to a user, keyed by verified issuer/provider namespace plus provider subject rather than email. See [FDR-023](fdr/FDR-023-authentication-and-sessions.md).

**CIMD (Client ID Metadata Document)** — Public OAuth client metadata served at the client's URL identifier and used by Chatto to bind that client identity to exact callbacks without prior operator registration. See [ADR-071](adr/ADR-071-cimd-identified-open-oauth-clients.md).

**Live Event** — Internal `livev1.LiveEvent` signal published on `live.sync.>` for ephemeral activity and latest-value invalidation. The server may expose a genuinely transient signal such as typing or presence through `RealtimeEventEnvelope`, or use the signal to assemble an authoritative `RealtimeProjectionOperation`; the internal shape is never the public contract. Durable EVT facts reach live subscribers through `live.evt.>` after server-side projection readiness and authorization checks. See [ADR-051](adr/ADR-051-server-scoped-resumable-client-projection.md).

**Client Projection** — Authenticated, server-scoped current state delivered by realtime protocol 2. Compacted bootstrap, resumable replay, live mutation, and lazy room hydration all use the same ordered projection operations and reducer. It is a convergence feed rather than an audit log and does not replace the resource-oriented `chatto.api.v1` integrations API. See [ADR-051](adr/ADR-051-server-scoped-resumable-client-projection.md).

**Republish** — JetStream feature that mirrors accepted stream messages onto another NATS subject. Chatto uses it to expose committed EVT facts on `live.evt.>`; `myEvents` treats that as an internal feed, not a client contract. See [`cli/AGENTS.md`](../cli/AGENTS.md).

**OCC (Optimistic Concurrency Control)** — Publishing with an expected stream sequence so concurrent writers cannot commit from stale aggregate state. See [ADR-016](adr/ADR-016-occ-for-message-publishing.md) and [ADR-087](adr/ADR-087-request-time-authorization-with-aggregate-occ.md).

**Nanoid** — Short URL-safe unique ID format. All Chatto entities are prefixed (`usr_…`, `rm_…`, `srv_…`). See [ADR-022](adr/ADR-022-nanoid-with-entity-prefixes.md).

**Crypto-shredding** — Deleting a user's data by destroying the app-owned DEK refs and KMS wrapping-key refs that protect their encrypted content rather than mutating storage. See [ADR-007](adr/ADR-007-per-user-encryption-with-crypto-shredding.md).
