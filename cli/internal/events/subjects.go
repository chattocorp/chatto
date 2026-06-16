package events

import (
	"strings"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Subject roots for the event log. The durable stream stores events on
// SubjectRoot; the stream's RePublish config forwards them onto
// LiveSubjectRoot for live delivery.
//
// We don't use "server.evt." here because the legacy SERVER_EVENTS stream
// claims "server.>", which NATS treats as overlapping. Keeping the new
// roots on a separate top-level token sidesteps that without refactoring
// the legacy stream's subject filters.
const (
	SubjectRoot     = "evt."
	LiveSubjectRoot = "live.evt."
)

// Aggregate type segments. Stable identifiers; once written, never renamed.
const (
	AggregateRoom   = "room"
	AggregateConfig = "config"
	AggregateGroup  = "group"
	AggregateLayout = "layout"
	AggregateUser   = "user"
	AggregateAsset  = "asset"
	AggregateRBAC   = "rbac"
	AggregateAuth   = "auth"
)

// ConfigSingletonID is the sentinel aggregate ID for server-wide config
// (ADR-034 singleton convention: server-scoped aggregates use a stable
// sentinel rather than introducing a different subject shape).
const ConfigSingletonID = "server"

// LayoutSingletonID is the sentinel aggregate ID for the singleton
// sidebar layout. Same convention as ConfigSingletonID.
const LayoutSingletonID = "default"

// RBACServerID is the aggregate ID for server-scoped RBAC state: role catalog,
// role order, assignments, and server-scoped permission decisions. Room and
// group scoped decisions use their room/group ID directly as the aggregate ID.
const RBACServerID = "server"

// AuthServerID is the singleton aggregate ID for anonymous/server-wide auth
// audit facts, such as registration code issuance before a user exists.
const AuthServerID = "server"

// Event-type tokens. NATS-idiomatic snake_case; the trailing segment of
// every event subject. Stable identifiers — once written, never renamed.
//
// The canonical mapping lives in EventTypeOf below; constants here are
// just symbolic names for the same strings so call sites don't repeat
// string literals.
const (
	// Room aggregate
	EventRoomCreated        = "room_created"
	EventRoomUpdated        = "room_updated"
	EventRoomArchived       = "room_archived"
	EventRoomUnarchived     = "room_unarchived"
	EventRoomDeleted        = "room_deleted"
	EventUserJoinedRoom     = "user_joined"
	EventUserLeftRoom       = "user_left"
	EventRoomMemberBanned   = "room_member_banned"
	EventRoomMemberUnbanned = "room_member_unbanned"

	// Messages (also under the room aggregate — every message event for
	// a room lives under evt.room.{R}.message_*, so a subscriber on
	// evt.room.{R}.> still receives the complete per-room timeline).
	// See issue #597. The "edited" and "retracted" tokens match the
	// MessageEditedEvent / MessageRetractedEvent proto names; if those
	// proto names are renamed in a future cleanup, subject tokens can
	// stay as-is (subjects are stable once written).
	EventMessagePosted            = "message_posted"
	EventMessageEdited            = "message_edited"
	EventMessageRetracted         = "message_retracted"
	EventMessageBody              = "message_body"
	EventThreadCreated            = "thread_created"
	EventAssetCreated             = "asset_created"
	EventAssetProcessingStarted   = "asset_processing_started"
	EventAssetProcessingSucceeded = "asset_processing_succeeded"
	EventAssetProcessingFailed    = "asset_processing_failed"
	EventAssetDeleted             = "asset_deleted"

	// Reactions (also under the room aggregate). Reaction state is
	// derived from these durable events by the reaction projection.
	EventReactionAdded   = "reaction_added"
	EventReactionRemoved = "reaction_removed"

	// Voice call participant state (also under the room aggregate).
	EventCallStarted           = "call_started"
	EventCallParticipantJoined = "call_joined"
	EventCallParticipantLeft   = "call_left"
	EventCallEnded             = "call_ended"

	// Group aggregate
	EventRoomGroupCreated        = "group_created"
	EventRoomGroupUpdated        = "group_updated"
	EventRoomGroupDeleted        = "group_deleted"
	EventRoomAddedToGroup        = "room_added"
	EventRoomRemovedFromGroup    = "room_removed"
	EventRoomsInGroupReordered   = "rooms_reordered"
	EventSidebarLinkAdded        = "sidebar_link_added"
	EventSidebarLinkUpdated      = "sidebar_link_updated"
	EventSidebarLinkRemoved      = "sidebar_link_removed"
	EventSidebarEntriesReordered = "sidebar_entries_reordered"

	// Layout aggregate (singleton)
	EventRoomGroupsReordered = "groups_reordered"

	// Config aggregate (singleton)
	EventServerNameChanged                  = "server_name_changed"
	EventServerDescriptionChanged           = "server_description_changed"
	EventServerWelcomeMessageChanged        = "server_welcome_message_changed"
	EventServerMotdChanged                  = "server_motd_changed"
	EventServerBlockedUsernamesChanged      = "server_blocked_usernames_changed"
	EventServerLogoSet                      = "server_logo_set"
	EventServerLogoCleared                  = "server_logo_cleared"
	EventServerBannerSet                    = "server_banner_set"
	EventServerBannerCleared                = "server_banner_cleared"
	EventUserTimezoneChanged                = "user_timezone_changed"
	EventUserTimezoneCleared                = "user_timezone_cleared"
	EventUserTimeFormatChanged              = "user_time_format_changed"
	EventUserTimeFormatCleared              = "user_time_format_cleared"
	EventUserServerNotificationLevelSet     = "user_server_notification_level_set"
	EventUserServerNotificationLevelCleared = "user_server_notification_level_cleared"
	EventUserRoomNotificationLevelSet       = "user_room_notification_level_set"
	EventUserRoomNotificationLevelCleared   = "user_room_notification_level_cleared"

	// User aggregate
	EventUserAccountCreated           = "account_created"
	EventUserLoginChanged             = "login_changed"
	EventUserDisplayNameChanged       = "display_name_changed"
	EventUserAvatarSet                = "avatar_set"
	EventUserAvatarCleared            = "avatar_cleared"
	EventUserVerifiedEmailAdded       = "verified_email_added"
	EventUserPasswordHashChanged      = "password_hash_changed"
	EventUserOIDCSubjectLinked        = "oidc_subject_linked"
	EventUserExternalIdentityLinked   = "external_identity_linked"
	EventUserServerPreferencesChanged = "server_preferences_changed"
	EventUserLoginCooldownStarted     = "login_cooldown_started"
	EventUserLoginCooldownCleared     = "login_cooldown_cleared"
	EventUserAccountDeleted           = "account_deleted"
	EventUserKeyShredded              = "user_key_shredded"
	EventUserDEKGenerated             = "dek_generated"

	// RBAC aggregate
	EventRBACRoleCreated            = "role_created"
	EventRBACRoleDisplayNameChanged = "role_display_name_changed"
	EventRBACRoleDescriptionChanged = "role_description_changed"
	EventRBACRolePingableChanged    = "role_pingable_changed"
	EventRBACRoleDeleted            = "role_deleted"
	EventRBACRolesReordered         = "roles_reordered"
	EventRBACRoleAssigned           = "role_assigned"
	EventRBACRoleRevoked            = "role_revoked"
	EventRBACPermissionGranted      = "permission_granted"
	EventRBACPermissionDenied       = "permission_denied"
	EventRBACPermissionCleared      = "permission_cleared"

	// Auth/security audit
	EventRegistrationVerificationCodeIssued = "registration_verification_code_issued"
	EventEmailVerificationCodeIssued        = "email_verification_code_issued"
	EventPasswordResetLinkIssued            = "password_reset_link_issued"
	EventAccountDeletionConfirmationIssued  = "account_deletion_confirmation_issued"
	EventPasswordResetCompleted             = "password_reset_completed"
	EventLoginSucceeded                     = "login_succeeded"
	EventLoginFailed                        = "login_failed"
	EventLogoutSucceeded                    = "logout_succeeded"
	EventAuthCodeIssued                     = "auth_code_issued"
	EventAuthCodeExchangeSucceeded          = "auth_code_exchange_succeeded"
	EventAuthCodeExchangeFailed             = "auth_code_exchange_failed"
	EventBearerTokenIssued                  = "bearer_token_issued"
	EventBearerTokenRevoked                 = "bearer_token_revoked"
	EventOAuthConsentGranted                = "oauth_consent_granted"
	EventOAuthConsentDenied                 = "oauth_consent_denied"
)

// EventTypeOf returns the canonical NATS subject token for an event's
// oneof variant. Returns "" if the event is nil or its oneof is unset.
//
// This is the single source of truth: the protobuf oneof drives the
// subject token, so the subject can't disagree with the payload by
// convention. New event types add a case here and nothing else changes.
func EventTypeOf(e *corev1.Event) string {
	if e == nil {
		return ""
	}
	switch e.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return EventRoomCreated
	case *corev1.Event_RoomUpdated:
		return EventRoomUpdated
	case *corev1.Event_RoomArchived:
		return EventRoomArchived
	case *corev1.Event_RoomUnarchived:
		return EventRoomUnarchived
	case *corev1.Event_RoomDeleted:
		return EventRoomDeleted
	case *corev1.Event_UserJoinedRoom:
		return EventUserJoinedRoom
	case *corev1.Event_UserLeftRoom:
		return EventUserLeftRoom
	case *corev1.Event_RoomMemberBanned:
		return EventRoomMemberBanned
	case *corev1.Event_RoomMemberUnbanned:
		return EventRoomMemberUnbanned
	case *corev1.Event_VoiceCallParticipantJoined:
		return EventCallParticipantJoined
	case *corev1.Event_VoiceCallParticipantLeft:
		return EventCallParticipantLeft
	case *corev1.Event_VoiceCallStarted:
		return EventCallStarted
	case *corev1.Event_VoiceCallEnded:
		return EventCallEnded

	case *corev1.Event_MessagePosted:
		return EventMessagePosted
	case *corev1.Event_MessageEdited:
		return EventMessageEdited
	case *corev1.Event_MessageRetracted:
		return EventMessageRetracted
	case *corev1.Event_MessageBody:
		return EventMessageBody
	case *corev1.Event_ThreadCreated:
		return EventThreadCreated
	case *corev1.Event_AssetCreated:
		return EventAssetCreated
	case *corev1.Event_AssetProcessingStarted:
		return EventAssetProcessingStarted
	case *corev1.Event_AssetProcessingSucceeded:
		return EventAssetProcessingSucceeded
	case *corev1.Event_AssetProcessingFailed:
		return EventAssetProcessingFailed
	case *corev1.Event_AssetDeleted:
		return EventAssetDeleted

	case *corev1.Event_ReactionAdded:
		return EventReactionAdded
	case *corev1.Event_ReactionRemoved:
		return EventReactionRemoved

	case *corev1.Event_RoomGroupCreated:
		return EventRoomGroupCreated
	case *corev1.Event_RoomGroupUpdated:
		return EventRoomGroupUpdated
	case *corev1.Event_RoomGroupDeleted:
		return EventRoomGroupDeleted
	case *corev1.Event_RoomAddedToGroup:
		return EventRoomAddedToGroup
	case *corev1.Event_RoomRemovedFromGroup:
		return EventRoomRemovedFromGroup
	case *corev1.Event_RoomsInGroupReordered:
		return EventRoomsInGroupReordered
	case *corev1.Event_SidebarLinkAddedToGroup:
		return EventSidebarLinkAdded
	case *corev1.Event_SidebarLinkUpdated:
		return EventSidebarLinkUpdated
	case *corev1.Event_SidebarLinkRemovedFromGroup:
		return EventSidebarLinkRemoved
	case *corev1.Event_SidebarGroupEntriesReordered:
		return EventSidebarEntriesReordered

	case *corev1.Event_RoomGroupsReordered:
		return EventRoomGroupsReordered

	case *corev1.Event_ServerNameChanged:
		return EventServerNameChanged
	case *corev1.Event_ServerDescriptionChanged:
		return EventServerDescriptionChanged
	case *corev1.Event_ServerWelcomeMessageChanged:
		return EventServerWelcomeMessageChanged
	case *corev1.Event_ServerMotdChanged:
		return EventServerMotdChanged
	case *corev1.Event_ServerBlockedUsernamesChanged:
		return EventServerBlockedUsernamesChanged
	case *corev1.Event_ServerLogoSet:
		return EventServerLogoSet
	case *corev1.Event_ServerLogoCleared:
		return EventServerLogoCleared
	case *corev1.Event_ServerBannerSet:
		return EventServerBannerSet
	case *corev1.Event_ServerBannerCleared:
		return EventServerBannerCleared
	case *corev1.Event_UserTimezoneChanged:
		return EventUserTimezoneChanged
	case *corev1.Event_UserTimezoneCleared:
		return EventUserTimezoneCleared
	case *corev1.Event_UserTimeFormatChanged:
		return EventUserTimeFormatChanged
	case *corev1.Event_UserTimeFormatCleared:
		return EventUserTimeFormatCleared
	case *corev1.Event_UserServerNotificationLevelSet:
		return EventUserServerNotificationLevelSet
	case *corev1.Event_UserServerNotificationLevelCleared:
		return EventUserServerNotificationLevelCleared
	case *corev1.Event_UserRoomNotificationLevelSet:
		return EventUserRoomNotificationLevelSet
	case *corev1.Event_UserRoomNotificationLevelCleared:
		return EventUserRoomNotificationLevelCleared

	case *corev1.Event_UserAccountCreated:
		return EventUserAccountCreated
	case *corev1.Event_UserLoginChanged:
		return EventUserLoginChanged
	case *corev1.Event_UserDisplayNameChanged:
		return EventUserDisplayNameChanged
	case *corev1.Event_UserAvatarSet:
		return EventUserAvatarSet
	case *corev1.Event_UserAvatarCleared:
		return EventUserAvatarCleared
	case *corev1.Event_UserVerifiedEmailAdded:
		return EventUserVerifiedEmailAdded
	case *corev1.Event_UserPasswordHashChanged:
		return EventUserPasswordHashChanged
	case *corev1.Event_UserOidcSubjectLinked:
		return EventUserOIDCSubjectLinked
	case *corev1.Event_UserExternalIdentityLinked:
		return EventUserExternalIdentityLinked
	case *corev1.Event_UserServerPreferencesChanged:
		return EventUserServerPreferencesChanged
	case *corev1.Event_UserLoginCooldownStarted:
		return EventUserLoginCooldownStarted
	case *corev1.Event_UserLoginCooldownCleared:
		return EventUserLoginCooldownCleared
	case *corev1.Event_UserAccountDeleted:
		return EventUserAccountDeleted
	case *corev1.Event_UserKeyShredded:
		return EventUserKeyShredded
	case *corev1.Event_UserDekGenerated:
		return EventUserDEKGenerated

	case *corev1.Event_RbacRoleCreated:
		return EventRBACRoleCreated
	case *corev1.Event_RbacRoleDisplayNameChanged:
		return EventRBACRoleDisplayNameChanged
	case *corev1.Event_RbacRoleDescriptionChanged:
		return EventRBACRoleDescriptionChanged
	case *corev1.Event_RbacRolePingableChanged:
		return EventRBACRolePingableChanged
	case *corev1.Event_RbacRoleDeleted:
		return EventRBACRoleDeleted
	case *corev1.Event_RbacRolesReordered:
		return EventRBACRolesReordered
	case *corev1.Event_RbacRoleAssigned:
		return EventRBACRoleAssigned
	case *corev1.Event_RbacRoleRevoked:
		return EventRBACRoleRevoked
	case *corev1.Event_RbacPermissionGranted:
		return EventRBACPermissionGranted
	case *corev1.Event_RbacPermissionDenied:
		return EventRBACPermissionDenied
	case *corev1.Event_RbacPermissionCleared:
		return EventRBACPermissionCleared

	case *corev1.Event_RegistrationVerificationCodeIssued:
		return EventRegistrationVerificationCodeIssued
	case *corev1.Event_EmailVerificationCodeIssued:
		return EventEmailVerificationCodeIssued
	case *corev1.Event_PasswordResetLinkIssued:
		return EventPasswordResetLinkIssued
	case *corev1.Event_AccountDeletionConfirmationIssued:
		return EventAccountDeletionConfirmationIssued
	case *corev1.Event_PasswordResetCompleted:
		return EventPasswordResetCompleted
	case *corev1.Event_LoginSucceeded:
		return EventLoginSucceeded
	case *corev1.Event_LoginFailed:
		return EventLoginFailed
	case *corev1.Event_LogoutSucceeded:
		return EventLogoutSucceeded
	case *corev1.Event_AuthCodeIssued:
		return EventAuthCodeIssued
	case *corev1.Event_AuthCodeExchangeSucceeded:
		return EventAuthCodeExchangeSucceeded
	case *corev1.Event_AuthCodeExchangeFailed:
		return EventAuthCodeExchangeFailed
	case *corev1.Event_BearerTokenIssued:
		return EventBearerTokenIssued
	case *corev1.Event_BearerTokenRevoked:
		return EventBearerTokenRevoked
	case *corev1.Event_OauthConsentGranted:
		return EventOAuthConsentGranted
	case *corev1.Event_OauthConsentDenied:
		return EventOAuthConsentDenied
	}
	return ""
}

// Aggregate identifies one event-sourced aggregate by type and ID. Every
// event for the aggregate lives under the prefix Subject("") returns;
// per-event subjects add an event-type trailing segment.
//
// Per-subject OCC against `Nats-Expected-Last-Subject-Sequence` operates
// at the (aggregate, event-type) granularity. Cross-event-type invariants
// use wildcard OCC against AllEventsFilter() via the
// `Nats-Expected-Last-Subject-Sequence-Subject` header (see
// Publisher.AppendAtFilter).
type Aggregate struct {
	Type string
	ID   string
}

// Subject returns the per-(aggregate, event-type) subject.
// Pattern: evt.{aggType}.{aggID}.{eventType}.
func (a Aggregate) Subject(eventType string) string {
	return SubjectRoot + a.Type + "." + a.ID + "." + eventType
}

// SubjectFor is like Subject but derives the event-type token from the
// event payload's oneof variant. Convenient when the caller already has
// the event built — pairs naturally with publisher helpers that take an
// Aggregate + Event rather than a raw subject.
func (a Aggregate) SubjectFor(e *corev1.Event) string {
	return a.Subject(EventTypeOf(e))
}

// AllEventsFilter returns the wildcard filter matching every event for
// THIS aggregate instance, across every event type. Used as the filter
// token for cross-event-type wildcard OCC (the "did anything else land
// on this aggregate?" guard) and as a wait-target for "any event on
// this aggregate."
// Pattern: evt.{aggType}.{aggID}.>
func (a Aggregate) AllEventsFilter() string {
	return SubjectRoot + a.Type + "." + a.ID + ".>"
}

// RoomAggregate is the typed constructor for a room-aggregate handle.
// All room lifecycle events (joins, leaves, deletes, renames, future
// additions) publish under RoomAggregate(roomID).
func RoomAggregate(roomID string) Aggregate {
	return Aggregate{Type: AggregateRoom, ID: roomID}
}

// GroupAggregate is the typed constructor for a room-group aggregate
// handle. All group lifecycle events and group room-membership events
// publish under GroupAggregate(groupID).
func GroupAggregate(groupID string) Aggregate {
	return Aggregate{Type: AggregateGroup, ID: groupID}
}

// LayoutAggregate is the typed constructor for the singleton sidebar
// layout aggregate. Owns inter-group ordering for the sidebar; the
// room-group set itself is owned by the group aggregate.
func LayoutAggregate() Aggregate {
	return Aggregate{Type: AggregateLayout, ID: LayoutSingletonID}
}

// ConfigAggregate is the typed constructor for the singleton server-
// config aggregate.
func ConfigAggregate() Aggregate {
	return Aggregate{Type: AggregateConfig, ID: ConfigSingletonID}
}

// ConfigSubjectAggregate is the typed constructor for dynamic config on any
// configurable subject. Subjects are canonical IDs/sentinels such as `server`,
// `U...`, or `R...`; callers should not add redundant type prefixes.
func ConfigSubjectAggregate(subject string) Aggregate {
	return Aggregate{Type: AggregateConfig, ID: subject}
}

// UserAggregate is the typed constructor for a user aggregate. It owns
// identity/profile state, verified-email indexes, password auth state,
// external identity links, server preferences, and account deletion.
func UserAggregate(userID string) Aggregate {
	return Aggregate{Type: AggregateUser, ID: userID}
}

// AssetAggregate is the typed constructor for an asset aggregate. It owns
// binary lifecycle and processing facts; room visibility is carried by the
// asset payload/projections, not by the subject namespace.
func AssetAggregate(assetID string) Aggregate {
	return Aggregate{Type: AggregateAsset, ID: assetID}
}

// RBACAggregate is the typed constructor for server-level RBAC events:
// role definitions/order and server-scoped permission decisions.
func RBACAggregate() Aggregate {
	return RBACServerAggregate()
}

// RBACServerAggregate is the typed constructor for server-level RBAC events.
func RBACServerAggregate() Aggregate {
	return Aggregate{Type: AggregateRBAC, ID: RBACServerID}
}

// RBACScopedAggregate is the typed constructor for scoped RBAC decisions. The
// scope ID is a Chatto entity ID, whose prefix already encodes whether it is a
// room (R...) or group (G...).
func RBACScopedAggregate(scopeID string) Aggregate {
	if scopeID == "" {
		return RBACServerAggregate()
	}
	return Aggregate{Type: AggregateRBAC, ID: scopeID}
}

// AuthAggregate is the typed constructor for server-wide auth audit facts.
func AuthAggregate() Aggregate {
	return Aggregate{Type: AggregateAuth, ID: AuthServerID}
}

// EventSubjectFilter returns the wildcard filter matching every event in the
// EVT stream. Use sparingly: most invariants should OCC against a narrower
// aggregate namespace, but cross-aggregate invariants may need the stream-wide
// boundary.
// Pattern: evt.>
func EventSubjectFilter() string { return SubjectRoot + ">" }

// RoomSubjectFilter returns the wildcard filter matching every event of
// every room aggregate, across all event types.
// Pattern: evt.room.>
func RoomSubjectFilter() string { return SubjectRoot + AggregateRoom + ".>" }

// GroupSubjectFilter returns the wildcard filter matching every event of
// every room-group aggregate.
// Pattern: evt.group.>
func GroupSubjectFilter() string { return SubjectRoot + AggregateGroup + ".>" }

// LayoutSubjectFilter returns the wildcard filter matching every event
// of the singleton layout aggregate.
// Pattern: evt.layout.>
func LayoutSubjectFilter() string { return SubjectRoot + AggregateLayout + ".>" }

// ConfigSubjectFilter returns the wildcard filter matching every event
// of the singleton config aggregate.
// Pattern: evt.config.>
func ConfigSubjectFilter() string { return SubjectRoot + AggregateConfig + ".>" }

// UserSubjectFilter returns the wildcard filter matching every user
// aggregate event.
// Pattern: evt.user.>
func UserSubjectFilter() string { return SubjectRoot + AggregateUser + ".>" }

// AssetSubjectFilter returns the wildcard filter matching every asset
// aggregate event.
// Pattern: evt.asset.>
func AssetSubjectFilter() string { return SubjectRoot + AggregateAsset + ".>" }

// RBACSubjectFilter returns the wildcard filter matching every RBAC aggregate
// event.
// Pattern: evt.rbac.>
func RBACSubjectFilter() string { return SubjectRoot + AggregateRBAC + ".>" }

// AuthSubjectFilter returns the wildcard filter matching server-wide auth
// audit facts.
// Pattern: evt.auth.>
func AuthSubjectFilter() string { return SubjectRoot + AggregateAuth + ".>" }

// AggregateEventTypeFilter returns a cross-aggregate, event-type-narrow
// filter — every event of the given type across every aggregate instance.
// Used by projections that only care about a subset of event types and don't
// want to receive the full evt.{aggregate}.> firehose.
// Pattern: evt.{aggregateType}.*.{eventType}
func AggregateEventTypeFilter(aggregateType, eventType string) string {
	return SubjectRoot + aggregateType + ".*." + eventType
}

// RoomEventTypeFilter returns a cross-aggregate, event-type-narrow
// filter for room aggregates.
// Pattern: evt.room.*.{eventType}
func RoomEventTypeFilter(eventType string) string {
	return AggregateEventTypeFilter(AggregateRoom, eventType)
}

// GroupEventTypeFilter is the group analogue of RoomEventTypeFilter.
// Pattern: evt.group.*.{eventType}
func GroupEventTypeFilter(eventType string) string {
	return AggregateEventTypeFilter(AggregateGroup, eventType)
}

// ConfigEventTypeFilter is the config analogue of RoomEventTypeFilter.
// Pattern: evt.config.*.{eventType}
func ConfigEventTypeFilter(eventType string) string {
	return AggregateEventTypeFilter(AggregateConfig, eventType)
}

// UserEventTypeFilter is the user analogue of RoomEventTypeFilter.
// Pattern: evt.user.*.{eventType}
func UserEventTypeFilter(eventType string) string {
	return AggregateEventTypeFilter(AggregateUser, eventType)
}

// AssetEventTypeFilter is the asset analogue of RoomEventTypeFilter.
// Pattern: evt.asset.*.{eventType}
func AssetEventTypeFilter(eventType string) string {
	return AggregateEventTypeFilter(AggregateAsset, eventType)
}

// RBACEventTypeFilter is the RBAC analogue of RoomEventTypeFilter.
// Pattern: evt.rbac.*.{eventType}
func RBACEventTypeFilter(eventType string) string {
	return AggregateEventTypeFilter(AggregateRBAC, eventType)
}

// ParseRoomSubject extracts the roomID from a room-aggregate event
// subject. Accepts both the durable form (evt.room.{R}.{type}) and the
// republished live form (live.evt.room.{R}.{type}). Returns ok=false if
// the subject doesn't match either shape.
func ParseRoomSubject(subject string) (roomID string, ok bool) {
	return parseAggregateSubject(subject, AggregateRoom)
}

// ParseGroupSubject extracts the groupID from a group-aggregate event
// subject. Accepts durable and republished live forms.
func ParseGroupSubject(subject string) (groupID string, ok bool) {
	return parseAggregateSubject(subject, AggregateGroup)
}

// ParseUserSubject extracts the userID from a user-aggregate event
// subject. Accepts durable and republished live forms.
func ParseUserSubject(subject string) (userID string, ok bool) {
	return parseAggregateSubject(subject, AggregateUser)
}

// ParseAssetSubject extracts the assetID from an asset-aggregate event
// subject. Accepts durable and republished live forms.
func ParseAssetSubject(subject string) (assetID string, ok bool) {
	return parseAggregateSubject(subject, AggregateAsset)
}

// parseAggregateSubject extracts the aggregate ID from a subject of the
// form evt.{aggType}.{id}.{eventType} (or its live.evt.* republished
// form). The trailing event-type segment is discarded.
func parseAggregateSubject(subject, aggType string) (string, bool) {
	s := stripLivePrefix(subject)
	prefix := SubjectRoot + aggType + "."
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	rest := s[len(prefix):]
	dot := strings.Index(rest, ".")
	if dot < 1 || dot == len(rest)-1 {
		return "", false
	}
	// rest = {id}.{eventType}[.{anything else — shouldn't happen}]
	return rest[:dot], true
}

// stripLivePrefix returns the subject with the "live." prefix removed if
// present. Lets parsers treat durable and republished subjects uniformly.
func stripLivePrefix(subject string) string {
	const live = "live."
	if strings.HasPrefix(subject, live) {
		return subject[len(live):]
	}
	return subject
}
