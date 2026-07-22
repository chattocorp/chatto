# Subject and Event Inventory

Key files: [`cli/internal/events/subjects.go`](../../cli/internal/events/subjects.go),
[`cli/internal/search/contract.go`](../../cli/internal/search/contract.go),
[`proto/chatto/core/v1/event.proto`](../../proto/chatto/core/v1/event.proto),
[`proto/chatto/core/v1/live_events.proto`](../../proto/chatto/core/v1/live_events.proto), and
[`proto/chatto/search/v1/search.proto`](../../proto/chatto/search/v1/search.proto)

Related decisions: [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
[ADR-034](../adr/ADR-034-single-event-stream.md), and
[ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md), and
[ADR-053](../adr/ADR-053-versioned-nats-service-namespaces.md).

## Event envelopes

Chatto uses `corev1.Event` as the durable EVT wrapper and `corev1.LiveEvent` as the transient NATS Core wrapper. The realtime API maps both through public protobuf frames, while the protobuf wire envelopes stay separate so live-only sync signals cannot leak into the durable audit/event log shape.

- **Wrapper fields**: `id`, `created_at`, `actor_id`
- **Concrete event**: `event` oneof on the relevant wire envelope; contextual fields (`roomId`, etc.) live on the concrete payloads.

The active `Event.event` oneof variants are all durable EVT payloads, regardless of numeric tag. Transient-only pubsub signals belong in `corev1.LiveEvent`, not `corev1.Event`.

Existing `Event` oneof field numbers are part of the persisted JetStream wire format; do not renumber or reuse them.

**Proto File Organization:**

| File | Contents | Safety |
| ---- | -------- | ------ |
| `event.proto` | Durable `Event` wrapper + persisted event message definitions | Changing field numbers/structure affects JetStream-stored data — requires careful migration |
| `live_events.proto` | Transient `LiveEvent` wrapper + live-only event message definitions | Safe to change freely — these are never persisted |

Both files share `package chatto.core.v1` and generate into the same Go package. `core.EventEnvelope` is the in-process realtime delivery interface that can carry durable EVT, transient LiveEvent, or a heartbeat through private concrete implementations.

**Event Categories:**

| Category                    | Storage    | Examples                                                    | Purpose                                                        |
| --------------------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored (room) | Stream     | RoomCreated, RoomUniversalChanged, MessagePosted, MessageEdited, MessageRetracted, ReactionAdded, ReactionRemoved, UserJoinedRoom, CallStarted, CallParticipantJoined, CallParticipantLeft, CallEnded | Ordering guarantees, historical replay, projection source of truth |
| Room live-only              | NATS Core  | UserTyping | Ephemeral room notifications where another store/projection is source of truth |
| Deployment live (user/config) | NATS Core  | UserCreated, ServerUpdated, MentionNotification, NotificationCreated, PresenceChanged | Cross-tab sync, notifications, server lifecycle |

The distinction between stored and live-only events is explicit in the wire envelope: durable facts use `corev1.Event`, transient signals use `corev1.LiveEvent`. Room queries and server subscriptions are delivery contexts, not separate wrapper types.

**Self-Contained Events:** Each concrete event contains all the IDs and context it needs:

- Room events contain `room_id`.
- Membership events contain relevant IDs (`room_id` for room joins/leaves).
- Self-initiated events (e.g., `PresenceChanged`) use the parent wrapper's `actor_id` instead of duplicating a `user_id` field.

**Event Publishing Strategy:**

User-facing live delivery is built from two internal NATS Core subject roots:

1. **Primary Stream** (persistent):
   - `EVT` (subjects `evt.>`) holds event-sourced domain state. Its stream-level `RePublish` config forwards every committed event once onto `live.evt.>`. This is a raw committed-event feed, not a client contract.
2. **Direct Live Publish** (transient):
   - Transient UI sync signals publish as `corev1.LiveEvent` via NATS Core to `live.sync.>` — no stream storage.

`MyEventsModel` sits behind the `ChattoCore.StreamMyEvents` facade. Its
process-wide `MyEventsHub` subscribes once to each of `live.sync.>` and
`live.evt.>`.

Ingress rejects non-deliverable event types from their subjects before protobuf
decoding, including private `message_body` facts. It then decodes each event and
waits once for the required local projections. RBAC facts wait for the matching
RBAC projection and rebuild each connected user's shared effective-room cache
before later events are considered. Role and permission changes can therefore
revoke implicit universal-room visibility without reconnecting.

Authorization-sensitive mutations use the singleton
`evt.authorization.server.fence_advanced` OCC lane. Every RBAC change,
room-group/layout change, and user lifecycle change that can alter effective
authority advances this lane atomically with its domain facts. Before evaluating
bounded scoped authority, writers wait the relevant RBAC, room directory,
room-group layout, and user projections through the captured EVT boundary. A
concurrent authorization change then conflicts and retries the complete
authorization decision, while unrelated messages and reactions do not contend.
The fence event carries no policy state; the owning domain projections remain
authoritative.

Deliverable events are authorized per user and fanned as shared immutable
pointers to independent session queues. Asset lifecycle events resolve room
authorization through `AssetProjection`, using the scope on `AssetCreatedEvent`
and inherited parent scope for derivatives.

New sessions hydrate visibility outside the dispatcher lock against stable
authoritative room-visibility and RBAC EVT tails. They register through a
dispatcher-owned channel after the process drains ingress already received.
Ordinary room chatter does not participate in the stable-tail check.

Visibility changes processed during hydration force a retry. Late
cross-publisher facts already covered by the snapshot are suppressed by EVT
stream sequence; admission does not assume global NATS publisher ordering.

Transient `LiveEvent` messages are adapted at this boundary into public
protobuf `/api/realtime` frames and remain live-only. Protocol v2 maps durable
facts to current public projection operations; fresh or unsafe resumes begin
with a compacted server projection. Subscriber overflow closes only that
session.

Process-wide ingress loss or projection-readiness failure quarantines
admission, closes every current session, flushes and drains the old
subscriptions, and opens a fresh ingress generation. No session continues or
reconnects across an unobservable gap.

The bundled web client watches server heartbeats for silent stalls. Its
in-memory server projection resumes a short socket gap or accepts a compacted
reset; page reload deliberately starts without a cursor. Protocol v2 creates no
long-lived per-connection JetStream consumer. See [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md)
and [ADR-051](../adr/ADR-051-server-scoped-resumable-client-projection.md).

## EVT subject patterns

| Stream                       | Wrapper          | Scope      | Description                                      |
| ---------------------------- | ---------------- | ---------- | ------------------------------------------------ |
| `EVT`                        | `corev1.Event`   | Server     | Event-sourcing log ([ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md) / [ADR-034](../adr/ADR-034-single-event-stream.md)). Subjects `evt.{aggregateType}.{aggregateId}.{eventType}`; republishes onto `live.evt.>` as the raw committed-event feed. Stores room membership/metadata, groups/layout, server config, users, messages/threads, reactions, assets, RBAC, and auth workflow audit facts. |
| Live Sync                    | `corev1.LiveEvent` | Transient  | Direct NATS Core pubsub on `live.sync.>` for ephemeral activity and latest-value invalidation signals. `StreamMyEvents` authorizes them; genuinely transient activity becomes public realtime events, while invalidations trigger authoritative projection operations. |

The republished `live.evt.{aggregateType}.{aggregateId}.{eventType}` subject is an internal server-side feed; `StreamMyEvents` waits for projections and authorization before delivering anything to clients.

| Pattern                                          | Description                                                                     |
| ------------------------------------------------ | ------------------------------------------------------------------------------- |
| `evt.>`                                          | All durable event-sourced facts                                                 |
| `evt.room.>`                                     | All room aggregate facts                                                        |
| `evt.room.{roomId}.{eventType}`                  | One room aggregate fact                                                         |
| `evt.room.*.{eventType}`                         | One room event type across all rooms                                            |
| `evt.asset.>`                                    | All asset aggregate facts                                                       |
| `evt.asset.{assetId}.{eventType}`                | One asset aggregate fact                                                        |
| `evt.asset.*.{eventType}`                        | One asset event type across all assets                                          |
| `evt.config.>`                                   | Dynamic server/user configuration and preferences                               |
| `evt.config.{subject}.{eventType}`               | Config fact for `server`, a user ID, or another configurable subject            |
| `evt.group.{groupId}.{eventType}`                | Room group metadata and group-owned sidebar item ordering/membership facts      |
| `evt.layout.default.{eventType}`                 | Singleton sidebar group ordering facts                                          |
| `evt.user.{userId}.{eventType}`                  | User/account/profile/auth lookup facts and user-scoped auth audit facts         |
| `evt.user.*.{eventType}`                         | One user event type across all users                                            |
| `evt.rbac.{server\|scopeId}.{eventType}`         | Server-level RBAC or scoped RBAC decision facts for a room/group ID             |
| `evt.authorization.server.fence_advanced`        | Singleton OCC fence for changes that can alter mutation authority               |
| `evt.auth.server.{eventType}`                    | Server-wide auth audit facts before a user aggregate exists                     |
| `live.evt.>`                                     | JetStream republish of committed `EVT` facts                                    |

The aggregate ID is intentionally part of the subject; actor/user and detailed context stay in the protobuf payload. Asset subjects are keyed by asset ID, while room scope lives in `AssetCreatedEvent` and is resolved by `AssetProjection`. Cross-event-type invariants use wildcard OCC filters such as `evt.room.>`, `evt.asset.>`, or `evt.rbac.>`.

## NATS service subjects

Trusted request/reply services use
`svc.{servingAuthority}.{service}.{majorVersion}.{endpoint}`. Chatto Core owns
`svc.chatto.>`, while replaceable providers, including bundled
implementations, own `svc.chatto_ext.>`. Payloads are protobuf, and standard
NATS micro error headers carry transport-level failures.

| Subject | Protobuf request / response | Queue group | Owner |
| ------- | --------------------------- | ----------- | ----- |
| `svc.chatto_ext.search.v1.query` | `chatto.search.v1.QueryRequest` / `QueryResponse` | `svc.chatto_ext.search.v1` | Any compatible message-search provider replica |
| `svc.chatto_ext.search.v1.status` | `chatto.search.v1.GetStatusRequest` / `GetStatusResponse` | `svc.chatto_ext.search.v1` | Queryable message-search provider replicas |
| `svc.chatto_ext.search.v1.status.startup` | `chatto.search.v1.GetStatusRequest` / `GetStatusResponse` | `svc.chatto_ext.search.v1` | Provider replicas still starting or indexing; queried only when no ready status responder exists |

The Search contract returns ordered message and room IDs. It does not grant
room visibility or make indexed content authoritative; Chatto Core rehydrates
and authorizes current message state before any public response. Provider
cursors are trusted integration coordinates and are not public API cursors.

## Durable EVT event inventory

| Subject pattern                                              | Protobuf event message                              |
| ------------------------------------------------------------ | --------------------------------------------------- |
| `evt.room.{roomId}.room_created`                             | `RoomCreatedEvent`                                  |
| `evt.room.{roomId}.room_updated`                             | `RoomUpdatedEvent`                                  |
| `evt.room.{roomId}.room_archived`                            | `RoomArchivedEvent`                                 |
| `evt.room.{roomId}.room_unarchived`                          | `RoomUnarchivedEvent`                               |
| `evt.room.{roomId}.room_universal_changed`                   | `RoomUniversalChangedEvent`                         |
| `evt.room.{roomId}.room_deleted`                             | `RoomDeletedEvent`                                  |
| `evt.room.{roomId}.user_joined`                              | `UserJoinedRoomEvent`                               |
| `evt.room.{roomId}.user_left`                                | `UserLeftRoomEvent`                                 |
| `evt.room.{roomId}.call_started`                             | `CallStartedEvent`                                  |
| `evt.room.{roomId}.call_joined`                              | `CallParticipantJoinedEvent`                        |
| `evt.room.{roomId}.call_left`                                | `CallParticipantLeftEvent`                          |
| `evt.room.{roomId}.call_ended`                               | `CallEndedEvent`                                    |
| `evt.room.{roomId}.room_member_banned`                       | `RoomMemberBannedEvent`                             |
| `evt.room.{roomId}.room_member_unbanned`                     | `RoomMemberUnbannedEvent`                           |
| `evt.room.{roomId}.room_member_added`                        | `RoomMemberAddedEvent`                              |
| `evt.room.{roomId}.room_member_removed`                      | `RoomMemberRemovedEvent`                            |
| `evt.room.{roomId}.message_body`                             | `MessageBodyEvent`                                  |
| `evt.room.{roomId}.message_posted`                           | `MessagePostedEvent`                                |
| `evt.room.{roomId}.message_edited`                           | `MessageEditedEvent`                                |
| `evt.room.{roomId}.message_retracted`                        | `MessageRetractedEvent`                             |
| `evt.room.{roomId}.thread_created`                           | `ThreadCreatedEvent`                                |
| `evt.room.{roomId}.thread_followed`                          | `ThreadFollowedEvent`                               |
| `evt.room.{roomId}.thread_unfollowed`                        | `ThreadUnfollowedEvent`                             |
| `evt.room.{roomId}.reaction_added`                           | `ReactionAddedEvent`                                |
| `evt.room.{roomId}.reaction_removed`                         | `ReactionRemovedEvent`                              |
| `evt.asset.{assetId}.asset_created`                          | `AssetCreatedEvent`                                 |
| `evt.asset.{assetId}.asset_processing_started`               | `AssetProcessingStartedEvent`                       |
| `evt.asset.{assetId}.asset_processing_succeeded`             | `AssetProcessingSucceededEvent`                     |
| `evt.asset.{assetId}.asset_processing_failed`                | `AssetProcessingFailedEvent`                        |
| `evt.asset.{assetId}.asset_deleted`                          | `AssetDeletedEvent`                                 |
| `evt.config.{subject}.server_name_changed`                   | `ServerNameChangedEvent`                            |
| `evt.config.{subject}.server_description_changed`            | `ServerDescriptionChangedEvent`                     |
| `evt.config.{subject}.server_welcome_message_changed`        | `ServerWelcomeMessageChangedEvent`                  |
| `evt.config.{subject}.server_motd_changed`                   | `ServerMotdChangedEvent`                            |
| `evt.config.{subject}.server_blocked_usernames_changed`      | `ServerBlockedUsernamesChangedEvent`                |
| `evt.config.{subject}.server_logo_set`                       | `ServerLogoSetEvent`                                |
| `evt.config.{subject}.server_logo_cleared`                   | `ServerLogoClearedEvent`                            |
| `evt.config.{subject}.server_banner_set`                     | `ServerBannerSetEvent`                              |
| `evt.config.{subject}.server_banner_cleared`                 | `ServerBannerClearedEvent`                          |
| `evt.config.{subject}.user_timezone_changed`                 | `UserTimezoneChangedEvent`                          |
| `evt.config.{subject}.user_timezone_cleared`                 | `UserTimezoneClearedEvent`                          |
| `evt.config.{subject}.user_time_format_changed`              | `UserTimeFormatChangedEvent`                        |
| `evt.config.{subject}.user_time_format_cleared`              | `UserTimeFormatClearedEvent`                        |

| `evt.config.{subject}.user_server_notification_level_set`    | `UserServerNotificationLevelSetEvent`               |
| `evt.config.{subject}.user_server_notification_level_cleared` | `UserServerNotificationLevelClearedEvent`          |
| `evt.config.{subject}.user_room_notification_level_set`      | `UserRoomNotificationLevelSetEvent`                 |
| `evt.config.{subject}.user_room_notification_level_cleared`  | `UserRoomNotificationLevelClearedEvent`             |
| `evt.group.{groupId}.group_created`                         | `RoomGroupCreatedEvent`                             |
| `evt.group.{groupId}.group_updated`                         | `RoomGroupUpdatedEvent`                             |
| `evt.group.{groupId}.group_deleted`                         | `RoomGroupDeletedEvent`                             |
| `evt.group.{groupId}.room_added`                            | `RoomAddedToGroupEvent`                             |
| `evt.group.{groupId}.room_removed`                          | `RoomRemovedFromGroupEvent`                         |
| `evt.group.{groupId}.rooms_reordered`                       | `RoomsInGroupReorderedEvent`                        |
| `evt.group.{groupId}.sidebar_link_added`                    | `SidebarLinkAddedToGroupEvent`                      |
| `evt.group.{groupId}.sidebar_link_updated`                  | `SidebarLinkUpdatedEvent`                           |
| `evt.group.{groupId}.sidebar_link_removed`                  | `SidebarLinkRemovedFromGroupEvent`                  |
| `evt.group.{groupId}.sidebar_entries_reordered`             | `SidebarGroupEntriesReorderedEvent`                 |
| `evt.layout.default.groups_reordered`                        | `RoomGroupsReorderedEvent`                          |
| `evt.user.{userId}.account_created`                         | `UserAccountCreatedEvent`                           |
| `evt.user.{userId}.login_changed`                           | `UserLoginChangedEvent`                             |
| `evt.user.{userId}.display_name_changed`                    | `UserDisplayNameChangedEvent`                       |
| `evt.user.{userId}.avatar_set`                              | `UserAvatarSetEvent`                                |
| `evt.user.{userId}.avatar_cleared`                          | `UserAvatarClearedEvent`                            |
| `evt.user.{userId}.custom_status_set`                       | `UserCustomStatusSetEvent`                          |
| `evt.user.{userId}.custom_status_cleared`                   | `UserCustomStatusClearedEvent`                      |
| `evt.user.{userId}.verified_email_added`                    | `UserVerifiedEmailAddedEvent`                       |
| `evt.user.{userId}.password_hash_changed`                   | `UserPasswordHashChangedEvent`                      |
| `evt.user.{userId}.oidc_subject_linked`                     | `UserOIDCSubjectLinkedEvent` (legacy replay)        |
| `evt.user.{userId}.external_identity_linked`                | `UserExternalIdentityLinkedEvent`                   |
| `evt.user.{userId}.external_identity_unlinked`              | `UserExternalIdentityUnlinkedEvent`                 |
| `evt.user.{userId}.server_preferences_changed`              | `UserServerPreferencesChangedEvent`                 |
| `evt.user.{userId}.login_cooldown_started`                  | `UserLoginCooldownStartedEvent`                     |
| `evt.user.{userId}.login_cooldown_cleared`                  | `UserLoginCooldownClearedEvent`                     |
| `evt.user.{userId}.account_deleted`                         | `UserAccountDeletedEvent`                           |
| `evt.user.{userId}.user_key_shredded`                       | `UserKeyShreddedEvent`                              |
| `evt.user.{userId}.dek_generated`                           | `UserDEKGeneratedEvent`                             |
| `evt.user.{userId}.email_verification_code_issued`          | `EmailVerificationCodeIssuedEvent`                  |
| `evt.user.{userId}.password_reset_link_issued`              | `PasswordResetLinkIssuedEvent`                      |
| `evt.user.{userId}.account_deletion_confirmation_issued`    | `AccountDeletionConfirmationIssuedEvent`            |
| `evt.user.{userId}.password_reset_completed`                | `PasswordResetCompletedEvent`                       |
| `evt.user.{userId}.login_succeeded`                         | `LoginSucceededEvent`                               |
| `evt.user.{userId}.logout_succeeded`                        | `LogoutSucceededEvent`                              |
| `evt.user.{userId}.auth_code_issued`                        | `AuthCodeIssuedEvent`                               |
| `evt.user.{userId}.auth_code_exchange_succeeded`            | `AuthCodeExchangeSucceededEvent`                    |
| `evt.user.{userId}.auth_code_exchange_failed`               | `AuthCodeExchangeFailedEvent`                       |
| `evt.user.{userId}.bearer_token_issued`                     | `BearerTokenIssuedEvent`                            |
| `evt.user.{userId}.bearer_token_revoked`                    | `BearerTokenRevokedEvent`                           |
| `evt.user.{userId}.oauth_consent_granted`                   | `OAuthConsentGrantedEvent`                          |
| `evt.user.{userId}.oauth_consent_denied`                    | `OAuthConsentDeniedEvent`                           |
| `evt.rbac.{server\|scopeId}.role_created`                   | `RbacRoleCreatedEvent`                             |
| `evt.rbac.{server\|scopeId}.role_display_name_changed`      | `RbacRoleDisplayNameChangedEvent`                  |
| `evt.rbac.{server\|scopeId}.role_description_changed`       | `RbacRoleDescriptionChangedEvent`                  |
| `evt.rbac.{server\|scopeId}.role_pingable_changed`          | `RbacRolePingableChangedEvent`                     |
| `evt.rbac.{server\|scopeId}.role_deleted`                   | `RbacRoleDeletedEvent`                             |
| `evt.rbac.{server\|scopeId}.roles_reordered`                | `RbacRolesReorderedEvent`                          |
| `evt.rbac.{server\|scopeId}.role_assigned`                  | `RbacRoleAssignedEvent`                            |
| `evt.rbac.{server\|scopeId}.role_revoked`                   | `RbacRoleRevokedEvent`                             |
| `evt.rbac.{server\|scopeId}.permission_granted`             | `RbacPermissionGrantedEvent`                       |
| `evt.rbac.{server\|scopeId}.permission_denied`              | `RbacPermissionDeniedEvent`                        |
| `evt.rbac.{server\|scopeId}.permission_cleared`             | `RbacPermissionClearedEvent`                       |
| `evt.authorization.server.fence_advanced`                    | `AuthorizationFenceAdvancedEvent`                  |
| `evt.auth.server.registration_verification_code_issued`    | `RegistrationVerificationCodeIssuedEvent`           |
| `evt.auth.server.login_failed`                             | `LoginFailedEvent`                                  |

Notes: Subject suffixes are stable NATS event tokens defined in [`cli/internal/events/subjects.go`](../../cli/internal/events/subjects.go). Protobuf message types are the concrete `corev1.Event` oneof payloads defined in [`proto/chatto/core/v1/event.proto`](../../proto/chatto/core/v1/event.proto) and sibling `*_events.proto` files. The current asset write path uses `evt.asset.{assetId}.*`; `AssetProjection` also consumes beta-era `evt.room.{roomId}.asset_*` histories for replay compatibility.

Failed or losing processing attempts perform bounded prompt cleanup by
appending ordinary derivative `AssetDeletedEvent` facts. If cleanup is
interrupted before a tombstone is appended, the unused derivative is not
durably discoverable. An ambiguous success append is checked by exact event ID;
if that confirmation also fails, the processor retains the output rather than
risk deleting assets referenced by a committed manifest.

## Transient live subjects

Transient sync signals use `corev1.LiveEvent` and are published directly on NATS Core. They are not persisted. Genuinely ephemeral activity may be mapped to a public transient event; latest-value invalidations are inputs to live projection assembly but are not replay facts themselves.

Patterns: `live.sync.>` for transient `LiveEvent` pubsub and `live.evt.>` for raw EVT committed facts. `myEvents` consumes both roots server-side:

- Direct NATS Core publishes (`publishLiveEvent()`): transient `corev1.LiveEvent` messages on `live.sync.>` with no stream storage.
- `EVT` RePublish (`evt.>` → `live.evt.>`): every committed event-sourced fact is re-emitted once by JetStream. Chatto replicas must wait for local projection readiness and authorize before exposing deliverable room or asset events to clients.

`SERVER_EVENTS` no longer has a `RePublish` live path and runtime code no longer writes legacy `server.>` mirrors. Historical `SERVER_EVENTS` streams may still appear in old backups, but current boot and live-delivery paths do not read or import them.

**Transient live sync events** (`live.sync.{user,config,room}.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.sync.user.{userId}.created`                        | User registration completed  |
| `live.sync.user.{userId}.profile_updated`                | User profile changed (broadcast for login/display/avatar updates; custom status set/clear is delivered from `live.evt.>`) |
| `live.sync.config.server_updated`                        | Public server profile/config changed (name/MOTD/welcome/logo/banner/description) |
| `live.sync.config.room_groups_updated`                   | Admin reordered the room sidebar / room-group layout |
| `live.sync.user.{userId}.mentioned`                      | User was @mentioned (legacy attention signal; suppressed during DND) |
| `live.sync.user.{userId}.dm_message`                     | New DM message received (legacy attention signal; suppressed during DND) |
| `live.sync.user.{userId}.notification_created`           | New notification created; may be marked silent for DND alert suppression |
| `live.sync.user.{userId}.notification_dismissed`         | Notification dismissed       |
| `live.sync.user.{userId}.notification_level_changed`     | Viewer's server/room notification level changed |
| `live.sync.user.{userId}.thread_follow_changed`          | Viewer's thread follow/unfollow toggled |
| `live.sync.user.{userId}.settings_updated`               | User preferences changed     |
| `live.sync.user.{userId}.room_read`                      | Room marked as read          |
| `live.sync.user.{userId}.session_terminated`             | Active session revoked (logout-other-devices, account deletion) |
| `live.sync.member.deleted`                                | Server-level membership invalidation after account deletion |
| `live.sync.room.{kind}.{roomId}.user_typing`             | User typing in a room        |

Voice call lifecycle and participant transitions are durable room EVT facts:
`evt.room.{roomId}.call_started`, `evt.room.{roomId}.call_joined`,
`evt.room.{roomId}.call_left`, and `evt.room.{roomId}.call_ended`. JetStream
republishes them to `live.evt.>` for realtime delivery. They drive active-call
state and indicators but remain hidden from normal room history timelines.

LiveKit room names include the active Chatto call ID suffix. Participant and
room-finished observations therefore apply only to the matching call session.
Only the replica holding `lease.livekit_reconciler` in `MEMORY_CACHE` runs the
periodic reconciliation loop.

Reconciliation appends `RECONCILIATION` facts for participant mismatches in the
matching call session. It disconnects participants from rooms that no longer
match a projected active call. At startup, it also replays durable `call_ended`
facts to retry any per-call E2EE key shredding that did not complete after the
original commit.

Missing or observed-empty LiveKit rooms end projected calls immediately after
a successful listing. A per-room `not_found` while listing participants is
treated as that room being gone or empty, so other rooms can still reconcile.

Listing failures increment the shared
`livekit.reconciliation.list_failures` key and retry on the normal ticker. They
end projected calls only after three consecutive failed elected reconciliation
cycles. A successful elected pass deletes the counter.

`VoiceCallService.GetActiveCall`, `BatchGetActiveCalls`, `GetCallToken`, and
`ListCallParticipants` expose the active call ID to integrations and command
flows. The bundled frontend receives complete authorized active-call state in
`active_calls_replace` projection operations and infers one-shot join/leave/end
presentation effects by comparing replacements. Room membership remains the
authorization boundary for live delivery.

The `/api/realtime` WebSocket is backed by the single core stream `StreamMyEvents`, which combines:

- One process-wide `ChanSubscribe("live.sync.>")` for transient `LiveEvent`
  messages and one `ChanSubscribe("live.evt.>")` for raw committed EVT facts.
  Subject classification and decoding happen once. Authorization then applies
  per connected user using shared room visibility, asset room membership,
  user/config/member subject gates, and projection readiness.
- Live delivery plus protocol-v2 bounded replay of durable facts as current public projection operations. The WebSocket subscribes to the hub before capturing its EVT cutoff, replays through that cutoff, then drops buffered duplicates before continuing live. Fresh and unsafe resumes receive a compacted server projection through the same operation stream; transient sync and presence signals remain live-only.
- The PresenceHub (single per-process KV watcher on `presence.>` fanning out per-user status changes to all subscribers).
- An in-process heartbeat ticker (synthetic `Heartbeat` event every 15s for client-side liveness detection).
