/* eslint-disable */
export type Maybe<T> = T | null;
export type InputMaybe<T> = T | null | undefined;
export type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]?: Maybe<T[SubKey]> };
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]: Maybe<T[SubKey]> };
export type MakeEmpty<T extends { [key: string]: unknown }, K extends keyof T> = { [_ in K]?: never };
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  /** 64-bit integer scalar for large values (bytes, storage, message counts, etc.). */
  Int64: { input: any; output: any; }
  /** Custom scalar for date/time values, formatted as RFC3339. */
  Time: { input: any; output: any; }
  /** Custom scalar for file uploads via GraphQL multipart requests. */
  Upload: { input: any; output: any; }
};

/** Point-in-time storage-account limits and usage. Intended for operator diagnostics. */
export type AccountInfo = {
  __typename?: 'AccountInfo';
  /** Consumer limit (-1 for unlimited) */
  consumers: Scalars['Int']['output'];
  /** Consumers in use */
  consumersUsed: Scalars['Int']['output'];
  /** Memory limit in bytes (-1 for unlimited) */
  memory: Scalars['Int64']['output'];
  /** Memory used in bytes */
  memoryUsed: Scalars['Int64']['output'];
  /** Storage limit in bytes (-1 for unlimited) */
  storage: Scalars['Int64']['output'];
  /** Storage used in bytes */
  storageUsed: Scalars['Int64']['output'];
  /** Stream limit (-1 for unlimited) */
  streams: Scalars['Int']['output'];
  /** Streams in use */
  streamsUsed: Scalars['Int']['output'];
};

/** Input for adding an emoji reaction to a message. */
export type AddReactionInput = {
  /** The emoji shortcode name (e.g., 'thumbsup', 'heart'). */
  emoji: Scalars['String']['input'];
  /** The event ID of the message to react to. */
  messageEventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
};

/** Admin mutations for security and user management. */
export type AdminMutations = {
  __typename?: 'AdminMutations';
  /** Clear the 30-day login change cooldown for a user, allowing them to immediately rename themselves. Idempotent. */
  clearUsernameCooldown: Scalars['Boolean']['output'];
  /** Update the newline-separated blocked-username list and return the effective saved value. Requires `server.manage`. */
  updateBlockedUsernames: Scalars['String']['output'];
  /** Update a user's login and/or display name. Bypasses the 30-day login change cooldown but otherwise reuses the same validation as updateProfile. */
  updateUser: User;
};


/** Admin mutations for security and user management. */
export type AdminMutationsClearUsernameCooldownArgs = {
  input: ClearUsernameCooldownInput;
};


/** Admin mutations for security and user management. */
export type AdminMutationsUpdateBlockedUsernamesArgs = {
  input: UpdateBlockedUsernamesInput;
};


/** Admin mutations for security and user management. */
export type AdminMutationsUpdateUserArgs = {
  input: AdminUpdateUserInput;
};

/** Admin-console query namespace. Returns null unless the viewer is authenticated. */
export type AdminQueries = {
  __typename?: 'AdminQueries';
  /** Browse the durable event log newest-first for operator diagnostics. `limit` defaults to 50, max 200. `before` is a sequence string; entries returned will have sequence < before. */
  eventLog: EventLogConnection;
  /** Fetch a single diagnostic event-log entry by sequence. Returns null if the sequence doesn't exist. */
  eventLogEntry?: Maybe<EventLogEntry>;
  /**
   * Resolve the explicit grants and denials configured for a role on a
   * specific room group. Returns empty arrays if neither side has any keys.
   */
  groupRolePermissions: RoomGroupRolePermissions;
  /**
   * Resolve the explicit grants and denials configured for a user on a
   * specific room group (user-level overrides at room-group scope).
   */
  groupUserPermissions: RoomGroupUserPermissions;
  /** Inspect point-in-time runtime state and rough memory estimates for event-sourced projections. */
  projections: Array<ProjectionState>;
  /** RBAC editor and inspection queries. */
  rbac: RbacQueries;
  /** List active room bans. Requires server-scope `room.ban-member`. */
  roomBans: Array<RoomBan>;
  /** Get server configuration. Requires `server.manage`. */
  serverConfig: AdminServerConfig;
  /** Get point-in-time operator diagnostics for connection, storage, and deployment counts. Requires the owner role. */
  systemInfo: SystemInfo;
};


/** Admin-console query namespace. Returns null unless the viewer is authenticated. */
export type AdminQueriesEventLogArgs = {
  before?: InputMaybe<Scalars['String']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
};


/** Admin-console query namespace. Returns null unless the viewer is authenticated. */
export type AdminQueriesEventLogEntryArgs = {
  sequence: Scalars['String']['input'];
};


/** Admin-console query namespace. Returns null unless the viewer is authenticated. */
export type AdminQueriesGroupRolePermissionsArgs = {
  groupId: Scalars['ID']['input'];
  roleName: Scalars['String']['input'];
};


/** Admin-console query namespace. Returns null unless the viewer is authenticated. */
export type AdminQueriesGroupUserPermissionsArgs = {
  groupId: Scalars['ID']['input'];
  userId: Scalars['ID']['input'];
};


/** Admin-console query namespace. Returns null unless the viewer is authenticated. */
export type AdminQueriesRoomBansArgs = {
  roomId?: InputMaybe<Scalars['ID']['input']>;
};

/** Server configuration section. */
export type AdminServerConfig = {
  __typename?: 'AdminServerConfig';
  /** Blocked usernames (newline-separated). Users cannot register with these names. */
  blockedUsernames?: Maybe<Scalars['String']['output']>;
  /** Short description of this server, used for OG link-preview metadata and the welcome card. */
  description?: Maybe<Scalars['String']['output']>;
  /** Message of the Day, displayed in the header bar. */
  motd?: Maybe<Scalars['String']['output']>;
  /** Server name, displayed in page titles. Defaults to 'Chatto' if not set. */
  serverName: Scalars['String']['output'];
  /** Welcome message shown on the login page (markdown supported). */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};

/** Input for AdminMutations.updateUser. At least one of login or displayName must be set. */
export type AdminUpdateUserInput = {
  /** New display name. */
  displayName?: InputMaybe<Scalars['String']['input']>;
  /** New login (username). When set, bypasses the 30-day cooldown but still validates against the blocked-username list and login rules. */
  login?: InputMaybe<Scalars['String']['input']>;
  /** ID of the user to update. */
  userId: Scalars['ID']['input'];
};

/** Input for archiving a room. */
export type ArchiveRoomInput = {
  /** The ID of the room to archive. */
  roomId: Scalars['ID']['input'];
};

/** Event: an asset has been deleted; subscribers should drop any local reference. */
export type AssetDeletedEvent = {
  __typename?: 'AssetDeletedEvent';
  /** The deleted asset ID. */
  assetId: Scalars['ID']['output'];
  /** The room ID, when the asset was room-scoped. */
  roomId?: Maybe<Scalars['ID']['output']>;
};

/** Event: asset processing reached a durable failed/unavailable outcome. */
export type AssetProcessingFailedEvent = {
  __typename?: 'AssetProcessingFailedEvent';
  /** The original asset ID that failed processing. */
  assetId: Scalars['ID']['output'];
  /** The event ID of the message containing the attachment, when message-owned and still available. */
  messageEventId?: Maybe<Scalars['ID']['output']>;
  /** Stable machine-readable reason. */
  reasonCode: Scalars['String']['output'];
  /** The room ID, when the processed asset is still associated with a room message. */
  roomId?: Maybe<Scalars['ID']['output']>;
};

/**
 * Event: asset processing has been enqueued. Emitted before SucceededEvent or
 * FailedEvent so subscribers can render a "processing…" placeholder.
 */
export type AssetProcessingStartedEvent = {
  __typename?: 'AssetProcessingStartedEvent';
  /** The original asset ID whose processing has been enqueued. */
  assetId: Scalars['ID']['output'];
  /** The event ID of the message containing the attachment, when message-owned and still available. */
  messageEventId?: Maybe<Scalars['ID']['output']>;
  /** The room ID, when the processed asset is still associated with a room message. */
  roomId?: Maybe<Scalars['ID']['output']>;
};

/** Event: asset processing produced a durable derivative manifest. */
export type AssetProcessingSucceededEvent = {
  __typename?: 'AssetProcessingSucceededEvent';
  /** The original asset ID that was processed. */
  assetId: Scalars['ID']['output'];
  /** The event ID of the message containing the processed attachment, when message-owned and still available. */
  messageEventId?: Maybe<Scalars['ID']['output']>;
  /** The room ID, when the processed asset is still associated with a room message. */
  roomId?: Maybe<Scalars['ID']['output']>;
};

/** A protected asset URL and the time its embedded access ticket expires. */
export type AssetUrl = {
  __typename?: 'AssetURL';
  /** Time after which the embedded access ticket is no longer valid. */
  expiresAt: Scalars['Time']['output'];
  /** URL to the asset on the owning host. */
  url: Scalars['String']['output'];
};

/** Input for assigning an server role to a user. */
export type AssignRoleInput = {
  /** The name of the role to assign. */
  roleName: Scalars['String']['input'];
  /** The ID of the user to assign the role to. */
  userId: Scalars['ID']['input'];
};

/** An attachment to a message (image, video, etc.). */
export type Attachment = {
  __typename?: 'Attachment';
  /** URL and expiry for the full attachment. Optional transform parameters for images. */
  assetUrl: AssetUrl;
  /** The MIME type (e.g., 'image/jpeg', 'video/mp4'). */
  contentType: Scalars['String']['output'];
  /** The original filename. */
  filename: Scalars['String']['output'];
  /** Image height in pixels (0 for non-images). */
  height: Scalars['Int']['output'];
  /** The attachment's unique ID. */
  id: Scalars['ID']['output'];
  /** The room ID where this attachment was posted. */
  roomId: Scalars['ID']['output'];
  /** The file size in bytes. */
  size: Scalars['Int64']['output'];
  /** URL and expiry for the thumbnail (null if no thumbnail). Optional transform parameters. */
  thumbnailAssetUrl?: Maybe<AssetUrl>;
  /** URL to download the thumbnail (null if no thumbnail). Optional transform parameters. */
  thumbnailUrl?: Maybe<Scalars['String']['output']>;
  /** URL to download the full attachment. Optional transform parameters for images. */
  url: Scalars['String']['output'];
  /** Video processing state (null for non-video attachments). */
  videoProcessing?: Maybe<VideoProcessing>;
  /** Image width in pixels (0 for non-images). */
  width: Scalars['Int']['output'];
};


/** An attachment to a message (image, video, etc.). */
export type AttachmentAssetUrlArgs = {
  fit?: InputMaybe<FitMode>;
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/** An attachment to a message (image, video, etc.). */
export type AttachmentThumbnailAssetUrlArgs = {
  fit?: InputMaybe<FitMode>;
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/** An attachment to a message (image, video, etc.). */
export type AttachmentThumbnailUrlArgs = {
  fit?: InputMaybe<FitMode>;
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/** An attachment to a message (image, video, etc.). */
export type AttachmentUrlArgs = {
  fit?: InputMaybe<FitMode>;
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};

/** External login provider metadata safe to expose before authentication. */
export type AuthProvider = {
  __typename?: 'AuthProvider';
  /** Stable provider ID used in login URLs and external identity links. */
  id: Scalars['ID']['output'];
  /** Human-readable label for login UI. */
  label: Scalars['String']['output'];
  /** Relative URL that starts this provider's login flow. */
  loginUrl: Scalars['String']['output'];
  /** Provider type, such as 'oidc', 'github', or 'google'. */
  type: Scalars['String']['output'];
};

/** Input for banning another member from a channel room. */
export type BanRoomMemberInput = {
  /** Optional expiry for a temporary ban. Null means indefinite. */
  expiresAt?: InputMaybe<Scalars['Time']['input']>;
  /** Moderator-entered reason stored for audit. */
  reason: Scalars['String']['input'];
  /** The ID of the channel room to ban the member from. */
  roomId: Scalars['ID']['input'];
  /** The ID of the user to ban from the room. */
  userId: Scalars['ID']['input'];
};

/** Event: A voice call ended in a room. */
export type CallEndedEvent = {
  __typename?: 'CallEndedEvent';
  /** The ID of this call session. */
  callId: Scalars['ID']['output'];
  /** The ID of the room where the call ended. */
  roomId: Scalars['ID']['output'];
};

/** A participant currently in a voice call. */
export type CallParticipant = {
  __typename?: 'CallParticipant';
  /** The active call session ID this participant belongs to. */
  callId: Scalars['ID']['output'];
  /** When the user joined the call. */
  joinedAt: Scalars['Time']['output'];
  /** The user currently participating in the call. */
  user: User;
};

/**
 * Event: A user joined a voice call in a room.
 * The user who joined is identified by the parent Event's actorId/actor.
 */
export type CallParticipantJoinedEvent = {
  __typename?: 'CallParticipantJoinedEvent';
  /** The ID of this call session. */
  callId: Scalars['ID']['output'];
  /** The ID of the room where the call is happening. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: A user left a voice call in a room.
 * The user who left is identified by the parent Event's actorId/actor.
 */
export type CallParticipantLeftEvent = {
  __typename?: 'CallParticipantLeftEvent';
  /** The ID of this call session. */
  callId: Scalars['ID']['output'];
  /** The ID of the room where the call was happening. */
  roomId: Scalars['ID']['output'];
};

/** Event: A voice call started in a room. */
export type CallStartedEvent = {
  __typename?: 'CallStartedEvent';
  /** The ID of this call session. */
  callId: Scalars['ID']['output'];
  /** The ID of the room where the call started. */
  roomId: Scalars['ID']['output'];
};

/** Input for clearing permission state on a role. */
export type ClearPermissionStateInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear permission state for. */
  roleName: Scalars['String']['input'];
};

/** Input for clearing a room-level permission override. */
export type ClearRoomPermissionInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear the permission for. */
  roleName: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/**
 * Input for clearing both grant and denial of a permission on a user.
 * Same scope rules as `GrantUserPermissionInput`.
 */
export type ClearUserPermissionStateInput = {
  /** Optional room-group ID. Mutually exclusive with `roomId`. */
  groupId?: InputMaybe<Scalars['ID']['input']>;
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** Optional room ID. Mutually exclusive with `groupId`. */
  roomId?: InputMaybe<Scalars['ID']['input']>;
  /** The user whose permission state to clear. */
  userId: Scalars['ID']['input'];
};

/** Input for AdminMutations.clearUsernameCooldown. */
export type ClearUsernameCooldownInput = {
  /** The user whose username cooldown to clear. */
  userId: Scalars['ID']['input'];
};

/** Point-in-time diagnostic information about the backing message broker connection. */
export type ConnectionInfo = {
  __typename?: 'ConnectionInfo';
  /** Whether the connection to NATS is currently active. */
  connected: Scalars['Boolean']['output'];
  /** Maximum message payload size in bytes. */
  maxPayload: Scalars['Int64']['output'];
  /** Round-trip time to the NATS server (e.g., '1.234ms'). */
  rtt: Scalars['String']['output'];
  /** Unique identifier of the connected NATS server. */
  serverId: Scalars['String']['output'];
  /** Human-readable name of the connected NATS server. */
  serverName: Scalars['String']['output'];
  /** NATS server version string. */
  version: Scalars['String']['output'];
};

/** Input for creating a new role. */
export type CreateRoleInput = {
  /** Role description. */
  description: Scalars['String']['input'];
  /** Human-readable display name. */
  displayName: Scalars['String']['input'];
  /** Role identifier (lowercase alphanumeric + underscores, max 32 chars). */
  name: Scalars['String']['input'];
  /** Whether @role pings notify users assigned to this role. Defaults to false. */
  pingable?: InputMaybe<Scalars['Boolean']['input']>;
};

/** Input for creating a new room group. */
export type CreateRoomGroupInput = {
  /** Optional operator-facing description. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** Display name for the new room group (e.g., 'Engineering', 'Public'). */
  name: Scalars['String']['input'];
};

/** Input for creating a new room. */
export type CreateRoomInput = {
  /** Optional description of the room's purpose. */
  description?: InputMaybe<Scalars['String']['input']>;
  /**
   * Room group ID to place the new channel room in. Channel room creation
   * requires an explicit group; DM rooms are created through the DM APIs and
   * do not use this input.
   */
  groupId: Scalars['ID']['input'];
  /** The name of the new room. */
  name: Scalars['String']['input'];
};

/**
 * Notification for new DM messages.
 * Created when someone sends a message in a DM conversation you're part of.
 */
export type DmMessageNotificationItem = {
  __typename?: 'DMMessageNotificationItem';
  /** User who triggered the notification */
  actor?: Maybe<User>;
  /** When the notification was created */
  createdAt: Scalars['Time']['output'];
  /** Unique notification ID */
  id: Scalars['ID']['output'];
  /** The DM conversation room */
  room: Room;
  /** Human-readable summary for display */
  summary: Scalars['String']['output'];
};

/** Input for deleting an attachment from a message. */
export type DeleteAttachmentInput = {
  /** The ID of the attachment to delete. */
  attachmentId: Scalars['ID']['input'];
  /** The event ID of the message containing the attachment. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
};

/** Input for deleting a user avatar. */
export type DeleteAvatarInput = {
  /** The ID of the user whose avatar to delete. Caller must be self or have admin permission. */
  userId: Scalars['ID']['input'];
};

/** Input for deleting a link preview from a message. */
export type DeleteLinkPreviewInput = {
  /** The event ID of the message containing the link preview. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
  /** The URL of the link preview to delete. */
  url: Scalars['String']['input'];
};

/** Input for deleting a message. */
export type DeleteMessageInput = {
  /** The event ID of the message to delete. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
};

/** Input for deleting the current user's account. */
export type DeleteMyAccountInput = {
  /** Confirmation token obtained from requestAccountDeletion. */
  confirmationToken: Scalars['String']['input'];
};

/** Input for deleting an server role. */
export type DeleteRoleInput = {
  /** The name of the role to delete. */
  name: Scalars['String']['input'];
};

/** Input for deleting a room group. Fails if the room group still contains any rooms. */
export type DeleteRoomGroupInput = {
  /** The room group's ID. */
  id: Scalars['ID']['input'];
};

/** Input for denying a permission for a role. */
export type DenyPermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  roleName: Scalars['String']['input'];
};

/** Input for denying a room-level permission for a role. */
export type DenyRoomPermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  roleName: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/**
 * Input for denying a permission directly to a user. Same scope rules as
 * `GrantUserPermissionInput`.
 */
export type DenyUserPermissionInput = {
  /** Optional room-group ID. Mutually exclusive with `roomId`. */
  groupId?: InputMaybe<Scalars['ID']['input']>;
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** Optional room ID. Mutually exclusive with `groupId`. */
  roomId?: InputMaybe<Scalars['ID']['input']>;
  /** The user to deny the permission for. */
  userId: Scalars['ID']['input'];
};

/** Input for dismissing a notification. */
export type DismissNotificationInput = {
  /** The ID of the notification to dismiss. */
  notificationId: Scalars['ID']['input'];
};

/**
 * Event wraps all typed Chatto events.
 *
 * Room queries and server subscriptions are delivery contexts over the same
 * event envelope. Room-scoped events are returned only when the current user can
 * see the affected room; deployment-scoped events are delivered according to
 * their audience.
 */
export type Event = {
  __typename?: 'Event';
  /** The user who triggered this event. May be null if user was deleted. */
  actor?: Maybe<User>;
  /** The ID of the user who triggered this event. Null for system/synthetic events. */
  actorId?: Maybe<Scalars['ID']['output']>;
  /** When this event was created. */
  createdAt: Scalars['Time']['output'];
  /** The concrete event data. */
  event: EventType;
  /** Universal event identifier. */
  id: Scalars['ID']['output'];
};

/** A page of diagnostic event-log entries, newest first. */
export type EventLogConnection = {
  __typename?: 'EventLogConnection';
  /** Pass as the next call's `before` to fetch the next (older) page. Null when there are no older entries. */
  endCursor?: Maybe<Scalars['String']['output']>;
  /** Entries on this page, ordered newest → oldest. */
  entries: Array<EventLogEntry>;
  /** True if older entries exist beyond this page. */
  hasOlder: Scalars['Boolean']['output'];
  /** Total messages currently in EVT, serialized as Int64 so large event logs do not overflow GraphQL Int. */
  totalCount: Scalars['Int64']['output'];
};

/** One diagnostic entry in the durable event log. Use this for operator inspection, not as a machine-parsed product feed. */
export type EventLogEntry = {
  __typename?: 'EventLogEntry';
  /** ID of the actor who triggered the event. May also be a synthetic actor like 'system:migration' or 'system:bootstrap'. */
  actorId: Scalars['String']['output'];
  /** Diagnostic aggregate identifier derived from storage metadata. */
  aggregateId: Scalars['String']['output'];
  /** Diagnostic aggregate category derived from storage metadata. */
  aggregateType: Scalars['String']['output'];
  /** When the event was created (per the event payload, not the stream). */
  createdAt: Scalars['Time']['output'];
  /** Per-event unique identifier from event.id. */
  eventId: Scalars['String']['output'];
  /** Diagnostic event variant label. Empty if the payload cannot be classified. */
  eventType: Scalars['String']['output'];
  /** Raw payload rendered as JSON for human inspection. Do not build clients that depend on this shape. */
  payloadJson: Scalars['String']['output'];
  /** Monotonic event-log sequence, serialized as a String so large values do not overflow GraphQL Int. */
  sequence: Scalars['String']['output'];
  /** Diagnostic storage subject. Useful for operators, but clients should not parse it as a stable product contract. */
  subject: Scalars['String']['output'];
};

/** Union of every typed event payload exposed by GraphQL. */
export type EventType = AssetDeletedEvent | AssetProcessingFailedEvent | AssetProcessingStartedEvent | AssetProcessingSucceededEvent | CallEndedEvent | CallParticipantJoinedEvent | CallParticipantLeftEvent | CallStartedEvent | HeartbeatEvent | MentionNotificationEvent | MentionStatusClearedEvent | MessageEditedEvent | MessagePostedEvent | MessageRetractedEvent | NewDirectMessageNotificationEvent | NotificationCreatedEvent | NotificationDismissedEvent | NotificationLevelChangedEvent | PresenceChangedEvent | ReactionAddedEvent | ReactionRemovedEvent | RoomArchivedEvent | RoomCreatedEvent | RoomDeletedEvent | RoomGroupsUpdatedEvent | RoomMarkedAsReadEvent | RoomMemberBannedEvent | RoomMemberUnbannedEvent | RoomUnarchivedEvent | RoomUpdatedEvent | ServerMemberDeletedEvent | ServerUpdatedEvent | ServerUserPreferencesUpdatedEvent | SessionTerminatedEvent | ThreadCreatedEvent | ThreadFollowChangedEvent | UserCreatedEvent | UserDeletedEvent | UserJoinedRoomEvent | UserLeftRoomEvent | UserProfileUpdatedEvent | UserTypingEvent;

/** Fit mode for image transformations. */
export enum FitMode {
  /** Fit within bounds while preserving aspect ratio (letterbox if needed). */
  Contain = 'CONTAIN',
  /** Fill bounds while preserving aspect ratio (center-crop if needed). */
  Cover = 'COVER',
  /** Stretch to exact dimensions (may distort aspect ratio). */
  Exact = 'EXACT'
}

/** Input for following a thread. */
export type FollowThreadInput = {
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['input'];
  /** The event ID of the thread root message. */
  threadRootEventId: Scalars['ID']['input'];
};

/**
 * A thread that the current user is following.
 * Contains metadata for display in the My Threads list.
 */
export type FollowedThread = {
  __typename?: 'FollowedThread';
  /** Whether this thread has unread replies since the user last opened it. */
  hasUnread: Scalars['Boolean']['output'];
  /** Timestamp of the most recent reply (null if no replies). */
  lastReplyAt?: Maybe<Scalars['Time']['output']>;
  /** Number of replies in this thread. */
  replyCount: Scalars['Int']['output'];
  /** The room containing the thread. */
  room: Room;
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['output'];
  /** The root message of the thread (for preview text). */
  rootMessage?: Maybe<Event>;
  /** Users who have participated in this thread (default 5, max 10) for preview. */
  threadParticipants: Array<User>;
  /** The event ID of the thread's root message. */
  threadRootEventId: Scalars['ID']['output'];
};


/**
 * A thread that the current user is following.
 * Contains metadata for display in the My Threads list.
 */
export type FollowedThreadThreadParticipantsArgs = {
  first?: InputMaybe<Scalars['Int']['input']>;
};

/** Paginated list of followed threads with metadata. */
export type FollowedThreadsConnection = {
  __typename?: 'FollowedThreadsConnection';
  /** Whether there are more followed threads beyond this page. */
  hasMore: Scalars['Boolean']['output'];
  /** The followed threads in this page. */
  threads: Array<FollowedThread>;
  /** Total count of followed threads before pagination. */
  totalCount: Scalars['Int']['output'];
};

/** Input for granting a permission to a role. */
export type GrantPermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  roleName: Scalars['String']['input'];
};

/** Input for granting a room-level permission to a role. */
export type GrantRoomPermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  roleName: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/**
 * Input for granting a permission directly to a user. Exactly one of
 * `roomId` or `groupId` may be provided; with neither, the grant applies
 * at server scope.
 */
export type GrantUserPermissionInput = {
  /**
   * Optional room-group ID for a group-scoped grant. Mutually exclusive
   * with `roomId`. Only works for permissions that support group scope.
   */
  groupId?: InputMaybe<Scalars['ID']['input']>;
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /**
   * Optional room ID for a room-scoped grant. Mutually exclusive with
   * `groupId`. Only works for permissions that support room scope.
   */
  roomId?: InputMaybe<Scalars['ID']['input']>;
  /** The user to grant the permission to. */
  userId: Scalars['ID']['input'];
};

/**
 * Input for granting a permission on a room group. The subject is either a role
 * (by name) or a user (by ID).
 */
export type GroupPermissionInput = {
  /** The room group to scope the grant to. */
  groupId: Scalars['ID']['input'];
  /** Permission identifier (e.g., 'message.post'). */
  permission: Scalars['String']['input'];
  /** Role name or user ID. (Role names are lowercase letters; user IDs start with `U`.) */
  subject: Scalars['String']['input'];
};

/**
 * Synthetic event emitted by the server on the `myEvents` subscription
 * every ~25 seconds. It has no payload — clients use its arrival cadence to
 * detect a dead subscription on an otherwise-healthy WebSocket and trigger
 * a reconnect. Safe to ignore in event handlers.
 */
export type HeartbeatEvent = {
  __typename?: 'HeartbeatEvent';
  /** Always true. Clients only need the event's arrival, not its contents. */
  alive: Scalars['Boolean']['output'];
};

/** Input for joining every joinable room in a group. */
export type JoinGroupInput = {
  /** The ID of the room group whose rooms the caller wants to join. */
  groupId: Scalars['ID']['input'];
};

/** Input for joining a room. */
export type JoinRoomInput = {
  /** The ID of the room to join. */
  roomId: Scalars['ID']['input'];
};

/** Input for leaving a room. */
export type LeaveRoomInput = {
  /** The ID of the room to leave. */
  roomId: Scalars['ID']['input'];
};

/** LinkPreview represents OpenGraph/oEmbed metadata extracted from a URL. */
export type LinkPreview = {
  __typename?: 'LinkPreview';
  /** The page description (from og:description or meta description). */
  description?: Maybe<Scalars['String']['output']>;
  /** Embed ID for rich embeds (e.g., YouTube video ID). */
  embedId?: Maybe<Scalars['String']['output']>;
  /** Type of embed: 'generic', 'youtube', 'vimeo', etc. */
  embedType?: Maybe<Scalars['String']['output']>;
  /** Asset ID of the preview image. Used by clients to pass back in LinkPreviewInput when posting a message. */
  imageAssetId?: Maybe<Scalars['String']['output']>;
  /** URL to the preview image. Optional transform parameters for resizing. */
  imageUrl?: Maybe<Scalars['String']['output']>;
  /** The site name (from og:site_name). */
  siteName?: Maybe<Scalars['String']['output']>;
  /** The page title (from og:title or <title>). */
  title?: Maybe<Scalars['String']['output']>;
  /** The original URL that was previewed. */
  url: Scalars['String']['output'];
};


/** LinkPreview represents OpenGraph/oEmbed metadata extracted from a URL. */
export type LinkPreviewImageUrlArgs = {
  fit?: InputMaybe<FitMode>;
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};

/**
 * Input type for passing link preview data from client to server.
 * The client fetches preview metadata via the linkPreview query, then includes
 * the data in the postMessage mutation so it can be attached to the message.
 */
export type LinkPreviewInput = {
  /** The page description. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** Embed ID for rich embeds (e.g., YouTube video ID). */
  embedId?: InputMaybe<Scalars['String']['input']>;
  /** Type of embed: 'generic', 'youtube', 'vimeo', etc. */
  embedType?: InputMaybe<Scalars['String']['input']>;
  /** Asset ID of the preview image (from the linkPreview query response). */
  imageAssetId?: InputMaybe<Scalars['String']['input']>;
  /** The site name. */
  siteName?: InputMaybe<Scalars['String']['input']>;
  /** The page title. */
  title?: InputMaybe<Scalars['String']['input']>;
  /** The URL that was previewed. */
  url: Scalars['String']['input'];
};

/** Input for marking a room as read. */
export type MarkRoomAsReadInput = {
  /** The ID of the room to mark as read. */
  roomId: Scalars['ID']['input'];
  /**
   * Optional event ID to mark as the read cursor. If provided, the marker is
   * set to this event (advance-only — never regresses past a more recent
   * marker). If omitted, the server uses the room's current latest event.
   */
  upToEventId?: InputMaybe<Scalars['ID']['input']>;
};

/** Result of marking a room as read. */
export type MarkRoomAsReadResult = {
  __typename?: 'MarkRoomAsReadResult';
  /** The timestamp of the last-read event (null if no messages in room). */
  lastReadAt?: Maybe<Scalars['Time']['output']>;
  /** The timestamp of the previously-read event (null if first time reading this room). */
  previousLastReadAt?: Maybe<Scalars['Time']['output']>;
};

/** Input for marking a thread as read. */
export type MarkThreadAsReadInput = {
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['input'];
  /** The event ID of the thread root message. */
  threadRootEventId: Scalars['ID']['input'];
  /**
   * Optional event ID (root or reply) to anchor the read cursor at. If
   * provided, the server records that event's timestamp (advance-only). If
   * omitted, the server records the current wall-clock time.
   */
  upToEventId?: InputMaybe<Scalars['ID']['input']>;
};

/** Result of marking a thread as read. */
export type MarkThreadAsReadResult = {
  __typename?: 'MarkThreadAsReadResult';
  /** The timestamp when the thread was previously read (null if never read before). */
  previousReadAt?: Maybe<Scalars['Time']['output']>;
};

/**
 * Notification: A user was mentioned in a message.
 * This is a live-only notification event for toast displays.
 * Persistent pending-attention state is represented separately by
 * NotificationCreatedEvent and the user's notification records.
 */
export type MentionNotificationEvent = {
  __typename?: 'MentionNotificationEvent';
  /** The user who mentioned you. */
  actor?: Maybe<User>;
  /** The room where the mention occurred (for display). */
  room: Room;
  /** The ID of the room where the mention occurred. */
  roomId: Scalars['ID']['output'];
};

/**
 * Notification for @mentions.
 * Created when someone mentions you in a message.
 */
export type MentionNotificationItem = {
  __typename?: 'MentionNotificationItem';
  /** User who triggered the notification */
  actor?: Maybe<User>;
  /** When the notification was created */
  createdAt: Scalars['Time']['output'];
  /** Event ID of the message containing the mention */
  eventId: Scalars['ID']['output'];
  /** Unique notification ID */
  id: Scalars['ID']['output'];
  /** Room where the mention occurred */
  room: Room;
  /** Human-readable summary for display */
  summary: Scalars['String']['output'];
  /** Thread root event ID if the mention is on a message inside a thread. Null for room-level messages. */
  threadRootEventId?: Maybe<Scalars['ID']['output']>;
};

/**
 * Legacy event: the mention indicator for a room was cleared for the current user.
 * Retained for wire compatibility; new builds derive orange dots from pending
 * notifications and do not publish this event.
 */
export type MentionStatusClearedEvent = {
  __typename?: 'MentionStatusClearedEvent';
  /** The ID of the room whose mention indicator was cleared. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: A message was edited.
 * Carries the updated message body inline so subscription clients can update
 * without refetching the affected message.
 */
export type MessageEditedEvent = {
  __typename?: 'MessageEditedEvent';
  /** Attachments after the edit. */
  attachments: Array<Attachment>;
  /** The decrypted message body, or null if the author was crypto-shredded. */
  body?: Maybe<Scalars['String']['output']>;
  /** Link preview after the edit. */
  linkPreview?: Maybe<LinkPreview>;
  /** The event ID of the message that was edited. */
  messageEventId: Scalars['ID']['output'];
  /** The ID of the room where the message was edited. */
  roomId: Scalars['ID']['output'];
  /** When the message was edited. */
  updatedAt?: Maybe<Scalars['Time']['output']>;
};

/** Event: A message was posted */
export type MessagePostedEvent = {
  __typename?: 'MessagePostedEvent';
  /** Attachments for this message. */
  attachments: Array<Attachment>;
  /** The message content. Null if deleted. */
  body?: Maybe<Scalars['String']['output']>;
  /** Event ID of the visible channel echo for this thread reply, if one exists. */
  channelEchoEventId?: Maybe<Scalars['ID']['output']>;
  /** The thread this echo originates from (null for non-echo messages). */
  echoFromThreadRootEventId?: Maybe<Scalars['ID']['output']>;
  /** Event ID of the original thread reply this echoes (null for non-echo messages). */
  echoOfEventId?: Maybe<Scalars['ID']['output']>;
  /** Event ID of the message this is replying to (null for top-level messages). */
  inReplyTo?: Maybe<Scalars['ID']['output']>;
  /** Timestamp of the most recent reply (null if no replies or not a root message). */
  lastReplyAt?: Maybe<Scalars['Time']['output']>;
  /** Link preview for the first URL in the message body. */
  linkPreview?: Maybe<LinkPreview>;
  /** Emoji reaction summaries on this message, aggregated by emoji. */
  reactions: Array<ReactionSummary>;
  /** Number of replies in this thread (0 for non-root messages or messages without replies). */
  replyCount: Scalars['Int']['output'];
  /** The ID of the room where the message was posted. */
  roomId: Scalars['ID']['output'];
  /** Users who have replied in this thread (empty for non-root messages or messages without replies). Returns up to `first` participants (default 5, max 10) for preview. */
  threadParticipants: Array<User>;
  /**
   * Replies to this message, when it is a thread root. Returns an empty page when
   * this message is itself a thread reply. Replies are returned in chronological
   * order and do not include this root event. Uses the same opaque cursor shape as
   * `Room.events`.
   */
  threadReplies: RoomEventsConnection;
  /**
   * Replies to this message centered around a reply event ID. Returns an empty
   * page when this message is itself a thread reply. The root event is not
   * included.
   */
  threadRepliesAround: RoomEventsConnection;
  /** Event ID of the thread root message (null for top-level messages). For direct replies, equals inReplyTo. For nested replies, references the original root. */
  threadRootEventId?: Maybe<Scalars['ID']['output']>;
  /** When the message was last updated (null if never edited). Lazy-loaded from body. */
  updatedAt?: Maybe<Scalars['Time']['output']>;
  /** Whether the current viewer is following this thread. Null for non-root messages or messages without replies. */
  viewerIsFollowingThread?: Maybe<Scalars['Boolean']['output']>;
};


/** Event: A message was posted */
export type MessagePostedEventThreadParticipantsArgs = {
  first?: InputMaybe<Scalars['Int']['input']>;
};


/** Event: A message was posted */
export type MessagePostedEventThreadRepliesArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  before?: InputMaybe<Scalars['String']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
};


/** Event: A message was posted */
export type MessagePostedEventThreadRepliesAroundArgs = {
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};

/** Event: A message was retracted. */
export type MessageRetractedEvent = {
  __typename?: 'MessageRetractedEvent';
  /** The event ID of the message that was retracted. */
  messageEventId: Scalars['ID']['output'];
  /** Optional human-readable retraction reason. */
  reason?: Maybe<Scalars['String']['output']>;
  /** The ID of the room where the message was retracted. */
  roomId: Scalars['ID']['output'];
};

/**
 * Input for moving a room into a different room group. Requires room.manage in
 * both the source and target room group.
 */
export type MoveRoomToGroupInput = {
  /** The destination room group. */
  groupId: Scalars['ID']['input'];
  /** The room to move. */
  roomId: Scalars['ID']['input'];
};

/** Root mutation type for modifying data. */
export type Mutation = {
  __typename?: 'Mutation';
  /**
   * Add an emoji reaction to a message.
   * The emoji parameter must be a shortcode name (e.g., "thumbsup", "heart").
   * Returns true if the reaction was added, false if it already existed.
   */
  addReaction: Scalars['Boolean']['output'];
  /** Admin mutations. Returns null unless the viewer is authenticated. Child fields enforce their own capabilities. */
  admin?: Maybe<AdminMutations>;
  /** Archive a room. Hides it from sidebar and Browse Rooms. Requires room.manage permission. */
  archiveRoom: Room;
  /**
   * Assign an server role to a user. Idempotent - assigning an already-assigned
   * role succeeds silently. Returns true on success.
   * Note: The 'everyone' role is implicit for all users and cannot be assigned.
   * Requires: role.assign permission.
   * Errors: If role doesn't exist or is 'everyone'.
   */
  assignRole: Scalars['Boolean']['output'];
  /**
   * Ban a target user from a channel room. Requires `room.ban-member` in the
   * room. DM rooms cannot be moderated this way. The reason is required for
   * moderation audit logs.
   */
  banRoomMember: Scalars['Boolean']['output'];
  /**
   * Clear both grant and denial for a permission on a room group, returning the
   * subject to neutral. Requires `role.manage`.
   */
  clearGroupPermissionState: Scalars['Boolean']['output'];
  /**
   * Clear any grant or denial state for a permission on a role, restoring neutral state.
   * Idempotent - clearing when no state exists succeeds silently. Returns true on success.
   * After clearing, this role neither grants nor denies the permission.
   * Requires: role.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  clearPermissionState: Scalars['Boolean']['output'];
  /**
   * Clear room-level grant and denial for a permission on a role.
   * Returns the permission to neutral (inherit from server defaults).
   * Requires: role.manage permission.
   */
  clearRoomPermission: Scalars['Boolean']['output'];
  /**
   * Clear both grant and denial of a permission on a user, restoring
   * normal role-based resolution. Idempotent.
   *
   * Authorization and roomId semantics mirror grantUserPermission.
   */
  clearUserPermissionState: Scalars['Boolean']['output'];
  /**
   * Create a new custom server role. Returns the created role with empty permissions.
   * System role names ('owner', 'admin', 'moderator', 'everyone') cannot be used.
   * Requires: role.manage permission.
   * Errors: If role name already exists or is a system role name.
   */
  createRole: Role;
  /** Create a new room. */
  createRoom: Room;
  /** Create a new room group. Requires `role.manage`. */
  createRoomGroup: RoomGroup;
  /**
   * Delete an attachment from a message. Only the message author can delete their attachments.
   * Removes the attachment from the message.
   * Returns true on success.
   */
  deleteAttachment: Scalars['Boolean']['output'];
  /**
   * Delete a user's avatar. Authorization: caller is self, OR caller
   * holds `role.assign`. Returns the updated user.
   */
  deleteAvatar: User;
  /**
   * Delete a link preview from a message. Only the message author can delete their link previews.
   * Returns true on success.
   */
  deleteLinkPreview: Scalars['Boolean']['output'];
  /**
   * Delete a message body for GDPR compliance.
   * The message remains as a retracted/deleted entry, but the content is removed.
   * Requires message.manage to delete another user's message; authors can delete
   * their own messages.
   * Returns true on success.
   */
  deleteMessage: Scalars['Boolean']['output'];
  /**
   * Permanently delete the current user's account.
   * This is a GDPR-compliant deletion that:
   * - Removes the user from the server and all rooms
   * - Crypto-shreds all message content (makes messages permanently unreadable)
   * - Deletes the user's profile, avatar, and associated data
   * Requires a confirmationToken obtained from requestAccountDeletion.
   * Returns true on success.
   */
  deleteMyAccount: Scalars['Boolean']['output'];
  /**
   * Delete a custom server role and all associated data. Returns true on success.
   * Deletes: role definition, all permission grants, and all user role assignments.
   * System roles ('owner', 'admin', 'moderator', 'everyone') cannot be deleted.
   * Requires: role.manage permission.
   * Errors: If role doesn't exist or is a system role.
   */
  deleteRole: Scalars['Boolean']['output'];
  /**
   * Delete a room group. Rejected if the room group still contains rooms —
   * operators must move all rooms out first. Requires `role.manage`.
   */
  deleteRoomGroup: Scalars['Boolean']['output'];
  /** Delete the server banner. Requires server.manage permission. */
  deleteServerBanner: Server;
  /** Delete the server logo. Requires server.manage permission. */
  deleteServerLogo: Server;
  /** Deny a permission on a room group (role or user subject). Requires `role.manage`. */
  denyGroupPermission: Scalars['Boolean']['output'];
  /**
   * Deny a permission for a role. Users with this role will be blocked from this
   * permission, regardless of what other roles grant it (deny-override pattern).
   * Clears any existing grant for the same permission. Returns true on success.
   * Note: Admin role is immune to role denials; denying a permission on admin has no effect.
   * Requires: role.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  denyPermission: Scalars['Boolean']['output'];
  /**
   * Deny a permission for a role at room level. Overrides server-level state for this room.
   * Clears any existing grant for the same permission in this room.
   * Requires: role.manage permission.
   */
  denyRoomPermission: Scalars['Boolean']['output'];
  /**
   * Deny a permission directly to a user. Any applicable deny wins over grants.
   * Useful for one-off moderation like suspending a user from posting
   * without revoking their roles.
   *
   * Authorization and roomId semantics mirror grantUserPermission.
   */
  denyUserPermission: Scalars['Boolean']['output'];
  /** Dismiss all notifications for the current user. Returns count of dismissed notifications. */
  dismissAllNotifications: Scalars['Int']['output'];
  /** Dismiss a single notification. Returns true if it existed and was dismissed. */
  dismissNotification: Scalars['Boolean']['output'];
  /** Follow a thread to receive notifications on new replies. Requires room membership. */
  followThread: Scalars['Boolean']['output'];
  /** Grant a permission on a room group (role or user subject). Requires `role.manage`. */
  grantGroupPermission: Scalars['Boolean']['output'];
  /**
   * Grant a permission to a role. Idempotent - granting an already-granted
   * permission succeeds silently. Returns true on success.
   * Requires: role.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  grantPermission: Scalars['Boolean']['output'];
  /**
   * Grant a permission to a role at room level. Overrides server-level state for this room.
   * Clears any existing denial for the same permission in this room.
   * Requires: role.manage permission.
   */
  grantRoomPermission: Scalars['Boolean']['output'];
  /**
   * Grant a permission directly to a user. Explicit denies still win over
   * grants, but user-level grants can add targeted privileges. Useful for
   * ad-hoc privileges like "let this one user moderate room X" without
   * inventing a custom role.
   *
   * Authorization: caller needs user.manage-permissions.
   *
   * Pass roomId to scope the grant to a specific room (room-scope perms
   * only). Omit roomId for a server-wide grant.
   */
  grantUserPermission: Scalars['Boolean']['output'];
  /**
   * Join every room in a group that the caller has `room.join` for and
   * hasn't already joined. Returns the IDs of the rooms that were newly
   * joined (already-joined and non-joinable rooms are silently skipped).
   * Powers the "Join all" affordance in the room directory.
   */
  joinGroup: Array<Scalars['ID']['output']>;
  /** Join the specified room. Returns the joined room. */
  joinRoom: Room;
  /** Join a room's voice call as the current user. */
  joinVoiceCall: Scalars['Boolean']['output'];
  /** Leave the specified room. */
  leaveRoom: Scalars['Boolean']['output'];
  /** Leave a room's voice call as the current user. */
  leaveVoiceCall: Scalars['Boolean']['output'];
  /**
   * Mark a room as read for the current user.
   * Stores the room's current last root message event ID as the user's read marker.
   * Returns the timestamps of the new and previous last-read events.
   */
  markRoomAsRead: MarkRoomAsReadResult;
  /**
   * Mark a thread as read by the current user.
   * Stores the current timestamp and returns the previous timestamp.
   * Used for showing unread separators in thread panes.
   */
  markThreadAsRead: MarkThreadAsReadResult;
  /**
   * Move a room into a different room group. The caller must have
   * `room.manage` in both the source room group and the target room group
   * because the move can change the room's inherited permissions. Permission
   * overrides on the room itself are preserved.
   */
  moveRoomToGroup: Room;
  /** Post a message to a room. Automatically marks the room as read since the user is viewing it. */
  postMessage: Event;
  /**
   * Remove an emoji reaction from a message.
   * The emoji parameter must be a shortcode name (e.g., "thumbsup", "heart").
   * Returns true if the reaction was removed, false if it didn't exist.
   */
  removeReaction: Scalars['Boolean']['output'];
  /**
   * Reorder server roles. Accepts an ordered list of custom role names.
   * System roles (owner, admin, moderator, everyone) maintain fixed positions and should not be included.
   * Positions are assigned based on array index (first role = position 1, second = 2, etc).
   * Requires: role.manage permission.
   * Returns: All server roles, sorted by position.
   */
  reorderRoles: Array<Role>;
  /**
   * Reorder all room groups. The provided ID list must contain every existing
   * room group exactly once. Requires `role.manage`.
   */
  reorderRoomGroups: Array<RoomGroup>;
  /**
   * Reorder rooms inside a single group. The provided ID list must contain
   * every current room in that group exactly once. Requires `role.manage`.
   */
  reorderRoomsInGroup: RoomGroup;
  /**
   * Request account deletion by generating a confirmation token.
   * The token is valid for 15 minutes and must be passed to deleteMyAccount.
   * This two-step process protects against XSS attacks.
   * Returns the confirmation token.
   */
  requestAccountDeletion: Scalars['String']['output'];
  /**
   * Revoke a permission grant from a role. Idempotent - revoking a non-granted
   * permission succeeds silently. Returns true on success.
   * Note: This only removes grants, not denials. Use clearPermissionState to remove both.
   * Note: Admin role has all permissions implicitly; revoking from admin has no effect.
   * Requires: role.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  revokePermission: Scalars['Boolean']['output'];
  /**
   * Revoke an server role from a user. Idempotent - revoking a non-assigned
   * role succeeds silently. Returns true on success.
   * Note: Users cannot revoke their own admin role (prevents self-lockout).
   * Note: The 'everyone' role is implicit and cannot be revoked.
   * Requires: role.assign permission.
   * Errors: If role doesn't exist, is 'everyone', or user tries to revoke own admin role.
   */
  revokeRole: Scalars['Boolean']['output'];
  /**
   * Send a typing indicator to other users in the room.
   * This is a live-only event (not stored). Clients should call this every ~2 seconds
   * while typing and implement 6-second timeout-based clearing.
   * Returns true on success.
   */
  sendTypingIndicator: Scalars['Boolean']['output'];
  /** Set the current user's notification level for a room. Pass DEFAULT to clear. */
  setRoomNotificationLevel: ViewerNotificationPreference;
  /** Set the current user's server-level notification level. Pass DEFAULT to clear. */
  setServerNotificationLevel: ViewerNotificationPreference;
  /**
   * Start a DM conversation with the given participants.
   * If a conversation already exists with exactly these participants, returns the existing one.
   * The current user is automatically included as a participant.
   */
  startDM: Room;
  /**
   * Subscribe to Web Push notifications.
   * Creates or updates a push subscription for the current user.
   * Returns true if successful.
   * Requires authentication.
   */
  subscribeToPush: Scalars['Boolean']['output'];
  /** Unarchive a previously archived room. Requires room.manage permission. */
  unarchiveRoom: Room;
  /**
   * Unban a user from a channel room. Requires `room.ban-member` in the room.
   * The reason is required for moderation audit logs.
   */
  unbanRoomMember: Scalars['Boolean']['output'];
  /** Unfollow a thread to stop receiving reply notifications. Requires room membership. */
  unfollowThread: Scalars['Boolean']['output'];
  /**
   * Unsubscribe from Web Push notifications.
   * Removes the subscription with the given endpoint for the current user.
   * Returns true if a subscription was removed, false if it didn't exist.
   * Requires authentication.
   */
  unsubscribeFromPush: Scalars['Boolean']['output'];
  /**
   * Update a message body. Only the message author can update their own messages,
   * within 3 hours of posting. The edit window may be configurable in the future.
   * Returns true on success.
   */
  updateMessage: Scalars['Boolean']['output'];
  /**
   * Update the current user's presence status.
   * Status applies to the current user on this server.
   * OFFLINE is not a valid input — to go offline, simply disconnect.
   */
  updateMyPresence: Scalars['Boolean']['output'];
  /**
   * Update a user's profile. Supports updating display name and/or login.
   * At least one field must be provided.
   * Login changes are subject to a 30-day cooldown (admins can use
   * `admin.updateUser` / `admin.clearUsernameCooldown` to bypass).
   * Authorization: caller is self, OR caller holds `role.assign`.
   * Returns the updated user.
   */
  updateProfile: User;
  /**
   * Update an server role's display name and description. Returns the updated role.
   * Role name cannot be changed after creation. System roles cannot be edited.
   * Requires: role.manage permission.
   * Errors: If role doesn't exist.
   */
  updateRole: Role;
  /** Update an existing room's name and description. Requires room.manage permission. */
  updateRoom: Room;
  /** Update a room group's name/description. Requires `role.manage`. */
  updateRoomGroup: RoomGroup;
  /** Update runtime-editable server configuration. Requires `server.manage`. */
  updateServerConfig: ServerProfile;
  /**
   * Update a user's display settings. Authorization: caller is self, OR
   * caller holds `role.assign`. Returns the updated settings.
   */
  updateSettings: UserSettings;
  /**
   * Upload an avatar for a user. Image will be resized to 256x256 max
   * and converted to WebP. Authorization: caller is self, OR caller
   * holds `role.assign`. Returns the updated user.
   */
  uploadAvatar: User;
  /** Upload a banner for the server. Requires server.manage permission. */
  uploadServerBanner: Server;
  /** Upload a logo for the server. Requires server.manage permission. */
  uploadServerLogo: Server;
};


/** Root mutation type for modifying data. */
export type MutationAddReactionArgs = {
  input: AddReactionInput;
};


/** Root mutation type for modifying data. */
export type MutationArchiveRoomArgs = {
  input: ArchiveRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationAssignRoleArgs = {
  input: AssignRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationBanRoomMemberArgs = {
  input: BanRoomMemberInput;
};


/** Root mutation type for modifying data. */
export type MutationClearGroupPermissionStateArgs = {
  input: GroupPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationClearPermissionStateArgs = {
  input: ClearPermissionStateInput;
};


/** Root mutation type for modifying data. */
export type MutationClearRoomPermissionArgs = {
  input: ClearRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationClearUserPermissionStateArgs = {
  input: ClearUserPermissionStateInput;
};


/** Root mutation type for modifying data. */
export type MutationCreateRoleArgs = {
  input: CreateRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationCreateRoomArgs = {
  input: CreateRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationCreateRoomGroupArgs = {
  input: CreateRoomGroupInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteAttachmentArgs = {
  input: DeleteAttachmentInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteAvatarArgs = {
  input: DeleteAvatarInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteLinkPreviewArgs = {
  input: DeleteLinkPreviewInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteMessageArgs = {
  input: DeleteMessageInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteMyAccountArgs = {
  input: DeleteMyAccountInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteRoleArgs = {
  input: DeleteRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteRoomGroupArgs = {
  input: DeleteRoomGroupInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyGroupPermissionArgs = {
  input: GroupPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyPermissionArgs = {
  input: DenyPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyRoomPermissionArgs = {
  input: DenyRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyUserPermissionArgs = {
  input: DenyUserPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDismissNotificationArgs = {
  input: DismissNotificationInput;
};


/** Root mutation type for modifying data. */
export type MutationFollowThreadArgs = {
  input: FollowThreadInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantGroupPermissionArgs = {
  input: GroupPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantPermissionArgs = {
  input: GrantPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantRoomPermissionArgs = {
  input: GrantRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantUserPermissionArgs = {
  input: GrantUserPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationJoinGroupArgs = {
  input: JoinGroupInput;
};


/** Root mutation type for modifying data. */
export type MutationJoinRoomArgs = {
  input: JoinRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationJoinVoiceCallArgs = {
  input: VoiceCallIntentInput;
};


/** Root mutation type for modifying data. */
export type MutationLeaveRoomArgs = {
  input: LeaveRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationLeaveVoiceCallArgs = {
  input: VoiceCallIntentInput;
};


/** Root mutation type for modifying data. */
export type MutationMarkRoomAsReadArgs = {
  input: MarkRoomAsReadInput;
};


/** Root mutation type for modifying data. */
export type MutationMarkThreadAsReadArgs = {
  input: MarkThreadAsReadInput;
};


/** Root mutation type for modifying data. */
export type MutationMoveRoomToGroupArgs = {
  input: MoveRoomToGroupInput;
};


/** Root mutation type for modifying data. */
export type MutationPostMessageArgs = {
  input: PostMessageInput;
};


/** Root mutation type for modifying data. */
export type MutationRemoveReactionArgs = {
  input: RemoveReactionInput;
};


/** Root mutation type for modifying data. */
export type MutationReorderRolesArgs = {
  input: ReorderRolesInput;
};


/** Root mutation type for modifying data. */
export type MutationReorderRoomGroupsArgs = {
  input: ReorderRoomGroupsInput;
};


/** Root mutation type for modifying data. */
export type MutationReorderRoomsInGroupArgs = {
  input: ReorderRoomsInGroupInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokePermissionArgs = {
  input: RevokePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokeRoleArgs = {
  input: RevokeRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationSendTypingIndicatorArgs = {
  input: SendTypingIndicatorInput;
};


/** Root mutation type for modifying data. */
export type MutationSetRoomNotificationLevelArgs = {
  input: SetRoomNotificationLevelInput;
};


/** Root mutation type for modifying data. */
export type MutationSetServerNotificationLevelArgs = {
  input: SetServerNotificationLevelInput;
};


/** Root mutation type for modifying data. */
export type MutationStartDmArgs = {
  input: StartDmInput;
};


/** Root mutation type for modifying data. */
export type MutationSubscribeToPushArgs = {
  input: PushSubscriptionInput;
};


/** Root mutation type for modifying data. */
export type MutationUnarchiveRoomArgs = {
  input: UnarchiveRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationUnbanRoomMemberArgs = {
  input: UnbanRoomMemberInput;
};


/** Root mutation type for modifying data. */
export type MutationUnfollowThreadArgs = {
  input: UnfollowThreadInput;
};


/** Root mutation type for modifying data. */
export type MutationUnsubscribeFromPushArgs = {
  input: UnsubscribeFromPushInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateMessageArgs = {
  input: UpdateMessageInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateMyPresenceArgs = {
  input: UpdateMyPresenceInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateProfileArgs = {
  input: UpdateProfileInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateRoleArgs = {
  input: UpdateRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateRoomArgs = {
  input: UpdateRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateRoomGroupArgs = {
  input: UpdateRoomGroupInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateServerConfigArgs = {
  input: UpdateServerConfigInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateSettingsArgs = {
  input: UpdateSettingsInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadAvatarArgs = {
  input: UploadAvatarInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadServerBannerArgs = {
  input: UploadServerBannerInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadServerLogoArgs = {
  input: UploadServerLogoInput;
};

/** Diagnostic state for one storage consumer. Raw consumer names and subjects are operator-facing diagnostics, not product concepts. */
export type NatsConsumerInfo = {
  __typename?: 'NatsConsumerInfo';
  /** Ack floor consumer sequence. */
  ackFloorConsumerSequence: Scalars['String']['output'];
  /** Ack floor stream sequence. */
  ackFloorStreamSequence: Scalars['String']['output'];
  /** Delivered messages awaiting acknowledgement. */
  ackPending: Scalars['Int']['output'];
  /** Ack policy, e.g. Explicit, All, or None. */
  ackPolicy: Scalars['String']['output'];
  /** Most recently delivered consumer sequence. */
  deliveredConsumerSequence: Scalars['String']['output'];
  /** Most recently delivered stream sequence. */
  deliveredStreamSequence: Scalars['String']['output'];
  /** Durable name, empty for ephemeral consumers. */
  durable: Scalars['String']['output'];
  /** Single filter subject, if configured. */
  filterSubject: Scalars['String']['output'];
  /** Multiple filter subjects, if configured. */
  filterSubjects: Array<Scalars['String']['output']>;
  /** Consumer name. */
  name: Scalars['String']['output'];
  /** Messages matching the consumer that have not yet been delivered. */
  pending: Scalars['Int64']['output'];
  /** True for pull consumers; false for push consumers. */
  pullBased: Scalars['Boolean']['output'];
  /** Whether a push consumer currently has an active subscription. */
  pushBound: Scalars['Boolean']['output'];
  /** Messages redelivered and still unacknowledged. */
  redelivered: Scalars['Int']['output'];
  /** Stream this consumer belongs to. */
  stream: Scalars['String']['output'];
  /** Active pull requests waiting for messages. */
  waiting: Scalars['Int']['output'];
};

/** Current stream and consumer diagnostics. Values are point-in-time and may change between refreshes. */
export type NatsStats = {
  __typename?: 'NatsStats';
  /** Consumers across all streams. */
  consumers: Array<NatsConsumerInfo>;
  /** Streams in the JetStream account. */
  streams: Array<NatsStreamInfo>;
  /** Total delivered-but-unacknowledged messages across listed consumers. */
  totalAckPending: Scalars['Int']['output'];
  /** Total retained bytes across listed streams. */
  totalBytes: Scalars['Int64']['output'];
  /** Total consumer backlog across listed consumers. */
  totalConsumerPending: Scalars['Int64']['output'];
  /** Total retained messages across listed streams. */
  totalMessages: Scalars['Int64']['output'];
};

/** Diagnostic state for one retained storage stream. Raw names and subjects are operator-facing diagnostics, not product concepts. */
export type NatsStreamInfo = {
  __typename?: 'NatsStreamInfo';
  /** Bytes currently retained. */
  bytes: Scalars['Int64']['output'];
  /** Cluster leader when running clustered JetStream, otherwise empty. */
  clusterLeader: Scalars['String']['output'];
  /** Consumers currently attached to this stream. */
  consumerCount: Scalars['Int']['output'];
  /** Optional stream description. */
  description: Scalars['String']['output'];
  /** First retained stream sequence. */
  firstSequence: Scalars['String']['output'];
  /** Last stream sequence. */
  lastSequence: Scalars['String']['output'];
  /** Messages currently retained. */
  messages: Scalars['Int64']['output'];
  /** Stream name. */
  name: Scalars['String']['output'];
  /** Configured replica count. */
  replicas: Scalars['Int']['output'];
  /** Storage backend, e.g. File or Memory. */
  storage: Scalars['String']['output'];
  /** Configured subject filters. */
  subjects: Array<Scalars['String']['output']>;
};

/**
 * Notification: A new message was posted in a DM conversation.
 * Published to all participants except the sender.
 */
export type NewDirectMessageNotificationEvent = {
  __typename?: 'NewDirectMessageNotificationEvent';
  /** The name of the conversation (derived from participants). */
  conversationName: Scalars['String']['output'];
  /** The ID of the DM conversation. */
  roomId: Scalars['ID']['output'];
  /** The user who sent the message. */
  sender?: Maybe<User>;
};

/**
 * Event published when a new notification is created.
 * Allows connected clients to update their notification list in real-time.
 */
export type NotificationCreatedEvent = {
  __typename?: 'NotificationCreatedEvent';
  /** Event ID for navigation, when available. */
  eventId?: Maybe<Scalars['ID']['output']>;
  /** Event ID of message being replied to (for reply notifications) */
  inReplyToId?: Maybe<Scalars['ID']['output']>;
  /** The notification ID */
  notificationId: Scalars['ID']['output'];
  /** Room ID for navigation */
  roomId: Scalars['ID']['output'];
};

/**
 * Event published when a notification is dismissed.
 * Allows other connected clients/devices to update their UI.
 */
export type NotificationDismissedEvent = {
  __typename?: 'NotificationDismissedEvent';
  /** The notification ID that was dismissed */
  notificationId: Scalars['ID']['output'];
};

/**
 * Union of all notification types.
 * Clients should check __typename to determine the notification type.
 */
export type NotificationItem = DmMessageNotificationItem | MentionNotificationItem | ReplyNotificationItem | RoomMessageNotificationItem;

/** Controls how a user receives notifications for the server or a room. */
export enum NotificationLevel {
  /** Like NORMAL, plus a notification for every new root message. */
  AllMessages = 'ALL_MESSAGES',
  /** Use inherited default (server-level default for rooms, NORMAL for the server). */
  Default = 'DEFAULT',
  /** Suppress all notifications and unread markers. */
  Muted = 'MUTED',
  /** Standard behavior: unread markers + notifications for mentions/DMs/threads. */
  Normal = 'NORMAL'
}

/**
 * Event: The user's notification level for the server or a room was changed.
 * Published to the user for multi-tab/multi-device sync.
 */
export type NotificationLevelChangedEvent = {
  __typename?: 'NotificationLevelChangedEvent';
  /** The effective level after inheritance. */
  effectiveLevel: NotificationLevel;
  /** The new notification level. */
  level: NotificationLevel;
  /** The room ID (null for server-level changes). */
  roomId?: Maybe<Scalars['ID']['output']>;
};

/** Paginated list of notifications with metadata. */
export type NotificationsConnection = {
  __typename?: 'NotificationsConnection';
  /** Whether there are more notifications beyond this page. */
  hasMore: Scalars['Boolean']['output'];
  /** The notifications in this page, newest first. */
  items: Array<NotificationItem>;
  /** Total count of notifications before pagination. */
  totalCount: Scalars['Int']['output'];
};

/** The kind of decision a role contributed at a given level. */
export enum PermissionDecisionKind {
  /** The role explicitly grants the permission. */
  Allow = 'ALLOW',
  /** The role explicitly denies the permission. */
  Deny = 'DENY',
  /** Used only for overall State; the resolver found no allow or deny anywhere. */
  None = 'NONE'
}

/**
 * The complete explanation for one permission for one user at one scope.
 * Mirrors the algorithm of the permission resolver: the first trace entry
 * is the winning decision; subsequent entries are also-saw context.
 */
export type PermissionExplanation = {
  __typename?: 'PermissionExplanation';
  /** The level of the winning decision; null if state is none. */
  decidedAt?: Maybe<PermissionLevel>;
  /** The role that produced the winning decision; null if state is none. */
  decidedByRole?: Maybe<Scalars['String']['output']>;
  /** The permission identifier (e.g., 'message.post'). */
  permission: Scalars['String']['output'];
  /** Overall outcome (allow, deny, or none if no role had an explicit decision). */
  state: PermissionDecisionKind;
  /** Full ordered trace; the head is the winning decision. */
  trace: Array<PermissionTraceEntry>;
};

/** The level at which a permission decision was reached during resolution. */
export enum PermissionLevel {
  /** Decision came from a per-room-group override (objectId=groupId). */
  Group = 'GROUP',
  /** Decision came from a per-room override (objectId=roomId). */
  Room = 'ROOM',
  /** Decision came from a role acting at server scope (objectId='any'). */
  Server = 'SERVER'
}

/**
 * One cell of the user-permission matrix: the per-permission, per-scope
 * intersection.
 */
export type PermissionMatrixCell = {
  __typename?: 'PermissionMatrixCell';
  /**
   * The **effective** decision the resolver would emit at this scope for
   * this user-permission pair, after walking room → group → server with
   * user-level overrides applied first. Drives the cell's tint.
   */
  effective: PermissionMatrixDecision;
  /**
   * The **explicit user-level override** at this scope, or NONE if the user
   * has no override here. NONE cells display only the inherited effective
   * state; ALLOW / DENY cells display as a solid override.
   */
  override: PermissionMatrixDecision;
  /** Permission identifier (e.g. `message.post`). */
  permission: Scalars['String']['output'];
  /** Scope id (matches `PermissionMatrixScope.id`). */
  scopeId: Scalars['String']['output'];
};

/** Trinary decision used in the user-permission matrix. */
export enum PermissionMatrixDecision {
  /** The permission is explicitly granted. */
  Allow = 'ALLOW',
  /** The permission is explicitly denied. */
  Deny = 'DENY',
  /** No explicit grant or denial applies at this scope. */
  None = 'NONE'
}

/**
 * A user's permission state across every scope where it can be configured —
 * the data the User Permissions page renders as a matrix.
 *
 * Each cell answers two questions:
 * 1. What's the **effective** decision after the full resolver walk (this
 *    is what governs runtime behavior)?
 * 2. Does the user have an **explicit user-level override** at this scope
 *    (and which way)? Cells with an override render solid; cells driven
 *    only by inheritance render faded.
 */
export type PermissionMatrixScope = {
  __typename?: 'PermissionMatrixScope';
  /**
   * Stable identifier for this scope:
   *   - `server` for the server tier (no group/room context),
   *   - `group:{groupID}` for a room-group scope,
   *   - `room:{roomID}` for a per-room scope.
   * Clients use it as a column key.
   */
  id: Scalars['String']['output'];
  /**
   * Scope kind. The frontend uses this to lay out columns (server tier first,
   * groups expandable, rooms nested under their group).
   */
  kind: PermissionMatrixScopeKind;
  /** Human-readable label for the scope (group name, room name, or 'Server'). */
  label: Scalars['String']['output'];
  /**
   * For room scopes, the parent group's ID — so the UI can nest rooms under
   * their group column. Empty string for server / group scopes.
   */
  parentGroupId: Scalars['ID']['output'];
};

/** Where a PermissionMatrixScope sits in the resolution scope tree. */
export enum PermissionMatrixScopeKind {
  /** A room group's scope (channel-room permissions). */
  Group = 'GROUP',
  /** A specific room's scope. */
  Room = 'ROOM',
  /** Server tier — no room/group context. */
  Server = 'SERVER'
}

/**
 * A single step in the permission resolution trace.
 * Only explicit allow or deny entries are emitted; roles with no decision at the
 * level being checked are silent.
 */
export type PermissionTraceEntry = {
  __typename?: 'PermissionTraceEntry';
  /** Whether this entry is the winning decision (matches the trace head). */
  applied: Scalars['Boolean']['output'];
  /** Whether the role allowed or denied the permission at this level. */
  decision: PermissionDecisionKind;
  /** The level at which this decision was observed. */
  level: PermissionLevel;
  /** The role that produced this decision. */
  roleName: Scalars['String']['output'];
};

/** Input for posting a message to a room. */
export type PostMessageInput = {
  /** Also echo this thread reply to the main channel for visibility (requires message.echo permission). */
  alsoSendToChannel?: InputMaybe<Scalars['Boolean']['input']>;
  /** Optional file attachments (images, videos, etc.). */
  attachments?: InputMaybe<Array<Scalars['Upload']['input']>>;
  /** The message content. Optional if attachments are provided. */
  body?: InputMaybe<Scalars['String']['input']>;
  /** Event ID of the message this responds to (attribution only, does not affect routing or permissions). */
  inReplyTo?: InputMaybe<Scalars['ID']['input']>;
  /** Link preview data from the composer. Server stores this directly without fetching. */
  linkPreview?: InputMaybe<LinkPreviewInput>;
  /** Short-lived token returned after a large mention confirmation prompt. Authorizes sending this exact message even if the current recipient count drifts. */
  mentionConfirmationToken?: InputMaybe<Scalars['String']['input']>;
  /** The ID of the room to post to. */
  roomId: Scalars['ID']['input'];
  /** Event ID of the thread root message. Determines thread membership and controls permission check (`message.post-in-thread` vs `message.post`). */
  threadRootEventId?: InputMaybe<Scalars['ID']['input']>;
};

/**
 * Event: A user's presence status changed.
 * The user whose presence changed is identified by the parent Event's actorId/actor.
 * Presence is server-wide.
 */
export type PresenceChangedEvent = {
  __typename?: 'PresenceChangedEvent';
  /** The user's new presence status. */
  status: PresenceStatus;
};

/** User presence status on the server. */
export enum PresenceStatus {
  /** User is connected but idle or inactive. */
  Away = 'AWAY',
  /** User has enabled do-not-disturb mode. */
  DoNotDisturb = 'DO_NOT_DISTURB',
  /** User is not connected to any client. */
  Offline = 'OFFLINE',
  /** User is actively connected. */
  Online = 'ONLINE'
}

/**
 * Presence statuses clients may explicitly set. Going offline is represented by
 * disconnecting and waiting for presence TTL expiry, not by sending OFFLINE.
 */
export enum PresenceStatusInput {
  /** User is connected but idle or inactive. */
  Away = 'AWAY',
  /** User has enabled do-not-disturb mode. */
  DoNotDisturb = 'DO_NOT_DISTURB',
  /** User is actively connected. */
  Online = 'ONLINE'
}

/** One named diagnostic count/byte bucket for a projection. */
export type ProjectionMetric = {
  __typename?: 'ProjectionMetric';
  /** Estimated bytes associated with this metric. Zero when the metric is count-only. */
  bytes: Scalars['Int64']['output'];
  /** Diagnostic metric identifier, e.g. 'timeline_entries' or 'event_id_index'. Names may evolve with projection implementation. */
  name: Scalars['String']['output'];
  /** Count associated with this metric. */
  value: Scalars['Int64']['output'];
};

/** Point-in-time runtime state for one event-sourced projection. */
export type ProjectionState = {
  __typename?: 'ProjectionState';
  /** estimatedBytes divided by entryCount, or zero when entryCount is zero. */
  averageEntryBytes: Scalars['Int64']['output'];
  /** Primary projected entry count for this projection. */
  entryCount: Scalars['Int64']['output'];
  /** Estimated bytes held in memory by this projection. */
  estimatedBytes: Scalars['Int64']['output'];
  /** Whether this projection has stopped after a fatal decode or apply error. */
  failed: Scalars['Boolean']['output'];
  /** Failed event-log sequence, serialized as String. Zero when the projection has not failed. */
  failedSequence: Scalars['String']['output'];
  /** Operator-facing failure summary. Empty when the projection has not failed. */
  failure: Scalars['String']['output'];
  /** Stable machine-readable projection key, suitable for metric labels and automation. */
  key: Scalars['String']['output'];
  /** Unapplied matching events, computed as matchingStreamSequence - lastAppliedSequence. */
  lag: Scalars['Int64']['output'];
  /** Highest event-log sequence applied by this projection, serialized as String to avoid GraphQL Int overflow. */
  lastAppliedSequence: Scalars['String']['output'];
  /** Highest event-log sequence currently matching this projection's subject filters. */
  matchingStreamSequence: Scalars['String']['output'];
  /** Breakdown of the projection's current state. */
  metrics: Array<ProjectionMetric>;
  /** Human-readable projection name. */
  name: Scalars['String']['output'];
  /** Whether the projector run loop has started. */
  started: Scalars['Boolean']['output'];
  /** Highest sequence in the event log, regardless of whether this projection consumes it. */
  streamLastSequence: Scalars['String']['output'];
  /** Diagnostic storage subject filters consumed by this projection. */
  subjects: Array<Scalars['String']['output']>;
};

/**
 * Input for subscribing to Web Push notifications.
 * All fields come from the PushSubscription object returned by the browser's Push API.
 */
export type PushSubscriptionInput = {
  /** Authentication secret for message encryption (from PushSubscription.keys.auth) */
  auth: Scalars['String']['input'];
  /** The push service endpoint URL (from PushSubscription.endpoint) */
  endpoint: Scalars['String']['input'];
  /** The client's P-256 ECDH public key for message encryption (from PushSubscription.keys.p256dh) */
  p256dh: Scalars['String']['input'];
  /** Optional user agent string for device identification */
  userAgent?: InputMaybe<Scalars['String']['input']>;
};

/** Root query type for fetching data. */
export type Query = {
  __typename?: 'Query';
  /**
   * Get room IDs that currently have active voice calls.
   * Returns empty list if LiveKit is not configured.
   * Requires server membership.
   */
  activeCallRoomIds: Array<Scalars['ID']['output']>;
  /** Admin-console queries. Returns null unless the viewer is authenticated. Child fields enforce their own capabilities. */
  admin?: Maybe<AdminQueries>;
  /**
   * Fetch link preview metadata for a URL.
   * Returns null if the URL cannot be previewed.
   * Requires authentication.
   */
  linkPreview?: Maybe<LinkPreview>;
  /** Get a specific room by ID. */
  room?: Maybe<Room>;
  /** Get information about this Chatto server. No authentication required. */
  server: Server;
  /** Get a specific user by ID. Requires authentication. */
  user?: Maybe<User>;
  /** Get a specific user by login. Requires authentication. Returns null if not found. */
  userByLogin?: Maybe<User>;
  /** The current authenticated user's server-level permissions. Null if not authenticated. */
  viewer?: Maybe<Viewer>;
};


/** Root query type for fetching data. */
export type QueryLinkPreviewArgs = {
  url: Scalars['String']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomArgs = {
  roomId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryUserArgs = {
  userId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryUserByLoginArgs = {
  login: Scalars['String']['input'];
};

/**
 * RBAC tooling namespace for role, permission, and permission inspection screens.
 * Individual fields enforce their own finer-grained authorization gates, such as
 * `role.manage` or `room.manage`.
 */
export type RbacQueries = {
  __typename?: 'RbacQueries';
  /**
   * Explain every applicable permission for a user at the given scope.
   * Authorization: admin/tooling-only, with no self-inspection path.
   */
  permissionExplanation: Array<PermissionExplanation>;
  /**
   * Permission matrix for a specific role. Authorization: viewer must hold
   * `role.manage` at server scope.
   */
  rolePermissionMatrix?: Maybe<RolePermissionMatrix>;
  /**
   * Return the full role-permission matrix at a tier: every applicable role
   * with its override and inherited baseline.
   *
   * Pass `roomId` for per-room override editing, `groupId` for room-group-scope
   * editing, or neither for server-scope editing. Passing both is rejected.
   */
  rolePermissionTierMatrix?: Maybe<TierRoles>;
  /**
   * Permission matrix for a specific user. Authorization mirrors user-level
   * permission mutations: viewer must hold `user.manage-permissions`.
   */
  userPermissionMatrix?: Maybe<UserPermissionMatrix>;
};


/**
 * RBAC tooling namespace for role, permission, and permission inspection screens.
 * Individual fields enforce their own finer-grained authorization gates, such as
 * `role.manage` or `room.manage`.
 */
export type RbacQueriesPermissionExplanationArgs = {
  roomId?: InputMaybe<Scalars['ID']['input']>;
  userId: Scalars['ID']['input'];
};


/**
 * RBAC tooling namespace for role, permission, and permission inspection screens.
 * Individual fields enforce their own finer-grained authorization gates, such as
 * `role.manage` or `room.manage`.
 */
export type RbacQueriesRolePermissionMatrixArgs = {
  roleName: Scalars['String']['input'];
};


/**
 * RBAC tooling namespace for role, permission, and permission inspection screens.
 * Individual fields enforce their own finer-grained authorization gates, such as
 * `role.manage` or `room.manage`.
 */
export type RbacQueriesRolePermissionTierMatrixArgs = {
  groupId?: InputMaybe<Scalars['ID']['input']>;
  roomId?: InputMaybe<Scalars['ID']['input']>;
};


/**
 * RBAC tooling namespace for role, permission, and permission inspection screens.
 * Individual fields enforce their own finer-grained authorization gates, such as
 * `role.manage` or `room.manage`.
 */
export type RbacQueriesUserPermissionMatrixArgs = {
  userId: Scalars['ID']['input'];
};

/** Event: A reaction was added to a message */
export type ReactionAddedEvent = {
  __typename?: 'ReactionAddedEvent';
  /** The emoji shortcode name (e.g., "thumbsup", "heart"). */
  emoji: Scalars['String']['output'];
  /** The event ID of the message that was reacted to. */
  messageEventId: Scalars['ID']['output'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['output'];
};

/** Event: A reaction was removed from a message */
export type ReactionRemovedEvent = {
  __typename?: 'ReactionRemovedEvent';
  /** The emoji shortcode name (e.g., "thumbsup", "heart"). */
  emoji: Scalars['String']['output'];
  /** The event ID of the message the reaction was removed from. */
  messageEventId: Scalars['ID']['output'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['output'];
};

/**
 * A reaction summary represents emoji responses to a message, aggregated by emoji type.
 * Emoji values are shortcode names (e.g., "thumbsup", "heart") — clients convert to Unicode for display.
 */
export type ReactionSummary = {
  __typename?: 'ReactionSummary';
  /** Total number of users who reacted with this emoji. */
  count: Scalars['Int']['output'];
  /** The emoji shortcode name (e.g., "thumbsup", "heart"). */
  emoji: Scalars['String']['output'];
  /** Whether the current user has reacted with this emoji. */
  hasReacted: Scalars['Boolean']['output'];
  /** Preview of users who reacted with this emoji (default 3, max 10). */
  users: Array<User>;
};


/**
 * A reaction summary represents emoji responses to a message, aggregated by emoji type.
 * Emoji values are shortcode names (e.g., "thumbsup", "heart") — clients convert to Unicode for display.
 */
export type ReactionSummaryUsersArgs = {
  first?: InputMaybe<Scalars['Int']['input']>;
};

/** Input for removing an emoji reaction from a message. */
export type RemoveReactionInput = {
  /** The emoji shortcode name (e.g., 'thumbsup', 'heart'). */
  emoji: Scalars['String']['input'];
  /** The event ID of the message to remove the reaction from. */
  messageEventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
};

/** Input for reordering server roles. */
export type ReorderRolesInput = {
  /** Ordered list of custom role names. System roles should not be included. */
  roleNames: Array<Scalars['String']['input']>;
};

/**
 * Input for reordering all room groups. The order must include every existing
 * room group ID exactly once; partial or unknown lists are rejected.
 */
export type ReorderRoomGroupsInput = {
  /** Room group IDs in the desired display order, first to last. */
  orderedIds: Array<Scalars['ID']['input']>;
};

/**
 * Input for reordering rooms inside a single group. The ID list must be a
 * permutation of the group's current rooms — partial or unknown lists are
 * rejected.
 */
export type ReorderRoomsInGroupInput = {
  /** The group whose room order is being rewritten. */
  groupId: Scalars['ID']['input'];
  /** Room IDs in the desired display order, first to last. */
  orderedRoomIds: Array<Scalars['ID']['input']>;
};

/**
 * Notification for replies to your messages.
 * Created when someone replies to one of your messages.
 */
export type ReplyNotificationItem = {
  __typename?: 'ReplyNotificationItem';
  /** User who triggered the notification */
  actor?: Maybe<User>;
  /** When the notification was created */
  createdAt: Scalars['Time']['output'];
  /** Event ID of the reply message */
  eventId: Scalars['ID']['output'];
  /** Unique notification ID */
  id: Scalars['ID']['output'];
  /** Event ID of your original message that was replied to */
  inReplyToId: Scalars['ID']['output'];
  /** Room where the reply occurred */
  room: Room;
  /** Human-readable summary for display */
  summary: Scalars['String']['output'];
  /** Thread root event ID if this is a thread reply. Null for room-level replies. */
  threadRootEventId?: Maybe<Scalars['ID']['output']>;
};

/** Input for revoking a permission from a role. */
export type RevokePermissionInput = {
  /** The permission identifier to revoke. */
  permission: Scalars['String']['input'];
  /** The role to revoke the permission from. */
  roleName: Scalars['String']['input'];
};

/** Input for revoking an server role from a user. */
export type RevokeRoleInput = {
  /** The name of the role to revoke. */
  roleName: Scalars['String']['input'];
  /** The ID of the user to revoke the role from. */
  userId: Scalars['ID']['input'];
};

/** A role with its granted and denied permissions. */
export type Role = {
  __typename?: 'Role';
  /** Role description. */
  description: Scalars['String']['output'];
  /** Human-readable name. */
  displayName: Scalars['String']['output'];
  /** Whether this is a system-defined role (cannot be deleted). */
  isSystem: Scalars['Boolean']['output'];
  /** Role identifier (e.g., 'admin', 'moderator'). */
  name: Scalars['String']['output'];
  /** List of permission identifiers denied by this role. Denials override grants from other roles. */
  permissionDenials: Array<Scalars['String']['output']>;
  /** List of permission identifiers granted (allowed) by this role. */
  permissions: Array<Scalars['String']['output']>;
  /** Whether @role pings notify users assigned to this role. */
  pingable: Scalars['Boolean']['output'];
  /** Display/order position. Owner=1000, admin=900, moderator=100, custom roles in 1..99, everyone=0. Not an authorization rank. */
  position: Scalars['Int']['output'];
};

/**
 * A role's permission state across every scope where it can be configured —
 * the data the Role Permissions page renders as a matrix.
 *
 * Each cell answers two questions:
 * 1. What's the role's **explicit override** at this scope (ALLOW / DENY /
 *    NONE)? Solid cells have an override; faded cells inherit from a
 *    broader scope.
 * 2. What's the **effective** decision the resolver would walk to for THIS
 *    role at this scope (room → group → server), considering only this
 *    role's own grants? Drives the faded baseline color.
 */
export type RolePermissionMatrix = {
  __typename?: 'RolePermissionMatrix';
  /**
   * Permissions to render as rows. Same identifiers used by the user
   * matrix, so the frontend can reuse its grouping / display-name
   * metadata.
   */
  applicablePermissions: Array<Scalars['String']['output']>;
  /**
   * One cell per (permission, scope) intersection. Sparse: a cell is
   * included iff the permission applies at that scope's tier.
   */
  cells: Array<PermissionMatrixCell>;
  /** The role this matrix describes. */
  roleName: Scalars['String']['output'];
  /**
   * Scopes to render as columns. Server scope first, then groups, then
   * rooms grouped under their parent group via `parentGroupId`.
   */
  scopes: Array<PermissionMatrixScope>;
};

/**
 * Room-level permission configuration for a single role.
 * Shows grants and denials that are specific to this room (not inherited from
 * the role's server-level state).
 */
export type RoleRoomPermissions = {
  __typename?: 'RoleRoomPermissions';
  /** Human-readable display name */
  displayName: Scalars['String']['output'];
  /** Whether this is a system-defined role */
  isSystem: Scalars['Boolean']['output'];
  /** Permissions denied at room level */
  permissionDenials: Array<Scalars['String']['output']>;
  /** Permissions granted at room level */
  permissions: Array<Scalars['String']['output']>;
  /** Display/order position (higher sorts before lower; not an authorization rank). */
  position: Scalars['Int']['output'];
  /** Role identifier */
  roleName: Scalars['String']['output'];
};

/** A Room is a chat channel on the server where users can exchange messages. */
export type Room = {
  __typename?: 'Room';
  /** Whether this room is archived. Archived rooms are hidden from sidebar and Browse Rooms. */
  archived: Scalars['Boolean']['output'];
  /** Permissions configurable at room scope. */
  availableRoomPermissions: Array<Scalars['String']['output']>;
  /** Participants currently in this room's voice call. Empty list if no call is active or LiveKit is not configured. */
  callParticipants: Array<CallParticipant>;
  /** Optional description of the room's purpose. */
  description?: Maybe<Scalars['String']['output']>;
  /** Fetch a single event in this room by event ID. Returns null if not found. */
  event?: Maybe<Event>;
  /**
   * Fetch historical events for this room (default limit: 50, max: 500;
   * larger values are silently clamped). Use the opaque `before` cursor
   * for backward pagination and `after` for forward pagination — pass the
   * `startCursor` / `endCursor` from a previous `RoomEventsConnection`
   * response. Cursors are opaque strings; clients must not attempt to
   * parse them.
   */
  events: RoomEventsConnection;
  /**
   * Fetch events in this room centered around a specific event (default
   * limit: 50, max: 500; larger values are silently clamped).
   * Returns a window of events with the target event roughly in the middle.
   * Used for "jump to message" when clicking reply links to messages not in the loaded range.
   */
  eventsAround: RoomEventsAroundResult;
  /**
   * Channel rooms belong to exactly one RoomGroup; this field identifies which
   * one. DM rooms return null because they do not participate in room-group
   * layout or ACLs.
   */
  groupId?: Maybe<Scalars['ID']['output']>;
  /**
   * Whether the room has unread messages for the current user.
   * Returns false if user is not a member or room has no new messages.
   */
  hasUnread: Scalars['Boolean']['output'];
  /** The room's unique ID. */
  id: Scalars['ID']['output'];
  /** List members in this room. */
  members: RoomMembersConnection;
  /** The room's name. Empty for DM rooms — clients derive the display name from `members`. */
  name: Scalars['String']['output'];
  /** Room-level permission overrides for all roles. */
  roomPermissionOverrides: Array<RoleRoomPermissions>;
  /** Kind of room — distinguishes regular channels from direct-message conversations. */
  type: RoomType;
  /** Whether the current user can ban members from this room. */
  viewerCanBanRoomMembers: Scalars['Boolean']['output'];
  /** Whether the current user can echo thread replies to the main channel. */
  viewerCanEchoMessage: Scalars['Boolean']['output'];
  /** Whether the current user can join this room (has room.join permission). */
  viewerCanJoinRoom: Scalars['Boolean']['output'];
  /**
   * Whether the current user can see this room in directories and other
   * surfaces that enumerate rooms (resolves `room.list` per room). Distinct
   * from `viewerCanJoinRoom`: a room may be listable without being directly
   * joinable, which is the state the directory uses to render a future
   * request-to-join affordance.
   */
  viewerCanListRoom: Scalars['Boolean']['output'];
  /**
   * Whether the current user can edit or delete other users' messages in
   * this room. Authors editing or deleting their own messages do not need
   * this permission.
   */
  viewerCanManageOthersMessage: Scalars['Boolean']['output'];
  /** Whether the current user can edit/configure this room (room.manage). */
  viewerCanManageRoom: Scalars['Boolean']['output'];
  /** Whether the current user can post messages in threads in this room. */
  viewerCanPostInThread: Scalars['Boolean']['output'];
  /** Whether the current user can post new root messages in this room. */
  viewerCanPostMessage: Scalars['Boolean']['output'];
  /** Whether the current user can add/remove reactions in this room. */
  viewerCanReact: Scalars['Boolean']['output'];
  /** The current user's notification preference for this room. */
  viewerNotificationPreference?: Maybe<ViewerNotificationPreference>;
  /**
   * Get a LiveKit join token for joining the voice call in this room.
   * Returns null if LiveKit is not configured on this server.
   */
  voiceCallToken?: Maybe<VoiceCallToken>;
};


/** A Room is a chat channel on the server where users can exchange messages. */
export type RoomEventArgs = {
  eventId: Scalars['ID']['input'];
};


/** A Room is a chat channel on the server where users can exchange messages. */
export type RoomEventsArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  before?: InputMaybe<Scalars['String']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
};


/** A Room is a chat channel on the server where users can exchange messages. */
export type RoomEventsAroundArgs = {
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


/** A Room is a chat channel on the server where users can exchange messages. */
export type RoomMembersArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};

/**
 * Event: A room was archived.
 * Archived rooms are hidden from sidebars and Browse Rooms.
 */
export type RoomArchivedEvent = {
  __typename?: 'RoomArchivedEvent';
  /** The ID of the archived room. */
  roomId: Scalars['ID']['output'];
};

/** An active room ban shown in server-admin moderation tools. */
export type RoomBan = {
  __typename?: 'RoomBan';
  /** When the ban was created. */
  createdAt: Scalars['Time']['output'];
  /** When this ban expires. Null means indefinite. */
  expiresAt?: Maybe<Scalars['Time']['output']>;
  /** The event ID that created the active ban. */
  id: Scalars['ID']['output'];
  /** The moderator who created the ban, if the account still exists. */
  moderator?: Maybe<User>;
  /** The moderator who created the ban. */
  moderatorId: Scalars['ID']['output'];
  /** Moderator-entered reason retained for audit. */
  reason: Scalars['String']['output'];
  /** The room this ban applies to, if it still exists. */
  room?: Maybe<Room>;
  /** The channel room this ban applies to. */
  roomId: Scalars['ID']['output'];
  /** The banned user, if the account still exists. */
  user?: Maybe<User>;
  /** The banned user. */
  userId: Scalars['ID']['output'];
};

/** Event: A room was created */
export type RoomCreatedEvent = {
  __typename?: 'RoomCreatedEvent';
  /** The room's description, or null if not set. */
  description?: Maybe<Scalars['String']['output']>;
  /** The room's name. */
  name: Scalars['String']['output'];
  /** The ID of the newly created room. */
  roomId: Scalars['ID']['output'];
};

/** Event: A room was deleted */
export type RoomDeletedEvent = {
  __typename?: 'RoomDeletedEvent';
  /** The ID of the deleted room. */
  roomId: Scalars['ID']['output'];
};

/**
 * Result of fetching events around a specific target event. `startCursor`
 * and `endCursor` are opaque pagination cursors usable on `Room.events`.
 */
export type RoomEventsAroundResult = {
  __typename?: 'RoomEventsAroundResult';
  /** Opaque cursor of the last event in this window (null if empty). */
  endCursor?: Maybe<Scalars['String']['output']>;
  /** The events in the window, in chronological order. */
  events: Array<Event>;
  /** Whether there are newer events after this window. */
  hasNewer: Scalars['Boolean']['output'];
  /** Whether there are older events before this window. */
  hasOlder: Scalars['Boolean']['output'];
  /** Opaque cursor of the first event in this window (null if empty). */
  startCursor?: Maybe<Scalars['String']['output']>;
  /** The index of the target event within the events array. */
  targetIndex: Scalars['Int']['output'];
};

/**
 * Paginated chronological events with metadata indicating whether more events
 * exist in either direction. `startCursor` and `endCursor` are opaque pagination
 * cursors — pass them as `before` / `after` on the same field that returned them.
 * Both are null when `events` is empty.
 */
export type RoomEventsConnection = {
  __typename?: 'RoomEventsConnection';
  /** Opaque cursor of the last event in this page (null if empty). */
  endCursor?: Maybe<Scalars['String']['output']>;
  /** The events in chronological order. */
  events: Array<Event>;
  /** Whether there are newer events after this page. */
  hasNewer: Scalars['Boolean']['output'];
  /** Whether there are older events before this page. */
  hasOlder: Scalars['Boolean']['output'];
  /** Opaque cursor of the first event in this page (null if empty). */
  startCursor?: Maybe<Scalars['String']['output']>;
};

/**
 * A RoomGroup is a named, ordered group of channel rooms. It also serves as
 * a permission container — each room group has its own ACL, with individual
 * rooms able to override on a per (role, permission) basis.
 */
export type RoomGroup = {
  __typename?: 'RoomGroup';
  /** Operator-facing description; may be empty. */
  description: Scalars['String']['output'];
  /** Unique ID for this room group. */
  id: Scalars['ID']['output'];
  /** Display name for this room group (e.g., 'General', 'Projects'). */
  name: Scalars['String']['output'];
  /** Ordered list of channel rooms in this room group. */
  rooms: Array<Room>;
};

/**
 * Per-room-group role permission inspector. Returns the explicit grants and
 * denials configured on a room group for a given role (no inheritance — to
 * see the effective permissions resolve per-room or per-user via the resolver
 * instead).
 */
export type RoomGroupRolePermissions = {
  __typename?: 'RoomGroupRolePermissions';
  /** The room group these permissions belong to. */
  groupId: Scalars['ID']['output'];
  /** Permissions explicitly denied to this role on this room group. */
  permissionDenials: Array<Scalars['String']['output']>;
  /** Permissions explicitly granted to this role on this room group. */
  permissions: Array<Scalars['String']['output']>;
  /** The role these permissions apply to. */
  roleName: Scalars['String']['output'];
};

/**
 * Per-room-group user permission inspector. Mirrors RoomGroupRolePermissions
 * for direct user-level grants/denials.
 */
export type RoomGroupUserPermissions = {
  __typename?: 'RoomGroupUserPermissions';
  /** The room group these permissions belong to. */
  groupId: Scalars['ID']['output'];
  /** Permissions explicitly denied to this user on this room group. */
  permissionDenials: Array<Scalars['String']['output']>;
  /** Permissions explicitly granted to this user on this room group. */
  permissions: Array<Scalars['String']['output']>;
  /** The user these permissions apply to. */
  userId: Scalars['ID']['output'];
};

/**
 * Event: The channel-room groups (ordering, names, or membership) were updated.
 * Clients should refetch `Server.roomGroups` to get the new shape.
 */
export type RoomGroupsUpdatedEvent = {
  __typename?: 'RoomGroupsUpdatedEvent';
  /** Always true. Vestigial — clients only need the event arrival to trigger a refetch of room groups. */
  changed: Scalars['Boolean']['output'];
};

/**
 * Event: A room was marked as read by the current user.
 * Published to the user when they mark a room as read (e.g., by entering it).
 * Enables real-time updates to unread indicators.
 */
export type RoomMarkedAsReadEvent = {
  __typename?: 'RoomMarkedAsReadEvent';
  /** The ID of the room that was marked as read. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: A user was banned from a room.
 * Reason and expiry are intentionally not exposed on the public event feed.
 */
export type RoomMemberBannedEvent = {
  __typename?: 'RoomMemberBannedEvent';
  /** The ID of the room the user was banned from. */
  roomId: Scalars['ID']['output'];
  /** The ID of the banned user. */
  userId: Scalars['ID']['output'];
};

/**
 * Event: A room ban was removed.
 * Reason is intentionally not exposed on the public event feed.
 */
export type RoomMemberUnbannedEvent = {
  __typename?: 'RoomMemberUnbannedEvent';
  /** The ID of the room the user was unbanned from. */
  roomId: Scalars['ID']['output'];
  /** The ID of the unbanned user. */
  userId: Scalars['ID']['output'];
};

/** Paginated list of room members with metadata. */
export type RoomMembersConnection = {
  __typename?: 'RoomMembersConnection';
  /** Whether there are more members beyond this page. */
  hasMore: Scalars['Boolean']['output'];
  /** Total count of members before pagination. */
  totalCount: Scalars['Int']['output'];
  /** The users who are members of this room. */
  users: Array<User>;
};

/**
 * Notification for a new message in a room (for users with ALL_MESSAGES level).
 * Created for every root message posted in a room the user is watching.
 */
export type RoomMessageNotificationItem = {
  __typename?: 'RoomMessageNotificationItem';
  /** User who posted the message. */
  actor?: Maybe<User>;
  /** When the notification was created. */
  createdAt: Scalars['Time']['output'];
  /** Event ID of the message. */
  eventId: Scalars['ID']['output'];
  /** Unique notification ID. */
  id: Scalars['ID']['output'];
  /** Room where the message was posted. */
  room: Room;
  /** Human-readable summary for display. */
  summary: Scalars['String']['output'];
};

/**
 * A user's notification preference for a specific room.
 * Used by the bulk roomNotificationPreferences query to return all preferences at once.
 */
export type RoomNotificationPreferenceItem = {
  __typename?: 'RoomNotificationPreferenceItem';
  /** The effective level after inheritance resolution (never DEFAULT). */
  effectiveLevel: NotificationLevel;
  /** The explicitly set level (DEFAULT if not explicitly configured). */
  level: NotificationLevel;
  /** The room this preference applies to. */
  roomId: Scalars['ID']['output'];
};

/**
 * The kind of room. Used to distinguish regular channels from direct-message
 * conversations, both of which can appear in a server's room list.
 */
export enum RoomType {
  /** A regular channel — has a name, optional layout placement, and is governed by the server's RBAC roles. */
  Channel = 'CHANNEL',
  /** A direct-message conversation — derives its display name from its participants and uses membership plus message permissions. */
  Dm = 'DM'
}

/**
 * Event: A room was unarchived.
 * The room becomes visible again in sidebars and Browse Rooms.
 */
export type RoomUnarchivedEvent = {
  __typename?: 'RoomUnarchivedEvent';
  /** The ID of the unarchived room. */
  roomId: Scalars['ID']['output'];
};

/** Event: A room was updated */
export type RoomUpdatedEvent = {
  __typename?: 'RoomUpdatedEvent';
  /** The room's updated description, or null if not set. */
  description?: Maybe<Scalars['String']['output']>;
  /** The room's updated name. */
  name: Scalars['String']['output'];
  /** The ID of the updated room. */
  roomId: Scalars['ID']['output'];
};

/** Input for sending a typing indicator. */
export type SendTypingIndicatorInput = {
  /** The ID of the room the user is typing in. */
  roomId: Scalars['ID']['input'];
  /** The event ID of the thread root message, if typing in a thread. */
  threadRootEventId?: InputMaybe<Scalars['ID']['input']>;
};

/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type Server = {
  __typename?: 'Server';
  /** Number of assets (attachments) uploaded to this server. */
  assetCount: Scalars['Int']['output'];
  /** External login providers enabled on this server. */
  authProviders: Array<AuthProvider>;
  /** List all available permission identifiers. */
  availablePermissions: Array<Scalars['String']['output']>;
  /** True if direct (email/password) registration is enabled on this server. */
  directRegistrationEnabled: Scalars['Boolean']['output'];
  /** LiveKit WebSocket URL for voice calls. Null if voice calls are disabled. */
  livekitUrl?: Maybe<Scalars['String']['output']>;
  /** Maximum upload size for regular attachments (images, files) in bytes. */
  maxUploadSize: Scalars['Int64']['output'];
  /** Maximum upload size for video attachments in bytes. Same as maxUploadSize when video processing is disabled. */
  maxVideoUploadSize: Scalars['Int64']['output'];
  /**
   * Get a single member of this server by user ID.
   * Returns null if the user is not a member.
   */
  member?: Maybe<User>;
  /** Number of members on this server. */
  memberCount: Scalars['Int']['output'];
  /**
   * List members of this server with optional search and pagination.
   * Search matches login and display name (case-insensitive partial match).
   */
  members: ServerMembersConnection;
  /** Duration in seconds after posting during which a user can edit their own message. Moderators with `message.manage` are not bound by this window. */
  messageEditWindowSeconds: Scalars['Int']['output'];
  /** Public-facing identity and branding for this server. */
  profile: ServerProfile;
  /** True if Web Push notifications are enabled on this server. */
  pushNotificationsEnabled: Scalars['Boolean']['output'];
  /** Get a single role by name. Returns null if not found. */
  role?: Maybe<Role>;
  /** Get users assigned to a specific role. */
  roleUsers: Array<User>;
  /** List all roles on this server. */
  roles: Array<Role>;
  /** Number of rooms on this server. */
  roomCount: Scalars['Int']['output'];
  /**
   * Ordered list of channel-room groups. Every server boots with at least the
   * seed "Lobby" group; the list is never empty for a configured server.
   */
  roomGroups: Array<RoomGroup>;
  /**
   * List of rooms on this server.
   *
   * When `type` is null or `CHANNEL`, the result includes regular channels. When
   * `type` is null or `DM`, the caller's direct-message conversations are merged
   * in through membership; the unified sidebar uses the null default to render
   * channels and DMs together. Pass `type: CHANNEL` for channels-only consumers
   * (e.g. the admin room-management UI); pass `type: DM` for DMs-only consumers.
   */
  rooms: Array<Room>;
  /**
   * Get a user's effective denied permissions at server scope. Mirrors
   * `userEffectivePermissions` but lists permissions whose first decision
   * is a deny.
   */
  userEffectiveDenials: Array<Scalars['String']['output']>;
  /**
   * Get a user's effective allowed permissions at server scope. Combines
   * role-based grants with user-level overrides (`grantUserPermission` /
   * `denyUserPermission`) — the same answer the authorization resolver
   * produces. For per-decision provenance use the permission explainer.
   */
  userEffectivePermissions: Array<Scalars['String']['output']>;
  /** VAPID public key for Web Push subscriptions. Null if push is disabled. */
  vapidPublicKey?: Maybe<Scalars['String']['output']>;
  /** The application version. */
  version: Scalars['String']['output'];
  /** True if video processing is enabled, allowing video attachments to be uploaded. */
  videoProcessingEnabled: Scalars['Boolean']['output'];
  /** Whether the current user can assign roles to users (has role.assign permission). */
  viewerCanAssignRoles: Scalars['Boolean']['output'];
  /** Whether the current user can create rooms (has room.create permission). */
  viewerCanCreateRoom: Scalars['Boolean']['output'];
  /** Whether the current user can manage roles (has role.manage permission). */
  viewerCanManageRoles: Scalars['Boolean']['output'];
  /** Whether the current user can manage rooms (has room.manage permission). */
  viewerCanManageRooms: Scalars['Boolean']['output'];
  /** Whether the current user can manage this server (has server.manage permission). */
  viewerCanManageServer: Scalars['Boolean']['output'];
  /** Whether the current user can administer the target user's profile. Self is allowed; other users require role.assign. */
  viewerCanManageUser: Scalars['Boolean']['output'];
  /** Whether the current user can edit direct per-user permission overrides (has user.manage-permissions permission). */
  viewerCanManageUserPermissions: Scalars['Boolean']['output'];
  /** Whether the current user has any admin.* permission (for showing the Admin link). */
  viewerHasAnyAdminPermission: Scalars['Boolean']['output'];
  /** Whether the current user has any unread messages in rooms they've joined. */
  viewerHasUnreadRooms: Scalars['Boolean']['output'];
  /** The current user's server-level notification preference. */
  viewerNotificationPreference?: Maybe<ViewerNotificationPreference>;
  /** Get the current user's permissions on this server. */
  viewerPermissions: Array<Scalars['String']['output']>;
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerMemberArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerMembersArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerRoleArgs = {
  name: Scalars['String']['input'];
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerRoleUsersArgs = {
  roleName: Scalars['String']['input'];
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerRoomsArgs = {
  type?: InputMaybe<RoomType>;
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerUserEffectiveDenialsArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerUserEffectivePermissionsArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerViewerCanManageUserArgs = {
  userId: Scalars['ID']['input'];
};

/**
 * Event: A server member's account was deleted.
 * Published to notify clients to update member lists and refresh messages
 * to show "Deleted User" and unavailable content.
 */
export type ServerMemberDeletedEvent = {
  __typename?: 'ServerMemberDeletedEvent';
  /** The ID of the deleted user. */
  userId: Scalars['ID']['output'];
};

/** Paginated list of server members with metadata. */
export type ServerMembersConnection = {
  __typename?: 'ServerMembersConnection';
  /** Whether there are more members beyond this page. */
  hasMore: Scalars['Boolean']['output'];
  /** Total count of members matching the search (before pagination). */
  totalCount: Scalars['Int']['output'];
  /** The users who are members of this server. */
  users: Array<User>;
};

/** How this server presents itself in logged-out and multi-server UI. */
export type ServerProfile = {
  __typename?: 'ServerProfile';
  /** URL to the server banner image, if set. */
  bannerUrl?: Maybe<Scalars['String']['output']>;
  /** Short description of this server, used for OG link-preview metadata and the welcome card. Null if not configured. */
  description?: Maybe<Scalars['String']['output']>;
  /** URL to the server logo, if set. */
  logoUrl?: Maybe<Scalars['String']['output']>;
  /** Message of the Day, displayed in the header bar. Null if not configured. */
  motd?: Maybe<Scalars['String']['output']>;
  /** Display name for this server. Defaults to 'Chatto'. */
  name: Scalars['String']['output'];
  /** Welcome message to display on the login screen (Markdown). Null if not configured. */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};

/** Aggregate counts for the deployment. Operator-facing only. */
export type ServerStats = {
  __typename?: 'ServerStats';
  /** Number of channel rooms. */
  channelRoomCount: Scalars['Int']['output'];
  /** Number of DM rooms. */
  dmRoomCount: Scalars['Int']['output'];
  /** Number of registered users. */
  userCount: Scalars['Int']['output'];
};

/**
 * Event: Public server profile/configuration changed.
 * Clients should refetch `Server.profile` and any authenticated server settings they display.
 */
export type ServerUpdatedEvent = {
  __typename?: 'ServerUpdatedEvent';
  /** The server's banner URL, or null if no banner is set. */
  bannerUrl?: Maybe<Scalars['String']['output']>;
  /** The server's description, or null if not set. */
  description?: Maybe<Scalars['String']['output']>;
  /** The server's logo URL, or null if no logo is set. */
  logoUrl?: Maybe<Scalars['String']['output']>;
  /** The server's updated name. */
  name: Scalars['String']['output'];
};

/**
 * Event: The current user's display preferences were updated.
 * Published to the user across all sessions for multi-tab sync.
 */
export type ServerUserPreferencesUpdatedEvent = {
  __typename?: 'ServerUserPreferencesUpdatedEvent';
  /** Time display format. */
  timeFormat: TimeFormat;
  /** IANA timezone name, or null to use the browser default. */
  timezone?: Maybe<Scalars['String']['output']>;
};

/**
 * Event: The user's session was terminated.
 * Published on logout or admin boot. The subscription closes after this event.
 */
export type SessionTerminatedEvent = {
  __typename?: 'SessionTerminatedEvent';
  /** Why the session was terminated (logout, admin_boot, account_deleted). */
  reason: Scalars['String']['output'];
};

/** Input for setting the notification level for a room. */
export type SetRoomNotificationLevelInput = {
  /** The notification level to set. */
  level: NotificationLevel;
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/** Input for setting the server-level notification level. */
export type SetServerNotificationLevelInput = {
  /** The notification level to set. */
  level: NotificationLevel;
};

/** Input for starting a DM conversation. */
export type StartDmInput = {
  /** The IDs of the users to start a conversation with. The current user is automatically included. */
  participantIds: Array<Scalars['ID']['input']>;
};

/** Root subscription type. */
export type Subscription = {
  __typename?: 'Subscription';
  /**
   * Subscribe to every event the current user is authorised to see on this
   * deployment.
   *
   * - **Room events** (messages, room lifecycle, typing indicators, reactions,
   *   video processing, voice calls) — delivered only for rooms the user is a
   *   member of. The membership set is tracked in real time; joining or
   *   leaving a room updates filtering immediately without reconnecting.
   * - **Server events** (config updates, profile updates, server lifecycle,
   *   notifications, thread-follow sync, server membership, room layout
   *   changes, session termination) — scoped per event type:
   *   - Config events: delivered to all authenticated users.
   *   - User profile updates: broadcast to authenticated users (profiles are
   *     public within the server).
   *   - Private user events (notification sync, preferences, session
   *     termination, server membership changes): delivered only to the target
   *     user. Powers cross-tab/cross-device sync.
   *   - Server membership events: delivered to all server members.
   *   - New-message-in-server events: additionally room-membership filtered.
   *
   * **Presence changes** are delivered for every authenticated user on the
   * deployment.
   *
   * **Side effects:**
   * - Subscribing sets the user's presence to ONLINE.
   * - Presence is refreshed every 30s (60s TTL); expires after the subscription
   *   closes.
   * - A SessionTerminatedEvent closes the subscription.
   */
  myEvents: Event;
};

/** Point-in-time operator diagnostics for this deployment. */
export type SystemInfo = {
  __typename?: 'SystemInfo';
  /** JetStream account limits and usage (aggregate totals). */
  account: AccountInfo;
  /** NATS connection status and server info. */
  connection: ConnectionInfo;
  /** Current JetStream stream and consumer diagnostics. */
  nats: NatsStats;
  /** Deployment-level counts surfaced in the admin dashboard. */
  stats: ServerStats;
};

/** Event: a thread was created for a root room message. */
export type ThreadCreatedEvent = {
  __typename?: 'ThreadCreatedEvent';
  /** The room that owns the thread. */
  roomId: Scalars['ID']['output'];
  /** The root message event ID that identifies the thread. */
  threadRootEventId: Scalars['ID']['output'];
};

/**
 * Event: The user's thread follow state changed (followed or unfollowed).
 * Published to the user for multi-tab/multi-device sync.
 */
export type ThreadFollowChangedEvent = {
  __typename?: 'ThreadFollowChangedEvent';
  /** Whether the user is now following the thread. */
  isFollowing: Scalars['Boolean']['output'];
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['output'];
  /** The root event ID of the thread. */
  threadRootEventId: Scalars['ID']['output'];
};

/**
 * A role's permission state at a single tier (server or room).
 * Returned as part of RBAC matrix results so callers can display explicit
 * allow/deny state for a tier.
 */
export type TierPermissions = {
  __typename?: 'TierPermissions';
  /** Permissions explicitly denied by this role at this tier. */
  permissionDenials: Array<Scalars['String']['output']>;
  /** Permissions explicitly granted by this role at this tier. */
  permissions: Array<Scalars['String']['output']>;
};

/**
 * A role's permission state at one tier, including the inherited baseline
 * from the tiers above (the resolved state if the override at this tier
 * were cleared). Used by the matrix UI to show inherited values faded
 * behind the explicit override.
 */
export type TierRole = {
  __typename?: 'TierRole';
  /** Role description. */
  description: Scalars['String']['output'];
  /** Human-readable display name. */
  displayName: Scalars['String']['output'];
  /**
   * Permissions allowed by inheritance from the tiers above this one. Empty
   * at server scope; at room scope it reflects the role's server-level state.
   */
  inheritedAllows: Array<Scalars['String']['output']>;
  /** Permissions denied by inheritance from the tiers above this one. */
  inheritedDenials: Array<Scalars['String']['output']>;
  /** Whether this is a system role and cannot be deleted. */
  isSystem: Scalars['Boolean']['output'];
  /**
   * Explicit allow/deny at the requested tier. Allow and deny lists may
   * both be empty for a role with no override at this tier.
   */
  override: TierPermissions;
  /** Display/order position. Owner=1000, admin=900, moderator=100, custom roles in 1..99, everyone=0. Not an authorization rank. */
  position: Scalars['Int']['output'];
  /** Internal role name (e.g. 'admin', 'moderator'). */
  roleName: Scalars['String']['output'];
};

/**
 * A full per-tier permission matrix: every role applicable at the
 * requested scope, with override + inherited baseline for each, plus the
 * list of permissions configurable at this scope.
 */
export type TierRoles = {
  __typename?: 'TierRoles';
  /**
   * Permissions configurable at this tier. The matrix renders one row per
   * entry in this list.
   */
  applicablePermissions: Array<Scalars['String']['output']>;
  /** All roles ordered by display position. */
  roles: Array<TierRole>;
};

/** Time display format preference. */
export enum TimeFormat {
  /** Use browser/locale default. */
  Auto = 'AUTO',
  /** 12-hour format (e.g., 2:30 PM). */
  TwelveHour = 'TWELVE_HOUR',
  /** 24-hour format (e.g., 14:30). */
  TwentyFourHour = 'TWENTY_FOUR_HOUR'
}

/** Input for unarchiving a room. */
export type UnarchiveRoomInput = {
  /** The ID of the room to unarchive. */
  roomId: Scalars['ID']['input'];
};

/** Input for removing a room ban. */
export type UnbanRoomMemberInput = {
  /** Moderator-entered reason stored for audit. */
  reason: Scalars['String']['input'];
  /** The ID of the channel room to unban the user from. */
  roomId: Scalars['ID']['input'];
  /** The ID of the user to unban. */
  userId: Scalars['ID']['input'];
};

/** Input for unfollowing a thread. */
export type UnfollowThreadInput = {
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['input'];
  /** The event ID of the thread root message. */
  threadRootEventId: Scalars['ID']['input'];
};

/** Input for unsubscribing from push notifications. */
export type UnsubscribeFromPushInput = {
  /** The push service endpoint URL to unsubscribe. */
  endpoint: Scalars['String']['input'];
};

/** Input for AdminMutations.updateBlockedUsernames. */
export type UpdateBlockedUsernamesInput = {
  /** Blocked usernames (newline-separated). Set to empty string to clear. */
  blockedUsernames: Scalars['String']['input'];
};

/** Input for updating a message. */
export type UpdateMessageInput = {
  /** For thread replies, whether the reply should have a visible channel echo after saving. Omit to preserve current echo state. */
  alsoSendToChannel?: InputMaybe<Scalars['Boolean']['input']>;
  /** The new message content. */
  body: Scalars['String']['input'];
  /** The event ID of the message to update. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
};

/** Input for updating the current user's presence status. */
export type UpdateMyPresenceInput = {
  /** The presence status to set. */
  status: PresenceStatusInput;
};

/** Input for updating a user's profile. */
export type UpdateProfileInput = {
  /** New display name. Omit to leave unchanged. */
  displayName?: InputMaybe<Scalars['String']['input']>;
  /** New login/username. Omit to leave unchanged. Subject to 30-day cooldown. */
  login?: InputMaybe<Scalars['String']['input']>;
  /** The ID of the user to update. Caller must be self or have admin permission. */
  userId: Scalars['ID']['input'];
};

/** Input for updating an existing role. */
export type UpdateRoleInput = {
  /** Role description. */
  description: Scalars['String']['input'];
  /** Human-readable display name. */
  displayName: Scalars['String']['input'];
  /** Role identifier of the role to update. */
  name: Scalars['String']['input'];
  /** Whether @role pings notify users assigned to this role. Omit to leave unchanged. */
  pingable?: InputMaybe<Scalars['Boolean']['input']>;
};

/** Input for updating an existing room group. */
export type UpdateRoomGroupInput = {
  /** Optional description. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** The room group's ID. */
  id: Scalars['ID']['input'];
  /** Display name. */
  name: Scalars['String']['input'];
};

/** Input for updating an existing room. */
export type UpdateRoomInput = {
  /** The new description for the room. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** The new name for the room. */
  name: Scalars['String']['input'];
  /** The ID of the room to update. */
  roomId: Scalars['ID']['input'];
};

/** Input for updating server configuration. */
export type UpdateServerConfigInput = {
  /** Short server description for OG link-preview metadata. Set to empty string to clear. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** Message of the Day for the header. Set to empty string to clear. */
  motd?: InputMaybe<Scalars['String']['input']>;
  /** Server name for page titles. Set to empty string to use default. */
  serverName?: InputMaybe<Scalars['String']['input']>;
  /** Welcome message shown on the login page. Set to empty string to clear. */
  welcomeMessage?: InputMaybe<Scalars['String']['input']>;
};

/**
 * Input for updating a user's settings. All preference fields are optional.
 * Only provided fields will be updated; omitted fields are left unchanged.
 */
export type UpdateSettingsInput = {
  /** Time display format. Set to AUTO to use browser locale default. */
  timeFormat?: InputMaybe<TimeFormat>;
  /** IANA timezone name. Set to null to clear (revert to browser default). */
  timezone?: InputMaybe<Scalars['String']['input']>;
  /** The ID of the user whose settings to update. Caller must be self or have admin permission. */
  userId: Scalars['ID']['input'];
};

/** Input for uploading a user avatar. */
export type UploadAvatarInput = {
  /** The avatar image file to upload. */
  file: Scalars['Upload']['input'];
  /** The ID of the user whose avatar to upload. Caller must be self or have admin permission. */
  userId: Scalars['ID']['input'];
};

/** Input for uploading the server banner. */
export type UploadServerBannerInput = {
  /** The banner image file. */
  file: Scalars['Upload']['input'];
};

/** Input for uploading the server logo. */
export type UploadServerLogoInput = {
  /** The logo image file. */
  file: Scalars['Upload']['input'];
};

/** A Chatto User. */
export type User = {
  __typename?: 'User';
  /** URL to the user's avatar image. Pass width, height, and fit for a resized thumbnail. */
  avatarUrl?: Maybe<Scalars['String']['output']>;
  /** When the user account was created. Null for users created before this field was added. */
  createdAt?: Maybe<Scalars['Time']['output']>;
  /** The user's display name. */
  displayName: Scalars['String']['output'];
  /** Whether this user has at least one verified email address. */
  hasVerifiedEmail: Scalars['Boolean']['output'];
  /** The user's unique ID. */
  id: Scalars['ID']['output'];
  /** When the user last changed their login/username. Null if never changed. Visible to the user themselves and to server admins. */
  lastLoginChange?: Maybe<Scalars['Time']['output']>;
  /** The user's login name (unique identifier for authentication). */
  login: Scalars['String']['output'];
  /** Get user's presence status. Returns OFFLINE if not present. */
  presenceStatus: PresenceStatus;
  /** Roles assigned to this user. Visible to any authenticated user. */
  roles: Array<Scalars['String']['output']>;
  /**
   * All room notification preferences for rooms the user has joined.
   * Returns one entry per joined room with its notification level.
   * Self-only: only the user themselves can query this.
   */
  roomNotificationPreferences: Array<RoomNotificationPreferenceItem>;
  /**
   * Rooms the user is a member of. Only visible to the user themselves.
   *
   * Pass `type: CHANNEL` for channels-only consumers; pass `type: DM` for DMs-only.
   * Null returns both (channels + the caller's DMs).
   */
  rooms: Array<Room>;
  /** The user's display preferences. Self-only: returns null for other users. */
  settings?: Maybe<UserSettings>;
  /**
   * The user's verified email addresses. Returns an empty list when the
   * caller is unauthorized. Authorization: caller is the user themselves,
   * OR caller holds the `admin.view-users` permission (the same permission
   * required to access the admin users page). Owners and admins via role
   * bypass this perm check.
   */
  verifiedEmails: Array<Scalars['String']['output']>;
  /** Whether the currently authenticated user can delete this account. */
  viewerCanDeleteAccount: Scalars['Boolean']['output'];
};


/** A Chatto User. */
export type UserAvatarUrlArgs = {
  fit?: InputMaybe<FitMode>;
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/** A Chatto User. */
export type UserRoomsArgs = {
  type?: InputMaybe<RoomType>;
};

/** Event: A user was created */
export type UserCreatedEvent = {
  __typename?: 'UserCreatedEvent';
  /** The user's display name. */
  displayName: Scalars['String']['output'];
  /** The user's login name. */
  login: Scalars['String']['output'];
  /** The ID of the newly created user. */
  userId: Scalars['ID']['output'];
};

/**
 * Event: A user deleted their account.
 * Published for audit logging and admin UI updates.
 */
export type UserDeletedEvent = {
  __typename?: 'UserDeletedEvent';
  /** The ID of the deleted user. */
  userId: Scalars['ID']['output'];
};

/** Event: A user joined a room */
export type UserJoinedRoomEvent = {
  __typename?: 'UserJoinedRoomEvent';
  /** The ID of the room the user joined. */
  roomId: Scalars['ID']['output'];
};

/** Event: A user left a room */
export type UserLeftRoomEvent = {
  __typename?: 'UserLeftRoomEvent';
  /** The ID of the room the user left. */
  roomId: Scalars['ID']['output'];
};

/**
 * Full snapshot of a user's permission matrix: the permissions that can
 * be configured anywhere, the scopes they can be configured at, and the
 * state of every cell.
 */
export type UserPermissionMatrix = {
  __typename?: 'UserPermissionMatrix';
  /**
   * Permissions to render as rows. Same identifiers used by the role
   * matrix, so the frontend can reuse its grouping / display-name
   * metadata.
   */
  applicablePermissions: Array<Scalars['String']['output']>;
  /**
   * One cell per (permission, scope) intersection. Sparse: a cell is
   * included iff the permission applies at that scope's tier.
   */
  cells: Array<PermissionMatrixCell>;
  /**
   * Scopes to render as columns. Server scope first, then groups, then
   * rooms grouped under their parent group via `parentGroupId`.
   */
  scopes: Array<PermissionMatrixScope>;
  /** The user this matrix describes. */
  userId: Scalars['ID']['output'];
};

/**
 * Event: A user's profile was updated.
 * Published when avatar, display name, or login changes, allowing real-time updates.
 */
export type UserProfileUpdatedEvent = {
  __typename?: 'UserProfileUpdatedEvent';
  /** The user's avatar URL, or null if no avatar is set. */
  avatarUrl?: Maybe<Scalars['String']['output']>;
  /** The user's updated display name. */
  displayName: Scalars['String']['output'];
  /** The user's current login/username. */
  login: Scalars['String']['output'];
  /** The ID of the user whose profile was updated. */
  userId: Scalars['ID']['output'];
};

/**
 * User display preferences for time and date formatting.
 * Preferences persist across devices.
 */
export type UserSettings = {
  __typename?: 'UserSettings';
  /** Preferred time display format. */
  timeFormat: TimeFormat;
  /** IANA timezone name (e.g., 'Europe/Berlin'). Null means use browser timezone. */
  timezone?: Maybe<Scalars['String']['output']>;
};

/**
 * Event: A user is typing in a room or thread.
 * This is a transient event.
 * Clients should implement timeout-based clearing (e.g., 6 seconds of inactivity).
 * The user who is typing is identified by the parent Event's actorId/actor.
 */
export type UserTypingEvent = {
  __typename?: 'UserTypingEvent';
  /** The ID of the room where the user is typing. */
  roomId: Scalars['ID']['output'];
  /** If typing in a thread, the root message event ID. Null for main room typing. */
  threadRootEventId?: Maybe<Scalars['ID']['output']>;
};

/** Video processing state for a video attachment. */
export type VideoProcessing = {
  __typename?: 'VideoProcessing';
  /** Video duration in milliseconds. */
  durationMs?: Maybe<Scalars['Int64']['output']>;
  /** Original video height in pixels. */
  height?: Maybe<Scalars['Int']['output']>;
  /** Stable machine-readable failure reason. */
  reasonCode?: Maybe<Scalars['String']['output']>;
  /** Whether the original uploaded video binary is available for fallback playback. */
  sourceAvailable: Scalars['Boolean']['output'];
  /** Current processing status. */
  status: VideoProcessingStatus;
  /** URL and expiry for the video thumbnail image. */
  thumbnailAssetUrl?: Maybe<AssetUrl>;
  /** URL for the video thumbnail image. */
  thumbnailUrl?: Maybe<Scalars['String']['output']>;
  /** Available quality variants. */
  variants: Array<VideoVariant>;
  /** Original video width in pixels. */
  width?: Maybe<Scalars['Int']['output']>;
};

/** Status of video processing. */
export enum VideoProcessingStatus {
  /** Transcoding finished; at least one variant is available for playback. */
  Completed = 'COMPLETED',
  /** Transcoding failed; `reasonCode` describes the failure and no variants are available. */
  Failed = 'FAILED',
  /** Upload received and queued for processing; no transcoded variants yet. */
  Pending = 'PENDING',
  /** Currently transcoding; the video is not yet playable. */
  Processing = 'PROCESSING'
}

/** A transcoded quality variant of a video. */
export type VideoVariant = {
  __typename?: 'VideoVariant';
  /** URL and expiry for streaming/downloading this variant. */
  assetUrl: AssetUrl;
  /** Video height in pixels. */
  height: Scalars['Int']['output'];
  /** Quality label (e.g., '720p', '480p'). */
  quality: Scalars['String']['output'];
  /** File size in bytes. */
  size: Scalars['Int64']['output'];
  /** URL to stream/download this variant. */
  url: Scalars['String']['output'];
  /** Video width in pixels. */
  width: Scalars['Int']['output'];
};

/**
 * The current authenticated user, together with their server-level
 * permissions. `Query.viewer` is null when no one is authenticated;
 * inside a non-null `Viewer`, `user` is guaranteed.
 */
export type Viewer = {
  __typename?: 'Viewer';
  /** Whether the viewer can create and edit server roles. */
  canAdminManageRoles: Scalars['Boolean']['output'];
  /** Whether the viewer can manage user role assignments. */
  canAdminManageUsers: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin audit log. */
  canAdminViewAudit: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin roles page. */
  canAdminViewRoles: Scalars['Boolean']['output'];
  /** Whether the viewer can view owner-only admin system diagnostics. */
  canAdminViewSystem: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin users page. */
  canAdminViewUsers: Scalars['Boolean']['output'];
  /** Whether the viewer can start DM conversations. Backed by message.post. */
  canStartDMs: Scalars['Boolean']['output'];
  /** Whether the viewer has at least one admin-capability entry point. */
  canViewAdmin: Scalars['Boolean']['output'];
  /**
   * Threads the current user is following on the server, sorted by last
   * activity (newest first). Requires server membership.
   */
  followedThreads: FollowedThreadsConnection;
  /** Whether the current user has any notifications (for bell icon indicator). */
  hasNotifications: Scalars['Boolean']['output'];
  /**
   * Whether the current user has any unread followed threads. Lightweight
   * query for sidebar unread indicators. Requires server membership.
   */
  hasUnreadFollowedThreads: Scalars['Boolean']['output'];
  /** Notifications for the current user, newest first. */
  notifications: NotificationsConnection;
  /** The authenticated user. */
  user: User;
};


/**
 * The current authenticated user, together with their server-level
 * permissions. `Query.viewer` is null when no one is authenticated;
 * inside a non-null `Viewer`, `user` is guaranteed.
 */
export type ViewerFollowedThreadsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


/**
 * The current authenticated user, together with their server-level
 * permissions. `Query.viewer` is null when no one is authenticated;
 * inside a non-null `Viewer`, `user` is guaranteed.
 */
export type ViewerNotificationsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};

/**
 * The viewer's notification preference for the server or a room.
 * Contains both the explicitly set level and the effective level after inheritance.
 */
export type ViewerNotificationPreference = {
  __typename?: 'ViewerNotificationPreference';
  /** The effective level after inheritance resolution (never DEFAULT). */
  effectiveLevel: NotificationLevel;
  /** The explicitly set level (DEFAULT if not explicitly configured). */
  level: NotificationLevel;
};

export type VoiceCallIntentInput = {
  /** The room whose voice call is being joined or left. */
  roomId: Scalars['ID']['input'];
};

/** Token for joining a LiveKit voice call. */
export type VoiceCallToken = {
  __typename?: 'VoiceCallToken';
  /** The active call session ID this token joins. */
  callId: Scalars['ID']['output'];
  /** Shared LiveKit E2EE key for this active call. Distributed by Chatto, never by LiveKit. */
  e2eeKey: Scalars['String']['output'];
  /** The LiveKit JWT token. */
  token: Scalars['String']['output'];
};
