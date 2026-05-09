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

// Implement GraphQL InstanceEventType interface for instance-scoped events.
func (*InstanceConfigUpdatedEvent) IsInstanceEventType()          {}
func (*UserCreatedEvent) IsInstanceEventType()                    {}
func (*UserDeletedEvent) IsInstanceEventType()                    {}
func (*UserProfileUpdatedEvent) IsInstanceEventType()             {}
func (*InstanceUserPreferencesUpdatedEvent) IsInstanceEventType() {}
func (*NotificationLevelChangedEvent) IsInstanceEventType()       {}
func (*ThreadFollowChangedEvent) IsInstanceEventType()            {}
func (*UserJoinedSpaceEvent) IsInstanceEventType()                {}
func (*UserLeftSpaceEvent) IsInstanceEventType()                  {}
func (*SpaceUpdatedEvent) IsInstanceEventType()                   {}
func (*MentionNotificationEvent) IsInstanceEventType()            {}
func (*NewDirectMessageNotificationEvent) IsInstanceEventType()   {}
func (*NotificationCreatedEvent) IsInstanceEventType()            {}
func (*NotificationDismissedEvent) IsInstanceEventType()          {}
func (*NewMessageInSpaceEvent) IsInstanceEventType()              {}
func (*RoomMarkedAsReadEvent) IsInstanceEventType()               {}
func (*RoomLayoutUpdatedEvent) IsInstanceEventType()              {}
func (*SessionTerminatedEvent) IsInstanceEventType()              {}
