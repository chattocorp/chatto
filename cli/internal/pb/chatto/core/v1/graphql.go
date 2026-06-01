package corev1

// Event payloads exposed through GraphQL implement the EventType union. The
// marker methods below give gqlgen the required interface check; the union is
// defined in events.graphqls.
//
// Every variant rides the same proto Event envelope. Whether a given instance
// is fetched through room history, delivered through myEvents, persisted in
// JetStream, or published live to NATS Core is a delivery/storage concern, not
// part of the GraphQL wrapper shape.

// Room-scoped events.

func (*RoomCreatedEvent) IsEventType()              {}
func (*RoomUpdatedEvent) IsEventType()              {}
func (*RoomDeletedEvent) IsEventType()              {}
func (*RoomArchivedEvent) IsEventType()             {}
func (*RoomUnarchivedEvent) IsEventType()           {}
func (*UserJoinedRoomEvent) IsEventType()           {}
func (*UserLeftRoomEvent) IsEventType()             {}
func (*SpaceMemberDeletedEvent) IsEventType()       {}
func (*MessagePostedEvent) IsEventType()            {}
func (*MessageEditedEvent) IsEventType()            {}
func (*MessageRetractedEvent) IsEventType()         {}
func (*MessageUpdatedEvent) IsEventType()           {}
func (*MessageDeletedEvent) IsEventType()           {}
func (*AssetProcessingStartedEvent) IsEventType()   {}
func (*AssetProcessingSucceededEvent) IsEventType() {}
func (*AssetProcessingFailedEvent) IsEventType()    {}
func (*AssetDeletedEvent) IsEventType()             {}
func (*ReactionAddedEvent) IsEventType()            {}
func (*ReactionRemovedEvent) IsEventType()          {}
func (*UserTypingEvent) IsEventType()               {}
func (*PresenceChangedEvent) IsEventType()          {}
func (*VideoProcessingCompletedEvent) IsEventType() {}
func (*CallParticipantJoinedEvent) IsEventType()    {}
func (*CallParticipantLeftEvent) IsEventType()      {}

// Deployment-scoped events.

func (*ServerConfigUpdatedEvent) IsEventType()          {}
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
