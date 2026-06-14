package corev1

// Event payloads exposed through GraphQL implement the EventType union. The
// marker methods below give gqlgen the required interface check; the union is
// defined in events.graphqls.
//
// GraphQL exposes one Event envelope, but the in-process model may wrap a
// durable EVT Event, a transient LiveEvent, or a synthetic heartbeat. Payload
// types implement this union independently of the transport/storage envelope.

// Room-scoped events.

func (*RoomCreatedEvent) IsEventType()              {}
func (*RoomUpdatedEvent) IsEventType()              {}
func (*RoomDeletedEvent) IsEventType()              {}
func (*RoomArchivedEvent) IsEventType()             {}
func (*RoomUnarchivedEvent) IsEventType()           {}
func (*UserJoinedRoomEvent) IsEventType()           {}
func (*UserLeftRoomEvent) IsEventType()             {}
func (*RoomMemberBannedEvent) IsEventType()         {}
func (*RoomMemberUnbannedEvent) IsEventType()       {}
func (*ServerMemberDeletedEvent) IsEventType()      {}
func (*MessagePostedEvent) IsEventType()            {}
func (*MessageEditedEvent) IsEventType()            {}
func (*MessageRetractedEvent) IsEventType()         {}
func (*ThreadCreatedEvent) IsEventType()            {}
func (*AssetProcessingStartedEvent) IsEventType()   {}
func (*AssetProcessingSucceededEvent) IsEventType() {}
func (*AssetProcessingFailedEvent) IsEventType()    {}
func (*AssetDeletedEvent) IsEventType()             {}
func (*ReactionAddedEvent) IsEventType()            {}
func (*ReactionRemovedEvent) IsEventType()          {}
func (*UserTypingEvent) IsEventType()               {}
func (*PresenceChangedEvent) IsEventType()          {}
func (*CallStartedEvent) IsEventType()              {}
func (*CallParticipantJoinedEvent) IsEventType()    {}
func (*CallParticipantLeftEvent) IsEventType()      {}
func (*CallEndedEvent) IsEventType()                {}

// Deployment-scoped events.

func (*UserCreatedEvent) IsEventType()                  {}
func (*UserDeletedEvent) IsEventType()                  {}
func (*UserProfileUpdatedEvent) IsEventType()           {}
func (*ServerUserPreferencesUpdatedEvent) IsEventType() {}
func (*NotificationLevelChangedEvent) IsEventType()     {}
func (*ThreadFollowChangedEvent) IsEventType()          {}
func (*ServerUpdatedEvent) IsEventType()                {}
func (*MentionNotificationEvent) IsEventType()          {}
func (*NewDirectMessageNotificationEvent) IsEventType() {}
func (*NotificationCreatedEvent) IsEventType()          {}
func (*NotificationDismissedEvent) IsEventType()        {}
func (*RoomMarkedAsReadEvent) IsEventType()             {}
func (*MentionStatusClearedEvent) IsEventType()         {}
func (*RoomGroupsUpdatedEvent) IsEventType()            {}
func (*SessionTerminatedEvent) IsEventType()            {}

// Synthetic, in-process only. Emitted by StreamMyEvents on a 25s ticker purely
// as a liveness signal for the client-side watchdog.

func (*HeartbeatEvent) IsEventType() {}
