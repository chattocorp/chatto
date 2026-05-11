package corev1

// All event types delivered through the unified myServerEvents subscription
// are exposed as members of the GraphQL ServerEventType union. The marker
// methods below give gqlgen the required interface check; the union itself
// is defined in events.graphqls.

// Room-scoped events. These flow through the SERVER_EVENTS JetStream + the
// live.server.room.>/live.server.member.> NATS-Core subjects, and travel as
// proto ServerEvent envelopes inside core.

func (*RoomCreatedEvent) IsServerEventType()              {}
func (*RoomUpdatedEvent) IsServerEventType()              {}
func (*RoomDeletedEvent) IsServerEventType()              {}
func (*RoomArchivedEvent) IsServerEventType()             {}
func (*RoomUnarchivedEvent) IsServerEventType()           {}
func (*UserJoinedRoomEvent) IsServerEventType()           {}
func (*UserLeftRoomEvent) IsServerEventType()             {}
func (*SpaceMemberDeletedEvent) IsServerEventType()       {}
func (*MessagePostedEvent) IsServerEventType()            {}
func (*MessageUpdatedEvent) IsServerEventType()           {}
func (*MessageDeletedEvent) IsServerEventType()           {}
func (*ReactionAddedEvent) IsServerEventType()            {}
func (*ReactionRemovedEvent) IsServerEventType()          {}
func (*UserTypingEvent) IsServerEventType()               {}
func (*PresenceChangedEvent) IsServerEventType()          {}
func (*VideoProcessingCompletedEvent) IsServerEventType() {}
func (*CallParticipantJoinedEvent) IsServerEventType()    {}
func (*CallParticipantLeftEvent) IsServerEventType()      {}

// Deployment-scoped events. These ride the live.server.{user,space,config}.>
// NATS-Core subjects and travel as proto LiveEvent envelopes inside core.

func (*ServerConfigUpdatedEvent) IsServerEventType()          {}
func (*UserCreatedEvent) IsServerEventType()                  {}
func (*UserDeletedEvent) IsServerEventType()                  {}
func (*UserProfileUpdatedEvent) IsServerEventType()           {}
func (*ServerUserPreferencesUpdatedEvent) IsServerEventType() {}
func (*NotificationLevelChangedEvent) IsServerEventType()     {}
func (*ThreadFollowChangedEvent) IsServerEventType()          {}
func (*UserJoinedSpaceEvent) IsServerEventType()              {}
func (*UserLeftSpaceEvent) IsServerEventType()                {}
func (*SpaceUpdatedEvent) IsServerEventType()                 {}
func (*MentionNotificationEvent) IsServerEventType()          {}
func (*NewDirectMessageNotificationEvent) IsServerEventType() {}
func (*NotificationCreatedEvent) IsServerEventType()          {}
func (*NotificationDismissedEvent) IsServerEventType()        {}
func (*NewMessageInSpaceEvent) IsServerEventType()            {}
func (*RoomMarkedAsReadEvent) IsServerEventType()             {}
func (*RoomLayoutUpdatedEvent) IsServerEventType()            {}
func (*SessionTerminatedEvent) IsServerEventType()            {}

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
func (*MessageUpdatedEvent) IsRoomEventType()           {}
func (*MessageDeletedEvent) IsRoomEventType()           {}
func (*ReactionAddedEvent) IsRoomEventType()            {}
func (*ReactionRemovedEvent) IsRoomEventType()          {}
func (*UserTypingEvent) IsRoomEventType()               {}
func (*PresenceChangedEvent) IsRoomEventType()          {}
func (*VideoProcessingCompletedEvent) IsRoomEventType() {}
func (*CallParticipantJoinedEvent) IsRoomEventType()    {}
func (*CallParticipantLeftEvent) IsRoomEventType()      {}
