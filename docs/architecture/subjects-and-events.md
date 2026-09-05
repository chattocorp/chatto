# Subject and Event Inventory

Key files: [`cli/internal/evtstream/subjects.go`](../../cli/internal/evtstream/subjects.go),
[`cli/internal/evtstream/publisher.go`](../../cli/internal/evtstream/publisher.go),
[`pkg/events/encoded_event_log.go`](../../pkg/events/encoded_event_log.go),
[`pkg/events/mutation.go`](../../pkg/events/mutation.go),
[`cli/internal/search/contract.go`](../../cli/internal/search/contract.go),
[`proto/chatto/core/evt/v1/event.proto`](../../proto/chatto/core/evt/v1/event.proto),
[`proto/chatto/core/notification/v1/notification.proto`](../../proto/chatto/core/notification/v1/notification.proto),
[`proto/chatto/core/pubsub/v1/event.proto`](../../proto/chatto/core/pubsub/v1/event.proto),
[`proto/chatto/realtime/v1/events.proto`](../../proto/chatto/realtime/v1/events.proto),
and [`proto/chatto/search/v1/search.proto`](../../proto/chatto/search/v1/search.proto)

Related decisions: [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
[ADR-034](../adr/ADR-034-single-event-stream.md),
[ADR-040](../adr/ADR-040-permission-only-rbac-with-owner-override.md),
[ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md),
[ADR-053](../adr/ADR-053-versioned-nats-service-namespaces.md),
[ADR-068](../adr/ADR-068-selectable-event-mutation-consistency-boundaries.md),
[ADR-076](../adr/ADR-076-deterministic-notification-occurrences.md),
[ADR-084](../adr/ADR-084-separate-internal-protobufs-by-storage-contract.md),
[ADR-093](../adr/ADR-093-use-a-public-realtime-event-union.md), and
[ADR-094](../adr/ADR-094-separate-durable-and-pubsub-event-envelopes.md).

## Event envelopes

Chatto uses `evtv1.Event` as the wrapper for durable EVT facts. It uses
`pubsubv1.PubSubEvent` as the wrapper for NATS Core pubsub events. The EVT
storage boundary accepts only `Event`.

- **Wrapper fields**: `id`, `created_at`, `actor_id`
- **Concrete event**: `event` oneof on the selected envelope; contextual fields (`room_id`, etc.) live on the concrete payloads.

Durable variants keep their existing field numbers. `PubSubEvent` field
numbers are local to its NATS wire shape.

Existing `Event` oneof field numbers are part of the persisted JetStream wire format; do not renumber or reuse them.

**Protobuf package organization:**

| Package | Contents | Safety |
| ---- | -------- | ------ |
| `chatto.core.evt.v1` | `Event` wrapper, durable facts, and fact-owned values | Existing field numbers and structures are stored in JetStream and need storage compatibility |
| `chatto.core.notification.v1` | Bounded `NotificationEvent` wrapper and lifecycle facts | Field numbers and structures are stored in JetStream and need storage compatibility |
| `chatto.core.pubsub.v1` | Restricted `PubSubEvent` wrapper and private control payloads; client-facing variants reference public realtime payloads | Records are not stored, but changes need rolling-wire review |
| `chatto.realtime.v1` | Public event union and dedicated public payload catalogue | Contains only client-visible fields; names and compact field numbers do not expose the internal source |

The packages generate separate Go packages. `core.EventEnvelope` is the
in-process realtime delivery interface. Private implementations let it carry a
durable EVT fact, a pubsub event, or a heartbeat.

**Event Categories:**

| Category                    | Storage    | Examples                                                    | Purpose                                                        |
| --------------------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored (room) | Stream     | RoomCreated, RoomUniversalChanged, RoomSlowModeChanged, MessagePosted, MessageEdited, MessageRetracted, ReactionAdded, ReactionRemoved, UserJoinedRoom, CallStarted, CallParticipantJoined, CallParticipantLeft, CallEnded | Ordering guarantees, historical replay, projection and recoverable-effect source of truth |
| Room pubsub                 | NATS Core  | UserTyping | Ephemeral room activity where another store or projection is the source of truth |
| User pubsub | NATS Core | NotificationOccurrencesChanged, NotificationUnreadStateChanged, RoomReadStateChanged, ThreadViewerStateChanged, SessionTerminated | Private latest-value convergence and session control |
| Process-local transient | In-memory hub | PresenceChanged | Ephemeral current presence changes |

The separate `Event` and `PubSubEvent` wrapper types make the distinction
between stored and non-durable events explicit. Client-facing `PubSubEvent`
variants reuse public payload messages, while the restricted private union
prevents durable-only variants from entering the cursorless path. The
publishers enforce this boundary.
Room queries and server subscriptions are delivery contexts.

**Self-Contained Events:** Each concrete event contains all the IDs and context it needs:

- Room events contain `room_id`.
- Membership events contain relevant IDs (`room_id` for room joins/leaves).
- Self-initiated events (e.g., `PresenceChanged`) use the parent wrapper's `actor_id` instead of duplicating a `user_id` field.

**Event Publishing Strategy:**

User-facing live delivery is built from two internal NATS Core subject roots:

1. **Primary Stream** (persistent):
   - `EVT` (subjects `evt.>`) holds event-sourced domain state. Its stream-level `RePublish` config forwards every committed event once onto `live.evt.>`. This is a raw committed-event feed, not a client contract.
2. **Direct Pubsub Publish** (non-durable):
   - `pubsubv1.PubSubEvent` values publish through NATS Core to `live.sync.>`
     with no stream storage.

On the durable write path,
[`evtstream.Publisher`](../../cli/internal/evtstream/publisher.go) validates the
Chatto envelope and encodes it with `proto.Marshal`. The underlying
[`events.EncodedEventLog`](../../pkg/events/encoded_event_log.go)
treats that result as opaque bytes while applying message-ID deduplication,
OCC, and atomic-batch headers. This boundary does not change the stored
protobuf bytes, subjects, headers, or sequence semantics; previous binaries can
read new records and current binaries can replay existing `EVT` history.

Authorization-sensitive commands use stable request-time authorization. They
capture the current `evt.rbac.>`, `evt.group.>`, and `evt.user.>` input tails,
wait for the related projections, evaluate the decision, and confirm that the
tails did not change during that decision. Low-frequency role-assignment checks
also stabilize `evt.room.>` because one role can contain decisions for many
rooms. Ordinary room commands use the exact room aggregate instead.

The final input validation is the authorization decision point. A later
cross-aggregate revocation is concurrent with the command and does not cancel
it. Domain events still use OCC for their owning aggregate or invariant. A
target aggregate conflict repeats the complete command from its original
intent.

Current writers do not publish `AuthorizationFenceAdvancedEvent`. The event,
protobuf field 830, event token, and
`evt.authorization.server.fence_advanced` subject remain historical replay
contracts.

User-facing reaction add/remove uses request-time authorization and a room
aggregate boundary. Each attempt waits the relevant room, reaction, group,
RBAC, and actor projections, reruns authorization and the reaction decision,
then appends against the captured room tail. A concurrent room mutation forces
a complete retry. A cross-aggregate authorization change does not
retroactively cancel an already-authorized, conflict-free reaction attempt;
subsequent requests observe the changed authorization state.

Pinned-message add/remove validates stable request-time authorization inputs,
captures the full room aggregate tail, and reruns `room.manage` plus message
lifecycle checks before a room-OCC append. A concurrent room change forces a
complete retry. Pins reference the canonical message ID; Room Timeline removes
an active association when that message is retracted.

`MyEventsModel` sits behind the `ChattoCore.StreamMyEvents` facade. Its
process-wide `MyEventsHub` subscribes once to each of `live.sync.>` and
`live.evt.>`.

Ingress rejects non-deliverable event types from their subjects before protobuf
decoding, including private `message_body` facts. It then decodes each event and
waits once for the required local projections. RBAC facts wait for the matching
RBAC projection and rebuild each connected user's shared effective-room cache
before later events are considered. Role and permission changes can therefore
revoke implicit universal-room visibility without reconnecting.

User-facing message batches guard the room aggregate tail. Posting, editing,
attachment removal, preview removal, and pinning validate stable request-time
authorization inputs before the append. The room guard protects membership,
lifecycle, message state, Slow Mode, Threading Mode, and other room-owned
invariants. Traffic in other rooms does not cause a conflict.

Slow Mode is checked during message preflight and again inside that guarded
commit authorization. `RoomTimelineProjection` supplies the latest successful
non-echo post for the room and author in O(1), so the full-room OCC conflict
forces concurrent same-author posts on separate replicas to retry the complete
decision. Effective `room.manage` or `message.manage` bypasses the check.

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

Pubsub events remain live-only. Protocol 4 maps selected durable and pubsub
events to an authorized public event
catalogue. The delivery boundary removes caller-specific hidden room
references. Durable events can have an opaque resume cursor. Fresh or unsafe
subscriptions use an exact authorized content snapshot or the caller's
live-only fallback. Subscriber overflow closes only that session.

Process-wide ingress loss or projection-readiness failure quarantines
admission, closes every current session, flushes and drains the old
subscriptions, and opens a fresh ingress generation. No session continues or
reconnects across an unobservable gap.

The bundled web client watches server heartbeats for silent stalls. Its
in-memory server projection resumes a short socket gap or rebuilds from an
exact WebSocket snapshot; page reload starts without a cursor. Protocol 4
creates no per-connection JetStream consumer. See [ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md)
[ADR-091](../adr/ADR-091-semantic-realtime-events-with-bounded-resume.md), and
[ADR-093](../adr/ADR-093-use-a-public-realtime-event-union.md).

## Durable and pubsub subject patterns

| Stream                       | Wrapper          | Scope      | Description                                      |
| ---------------------------- | ---------------- | ---------- | ------------------------------------------------ |
| `EVT`                        | `evtv1.Event`   | Server     | Event-sourcing log ([ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md) / [ADR-034](../adr/ADR-034-single-event-stream.md)). Subjects `evt.{aggregateType}.{aggregateId}.{eventType}`; republishes onto `live.evt.>` as the raw committed-event feed. Stores room membership/metadata, groups/layout, server config, users, messages/threads, reactions, assets, RBAC, OAuth client authorization/policy, and auth workflow audit facts. Notification materialization derives exact-sequence output directly from existing source/lifecycle facts; it adds no notification-only EVT facts or prepared-work records. |
| `NOTIFICATIONS`              | `notificationv1.NotificationEvent` | User occurrence | Bounded 90-day notification lifecycle log on four fixed subjects. A 24-hour broker cleanup grace follows the application expiry. Its projector owns the current list; the push worker consumes signalled facts directly. |
| Live Sync                    | `pubsubv1.PubSubEvent` | Non-durable | Direct NATS Core pubsub on `live.sync.>` for ephemeral activity and latest-value invalidations. `StreamMyEvents` authorizes them. Selected values become public realtime events. Internal controls can become protocol frames. |

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
| `notifications.signalled`                       | Rich immutable per-recipient notification signal and initial delivery state     |
| `notifications.read`                            | Idempotent transition of one occurrence to Read                                 |
| `notifications.removed`                         | Minimal anti-recreation tombstone for one removed occurrence                    |
| `notifications.alert_resolved`                  | Single terminal outcome for push delivery                                        |
| `evt.group.{groupId}.{eventType}`                | Room group metadata and group-owned sidebar item ordering/membership facts      |
| `evt.layout.default.{eventType}`                 | Singleton sidebar group ordering facts                                          |
| `evt.user.{userId}.{eventType}`                  | User/account/profile/auth lookup facts and user-scoped auth audit facts         |
| `evt.user.*.{eventType}`                         | One user event type across all users                                            |
| `evt.rbac.{server\|dm\|scopeId}.{eventType}`      | Server, global DM, room-group, or room RBAC facts                                |
| `evt.authorization.server.fence_advanced`        | Historical authorization-fence records; current writers do not publish them     |
| `evt.auth.server.{eventType}`                    | Server-wide auth audit facts before a user aggregate exists                     |
| `evt.invitation.{invitationId}.{eventType}`      | Invitation creation, redemption, and revocation facts                           |
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
| `evt.room.{roomId}.room_slow_mode_changed`                   | `RoomSlowModeChangedEvent`                          |
| `evt.room.{roomId}.room_threading_mode_changed`              | `RoomThreadingModeChangedEvent`                     |
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
| `evt.room.{roomId}.message_pinned`                           | `MessagePinnedEvent`                                |
| `evt.room.{roomId}.message_unpinned`                         | `MessageUnpinnedEvent`                              |
| `evt.room.{roomId}.thread_created`                           | `ThreadCreatedEvent`                                |
| `evt.room.{roomId}.thread_followed`                          | `ThreadFollowedEvent`                               |
| `evt.room.{roomId}.thread_unfollowed`                        | `ThreadUnfollowedEvent`                             |
| `evt.room.{roomId}.reaction_added`                           | `ReactionAddedEvent`                                |
| `evt.room.{roomId}.reaction_removed`                         | `ReactionRemovedEvent`                              |
| `evt.asset.{assetId}.asset_created`                          | `AssetCreatedEvent`                                 |
| `evt.asset.{assetId}.asset_attached`                         | `AssetAttachedEvent`; uploader-bound exclusive attachment to one room/message committed atomically with the message |
| `evt.asset.{assetId}.asset_processing_started`               | `AssetProcessingStartedEvent`; PENDING fact and durable asset-processing queue item |
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
| `evt.config.server.server_neighbor_created`                  | `ServerNeighborCreatedEvent`; creates one advertised Neighbor; the legacy testimonial field is ignored |
| `evt.config.server.server_neighbor_origin_changed`           | `ServerNeighborOriginChangedEvent`; changes one advertised origin |
| `evt.config.server.server_neighbor_testimonial_changed`      | `ServerNeighborTestimonialChangedEvent`; legacy replay contract only; the text is ignored, but the event advances the Neighbor revision |
| `evt.config.server.server_neighbor_deleted`                  | `ServerNeighborDeletedEvent`; removes one advertised Neighbor |
| `evt.config.{subject}.user_timezone_changed`                 | `UserTimezoneChangedEvent`                          |
| `evt.config.{subject}.user_timezone_cleared`                 | `UserTimezoneClearedEvent`                          |
| `evt.config.{subject}.user_timezone_sharing_changed`         | `UserTimezoneSharingChangedEvent`                   |
| `evt.config.{subject}.user_time_format_changed`              | `UserTimeFormatChangedEvent`                        |
| `evt.config.{subject}.user_time_format_cleared`              | `UserTimeFormatClearedEvent`                        |
| `evt.config.{subject}.user_server_notification_level_set`    | `UserServerNotificationLevelSetEvent` (historical decode only; ignored by current projections) |
| `evt.config.{subject}.user_server_notification_level_cleared` | `UserServerNotificationLevelClearedEvent` (historical decode only; ignored by current projections) |
| `evt.config.{subject}.user_room_notification_level_set`      | `UserRoomNotificationLevelSetEvent` (historical decode only; ignored by current projections) |
| `evt.config.{subject}.user_room_notification_level_cleared`  | `UserRoomNotificationLevelClearedEvent` (historical decode only; ignored by current projections) |
| `evt.config.{subject}.user_notification_policy_changed`      | `UserNotificationPolicyChangedEvent` (complete overrides for one user/scope) |
| `evt.config.{subject}.user_room_group_notification_policy_changed` | `UserRoomGroupNotificationPolicyChangedEvent` (complete overrides for one user and room group; unknown older binaries ignore this distinct variant) |
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
| `evt.user.{userId}.bot_api_key_created`                    | `BotApiKeyCreatedEvent`; initial stable key ID, manager-defined name, HMAC verifier, and issue timestamp, never the raw key. Historical events without ID or name project as the `legacy` default key |
| `evt.user.{userId}.bot_api_key_added`                      | `BotApiKeyAddedEvent`; stable key ID, manager-defined name, HMAC verifier, and issue timestamp for one additional key |
| `evt.user.{userId}.bot_api_key_revoked`                    | `BotApiKeyRevokedEvent`; key ID that invalidates only the selected verifier |
| `evt.user.{userId}.bot_api_key_rotated`                    | Compatibility `BotApiKeyRotatedEvent`; historical replace-all verifier, or a targeted revocation fence with a revoked-key ID and an unissued verifier so an old binary cannot restore the revoked raw key. Current commands do not write replace-all rotations |
| `evt.user.{userId}.bot_owner_reassigned`                   | `BotOwnerReassignedEvent`; previous and new human owner IDs, with no credential change |
| `evt.user.{userId}.bot_incoming_webhook_enabled`           | `BotIncomingWebhookCreatedEvent`; stable webhook ID, manager-defined name, HMAC verifier, and creation timestamp, never the raw credential. The legacy `enabled` subject token remains stable |
| `evt.user.{userId}.bot_incoming_webhook_rotated`           | Compatibility-only `BotIncomingWebhookRotatedEvent`; replacement HMAC verifier from the unreleased implementation. Current servers read but do not write this event |
| `evt.user.{userId}.bot_incoming_webhook_disabled`          | `BotIncomingWebhookRevokedEvent`; webhook ID that invalidates the selected verifier. The legacy `disabled` subject token remains stable |
| `evt.user.{userId}.login_changed`                           | `UserLoginChangedEvent`                             |
| `evt.user.{userId}.display_name_changed`                    | `UserDisplayNameChangedEvent`                       |
| `evt.user.{userId}.bio_changed`                             | `UserBioChangedEvent`; encrypted PII bio text (empty payload clears the bio) |
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
| `evt.user.{userId}.user_key_shredding_requested`            | `UserKeyShreddingRequestedEvent`; logical privacy boundary and durable worker request |
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
| `evt.user.{userId}.privileged_mode_activated`               | `PrivilegedModeActivatedEvent`; audit-only transition with the fixed deadline |
| `evt.user.{userId}.privileged_mode_deactivated`             | `PrivilegedModeDeactivatedEvent`; audit-only explicit transition before expiry |
| `evt.user.{userId}.oauth_consent_granted`                   | `OAuthConsentGrantedEvent`                          |
| `evt.user.{userId}.oauth_consent_denied`                    | `OAuthConsentDeniedEvent`                           |
| `evt.user.{userId}.oauth_scoped_consent_granted`            | `OAuthScopedConsentGrantedEvent`; exact resource and scope grant that older projectors ignore |
| `evt.user.{userId}.oauth_scoped_consent_denied`             | `OAuthScopedConsentDeniedEvent`                     |
| `evt.rbac.{server\|scopeId}.role_created`                   | `RbacRoleCreatedEvent`                             |
| `evt.rbac.{server\|scopeId}.role_display_name_changed`      | `RbacRoleDisplayNameChangedEvent`                  |
| `evt.rbac.{server\|scopeId}.role_description_changed`       | `RbacRoleDescriptionChangedEvent`                  |
| `evt.rbac.{server\|scopeId}.role_pingable_changed`          | `RbacRolePingableChangedEvent`                     |
| `evt.rbac.{server\|scopeId}.role_deleted`                   | `RbacRoleDeletedEvent`                             |
| `evt.rbac.{server\|scopeId}.roles_reordered`                | `RbacRolesReorderedEvent`                          |
| `evt.rbac.{server\|scopeId}.role_assigned`                  | `RbacRoleAssignedEvent`                            |
| `evt.rbac.{server\|scopeId}.role_revoked`                   | `RbacRoleRevokedEvent`                             |
| `evt.rbac.{server\|dm\|scopeId}.permission_granted`          | `RbacPermissionGrantedEvent`                       |
| `evt.rbac.{server\|dm\|scopeId}.permission_denied`           | `RbacPermissionDeniedEvent`                        |
| `evt.rbac.{server\|dm\|scopeId}.permission_cleared`          | `RbacPermissionClearedEvent`                       |
| `evt.authorization.server.fence_advanced`                    | `AuthorizationFenceAdvancedEvent` (historical replay only) |
| `evt.auth.server.registration_verification_code_issued`    | `RegistrationVerificationCodeIssuedEvent`           |
| `evt.auth.server.login_failed`                             | `LoginFailedEvent`                                  |
| `evt.invitation.{invitationId}.created`                    | `InvitationCreatedEvent`                            |
| `evt.invitation.{invitationId}.redeemed`                   | `InvitationRedeemedEvent`                           |
| `evt.invitation.{invitationId}.revoked`                    | `InvitationRevokedEvent`                            |

Notes: Subject suffixes are stable NATS event tokens defined in [`cli/internal/evtstream/subjects.go`](../../cli/internal/evtstream/subjects.go). Protobuf message types are the concrete `evtv1.Event` oneof payloads defined in [`proto/chatto/core/evt/v1/event.proto`](../../proto/chatto/core/evt/v1/event.proto) and sibling `*_events.proto` files. The current asset write path uses `evt.asset.{assetId}.*`; `AssetProjection` also consumes beta-era `evt.room.{roomId}.asset_*` histories for replay compatibility.

Room-layout structural commands use atomic EVT batches. Channel-room creation
commits `RoomCreatedEvent` with `RoomAddedToGroupEvent`. Channel-room deletion
commits `RoomDeletedEvent` with `RoomRemovedFromGroupEvent`. Room-group creation
and deletion commit the group lifecycle fact with the resulting
`RoomGroupsReorderedEvent`. Each batch guards every room, group, layout, and
authorization boundary that its decision uses. Room moves also guard the
room-deletion subject, so a concurrent delete cannot leave a stale group
membership. See ADR-086.

For every attachment message, its `AssetAttachedEvent` is committed in the same
atomic OCC batch as the owning message body and posted fact. Video messages add
the Started fact to that batch. The batch guards the room and authorization
boundaries and the complete aggregate of every attached asset, so concurrent
attachments, pending expiry, and deletion cannot commit conflicting transitions.

Failed or losing processing attempts perform bounded prompt cleanup by
appending ordinary derivative `AssetDeletedEvent` facts. If cleanup is
interrupted before a tombstone is appended, the unused derivative is not
durably discoverable. An ambiguous success append is checked by exact event ID;
if that confirmation also fails, the processor retains the output rather than
risk deleting assets referenced by a committed manifest.

## Pubsub subjects

Pubsub activity uses `pubsubv1.PubSubEvent` values and is published directly
on NATS Core. It is not persisted. Genuinely ephemeral activity
can be a public cursorless event. Latest-value invalidations are inputs to live
projection assembly but are not replay facts.

Patterns: `live.sync.>` for `PubSubEvent` values and `live.evt.>`
for raw EVT committed facts. `myEvents` consumes both roots server-side:

- Direct NATS Core publishes: `PubSubEvent` messages on `live.sync.>` with
  no stream storage.
- `EVT` RePublish (`evt.>` → `live.evt.>`): every committed event-sourced fact is re-emitted once by JetStream. Chatto replicas must wait for local projection readiness and authorize before exposing deliverable room or asset events to clients.

`SERVER_EVENTS` no longer has a `RePublish` live path and runtime code no longer writes legacy `server.>` mirrors. Historical `SERVER_EVENTS` streams may still appear in old backups, but current boot and live-delivery paths do not read or import them.

**Pubsub events** (`live.sync.{user,config,room}.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.sync.user.{userId}.notification_v2`                | Notification occurrence created, triaged, removed, or delivery eligibility changed; requests a current occurrence/count read and includes the notification ID on creation, independent of sound policy |
| `live.sync.user.{userId}.notification_unread`            | Badge attention changed; triggers authoritative room viewer-state replacement. A thread marker contributes to its parent room state |
| `live.sync.user.{userId}.thread_viewer_state`            | Current thread follow or read state changed outside the durable follow command |
| `live.sync.user.{userId}.room_read`                      | Room marked as read          |
| `live.sync.user.{userId}.session_terminated`             | Active session revoked (logout-other-devices, account deletion) |
| `live.sync.room.{kind}.{roomId}.user_typing`             | User typing in a room        |

User profile, private preference, public server profile, and durable thread
follow changes use their EVT facts on `live.evt.>`. The realtime mapper groups
related facts into `UserProfileChangedEvent`, `ViewerPreferencesChangedEvent`,
`ServerProfileChangedEvent`, or `ThreadViewerStateChangedEvent`. Presence comes
from the process-local `PresenceHub`; it does not use a `live.sync.>` subject.

Room-group and sidebar-layout changes use durable group or layout facts only.
The command path waits until the local `ServerContentView` applies the final
fact. JetStream republishes each fact on `live.evt.>` for realtime delivery.

Voice call lifecycle and participant transitions are durable room EVT facts:
`evt.room.{roomId}.call_started`, `evt.room.{roomId}.call_joined`,
`evt.room.{roomId}.call_left`, and `evt.room.{roomId}.call_ended`. JetStream
republishes them to `live.evt.>` for realtime delivery. They drive active-call
state and indicators. Call-started and call-ended facts are also visible room
timeline entries; participant join/leave facts remain hidden from room history.

LiveKit room names include the active Chatto call ID suffix. Participant and
room-finished observations therefore apply only to the matching call session.
Join-token participant metadata carries the login, avatar URL, and account kind
needed to preserve canonical user identity, including bot markers, while the
client is rendering directly from LiveKit state.
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
flows. The bundled frontend receives complete authorized active-call state
with semantic call events and infers one-shot join, leave, and end presentation
effects by comparing current state. Room membership remains the authorization
boundary for live delivery.

The `/api/realtime` WebSocket is backed by the single core stream `StreamMyEvents`, which combines:

- One process-wide `ChanSubscribe("live.sync.>")` for `PubSubEvent`
  messages and one `ChanSubscribe("live.evt.>")` for raw committed EVT facts.
  Subject classification and decoding happen once. Authorization then applies
  per connected user using shared room visibility, asset room membership,
  user/config/member subject gates, and projection readiness.
- Live delivery plus protocol-4 bounded replay of authorized public durable
  events. The WebSocket subscribes to the hub before it captures its EVT
  cutoff, replays through that cutoff, and then drops buffered duplicates
  before it continues live. Fresh and unsafe subscriptions use an exact
  authorized content snapshot or the requested live-only fallback. Cursorless
  pubsub activity remains live-only.
- The PresenceHub (single per-process KV watcher on `presence.>` fanning out per-user status changes to all subscribers).
- An in-process heartbeat ticker (synthetic `Heartbeat` event every 15s for client-side liveness detection).

## Outbound bot webhooks

Configuration uses `evt.user.{botId}.bot_outbound_webhook_configured` with
user-aggregate OCC and encrypted credentials. Queue jobs use
`jobs.bot_webhook.deliver` on `JOBS`, encoded as
[`BotWebhookDeliveryJob`](../../proto/chatto/core/jobs/v1/bot_webhook.proto).
The delivery ID hashes the bot, configuration, and source event IDs.

Only terminal failures append
`evt.bot_webhook_delivery.{deliveryId}.bot_webhook_delivery_completed`.
The status is `failed`; OCC permits one such fact per delivery. No request,
success, or skip fact enters EVT. Failure payloads are defined in
[`bot_webhook_events.proto`](../../proto/chatto/core/evt/v1/bot_webhook_events.proto).
These facts are internal and do not enter the public realtime catalogue.
