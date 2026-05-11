package corev1

// Implement GraphQL RoomEventType interface for room-scoped events.
func (*RoomCreatedEvent) IsRoomEventType()        {}
func (*RoomUpdatedEvent) IsRoomEventType()        {}
func (*RoomDeletedEvent) IsRoomEventType()        {}
func (*RoomArchivedEvent) IsRoomEventType()       {}
func (*RoomUnarchivedEvent) IsRoomEventType()     {}
func (*UserJoinedRoomEvent) IsRoomEventType()     {}
func (*UserLeftRoomEvent) IsRoomEventType()       {}
func (*SpaceMemberDeletedEvent) IsRoomEventType() {}
func (*MessagePostedEvent) IsRoomEventType()              {}
func (*MessageUpdatedEvent) IsRoomEventType()             {}
func (*MessageDeletedEvent) IsRoomEventType()             {}
func (*ReactionAddedEvent) IsRoomEventType()              {}
func (*ReactionRemovedEvent) IsRoomEventType()            {}
func (*UserTypingEvent) IsRoomEventType()                 {}
func (*PresenceChangedEvent) IsRoomEventType()            {}
func (*VideoProcessingCompletedEvent) IsRoomEventType()   {}
func (*CallParticipantJoinedEvent) IsRoomEventType()      {}
func (*CallParticipantLeftEvent) IsRoomEventType()        {}

// Implement GraphQL InstanceEventType interface for server-scoped events.
// The GraphQL union is still named InstanceEventType pending phase 4 of the
// rename; the proto message names have already moved to *Server*.
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
