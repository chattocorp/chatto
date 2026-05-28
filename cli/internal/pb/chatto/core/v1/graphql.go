package corev1

// All event types delivered through the unified myServerEvents subscription
// are exposed as members of the GraphQL ServerEventType union. The marker
// methods below give gqlgen the required interface check; the union itself
// is defined in events.graphqls.
//
// Every variant rides the same proto ServerEvent envelope; whether a given
// instance is persisted in JetStream or published live to NATS Core is a
// publisher-side choice, not part of the wrapper type.

// Room-scoped events. These flow through the SERVER_EVENTS JetStream + the
// live.server.room.>/live.server.member.> NATS-Core subjects.

func (*RoomCreatedEvent) IsServerEventType()              {}
func (*RoomUpdatedEvent) IsServerEventType()              {}
func (*RoomDeletedEvent) IsServerEventType()              {}
func (*RoomArchivedEvent) IsServerEventType()             {}
func (*RoomUnarchivedEvent) IsServerEventType()           {}
func (*UserJoinedRoomEvent) IsServerEventType()           {}
func (*UserLeftRoomEvent) IsServerEventType()             {}
func (*SpaceMemberDeletedEvent) IsServerEventType()       {}
func (*MessagePostedEvent) IsServerEventType()            {}
func (*MessageEditedEvent) IsServerEventType()            {}
func (*MessageRetractedEvent) IsServerEventType()         {}
func (*MessageUpdatedEvent) IsServerEventType()           {}
func (*MessageDeletedEvent) IsServerEventType()           {}
func (*AssetProcessingStartedEvent) IsServerEventType()   {}
func (*AssetProcessingSucceededEvent) IsServerEventType() {}
func (*AssetProcessingFailedEvent) IsServerEventType()    {}
func (*AssetDeletedEvent) IsServerEventType()             {}
func (*ReactionAddedEvent) IsServerEventType()            {}
func (*ReactionRemovedEvent) IsServerEventType()          {}
func (*UserTypingEvent) IsServerEventType()               {}
func (*PresenceChangedEvent) IsServerEventType()          {}
func (*VideoProcessingCompletedEvent) IsServerEventType() {}
func (*CallParticipantJoinedEvent) IsServerEventType()    {}
func (*CallParticipantLeftEvent) IsServerEventType()      {}

// Deployment-scoped events. These ride the live.server.{user,space,config}.>
// NATS-Core subjects.

func (*ServerConfigUpdatedEvent) IsServerEventType()          {}
func (*UserCreatedEvent) IsServerEventType()                  {}
func (*UserDeletedEvent) IsServerEventType()                  {}
func (*UserProfileUpdatedEvent) IsServerEventType()           {}
func (*ServerUserPreferencesUpdatedEvent) IsServerEventType() {}
func (*NotificationLevelChangedEvent) IsServerEventType()     {}
func (*ThreadFollowChangedEvent) IsServerEventType()          {}
func (*ServerCreatedEvent) IsServerEventType()                {}
func (*ServerUpdatedEvent) IsServerEventType()                {}
func (*ServerDeletedEvent) IsServerEventType()                {}
func (*MentionNotificationEvent) IsServerEventType()          {}
func (*NewDirectMessageNotificationEvent) IsServerEventType() {}
func (*NotificationCreatedEvent) IsServerEventType()          {}
func (*NotificationDismissedEvent) IsServerEventType()        {}
func (*RoomMarkedAsReadEvent) IsServerEventType()             {}
func (*MentionStatusClearedEvent) IsServerEventType()         {}
func (*RoomGroupsUpdatedEvent) IsServerEventType()            {}
func (*SessionTerminatedEvent) IsServerEventType()            {}

// Synthetic, in-process only. Emitted by StreamMyEvents on a 25s ticker
// purely as a liveness signal for the client-side watchdog.

func (*HeartbeatEvent) IsServerEventType() {}

// Room-event interface markers, retained because RoomEvent (the query-side
// wrapper) and its RoomEventType union still exist for historical-message
// fetches. The subscription has consolidated onto ServerEventType above.

func (*RoomCreatedEvent) IsRoomEventType()              {}
func (*RoomUpdatedEvent) IsRoomEventType()              {}
func (*RoomDeletedEvent) IsRoomEventType()              {}
func (*RoomArchivedEvent) IsRoomEventType()             {}
func (*RoomUnarchivedEvent) IsRoomEventType()           {}
func (*UserJoinedRoomEvent) IsRoomEventType()           {}
func (*UserLeftRoomEvent) IsRoomEventType()             {}
func (*SpaceMemberDeletedEvent) IsRoomEventType()       {}
func (*MessagePostedEvent) IsRoomEventType()            {}
func (*MessageEditedEvent) IsRoomEventType()            {}
func (*MessageRetractedEvent) IsRoomEventType()         {}
func (*MessageUpdatedEvent) IsRoomEventType()           {}
func (*MessageDeletedEvent) IsRoomEventType()           {}
func (*AssetProcessingStartedEvent) IsRoomEventType()   {}
func (*AssetProcessingSucceededEvent) IsRoomEventType() {}
func (*AssetProcessingFailedEvent) IsRoomEventType()    {}
func (*AssetDeletedEvent) IsRoomEventType()             {}
func (*ReactionAddedEvent) IsRoomEventType()            {}
func (*ReactionRemovedEvent) IsRoomEventType()          {}
func (*UserTypingEvent) IsRoomEventType()               {}
func (*PresenceChangedEvent) IsRoomEventType()          {}
func (*VideoProcessingCompletedEvent) IsRoomEventType() {}
func (*CallParticipantJoinedEvent) IsRoomEventType()    {}
func (*CallParticipantLeftEvent) IsRoomEventType()      {}
