/* eslint-disable */
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
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

/** Admin mutations for configuration management. */
export type AdminMutations = {
  __typename?: 'AdminMutations';
  /** Clear the 30-day login change cooldown for a user, allowing them to immediately rename themselves. Idempotent. */
  clearUsernameCooldown: Scalars['Boolean']['output'];
  /** Update server configuration. Returns the updated config section. */
  updateServerConfig: AdminServerConfig;
  /** Update a user's login and/or display name. Bypasses the 30-day login change cooldown but otherwise reuses the same validation as updateProfile. */
  updateUser: User;
};


/** Admin mutations for configuration management. */
export type AdminMutationsClearUsernameCooldownArgs = {
  input: ClearUsernameCooldownInput;
};


/** Admin mutations for configuration management. */
export type AdminMutationsUpdateServerConfigArgs = {
  input: UpdateServerConfigInput;
};


/** Admin mutations for configuration management. */
export type AdminMutationsUpdateUserArgs = {
  input: AdminUpdateUserInput;
};

/** Admin-only query namespace for operator tooling. Returns null if the user is not a server admin. */
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
  /** Get server configuration. */
  serverConfig: AdminServerConfig;
  /** List all available server permission identifiers. */
  serverPermissions: Array<Scalars['String']['output']>;
  /** Get point-in-time operator diagnostics for connection, storage, and deployment counts. */
  systemInfo: SystemInfo;
};


/** Admin-only query namespace for operator tooling. Returns null if the user is not a server admin. */
export type AdminQueriesEventLogArgs = {
  before?: InputMaybe<Scalars['String']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
};


/** Admin-only query namespace for operator tooling. Returns null if the user is not a server admin. */
export type AdminQueriesEventLogEntryArgs = {
  sequence: Scalars['String']['input'];
};


/** Admin-only query namespace for operator tooling. Returns null if the user is not a server admin. */
export type AdminQueriesGroupRolePermissionsArgs = {
  groupId: Scalars['ID']['input'];
  roleName: Scalars['String']['input'];
};


/** Admin-only query namespace for operator tooling. Returns null if the user is not a server admin. */
export type AdminQueriesGroupUserPermissionsArgs = {
  groupId: Scalars['ID']['input'];
  userId: Scalars['ID']['input'];
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
  /** The event ID of the message containing the attachment, when message-owned. */
  messageEventId: Scalars['ID']['output'];
  /** Stable machine-readable reason. */
  reasonCode: Scalars['String']['output'];
  /** The room ID. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: asset processing has been enqueued. Emitted before SucceededEvent or
 * FailedEvent so subscribers can render a "processing…" placeholder.
 */
export type AssetProcessingStartedEvent = {
  __typename?: 'AssetProcessingStartedEvent';
  /** The original asset ID whose processing has been enqueued. */
  assetId: Scalars['ID']['output'];
  /** The event ID of the message containing the attachment, when message-owned. */
  messageEventId: Scalars['ID']['output'];
  /** The room ID. */
  roomId: Scalars['ID']['output'];
};

/** Event: asset processing produced a durable derivative manifest. */
export type AssetProcessingSucceededEvent = {
  __typename?: 'AssetProcessingSucceededEvent';
  /** The original asset ID that was processed. */
  assetId: Scalars['ID']['output'];
  /** The event ID of the message containing the processed attachment, when message-owned. */
  messageEventId: Scalars['ID']['output'];
  /** The room ID. */
  roomId: Scalars['ID']['output'];
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
  size: Scalars['Int']['output'];
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

/** A participant currently in a voice call. */
export type CallParticipant = {
  __typename?: 'CallParticipant';
  /** The user's avatar URL (may be null if no avatar is set). */
  avatarUrl?: Maybe<Scalars['String']['output']>;
  /** The user's display name. */
  displayName: Scalars['String']['output'];
  /** Unix timestamp (seconds) when the user joined the call. */
  joinedAt: Scalars['Int']['output'];
  /** The user's login handle. */
  login: Scalars['String']['output'];
  /** The user's ID. */
  userId: Scalars['ID']['output'];
};

/**
 * Event: A user joined a voice call in a room.
 * The user who joined is identified by the parent Event's actorId/actor.
 */
export type CallParticipantJoinedEvent = {
  __typename?: 'CallParticipantJoinedEvent';
  /** The ID of the room where the call is happening. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: A user left a voice call in a room.
 * The user who left is identified by the parent Event's actorId/actor.
 */
export type CallParticipantLeftEvent = {
  __typename?: 'CallParticipantLeftEvent';
  /** The ID of the room where the call was happening. */
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
  /** The ID of the user who triggered this event. */
  actorId: Scalars['ID']['output'];
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
export type EventType = AssetDeletedEvent | AssetProcessingFailedEvent | AssetProcessingStartedEvent | AssetProcessingSucceededEvent | CallParticipantJoinedEvent | CallParticipantLeftEvent | HeartbeatEvent | MentionNotificationEvent | MentionStatusClearedEvent | MessageEditedEvent | MessagePostedEvent | MessageRetractedEvent | NewDirectMessageNotificationEvent | NotificationCreatedEvent | NotificationDismissedEvent | NotificationLevelChangedEvent | PresenceChangedEvent | ReactionAddedEvent | ReactionRemovedEvent | RoomArchivedEvent | RoomCreatedEvent | RoomDeletedEvent | RoomGroupsUpdatedEvent | RoomMarkedAsReadEvent | RoomUnarchivedEvent | RoomUpdatedEvent | ServerConfigUpdatedEvent | ServerMemberDeletedEvent | ServerUpdatedEvent | ServerUserPreferencesUpdatedEvent | SessionTerminatedEvent | ThreadCreatedEvent | ThreadFollowChangedEvent | UserCreatedEvent | UserDeletedEvent | UserJoinedRoomEvent | UserLeftRoomEvent | UserProfileUpdatedEvent | UserTypingEvent | VideoProcessingCompletedEvent;

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
  /** Users who have participated in this thread. */
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
 * Synthetic event emitted by the server on the `myServerEvents` subscription
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
  /** Emoji reactions on this message, aggregated by emoji. */
  reactions: Array<Reaction>;
  /** Number of replies in this thread (0 for non-root messages or messages without replies). */
  replyCount: Scalars['Int']['output'];
  /** The ID of the room where the message was posted. */
  roomId: Scalars['ID']['output'];
  /** Users who have replied in this thread (empty for non-root messages or messages without replies). Returns up to `first` participants (default 5) for preview. */
  threadParticipants: Array<User>;
  /**
   * Replies to this message, when it is a thread root. Returns an empty list when
   * this message is itself a thread reply. Replies are returned in chronological
   * order and do not include this root event.
   */
  threadReplies: Array<Event>;
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
 * both the source and target room group (ADR-031).
 */
export type MoveRoomToSetInput = {
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
  /** Admin mutations. Returns null if user lacks admin permission. */
  admin?: Maybe<AdminMutations>;
  /** Archive a room. Hides it from sidebar and Browse Rooms. Requires room.manage permission. */
  archiveRoom: Room;
  /**
   * Assign an server role to a user. Idempotent - assigning an already-assigned
   * role succeeds silently. Returns true on success.
   * Note: The 'everyone' role is implicit for all users and cannot be assigned.
   * Requires: role.assign permission and outranking the target user.
   * Errors: If role doesn't exist or is 'everyone'.
   */
  assignRole: Scalars['Boolean']['output'];
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
   * Authorization and roomId semantics mirror grantUserPermission. In
   * particular, self-clear is not permitted (no self-bypass).
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
   * holds `role.assign` AND either is an owner or outranks the target
   * user by role hierarchy. Returns the updated user.
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
   * Deny a permission directly to a user. Beats any role grant —
   * user-level decisions are checked before the role-hierarchy walk.
   * Useful for one-off moderation like suspending a user from posting
   * without revoking their roles.
   *
   * Authorization and roomId semantics mirror grantUserPermission. In
   * particular, self-deny is not permitted (no self-bypass).
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
   * Grant a permission directly to a user. Beats any role-level decision —
   * user-level grants are checked before roles in the resolver. Useful for
   * ad-hoc privileges like "let this one user moderate room X" without
   * inventing a custom role.
   *
   * Authorization: caller needs role.manage AND must strictly outrank the
   * target user. Self-action is NOT permitted — granting yourself a
   * permission is a privilege boundary change, not an identity edit, so
   * the strict-outrank step (which always fails on self) closes that path.
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
  /** Leave the specified room. */
  leaveRoom: Scalars['Boolean']['output'];
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
   * (ADR-031). Permission overrides on the room itself are preserved.
   */
  moveRoomToSet: Room;
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
   * Requires: role.assign permission and outranking the target user.
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
   * Authorization: caller is self, OR caller holds `role.assign` AND
   * either is an owner or outranks the target user by role hierarchy.
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
  /** Update the server's name. Requires server.manage permission. */
  updateServer: Server;
  /**
   * Update a user's display settings. Authorization: caller is self, OR
   * caller holds `role.assign` AND either is an owner or outranks the
   * target user by role hierarchy. Returns the updated settings.
   */
  updateSettings: UserSettings;
  /**
   * Upload an avatar for a user. Image will be resized to 256x256 max
   * and converted to WebP. Authorization: caller is self, OR caller
   * holds `role.assign` AND either is an owner or outranks the target
   * user by role hierarchy. Returns the updated user.
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
export type MutationLeaveRoomArgs = {
  input: LeaveRoomInput;
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
export type MutationMoveRoomToSetArgs = {
  input: MoveRoomToSetInput;
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
export type MutationUpdateServerArgs = {
  input: UpdateServerInput;
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
  /** Event ID for navigation (may be empty) */
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
  /** The ID of the room to post to. */
  roomId: Scalars['ID']['input'];
  /** Event ID of the thread root message. Determines thread membership and controls permission check (message.start_thread vs message.post_in_thread vs message.post). */
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
  /** Admin-only queries. Returns null if user lacks admin permission. */
  admin?: Maybe<AdminQueries>;
  /**
   * Fetch link preview metadata for a URL.
   * Returns null if the URL cannot be previewed.
   * Requires authentication.
   */
  linkPreview?: Maybe<LinkPreview>;
  /**
   * Explain every applicable permission for a user at the given scope.
   * - userId only → server-scoped permissions.
   * - userId + roomId → room-scoped permissions.
   * Authorization: The viewer must be either the target user (self-inspection
   * at any scope they are a member of) or a server admin at the requested
   * scope.
   */
  permissionExplanation: Array<PermissionExplanation>;
  /**
   * Permission matrix for a specific role. Authorization: viewer must
   * hold `role.manage` at server scope (the permission that gates editing
   * role grants/denials). Same audience as the per-tier role editor in
   * /server-admin/permissions.
   *
   * The cell shape mirrors `UserPermissionMatrix`, but `effective` here
   * walks only this role's own grants (no rank, no user overrides) — it
   * reflects what the role contributes to the resolver, not what any
   * particular user receives.
   */
  rolePermissionMatrix?: Maybe<RolePermissionMatrix>;
  /**
   * Resolve a single role's permission state across every applicable tier
   * in one round-trip. Useful for permission editors that need to show
   * values inherited from the tiers above the one being edited.
   *
   * Authorization: server scope requires server admin; room scope requires
   * role.manage on the server or server admin.
   */
  rolePermissions?: Maybe<RoleAcrossTiers>;
  /** Get a specific room by ID. */
  room?: Maybe<Room>;
  /** Get information about this Chatto server. No authentication required. */
  server: Server;
  /**
   * Return the full permission matrix at a tier: every applicable role
   * with its override and inherited baseline. Authorization mirrors
   * rolePermissions.
   *
   * Pass `roomId` for per-room override editing (inherits from the room's
   * room group), `groupId` for room-group-scope editing (no inheritance —
   * room groups are top-level for channel-room permissions). Pass neither
   * for server scope.
   * Passing both is rejected.
   */
  tierRoles?: Maybe<TierRoles>;
  /** Get a specific user by ID. Requires authentication. */
  user?: Maybe<User>;
  /** Get a specific user by login. Requires authentication. Returns null if not found. */
  userByLogin?: Maybe<User>;
  /**
   * Permission matrix for a specific user. Authorization: viewer must
   * pass `requireUserPermissionTarget` (role.manage + strictly outrank
   * the target) — the same gate used by grantUserPermission and friends.
   *
   * Self-introspection is not allowed; the matrix is an admin surface.
   */
  userPermissionMatrix?: Maybe<UserPermissionMatrix>;
  /** The current authenticated user's server-level permissions. Null if not authenticated. */
  viewer?: Maybe<Viewer>;
};


/** Root query type for fetching data. */
export type QueryLinkPreviewArgs = {
  url: Scalars['String']['input'];
};


/** Root query type for fetching data. */
export type QueryPermissionExplanationArgs = {
  roomId?: InputMaybe<Scalars['ID']['input']>;
  userId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRolePermissionMatrixArgs = {
  roleName: Scalars['String']['input'];
};


/** Root query type for fetching data. */
export type QueryRolePermissionsArgs = {
  roleName: Scalars['String']['input'];
  roomId?: InputMaybe<Scalars['ID']['input']>;
};


/** Root query type for fetching data. */
export type QueryRoomArgs = {
  roomId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryTierRolesArgs = {
  groupId?: InputMaybe<Scalars['ID']['input']>;
  roomId?: InputMaybe<Scalars['ID']['input']>;
};


/** Root query type for fetching data. */
export type QueryUserArgs = {
  userId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryUserByLoginArgs = {
  login: Scalars['String']['input'];
};


/** Root query type for fetching data. */
export type QueryUserPermissionMatrixArgs = {
  userId: Scalars['ID']['input'];
};

/**
 * A reaction represents emoji responses to a message, aggregated by emoji type.
 * Emoji values are shortcode names (e.g., "thumbsup", "heart") — clients convert to Unicode for display.
 */
export type Reaction = {
  __typename?: 'Reaction';
  /** Total number of users who reacted with this emoji. */
  count: Scalars['Int']['output'];
  /** The emoji shortcode name (e.g., "thumbsup", "heart"). */
  emoji: Scalars['String']['output'];
  /** Whether the current user has reacted with this emoji. */
  hasReacted: Scalars['Boolean']['output'];
  /** List of users who reacted with this emoji. */
  users: Array<User>;
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
  /** Hierarchy position: higher = higher rank. Owner=1000, admin=900, moderator=100, custom roles in 1..99, everyone=0. */
  position: Scalars['Int']['output'];
};

/**
 * A single role's permission state at every applicable tier.
 *
 * - rolePermissions(roleName) → server only.
 * - rolePermissions(roleName, roomId) → server + room.
 */
export type RoleAcrossTiers = {
  __typename?: 'RoleAcrossTiers';
  /**
   * Permissions configurable at the deepest requested scope. Use this as the
   * set of permissions to render in a permission editor for this scope.
   */
  applicablePermissions: Array<Scalars['String']['output']>;
  /** Role description. */
  description: Scalars['String']['output'];
  /** Human-readable display name. */
  displayName: Scalars['String']['output'];
  /** Whether this is a system role and cannot be deleted. */
  isSystem: Scalars['Boolean']['output'];
  /** Hierarchy position: higher = higher rank. Owner=1000, admin=900, moderator=100, custom roles in 1..99, everyone=0. */
  position: Scalars['Int']['output'];
  /** Internal role name (e.g. 'admin', 'moderator'). */
  roleName: Scalars['String']['output'];
  /** Permission state at room scope (null when roomId not provided). */
  room?: Maybe<TierPermissions>;
  /** Permission state at server scope (the role's defaults everywhere). */
  server: TierPermissions;
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
  cells: Array<UserPermissionCell>;
  /** The role this matrix describes. */
  roleName: Scalars['String']['output'];
  /**
   * Scopes to render as columns. Server scope first, then groups, then
   * rooms grouped under their parent group via `parentGroupId`. Same
   * shape as `UserPermissionMatrix.scopes`.
   */
  scopes: Array<UserPermissionScope>;
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
  /** Hierarchy position (higher = higher rank; see Role.position). */
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
   * one. DM rooms return an empty string sentinel because they do not
   * participate in room-group layout or ACLs (see ADR-031).
   */
  groupId: Scalars['ID']['output'];
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
   * this room (subject to also strictly outranking the author). Authors
   * editing or deleting their own messages do not need this permission.
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

/** Event: A room was created */
export type RoomCreatedEvent = {
  __typename?: 'RoomCreatedEvent';
  /** The room's description. */
  description: Scalars['String']['output'];
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
 * Paginated room events with metadata indicating whether more events exist
 * in either direction. `startCursor` and `endCursor` are opaque pagination
 * cursors — pass them as `before` / `after` on a subsequent `Room.events`
 * call. Both are null when `events` is empty.
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
 * rooms able to override on a per (role, permission) basis (see ADR-031).
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
  /** The room's updated description. */
  description: Scalars['String']['output'];
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
  /** List all available permission identifiers. */
  availablePermissions: Array<Scalars['String']['output']>;
  /** True if direct (email/password) registration is enabled on this server. */
  directRegistrationEnabled: Scalars['Boolean']['output'];
  /** List of enabled SSO provider names (e.g., 'google', 'github'). */
  enabledAuthProviders: Array<Scalars['String']['output']>;
  /** LiveKit WebSocket URL for voice calls. Null if voice calls are disabled. */
  livekitUrl?: Maybe<Scalars['String']['output']>;
  /** Maximum upload size for regular attachments (images, files) in bytes. */
  maxUploadSize: Scalars['Int']['output'];
  /** Maximum upload size for video attachments in bytes. Same as maxUploadSize when video processing is disabled. */
  maxVideoUploadSize: Scalars['Int']['output'];
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
  /** Duration in seconds after posting during which a user can edit their own message. Moderators with `message.edit-any` are not bound by this window. */
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
   * Ordered list of channel-room groups (ADR-031). Every server boots with at
   * least the seed "Lobby" group; the list is never empty for a configured
   * server.
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
  /**
   * UI hint reporting whether the viewer outranks the target user by role
   * hierarchy. **This is a rank check only**, not an authorization gate —
   * capabilities like "edit this user's profile" additionally require a
   * permission (e.g. `role.assign`). Use this for showing/hiding admin UI
   * affordances; never as the sole basis for permitting a mutation. See
   * `.claude/rules/authorization.md` (`permission AND OutranksUser`).
   */
  viewerCanManageUser: Scalars['Boolean']['output'];
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
 * Event: Server configuration was updated.
 * Clients should refetch server info to get the new values.
 */
export type ServerConfigUpdatedEvent = {
  __typename?: 'ServerConfigUpdatedEvent';
  /** The updated blocked usernames (null if cleared). */
  blockedUsernames?: Maybe<Scalars['String']['output']>;
  /** The updated MOTD (null if cleared). */
  motd?: Maybe<Scalars['String']['output']>;
  /** The updated server name. */
  serverName: Scalars['String']['output'];
  /** The updated welcome message (null if cleared). */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
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

/** Event: The server was updated. */
export type ServerUpdatedEvent = {
  __typename?: 'ServerUpdatedEvent';
  /** The server's banner URL (empty if no banner). */
  bannerUrl: Scalars['String']['output'];
  /** The server's description (empty if not set). */
  description: Scalars['String']['output'];
  /** The server's logo URL (empty if no logo). */
  logoUrl: Scalars['String']['output'];
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
  /** IANA timezone name (empty string = browser default). */
  timezone: Scalars['String']['output'];
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
   *   - Space-scoped events: delivered to all server members.
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
   *
   * Only delivers new events — no replay of historical traffic.
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
 * Returned as part of RoleAcrossTiers so callers can display inheritance
 * without making separate per-tier queries.
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
  /** Hierarchy position: higher = higher rank. Owner=1000, admin=900, moderator=100, custom roles in 1..99, everyone=0. */
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
  /** All roles ordered by position (lowest = highest rank first). */
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

/** Input for updating a message. */
export type UpdateMessageInput = {
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
  /** Blocked usernames (newline-separated). Set to empty string to clear. */
  blockedUsernames?: InputMaybe<Scalars['String']['input']>;
  /** Short server description for OG link-preview metadata. Set to empty string to clear. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** Message of the Day for the header. Set to empty string to clear. */
  motd?: InputMaybe<Scalars['String']['input']>;
  /** Server name for page titles. Set to empty string to use default. */
  serverName?: InputMaybe<Scalars['String']['input']>;
  /** Welcome message shown on the login page. Set to empty string to clear. */
  welcomeMessage?: InputMaybe<Scalars['String']['input']>;
};

/** Input for updating the server. */
export type UpdateServerInput = {
  /** The new description for the server. Set to empty string to clear. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** Message of the Day, displayed in the chat header. Set to empty string to clear. */
  motd?: InputMaybe<Scalars['String']['input']>;
  /** The new name for the server. */
  name: Scalars['String']['input'];
  /** Welcome message shown on the login page (markdown supported). Set to empty string to clear. */
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
 * One cell of the user-permission matrix: the per-permission, per-scope
 * intersection.
 */
export type UserPermissionCell = {
  __typename?: 'UserPermissionCell';
  /**
   * The **effective** decision the resolver would emit at this scope for
   * this user-permission pair, after walking room → group → server with
   * user-level overrides applied first. Drives the cell's tint.
   */
  effective: UserPermissionDecision;
  /**
   * The **explicit user-level override** at this scope, or NONE if the user
   * has no override here. NONE cells display only the inherited effective
   * state; ALLOW / DENY cells display as a solid override.
   */
  override: UserPermissionDecision;
  /** Permission identifier (e.g. `message.post`). */
  permission: Scalars['String']['output'];
  /** Scope id (matches `UserPermissionScope.id`). */
  scopeId: Scalars['String']['output'];
};

/** Trinary decision used in the user-permission matrix. */
export enum UserPermissionDecision {
  /** The permission is explicitly granted. */
  Allow = 'ALLOW',
  /** The permission is explicitly denied. */
  Deny = 'DENY',
  /** No explicit grant or denial applies at this scope. */
  None = 'NONE'
}

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
  cells: Array<UserPermissionCell>;
  /**
   * Scopes to render as columns. Server scope first, then groups, then
   * rooms grouped under their parent group via `parentGroupId`.
   */
  scopes: Array<UserPermissionScope>;
  /** The user this matrix describes. */
  userId: Scalars['ID']['output'];
};

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
export type UserPermissionScope = {
  __typename?: 'UserPermissionScope';
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
  kind: UserPermissionScopeKind;
  /** Human-readable label for the scope (group name, room name, or 'Server'). */
  label: Scalars['String']['output'];
  /**
   * For room scopes, the parent group's ID — so the UI can nest rooms under
   * their group column. Empty string for server / group scopes.
   */
  parentGroupId: Scalars['ID']['output'];
};

/** Where a UserPermissionScope sits in the resolution hierarchy. */
export enum UserPermissionScopeKind {
  /** A room group's scope (channel-room permissions). */
  Group = 'GROUP',
  /** A specific room's scope. */
  Room = 'ROOM',
  /** Server tier — no room/group context. */
  Server = 'SERVER'
}

/**
 * Event: A user's profile was updated.
 * Published when avatar, display name, or login changes, allowing real-time updates.
 */
export type UserProfileUpdatedEvent = {
  __typename?: 'UserProfileUpdatedEvent';
  /** The user's avatar URL (empty string if no avatar). */
  avatarUrl: Scalars['String']['output'];
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

/**
 * Event: Video processing completed (or failed).
 * Published when a video attachment finishes transcoding. The frontend uses this
 * to refetch the message and display the video player.
 */
export type VideoProcessingCompletedEvent = {
  __typename?: 'VideoProcessingCompletedEvent';
  /** The original attachment ID that was processed. */
  attachmentId: Scalars['ID']['output'];
  /** The event ID of the message containing the processed attachment. */
  messageEventId: Scalars['ID']['output'];
  /** The room ID. */
  roomId: Scalars['ID']['output'];
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
  /** Whether the viewer can view admin system and data pages. */
  canAdminViewSystem: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin users page. */
  canAdminViewUsers: Scalars['Boolean']['output'];
  /** Whether the viewer can start DM conversations. Backed by message.post. */
  canStartDMs: Scalars['Boolean']['output'];
  /** Whether the viewer can access the admin panel (includes config-admin check). */
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

/** Token for joining a LiveKit voice call. */
export type VoiceCallToken = {
  __typename?: 'VoiceCallToken';
  /** The LiveKit JWT token. */
  token: Scalars['String']['output'];
};

export type CreateRoomMutationVariables = Exact<{
  input: CreateRoomInput;
}>;


export type CreateRoomMutation = { __typename?: 'Mutation', createRoom: { __typename?: 'Room', id: string, name: string, description?: string | null } };

export type JoinRoomMutationVariables = Exact<{
  input: JoinRoomInput;
}>;


export type JoinRoomMutation = { __typename?: 'Mutation', joinRoom: { __typename?: 'Room', id: string } };

export type ServerSettingsModalQueryVariables = Exact<{ [key: string]: never; }>;


export type ServerSettingsModalQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerCanManageServer: boolean, profile: { __typename?: 'ServerProfile', name: string, description?: string | null, motd?: string | null, welcomeMessage?: string | null, logoUrl?: string | null, bannerUrl?: string | null } } };

export type UpdateServerSettingsModalMutationVariables = Exact<{
  input: UpdateServerInput;
}>;


export type UpdateServerSettingsModalMutation = { __typename?: 'Mutation', updateServer: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', name: string, description?: string | null, motd?: string | null, welcomeMessage?: string | null } } };

export type UploadInstanceLogoMutationVariables = Exact<{
  input: UploadServerLogoInput;
}>;


export type UploadInstanceLogoMutation = { __typename?: 'Mutation', uploadServerLogo: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', logoUrl?: string | null } } };

export type DeleteInstanceLogoMutationVariables = Exact<{ [key: string]: never; }>;


export type DeleteInstanceLogoMutation = { __typename?: 'Mutation', deleteServerLogo: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', logoUrl?: string | null } } };

export type UploadInstanceBannerMutationVariables = Exact<{
  input: UploadServerBannerInput;
}>;


export type UploadInstanceBannerMutation = { __typename?: 'Mutation', uploadServerBanner: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', bannerUrl?: string | null } } };

export type DeleteInstanceBannerMutationVariables = Exact<{ [key: string]: never; }>;


export type DeleteInstanceBannerMutation = { __typename?: 'Mutation', deleteServerBanner: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', bannerUrl?: string | null } } };

export type InstanceInitQueryVariables = Exact<{ [key: string]: never; }>;


export type InstanceInitQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerHasUnreadRooms: boolean, profile: { __typename?: 'ServerProfile', name: string, logoUrl?: string | null }, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null, rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> }, viewer?: { __typename?: 'Viewer', canViewAdmin: boolean, canStartDMs: boolean, canAdminViewUsers: boolean, canAdminManageUsers: boolean, canAdminViewRoles: boolean, canAdminManageRoles: boolean, canAdminViewSystem: boolean, canAdminViewAudit: boolean, user: { __typename?: 'User', roomNotificationPreferences: Array<{ __typename?: 'RoomNotificationPreferenceItem', roomId: string, level: NotificationLevel, effectiveLevel: NotificationLevel }> } } | null };

export type InstanceIconRefreshQueryVariables = Exact<{ [key: string]: never; }>;


export type InstanceIconRefreshQuery = { __typename?: 'Query', server: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', name: string, logoUrl?: string | null } } };

export type FirstUnreadRoomQueryVariables = Exact<{ [key: string]: never; }>;


export type FirstUnreadRoomQuery = { __typename?: 'Query', server: { __typename?: 'Server', rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean }> } };

export type RefreshMessageAttachmentUrlsQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type RefreshMessageAttachmentUrlsQuery = { __typename?: 'Query', room?: { __typename?: 'Room', event?: { __typename?: 'Event', event:
        | { __typename: 'AssetDeletedEvent' }
        | { __typename: 'AssetProcessingFailedEvent' }
        | { __typename: 'AssetProcessingStartedEvent' }
        | { __typename: 'AssetProcessingSucceededEvent' }
        | { __typename: 'CallParticipantJoinedEvent' }
        | { __typename: 'CallParticipantLeftEvent' }
        | { __typename: 'HeartbeatEvent' }
        | { __typename: 'MentionNotificationEvent' }
        | { __typename: 'MentionStatusClearedEvent' }
        | { __typename: 'MessageEditedEvent' }
        | { __typename: 'MessagePostedEvent', attachments: Array<{ __typename?: 'Attachment', id: string, assetUrl: { __typename?: 'AssetURL', url: string, expiresAt: any }, thumbnailAssetUrl?: { __typename?: 'AssetURL', url: string, expiresAt: any } | null, videoProcessing?: { __typename?: 'VideoProcessing', thumbnailAssetUrl?: { __typename?: 'AssetURL', url: string, expiresAt: any } | null, variants: Array<{ __typename?: 'VideoVariant', quality: string, assetUrl: { __typename?: 'AssetURL', url: string, expiresAt: any } }> } | null }> }
        | { __typename: 'MessageRetractedEvent' }
        | { __typename: 'NewDirectMessageNotificationEvent' }
        | { __typename: 'NotificationCreatedEvent' }
        | { __typename: 'NotificationDismissedEvent' }
        | { __typename: 'NotificationLevelChangedEvent' }
        | { __typename: 'PresenceChangedEvent' }
        | { __typename: 'ReactionAddedEvent' }
        | { __typename: 'ReactionRemovedEvent' }
        | { __typename: 'RoomArchivedEvent' }
        | { __typename: 'RoomCreatedEvent' }
        | { __typename: 'RoomDeletedEvent' }
        | { __typename: 'RoomGroupsUpdatedEvent' }
        | { __typename: 'RoomMarkedAsReadEvent' }
        | { __typename: 'RoomUnarchivedEvent' }
        | { __typename: 'RoomUpdatedEvent' }
        | { __typename: 'ServerConfigUpdatedEvent' }
        | { __typename: 'ServerMemberDeletedEvent' }
        | { __typename: 'ServerUpdatedEvent' }
        | { __typename: 'ServerUserPreferencesUpdatedEvent' }
        | { __typename: 'SessionTerminatedEvent' }
        | { __typename: 'ThreadCreatedEvent' }
        | { __typename: 'ThreadFollowChangedEvent' }
        | { __typename: 'UserCreatedEvent' }
        | { __typename: 'UserDeletedEvent' }
        | { __typename: 'UserJoinedRoomEvent' }
        | { __typename: 'UserLeftRoomEvent' }
        | { __typename: 'UserProfileUpdatedEvent' }
        | { __typename: 'UserTypingEvent' }
        | { __typename: 'VideoProcessingCompletedEvent' }
       } | null } | null };

export type LoadCurrentUserQueryVariables = Exact<{ [key: string]: never; }>;


export type LoadCurrentUserQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus, hasVerifiedEmail: boolean, settings?: { __typename?: 'UserSettings', timezone?: string | null, timeFormat: TimeFormat } | null } } | null };

export type LinkPreviewViewFragment = { __typename?: 'LinkPreview', url: string, title?: string | null, description?: string | null, imageUrl?: string | null, siteName?: string | null, embedType?: string | null, embedId?: string | null } & { ' $fragmentName'?: 'LinkPreviewViewFragment' };

export type MessagePreviewQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type MessagePreviewQuery = { __typename?: 'Query', server: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', name: string } }, room?: { __typename?: 'Room', id: string, name: string, event?: { __typename?: 'Event', id: string, createdAt: any, actor?: (
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      ) | null, event:
        | { __typename: 'AssetDeletedEvent' }
        | { __typename: 'AssetProcessingFailedEvent' }
        | { __typename: 'AssetProcessingStartedEvent' }
        | { __typename: 'AssetProcessingSucceededEvent' }
        | { __typename: 'CallParticipantJoinedEvent' }
        | { __typename: 'CallParticipantLeftEvent' }
        | { __typename: 'HeartbeatEvent' }
        | { __typename: 'MentionNotificationEvent' }
        | { __typename: 'MentionStatusClearedEvent' }
        | { __typename: 'MessageEditedEvent' }
        | { __typename: 'MessagePostedEvent', body?: string | null, attachments: Array<{ __typename?: 'Attachment', id: string, filename: string, contentType: string, thumbnailUrl?: string | null }> }
        | { __typename: 'MessageRetractedEvent' }
        | { __typename: 'NewDirectMessageNotificationEvent' }
        | { __typename: 'NotificationCreatedEvent' }
        | { __typename: 'NotificationDismissedEvent' }
        | { __typename: 'NotificationLevelChangedEvent' }
        | { __typename: 'PresenceChangedEvent' }
        | { __typename: 'ReactionAddedEvent' }
        | { __typename: 'ReactionRemovedEvent' }
        | { __typename: 'RoomArchivedEvent' }
        | { __typename: 'RoomCreatedEvent' }
        | { __typename: 'RoomDeletedEvent' }
        | { __typename: 'RoomGroupsUpdatedEvent' }
        | { __typename: 'RoomMarkedAsReadEvent' }
        | { __typename: 'RoomUnarchivedEvent' }
        | { __typename: 'RoomUpdatedEvent' }
        | { __typename: 'ServerConfigUpdatedEvent' }
        | { __typename: 'ServerMemberDeletedEvent' }
        | { __typename: 'ServerUpdatedEvent' }
        | { __typename: 'ServerUserPreferencesUpdatedEvent' }
        | { __typename: 'SessionTerminatedEvent' }
        | { __typename: 'ThreadCreatedEvent' }
        | { __typename: 'ThreadFollowChangedEvent' }
        | { __typename: 'UserCreatedEvent' }
        | { __typename: 'UserDeletedEvent' }
        | { __typename: 'UserJoinedRoomEvent' }
        | { __typename: 'UserLeftRoomEvent' }
        | { __typename: 'UserProfileUpdatedEvent' }
        | { __typename: 'UserTypingEvent' }
        | { __typename: 'VideoProcessingCompletedEvent' }
       } | null } | null };

export type QuickSwitcherServerQueryVariables = Exact<{ [key: string]: never; }>;


export type QuickSwitcherServerQuery = { __typename?: 'Query', server: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', name: string, logoUrl?: string | null } } };

export type QuickSwitcherRoomsQueryVariables = Exact<{ [key: string]: never; }>;


export type QuickSwitcherRoomsQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', id: string, rooms: Array<{ __typename?: 'Room', id: string, name: string, type: RoomType, members: { __typename?: 'RoomMembersConnection', users: Array<(
            { __typename?: 'User' }
            & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
          )> } }> } } | null };

export type UserAvatarUserFragment = { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } & { ' $fragmentName'?: 'UserAvatarUserFragment' };

export type ValidateSpaceAccessQueryVariables = Exact<{ [key: string]: never; }>;


export type ValidateSpaceAccessQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerHasAnyAdminPermission: boolean, viewerCanManageServer: boolean, viewerCanManageRooms: boolean, viewerCanManageRoles: boolean, viewerCanAssignRoles: boolean, profile: { __typename?: 'ServerProfile', name: string, bannerUrl?: string | null } } };

export type PostMessageMutationVariables = Exact<{
  input: PostMessageInput;
}>;


export type PostMessageMutation = { __typename?: 'Mutation', postMessage: { __typename?: 'Event', id: string } };

export type UpdateMessageFromInputMutationVariables = Exact<{
  input: UpdateMessageInput;
}>;


export type UpdateMessageFromInputMutation = { __typename?: 'Mutation', updateMessage: boolean };

export type LinkPreviewForComposerQueryVariables = Exact<{
  url: Scalars['String']['input'];
}>;


export type LinkPreviewForComposerQuery = { __typename?: 'Query', linkPreview?: (
    { __typename?: 'LinkPreview', imageAssetId?: string | null }
    & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
  ) | null };

export type MatrixTierRolesQueryVariables = Exact<{
  roomId?: InputMaybe<Scalars['ID']['input']>;
  groupId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type MatrixTierRolesQuery = { __typename?: 'Query', tierRoles?: { __typename?: 'TierRoles', applicablePermissions: Array<string>, roles: Array<{ __typename?: 'TierRole', roleName: string, displayName: string, description: string, isSystem: boolean, position: number, inheritedAllows: Array<string>, inheritedDenials: Array<string>, override: { __typename?: 'TierPermissions', permissions: Array<string>, permissionDenials: Array<string> } }> } | null };

export type RolePermissionsMatrixQueryQueryVariables = Exact<{
  roleName: Scalars['String']['input'];
}>;


export type RolePermissionsMatrixQueryQuery = { __typename?: 'Query', rolePermissionMatrix?: { __typename?: 'RolePermissionMatrix', roleName: string, applicablePermissions: Array<string>, scopes: Array<{ __typename?: 'UserPermissionScope', id: string, label: string, kind: UserPermissionScopeKind, parentGroupId: string }>, cells: Array<{ __typename?: 'UserPermissionCell', permission: string, scopeId: string, override: UserPermissionDecision, effective: UserPermissionDecision }> } | null };

export type UserPermissionsMatrixQueryQueryVariables = Exact<{
  userId: Scalars['ID']['input'];
}>;


export type UserPermissionsMatrixQueryQuery = { __typename?: 'Query', userPermissionMatrix?: { __typename?: 'UserPermissionMatrix', userId: string, applicablePermissions: Array<string>, scopes: Array<{ __typename?: 'UserPermissionScope', id: string, label: string, kind: UserPermissionScopeKind, parentGroupId: string }>, cells: Array<{ __typename?: 'UserPermissionCell', permission: string, scopeId: string, override: UserPermissionDecision, effective: UserPermissionDecision }> } | null };

export type MatrixGrantGroupPermMutationVariables = Exact<{
  input: GroupPermissionInput;
}>;


export type MatrixGrantGroupPermMutation = { __typename?: 'Mutation', grantGroupPermission: boolean };

export type MatrixDenyGroupPermMutationVariables = Exact<{
  input: GroupPermissionInput;
}>;


export type MatrixDenyGroupPermMutation = { __typename?: 'Mutation', denyGroupPermission: boolean };

export type MatrixClearGroupPermMutationVariables = Exact<{
  input: GroupPermissionInput;
}>;


export type MatrixClearGroupPermMutation = { __typename?: 'Mutation', clearGroupPermissionState: boolean };

export type MatrixGrantRoomPermMutationVariables = Exact<{
  input: GrantRoomPermissionInput;
}>;


export type MatrixGrantRoomPermMutation = { __typename?: 'Mutation', grantRoomPermission: boolean };

export type MatrixDenyRoomPermMutationVariables = Exact<{
  input: DenyRoomPermissionInput;
}>;


export type MatrixDenyRoomPermMutation = { __typename?: 'Mutation', denyRoomPermission: boolean };

export type MatrixClearRoomPermMutationVariables = Exact<{
  input: ClearRoomPermissionInput;
}>;


export type MatrixClearRoomPermMutation = { __typename?: 'Mutation', clearRoomPermission: boolean };

export type MatrixGrantServerPermMutationVariables = Exact<{
  input: GrantPermissionInput;
}>;


export type MatrixGrantServerPermMutation = { __typename?: 'Mutation', grantPermission: boolean };

export type MatrixDenyServerPermMutationVariables = Exact<{
  input: DenyPermissionInput;
}>;


export type MatrixDenyServerPermMutation = { __typename?: 'Mutation', denyPermission: boolean };

export type MatrixClearServerPermMutationVariables = Exact<{
  input: ClearPermissionStateInput;
}>;


export type MatrixClearServerPermMutation = { __typename?: 'Mutation', clearPermissionState: boolean };

export type MatrixGrantUserPermMutationVariables = Exact<{
  input: GrantUserPermissionInput;
}>;


export type MatrixGrantUserPermMutation = { __typename?: 'Mutation', grantUserPermission: boolean };

export type MatrixDenyUserPermMutationVariables = Exact<{
  input: DenyUserPermissionInput;
}>;


export type MatrixDenyUserPermMutation = { __typename?: 'Mutation', denyUserPermission: boolean };

export type MatrixClearUserPermMutationVariables = Exact<{
  input: ClearUserPermissionStateInput;
}>;


export type MatrixClearUserPermMutation = { __typename?: 'Mutation', clearUserPermissionState: boolean };

export type StartDmMutationVariables = Exact<{
  input: StartDmInput;
}>;


export type StartDmMutation = { __typename?: 'Mutation', startDM: { __typename?: 'Room', id: string } };

export type MyServerEventsSubscriptionVariables = Exact<{ [key: string]: never; }>;


export type MyServerEventsSubscription = { __typename?: 'Subscription', myEvents: { __typename?: 'Event', id: string, createdAt: any, actorId: string, actor?: (
      { __typename?: 'User' }
      & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
    ) | null, event:
      | { __typename: 'AssetDeletedEvent', assetId: string, deletedRoomId?: string | null }
      | { __typename: 'AssetProcessingFailedEvent', roomId: string, assetId: string, messageEventId: string }
      | { __typename: 'AssetProcessingStartedEvent', roomId: string, assetId: string, messageEventId: string }
      | { __typename: 'AssetProcessingSucceededEvent', roomId: string, assetId: string, messageEventId: string }
      | { __typename: 'CallParticipantJoinedEvent', roomId: string }
      | { __typename: 'CallParticipantLeftEvent', roomId: string }
      | { __typename: 'HeartbeatEvent', alive: boolean }
      | { __typename: 'MentionNotificationEvent', roomId: string, room: { __typename?: 'Room', name: string }, actor?: { __typename?: 'User', id: string, displayName: string } | null }
      | { __typename: 'MentionStatusClearedEvent' }
      | { __typename: 'MessageEditedEvent', roomId: string, messageEventId: string, body?: string | null, updatedAt?: any | null, attachments: Array<(
          { __typename?: 'Attachment' }
          & { ' $fragmentRefs'?: { 'MessageAttachmentViewFragment': MessageAttachmentViewFragment } }
        )>, linkPreview?: (
          { __typename?: 'LinkPreview' }
          & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
        ) | null }
      | { __typename: 'MessagePostedEvent', roomId: string, body?: string | null, updatedAt?: any | null, inReplyTo?: string | null, threadRootEventId?: string | null, echoOfEventId?: string | null, echoFromThreadRootEventId?: string | null, replyCount: number, lastReplyAt?: any | null, viewerIsFollowingThread?: boolean | null, attachments: Array<(
          { __typename?: 'Attachment' }
          & { ' $fragmentRefs'?: { 'MessageAttachmentViewFragment': MessageAttachmentViewFragment } }
        )>, linkPreview?: (
          { __typename?: 'LinkPreview' }
          & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
        ) | null, reactions: Array<{ __typename?: 'Reaction', emoji: string, count: number, hasReacted: boolean, users: Array<{ __typename?: 'User', id: string, displayName: string }> }>, threadParticipants: Array<(
          { __typename?: 'User' }
          & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
        )> }
      | { __typename: 'MessageRetractedEvent', roomId: string, messageEventId: string, retractedReason?: string | null }
      | { __typename: 'NewDirectMessageNotificationEvent', roomId: string, conversationName: string, sender?: { __typename?: 'User', id: string, displayName: string, avatarUrl?: string | null } | null }
      | { __typename: 'NotificationCreatedEvent', notificationId: string, roomId: string, eventId?: string | null, inReplyToId?: string | null }
      | { __typename: 'NotificationDismissedEvent', notificationId: string }
      | { __typename: 'NotificationLevelChangedEvent', level: NotificationLevel, effectiveLevel: NotificationLevel, nlcRoomId?: string | null }
      | { __typename: 'PresenceChangedEvent', status: PresenceStatus }
      | { __typename: 'ReactionAddedEvent', roomId: string, messageEventId: string, emoji: string }
      | { __typename: 'ReactionRemovedEvent', roomId: string, messageEventId: string, emoji: string }
      | { __typename: 'RoomArchivedEvent', roomId: string }
      | { __typename: 'RoomCreatedEvent', roomId: string }
      | { __typename: 'RoomDeletedEvent', roomId: string }
      | { __typename: 'RoomGroupsUpdatedEvent', changed: boolean }
      | { __typename: 'RoomMarkedAsReadEvent', roomId: string }
      | { __typename: 'RoomUnarchivedEvent', roomId: string }
      | { __typename: 'RoomUpdatedEvent', roomId: string }
      | { __typename: 'ServerConfigUpdatedEvent', serverName: string, motd?: string | null, welcomeMessage?: string | null }
      | { __typename: 'ServerMemberDeletedEvent', userId: string }
      | { __typename: 'ServerUpdatedEvent', name: string, description: string, logoUrl: string, bannerUrl: string }
      | { __typename: 'ServerUserPreferencesUpdatedEvent', timezone: string, timeFormat: TimeFormat }
      | { __typename: 'SessionTerminatedEvent', reason: string }
      | { __typename: 'ThreadCreatedEvent' }
      | { __typename: 'ThreadFollowChangedEvent', isFollowing: boolean, tfcRoomId: string, tfcThreadRootEventId: string }
      | { __typename: 'UserCreatedEvent' }
      | { __typename: 'UserDeletedEvent' }
      | { __typename: 'UserJoinedRoomEvent', roomId: string }
      | { __typename: 'UserLeftRoomEvent', roomId: string }
      | { __typename: 'UserProfileUpdatedEvent', userId: string, displayName: string, avatarUrl: string, login: string }
      | { __typename: 'UserTypingEvent', roomId: string, typingThreadRootEventId?: string | null }
      | { __typename: 'VideoProcessingCompletedEvent', roomId: string, attachmentId: string, messageEventId: string }
     } };

export type AddReactionFromActionsMutationVariables = Exact<{
  input: AddReactionInput;
}>;


export type AddReactionFromActionsMutation = { __typename?: 'Mutation', addReaction: boolean };

export type RemoveReactionFromActionsMutationVariables = Exact<{
  input: RemoveReactionInput;
}>;


export type RemoveReactionFromActionsMutation = { __typename?: 'Mutation', removeReaction: boolean };

export type GetRoomQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetRoomQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string, type: RoomType, viewerCanPostMessage: boolean, viewerCanPostInThread: boolean, viewerCanReact: boolean, viewerCanManageOthersMessage: boolean, viewerCanEchoMessage: boolean, viewerCanManageRoom: boolean, members: { __typename?: 'RoomMembersConnection', users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } } | null, server: { __typename?: 'Server', viewerCanManageRooms: boolean, profile: { __typename?: 'ServerProfile', name: string } } };

export type GetDmRoomMembersQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetDmRoomMembersQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, members: { __typename?: 'RoomMembersConnection', users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } } | null, viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', id: string } } | null };

export type GetRoomMembersForStoreQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetRoomMembersForStoreQuery = { __typename?: 'Query', room?: { __typename?: 'Room', members: { __typename?: 'RoomMembersConnection', users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } } | null };

export type MarkRoomAsReadMutationVariables = Exact<{
  input: MarkRoomAsReadInput;
}>;


export type MarkRoomAsReadMutation = { __typename?: 'Mutation', markRoomAsRead: { __typename?: 'MarkRoomAsReadResult', previousLastReadAt?: any | null, lastReadAt?: any | null } };

export type SendTypingIndicatorMutationVariables = Exact<{
  input: SendTypingIndicatorInput;
}>;


export type SendTypingIndicatorMutation = { __typename?: 'Mutation', sendTypingIndicator: boolean };

export type SubscribeToPushMutationVariables = Exact<{
  input: PushSubscriptionInput;
}>;


export type SubscribeToPushMutation = { __typename?: 'Mutation', subscribeToPush: boolean };

export type UnsubscribeFromPushMutationVariables = Exact<{
  input: UnsubscribeFromPushInput;
}>;


export type UnsubscribeFromPushMutation = { __typename?: 'Mutation', unsubscribeFromPush: boolean };

export type UpdateMyPresenceMutationVariables = Exact<{
  input: UpdateMyPresenceInput;
}>;


export type UpdateMyPresenceMutation = { __typename?: 'Mutation', updateMyPresence: boolean };

export type RoomMessagesLatestQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
}>;


export type RoomMessagesLatestQuery = { __typename?: 'Query', room?: { __typename?: 'Room', events: { __typename?: 'RoomEventsConnection', startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
        { __typename?: 'Event' }
        & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
      )> } } | null };

export type RoomMessagesBeforeQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  before?: InputMaybe<Scalars['String']['input']>;
}>;


export type RoomMessagesBeforeQuery = { __typename?: 'Query', room?: { __typename?: 'Room', events: { __typename?: 'RoomEventsConnection', startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
        { __typename?: 'Event' }
        & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
      )> } } | null };

export type RoomMessagesAfterQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  after?: InputMaybe<Scalars['String']['input']>;
}>;


export type RoomMessagesAfterQuery = { __typename?: 'Query', room?: { __typename?: 'Room', events: { __typename?: 'RoomEventsConnection', startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
        { __typename?: 'Event' }
        & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
      )> } } | null };

export type RoomMessagesAroundQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
}>;


export type RoomMessagesAroundQuery = { __typename?: 'Query', room?: { __typename?: 'Room', eventsAround: { __typename?: 'RoomEventsAroundResult', targetIndex: number, startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
        { __typename?: 'Event' }
        & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
      )> } } | null };

export type RoomMessagesRefetchOneQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type RoomMessagesRefetchOneQuery = { __typename?: 'Query', room?: { __typename?: 'Room', event?: (
      { __typename?: 'Event' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    ) | null } | null };

export type ThreadMessagesAllQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  threadRootEventId: Scalars['ID']['input'];
}>;


export type ThreadMessagesAllQuery = { __typename?: 'Query', room?: { __typename?: 'Room', event?: (
      { __typename?: 'Event', event:
        | { __typename?: 'AssetDeletedEvent' }
        | { __typename?: 'AssetProcessingFailedEvent' }
        | { __typename?: 'AssetProcessingStartedEvent' }
        | { __typename?: 'AssetProcessingSucceededEvent' }
        | { __typename?: 'CallParticipantJoinedEvent' }
        | { __typename?: 'CallParticipantLeftEvent' }
        | { __typename?: 'HeartbeatEvent' }
        | { __typename?: 'MentionNotificationEvent' }
        | { __typename?: 'MentionStatusClearedEvent' }
        | { __typename?: 'MessageEditedEvent' }
        | { __typename?: 'MessagePostedEvent', threadReplies: Array<(
            { __typename?: 'Event' }
            & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
          )> }
        | { __typename?: 'MessageRetractedEvent' }
        | { __typename?: 'NewDirectMessageNotificationEvent' }
        | { __typename?: 'NotificationCreatedEvent' }
        | { __typename?: 'NotificationDismissedEvent' }
        | { __typename?: 'NotificationLevelChangedEvent' }
        | { __typename?: 'PresenceChangedEvent' }
        | { __typename?: 'ReactionAddedEvent' }
        | { __typename?: 'ReactionRemovedEvent' }
        | { __typename?: 'RoomArchivedEvent' }
        | { __typename?: 'RoomCreatedEvent' }
        | { __typename?: 'RoomDeletedEvent' }
        | { __typename?: 'RoomGroupsUpdatedEvent' }
        | { __typename?: 'RoomMarkedAsReadEvent' }
        | { __typename?: 'RoomUnarchivedEvent' }
        | { __typename?: 'RoomUpdatedEvent' }
        | { __typename?: 'ServerConfigUpdatedEvent' }
        | { __typename?: 'ServerMemberDeletedEvent' }
        | { __typename?: 'ServerUpdatedEvent' }
        | { __typename?: 'ServerUserPreferencesUpdatedEvent' }
        | { __typename?: 'SessionTerminatedEvent' }
        | { __typename?: 'ThreadCreatedEvent' }
        | { __typename?: 'ThreadFollowChangedEvent' }
        | { __typename?: 'UserCreatedEvent' }
        | { __typename?: 'UserDeletedEvent' }
        | { __typename?: 'UserJoinedRoomEvent' }
        | { __typename?: 'UserLeftRoomEvent' }
        | { __typename?: 'UserProfileUpdatedEvent' }
        | { __typename?: 'UserTypingEvent' }
        | { __typename?: 'VideoProcessingCompletedEvent' }
       }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    ) | null } | null };

export type GetActiveCallRoomIdsQueryVariables = Exact<{ [key: string]: never; }>;


export type GetActiveCallRoomIdsQuery = { __typename?: 'Query', activeCallRoomIds: Array<string> };

export type GetSidebarCallParticipantsQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetSidebarCallParticipantsQuery = { __typename?: 'Query', room?: { __typename?: 'Room', callParticipants: Array<{ __typename?: 'CallParticipant', userId: string, displayName: string, login: string, avatarUrl?: string | null, joinedAt: number }> } | null };

export type GetCallParticipantsQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetCallParticipantsQuery = { __typename?: 'Query', room?: { __typename?: 'Room', callParticipants: Array<{ __typename?: 'CallParticipant', userId: string, displayName: string, login: string, avatarUrl?: string | null, joinedAt: number }> } | null };

export type NotificationsQueryVariables = Exact<{ [key: string]: never; }>;


export type NotificationsQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', notifications: { __typename?: 'NotificationsConnection', items: Array<
        | { __typename: 'DMMessageNotificationItem', id: string, createdAt: any, summary: string, actor?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } | null, room: { __typename?: 'Room', id: string } }
        | { __typename: 'MentionNotificationItem', id: string, createdAt: any, summary: string, mentionEventId: string, mentionInThread?: string | null, actor?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } | null, mentionRoom: { __typename?: 'Room', id: string, name: string } }
        | { __typename: 'ReplyNotificationItem', id: string, createdAt: any, summary: string, inReplyToId: string, replyEventId: string, replyInThread?: string | null, actor?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } | null, replyRoom: { __typename?: 'Room', id: string, name: string } }
        | { __typename: 'RoomMessageNotificationItem', id: string, createdAt: any, summary: string, roomMsgEventId: string, actor?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } | null, roomMsgRoom: { __typename?: 'Room', id: string, name: string } }
      > } } | null };

export type HasNotificationsQueryVariables = Exact<{ [key: string]: never; }>;


export type HasNotificationsQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', hasNotifications: boolean } | null };

export type NotificationInstanceNameQueryVariables = Exact<{ [key: string]: never; }>;


export type NotificationInstanceNameQuery = { __typename?: 'Query', server: { __typename?: 'Server', profile: { __typename?: 'ServerProfile', name: string } } };

export type DismissNotificationMutationVariables = Exact<{
  input: DismissNotificationInput;
}>;


export type DismissNotificationMutation = { __typename?: 'Mutation', dismissNotification: boolean };

export type DismissAllNotificationsMutationVariables = Exact<{ [key: string]: never; }>;


export type DismissAllNotificationsMutation = { __typename?: 'Mutation', dismissAllNotifications: number };

export type GetServerInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type GetServerInfoQuery = { __typename?: 'Query', server: { __typename?: 'Server', directRegistrationEnabled: boolean, profile: { __typename?: 'ServerProfile', name: string, welcomeMessage?: string | null, description?: string | null, logoUrl?: string | null, bannerUrl?: string | null } } };

export type GetAuthenticatedServerSettingsQueryVariables = Exact<{ [key: string]: never; }>;


export type GetAuthenticatedServerSettingsQuery = { __typename?: 'Query', server: { __typename?: 'Server', pushNotificationsEnabled: boolean, vapidPublicKey?: string | null, livekitUrl?: string | null, videoProcessingEnabled: boolean, maxUploadSize: number, maxVideoUploadSize: number, messageEditWindowSeconds: number, profile: { __typename?: 'ServerProfile', motd?: string | null } } };

export type GetVoiceCallTokenQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetVoiceCallTokenQuery = { __typename?: 'Query', room?: { __typename?: 'Room', voiceCallToken?: { __typename?: 'VoiceCallToken', token: string } | null } | null };

export type GetAllRoomsInSpaceQueryVariables = Exact<{ [key: string]: never; }>;


export type GetAllRoomsInSpaceQuery = { __typename?: 'Query', server: { __typename?: 'Server', rooms: Array<{ __typename?: 'Room', id: string, name: string, description?: string | null, archived: boolean, viewerCanJoinRoom: boolean }> } };

export type JoinRoomFromDirectoryMutationVariables = Exact<{
  input: JoinRoomInput;
}>;


export type JoinRoomFromDirectoryMutation = { __typename?: 'Mutation', joinRoom: { __typename?: 'Room', id: string } };

export type LeaveRoomFromDirectoryStoreMutationVariables = Exact<{
  input: LeaveRoomInput;
}>;


export type LeaveRoomFromDirectoryStoreMutation = { __typename?: 'Mutation', leaveRoom: boolean };

export type JoinGroupFromDirectoryMutationVariables = Exact<{
  input: JoinGroupInput;
}>;


export type JoinGroupFromDirectoryMutation = { __typename?: 'Mutation', joinGroup: Array<string> };

export type GetMyRoomsInSpaceQueryVariables = Exact<{ [key: string]: never; }>;


export type GetMyRoomsInSpaceQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', id: string, rooms: Array<{ __typename?: 'Room', id: string, name: string, type: RoomType, hasUnread: boolean, archived: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null, members: { __typename?: 'RoomMembersConnection', users: Array<(
            { __typename?: 'User' }
            & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
          )> } }> } } | null, server: { __typename?: 'Server', roomGroups: Array<{ __typename?: 'RoomGroup', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } };

export type LeaveRoomFromModalMutationVariables = Exact<{
  input: LeaveRoomInput;
}>;


export type LeaveRoomFromModalMutation = { __typename?: 'Mutation', leaveRoom: boolean };

export type DeleteMessageFromModalMutationVariables = Exact<{
  input: DeleteMessageInput;
}>;


export type DeleteMessageFromModalMutation = { __typename?: 'Mutation', deleteMessage: boolean };

export type DeleteLinkPreviewFromModalMutationVariables = Exact<{
  input: DeleteLinkPreviewInput;
}>;


export type DeleteLinkPreviewFromModalMutation = { __typename?: 'Mutation', deleteLinkPreview: boolean };

export type DeleteAttachmentFromModalMutationVariables = Exact<{
  input: DeleteAttachmentInput;
}>;


export type DeleteAttachmentFromModalMutation = { __typename?: 'Mutation', deleteAttachment: boolean };

export type MessageAttachmentViewFragment = { __typename?: 'Attachment', id: string, filename: string, contentType: string, width: number, height: number, assetUrl: { __typename?: 'AssetURL', url: string, expiresAt: any }, thumbnailAssetUrl?: { __typename?: 'AssetURL', url: string, expiresAt: any } | null, videoProcessing?: { __typename?: 'VideoProcessing', status: VideoProcessingStatus, durationMs?: any | null, width?: number | null, height?: number | null, sourceAvailable: boolean, reasonCode?: string | null, thumbnailAssetUrl?: { __typename?: 'AssetURL', url: string, expiresAt: any } | null, variants: Array<{ __typename?: 'VideoVariant', quality: string, width: number, height: number, size: any, assetUrl: { __typename?: 'AssetURL', url: string, expiresAt: any } }> } | null } & { ' $fragmentName'?: 'MessageAttachmentViewFragment' };

export type FollowThreadMutationVariables = Exact<{
  input: FollowThreadInput;
}>;


export type FollowThreadMutation = { __typename?: 'Mutation', followThread: boolean };

export type UnfollowThreadMutationVariables = Exact<{
  input: UnfollowThreadInput;
}>;


export type UnfollowThreadMutation = { __typename?: 'Mutation', unfollowThread: boolean };

export type ReplyPreviewQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type ReplyPreviewQuery = { __typename?: 'Query', room?: { __typename?: 'Room', event?: (
      { __typename?: 'Event' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    ) | null } | null };

export type AddReactionMutationVariables = Exact<{
  input: AddReactionInput;
}>;


export type AddReactionMutation = { __typename?: 'Mutation', addReaction: boolean };

export type RemoveReactionMutationVariables = Exact<{
  input: RemoveReactionInput;
}>;


export type RemoveReactionMutation = { __typename?: 'Mutation', removeReaction: boolean };

export type RoomEventViewFragment = { __typename?: 'Event', id: string, createdAt: any, actorId: string, actor?: (
    { __typename?: 'User' }
    & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
  ) | null, event:
    | { __typename: 'AssetDeletedEvent', assetId: string, deletedRoomId?: string | null }
    | { __typename: 'AssetProcessingFailedEvent', roomId: string, assetId: string, messageEventId: string }
    | { __typename: 'AssetProcessingStartedEvent', roomId: string, assetId: string, messageEventId: string }
    | { __typename: 'AssetProcessingSucceededEvent', roomId: string, assetId: string, messageEventId: string }
    | { __typename: 'CallParticipantJoinedEvent', roomId: string }
    | { __typename: 'CallParticipantLeftEvent', roomId: string }
    | { __typename: 'HeartbeatEvent' }
    | { __typename: 'MentionNotificationEvent' }
    | { __typename: 'MentionStatusClearedEvent' }
    | { __typename: 'MessageEditedEvent', roomId: string, messageEventId: string, body?: string | null, updatedAt?: any | null, attachments: Array<(
        { __typename?: 'Attachment' }
        & { ' $fragmentRefs'?: { 'MessageAttachmentViewFragment': MessageAttachmentViewFragment } }
      )>, linkPreview?: (
        { __typename?: 'LinkPreview' }
        & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
      ) | null }
    | { __typename: 'MessagePostedEvent', roomId: string, body?: string | null, updatedAt?: any | null, inReplyTo?: string | null, threadRootEventId?: string | null, echoOfEventId?: string | null, echoFromThreadRootEventId?: string | null, replyCount: number, lastReplyAt?: any | null, viewerIsFollowingThread?: boolean | null, attachments: Array<(
        { __typename?: 'Attachment' }
        & { ' $fragmentRefs'?: { 'MessageAttachmentViewFragment': MessageAttachmentViewFragment } }
      )>, linkPreview?: (
        { __typename?: 'LinkPreview' }
        & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
      ) | null, reactions: Array<{ __typename?: 'Reaction', emoji: string, count: number, hasReacted: boolean, users: Array<{ __typename?: 'User', id: string, displayName: string }> }>, threadParticipants: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> }
    | { __typename: 'MessageRetractedEvent', roomId: string, messageEventId: string, retractedReason?: string | null }
    | { __typename: 'NewDirectMessageNotificationEvent' }
    | { __typename: 'NotificationCreatedEvent' }
    | { __typename: 'NotificationDismissedEvent' }
    | { __typename: 'NotificationLevelChangedEvent' }
    | { __typename: 'PresenceChangedEvent', status: PresenceStatus }
    | { __typename: 'ReactionAddedEvent', roomId: string, messageEventId: string, emoji: string }
    | { __typename: 'ReactionRemovedEvent', roomId: string, messageEventId: string, emoji: string }
    | { __typename: 'RoomArchivedEvent', roomId: string }
    | { __typename: 'RoomCreatedEvent' }
    | { __typename: 'RoomDeletedEvent', roomId: string }
    | { __typename: 'RoomGroupsUpdatedEvent' }
    | { __typename: 'RoomMarkedAsReadEvent' }
    | { __typename: 'RoomUnarchivedEvent', roomId: string }
    | { __typename: 'RoomUpdatedEvent', roomId: string }
    | { __typename: 'ServerConfigUpdatedEvent' }
    | { __typename: 'ServerMemberDeletedEvent', userId: string }
    | { __typename: 'ServerUpdatedEvent' }
    | { __typename: 'ServerUserPreferencesUpdatedEvent' }
    | { __typename: 'SessionTerminatedEvent' }
    | { __typename: 'ThreadCreatedEvent' }
    | { __typename: 'ThreadFollowChangedEvent' }
    | { __typename: 'UserCreatedEvent' }
    | { __typename: 'UserDeletedEvent' }
    | { __typename: 'UserJoinedRoomEvent', roomId: string }
    | { __typename: 'UserLeftRoomEvent', roomId: string }
    | { __typename: 'UserProfileUpdatedEvent' }
    | { __typename: 'UserTypingEvent', roomId: string, typingThreadRootEventId?: string | null }
    | { __typename: 'VideoProcessingCompletedEvent', roomId: string, attachmentId: string, messageEventId: string }
   } & { ' $fragmentName'?: 'RoomEventViewFragment' };

export type FollowThreadFromPaneMutationVariables = Exact<{
  input: FollowThreadInput;
}>;


export type FollowThreadFromPaneMutation = { __typename?: 'Mutation', followThread: boolean };

export type UnfollowThreadFromPaneMutationVariables = Exact<{
  input: UnfollowThreadInput;
}>;


export type UnfollowThreadFromPaneMutation = { __typename?: 'Mutation', unfollowThread: boolean };

export type MarkThreadAsReadMutationVariables = Exact<{
  input: MarkThreadAsReadInput;
}>;


export type MarkThreadAsReadMutation = { __typename?: 'Mutation', markThreadAsRead: { __typename?: 'MarkThreadAsReadResult', previousReadAt?: any | null } };

export type ResolveMessageLinkQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type ResolveMessageLinkQuery = { __typename?: 'Query', room?: { __typename?: 'Room', event?: { __typename?: 'Event', id: string, event:
        | { __typename: 'AssetDeletedEvent' }
        | { __typename: 'AssetProcessingFailedEvent' }
        | { __typename: 'AssetProcessingStartedEvent' }
        | { __typename: 'AssetProcessingSucceededEvent' }
        | { __typename: 'CallParticipantJoinedEvent' }
        | { __typename: 'CallParticipantLeftEvent' }
        | { __typename: 'HeartbeatEvent' }
        | { __typename: 'MentionNotificationEvent' }
        | { __typename: 'MentionStatusClearedEvent' }
        | { __typename: 'MessageEditedEvent' }
        | { __typename: 'MessagePostedEvent', threadRootEventId?: string | null }
        | { __typename: 'MessageRetractedEvent' }
        | { __typename: 'NewDirectMessageNotificationEvent' }
        | { __typename: 'NotificationCreatedEvent' }
        | { __typename: 'NotificationDismissedEvent' }
        | { __typename: 'NotificationLevelChangedEvent' }
        | { __typename: 'PresenceChangedEvent' }
        | { __typename: 'ReactionAddedEvent' }
        | { __typename: 'ReactionRemovedEvent' }
        | { __typename: 'RoomArchivedEvent' }
        | { __typename: 'RoomCreatedEvent' }
        | { __typename: 'RoomDeletedEvent' }
        | { __typename: 'RoomGroupsUpdatedEvent' }
        | { __typename: 'RoomMarkedAsReadEvent' }
        | { __typename: 'RoomUnarchivedEvent' }
        | { __typename: 'RoomUpdatedEvent' }
        | { __typename: 'ServerConfigUpdatedEvent' }
        | { __typename: 'ServerMemberDeletedEvent' }
        | { __typename: 'ServerUpdatedEvent' }
        | { __typename: 'ServerUserPreferencesUpdatedEvent' }
        | { __typename: 'SessionTerminatedEvent' }
        | { __typename: 'ThreadCreatedEvent' }
        | { __typename: 'ThreadFollowChangedEvent' }
        | { __typename: 'UserCreatedEvent' }
        | { __typename: 'UserDeletedEvent' }
        | { __typename: 'UserJoinedRoomEvent' }
        | { __typename: 'UserLeftRoomEvent' }
        | { __typename: 'UserProfileUpdatedEvent' }
        | { __typename: 'UserTypingEvent' }
        | { __typename: 'VideoProcessingCompletedEvent' }
       } | null } | null };

export type GetSpaceNotificationPreferencesQueryVariables = Exact<{ [key: string]: never; }>;


export type GetSpaceNotificationPreferencesQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }, viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', rooms: Array<{ __typename?: 'Room', id: string, name: string, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> } } | null };

export type SetServerNotificationLevelMutationVariables = Exact<{
  input: SetServerNotificationLevelInput;
}>;


export type SetServerNotificationLevelMutation = { __typename?: 'Mutation', setServerNotificationLevel: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } };

export type SetRoomNotificationLevelMutationVariables = Exact<{
  input: SetRoomNotificationLevelInput;
}>;


export type SetRoomNotificationLevelMutation = { __typename?: 'Mutation', setRoomNotificationLevel: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } };

export type AdminEventLogQueryVariables = Exact<{
  limit?: InputMaybe<Scalars['Int']['input']>;
  before?: InputMaybe<Scalars['String']['input']>;
}>;


export type AdminEventLogQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', eventLog: { __typename?: 'EventLogConnection', hasOlder: boolean, endCursor?: string | null, totalCount: any, entries: Array<{ __typename?: 'EventLogEntry', sequence: string, subject: string, aggregateType: string, aggregateId: string, eventType: string, eventId: string, actorId: string, createdAt: any }> } } | null };

export type AdminEventLogEntryQueryVariables = Exact<{
  sequence: Scalars['String']['input'];
}>;


export type AdminEventLogEntryQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', eventLogEntry?: { __typename?: 'EventLogEntry', sequence: string, subject: string, aggregateType: string, aggregateId: string, eventType: string, eventId: string, actorId: string, createdAt: any, payloadJson: string } | null } | null };

export type SpaceMembersQueryVariables = Exact<{
  search?: InputMaybe<Scalars['String']['input']>;
}>;


export type SpaceMembersQuery = { __typename?: 'Query', server: { __typename?: 'Server', roles: Array<{ __typename?: 'Role', name: string, displayName: string }>, members: { __typename?: 'ServerMembersConnection', totalCount: number, users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, roles: Array<string>, createdAt?: any | null }> } } };

export type SpaceMemberDetailsQueryVariables = Exact<{
  userId: Scalars['ID']['input'];
}>;


export type SpaceMemberDetailsQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', id: string, roles: Array<string> } } | null, user?: { __typename?: 'User', lastLoginChange?: any | null } | null, server: { __typename?: 'Server', viewerCanAssignRoles: boolean, viewerCanManageRoles: boolean, availablePermissions: Array<string>, roles: Array<{ __typename?: 'Role', name: string, displayName: string, position: number, permissions: Array<string>, permissionDenials: Array<string> }>, member?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, roles: Array<string> } | null } };

export type AdminUpdateUserMutationVariables = Exact<{
  input: AdminUpdateUserInput;
}>;


export type AdminUpdateUserMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', updateUser: { __typename?: 'User', id: string, login: string, displayName: string } } | null };

export type AdminClearUsernameCooldownMutationVariables = Exact<{
  input: ClearUsernameCooldownInput;
}>;


export type AdminClearUsernameCooldownMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', clearUsernameCooldown: boolean } | null };

export type RevokeRoleFromMemberMutationVariables = Exact<{
  input: RevokeRoleInput;
}>;


export type RevokeRoleFromMemberMutation = { __typename?: 'Mutation', revokeRole: boolean };

export type AssignRoleToMemberMutationVariables = Exact<{
  input: AssignRoleInput;
}>;


export type AssignRoleToMemberMutation = { __typename?: 'Mutation', assignRole: boolean };

export type SpaceRolesGateQueryVariables = Exact<{ [key: string]: never; }>;


export type SpaceRolesGateQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerCanManageRoles: boolean } };

export type SpaceRoleDetailQueryVariables = Exact<{
  name: Scalars['String']['input'];
}>;


export type SpaceRoleDetailQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerCanManageRoles: boolean, viewerCanAssignRoles: boolean, role?: { __typename?: 'Role', name: string, displayName: string, description: string, permissions: Array<string>, permissionDenials: Array<string>, isSystem: boolean, position: number } | null, roleUsers: Array<{ __typename?: 'User', id: string, login: string, displayName: string }> } };

export type UpdateRoleDetailPageMutationVariables = Exact<{
  input: UpdateRoleInput;
}>;


export type UpdateRoleDetailPageMutation = { __typename?: 'Mutation', updateRole: { __typename?: 'Role', name: string, displayName: string, description: string } };

export type DeleteRoleDetailPageMutationVariables = Exact<{
  input: DeleteRoleInput;
}>;


export type DeleteRoleDetailPageMutation = { __typename?: 'Mutation', deleteRole: boolean };

export type SpaceRolesNewCheckQueryVariables = Exact<{ [key: string]: never; }>;


export type SpaceRolesNewCheckQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerCanManageRoles: boolean } };

export type CreateRoleNewPageMutationVariables = Exact<{
  input: CreateRoleInput;
}>;


export type CreateRoleNewPageMutation = { __typename?: 'Mutation', createRole: { __typename?: 'Role', name: string, displayName: string, description: string } };

export type AdminRoomGroupsQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminRoomGroupsQuery = { __typename?: 'Query', server: { __typename?: 'Server', rooms: Array<{ __typename?: 'Room', id: string, name: string, description?: string | null, archived: boolean }>, roomGroups: Array<{ __typename?: 'RoomGroup', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } };

export type AdminCreateRoomGroupMutationVariables = Exact<{
  input: CreateRoomGroupInput;
}>;


export type AdminCreateRoomGroupMutation = { __typename?: 'Mutation', createRoomGroup: { __typename?: 'RoomGroup', id: string, name: string } };

export type AdminUpdateRoomGroupMutationVariables = Exact<{
  input: UpdateRoomGroupInput;
}>;


export type AdminUpdateRoomGroupMutation = { __typename?: 'Mutation', updateRoomGroup: { __typename?: 'RoomGroup', id: string, name: string } };

export type AdminDeleteRoomGroupMutationVariables = Exact<{
  input: DeleteRoomGroupInput;
}>;


export type AdminDeleteRoomGroupMutation = { __typename?: 'Mutation', deleteRoomGroup: boolean };

export type AdminReorderRoomGroupsMutationVariables = Exact<{
  input: ReorderRoomGroupsInput;
}>;


export type AdminReorderRoomGroupsMutation = { __typename?: 'Mutation', reorderRoomGroups: Array<{ __typename?: 'RoomGroup', id: string }> };

export type AdminMoveRoomToSetMutationVariables = Exact<{
  input: MoveRoomToSetInput;
}>;


export type AdminMoveRoomToSetMutation = { __typename?: 'Mutation', moveRoomToSet: { __typename?: 'Room', id: string } };

export type AdminReorderRoomsInGroupMutationVariables = Exact<{
  input: ReorderRoomsInGroupInput;
}>;


export type AdminReorderRoomsInGroupMutation = { __typename?: 'Mutation', reorderRoomsInGroup: { __typename?: 'RoomGroup', id: string } };

export type AdminUpdateRoomMutationVariables = Exact<{
  input: UpdateRoomInput;
}>;


export type AdminUpdateRoomMutation = { __typename?: 'Mutation', updateRoom: { __typename?: 'Room', id: string, name: string, description?: string | null } };

export type ArchiveRoomMutationVariables = Exact<{
  input: ArchiveRoomInput;
}>;


export type ArchiveRoomMutation = { __typename?: 'Mutation', archiveRoom: { __typename?: 'Room', id: string, archived: boolean } };

export type UnarchiveRoomMutationVariables = Exact<{
  input: UnarchiveRoomInput;
}>;


export type UnarchiveRoomMutation = { __typename?: 'Mutation', unarchiveRoom: { __typename?: 'Room', id: string, archived: boolean } };

export type AdminGroupPermissionsNameQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminGroupPermissionsNameQuery = { __typename?: 'Query', server: { __typename?: 'Server', roomGroups: Array<{ __typename?: 'RoomGroup', id: string, name: string }> } };

export type AdminRoomPermissionsNameQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type AdminRoomPermissionsNameQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string } | null };

export type AdminSecurityConfigQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSecurityConfigQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', serverConfig: { __typename?: 'AdminServerConfig', blockedUsernames?: string | null } } | null };

export type UpdateSecurityConfigMutationVariables = Exact<{
  input: UpdateServerConfigInput;
}>;


export type UpdateSecurityConfigMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', updateServerConfig: { __typename?: 'AdminServerConfig', blockedUsernames?: string | null } } | null };

export type AdminSystemInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSystemInfoQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', systemInfo: { __typename?: 'SystemInfo', connection: { __typename?: 'ConnectionInfo', connected: boolean, serverId: string, serverName: string, version: string, maxPayload: any, rtt: string }, account: { __typename?: 'AccountInfo', memory: any, memoryUsed: any, storage: any, storageUsed: any, streams: number, streamsUsed: number, consumers: number, consumersUsed: number }, nats: { __typename?: 'NatsStats', totalMessages: any, totalBytes: any, totalConsumerPending: any, totalAckPending: number, streams: Array<{ __typename?: 'NatsStreamInfo', name: string, description: string, subjects: Array<string>, storage: string, messages: any, bytes: any, firstSequence: string, lastSequence: string, consumerCount: number, replicas: number, clusterLeader: string }>, consumers: Array<{ __typename?: 'NatsConsumerInfo', stream: string, name: string, durable: string, filterSubject: string, filterSubjects: Array<string>, ackPolicy: string, pullBased: boolean, pushBound: boolean, pending: any, ackPending: number, redelivered: number, waiting: number, deliveredConsumerSequence: string, deliveredStreamSequence: string, ackFloorConsumerSequence: string, ackFloorStreamSequence: string }> } }, projections: Array<{ __typename?: 'ProjectionState', name: string, subjects: Array<string>, started: boolean, lastAppliedSequence: string, matchingStreamSequence: string, streamLastSequence: string, lag: any, entryCount: any, estimatedBytes: any, averageEntryBytes: any }> } | null };

export type GetMyLastLoginChangeQueryVariables = Exact<{ [key: string]: never; }>;


export type GetMyLastLoginChangeQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', id: string, lastLoginChange?: any | null } } | null };

export type UploadAvatarMutationVariables = Exact<{
  input: UploadAvatarInput;
}>;


export type UploadAvatarMutation = { __typename?: 'Mutation', uploadAvatar: { __typename?: 'User', id: string, avatarUrl?: string | null } };

export type DeleteAvatarMutationVariables = Exact<{
  input: DeleteAvatarInput;
}>;


export type DeleteAvatarMutation = { __typename?: 'Mutation', deleteAvatar: { __typename?: 'User', id: string, avatarUrl?: string | null } };

export type UpdateProfileMutationVariables = Exact<{
  input: UpdateProfileInput;
}>;


export type UpdateProfileMutation = { __typename?: 'Mutation', updateProfile: { __typename?: 'User', id: string, displayName: string, login: string } };

export type AccountPermissionsQueryVariables = Exact<{ [key: string]: never; }>;


export type AccountPermissionsQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', user: { __typename?: 'User', viewerCanDeleteAccount: boolean } } | null };

export type RequestAccountDeletionMutationVariables = Exact<{ [key: string]: never; }>;


export type RequestAccountDeletionMutation = { __typename?: 'Mutation', requestAccountDeletion: string };

export type DeleteMyAccountMutationVariables = Exact<{
  input: DeleteMyAccountInput;
}>;


export type DeleteMyAccountMutation = { __typename?: 'Mutation', deleteMyAccount: boolean };

export type UpdateSettingsMutationVariables = Exact<{
  input: UpdateSettingsInput;
}>;


export type UpdateSettingsMutation = { __typename?: 'Mutation', updateSettings: { __typename?: 'UserSettings', timezone?: string | null, timeFormat: TimeFormat } };

export type MyFollowedThreadsQueryVariables = Exact<{ [key: string]: never; }>;


export type MyFollowedThreadsQuery = { __typename?: 'Query', viewer?: { __typename?: 'Viewer', followedThreads: { __typename?: 'FollowedThreadsConnection', threads: Array<{ __typename?: 'FollowedThread', roomId: string, threadRootEventId: string, replyCount: number, lastReplyAt?: any | null, hasUnread: boolean, room: { __typename?: 'Room', name: string }, rootMessage?: (
          { __typename?: 'Event' }
          & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
        ) | null, threadParticipants: Array<(
          { __typename?: 'User' }
          & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
        )> }> } } | null };

export type LoginPageInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type LoginPageInfoQuery = { __typename?: 'Query', server: { __typename?: 'Server', enabledAuthProviders: Array<string>, directRegistrationEnabled: boolean } };

export const UserAvatarUserFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<UserAvatarUserFragment, unknown>;
export const MessageAttachmentViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}}]} as unknown as DocumentNode<MessageAttachmentViewFragment, unknown>;
export const LinkPreviewViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<LinkPreviewViewFragment, unknown>;
export const RoomEventViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<RoomEventViewFragment, unknown>;
export const CreateRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateRoomMutation, CreateRoomMutationVariables>;
export const JoinRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<JoinRoomMutation, JoinRoomMutationVariables>;
export const ServerSettingsModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ServerSettingsModal"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageServer"}}]}}]}}]} as unknown as DocumentNode<ServerSettingsModalQuery, ServerSettingsModalQueryVariables>;
export const UpdateServerSettingsModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateServerSettingsModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateServerInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateServer"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateServerSettingsModalMutation, UpdateServerSettingsModalMutationVariables>;
export const UploadInstanceLogoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadInstanceLogo"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadServerLogoInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadServerLogo"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]}}]} as unknown as DocumentNode<UploadInstanceLogoMutation, UploadInstanceLogoMutationVariables>;
export const DeleteInstanceLogoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteInstanceLogo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteServerLogo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]}}]} as unknown as DocumentNode<DeleteInstanceLogoMutation, DeleteInstanceLogoMutationVariables>;
export const UploadInstanceBannerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadInstanceBanner"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadServerBannerInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadServerBanner"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]}}]} as unknown as DocumentNode<UploadInstanceBannerMutation, UploadInstanceBannerMutationVariables>;
export const DeleteInstanceBannerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteInstanceBanner"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteServerBanner"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]}}]} as unknown as DocumentNode<DeleteInstanceBannerMutation, DeleteInstanceBannerMutationVariables>;
export const InstanceInitDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InstanceInit"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerHasUnreadRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"DM"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomNotificationPreferences"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"canViewAdmin"}},{"kind":"Field","name":{"kind":"Name","value":"canStartDMs"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewUsers"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminManageUsers"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewRoles"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewSystem"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewAudit"}}]}}]}}]} as unknown as DocumentNode<InstanceInitQuery, InstanceInitQueryVariables>;
export const InstanceIconRefreshDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InstanceIconRefresh"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]}}]} as unknown as DocumentNode<InstanceIconRefreshQuery, InstanceIconRefreshQueryVariables>;
export const FirstUnreadRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FirstUnreadRoom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}}]}}]}}]}}]} as unknown as DocumentNode<FirstUnreadRoomQuery, FirstUnreadRoomQueryVariables>;
export const RefreshMessageAttachmentUrlsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RefreshMessageAttachmentUrls"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}}]}}]}}]}}]}}]}}]}}]}}]}}]} as unknown as DocumentNode<RefreshMessageAttachmentUrlsQuery, RefreshMessageAttachmentUrlsQueryVariables>;
export const LoadCurrentUserDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoadCurrentUser"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}},{"kind":"Field","name":{"kind":"Name","value":"hasVerifiedEmail"}},{"kind":"Field","name":{"kind":"Name","value":"settings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}}]}}]}}]}}]} as unknown as DocumentNode<LoadCurrentUserQuery, LoadCurrentUserQueryVariables>;
export const MessagePreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MessagePreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"120"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"120"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"COVER"}}]}]}}]}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<MessagePreviewQuery, MessagePreviewQueryVariables>;
export const QuickSwitcherServerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherServer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]}}]} as unknown as DocumentNode<QuickSwitcherServerQuery, QuickSwitcherServerQueryVariables>;
export const QuickSwitcherRoomsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherRooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<QuickSwitcherRoomsQuery, QuickSwitcherRoomsQueryVariables>;
export const ValidateSpaceAccessDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ValidateSpaceAccess"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerHasAnyAdminPermission"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageServer"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}}]}}]}}]} as unknown as DocumentNode<ValidateSpaceAccessQuery, ValidateSpaceAccessQueryVariables>;
export const PostMessageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PostMessage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PostMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"postMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<PostMessageMutation, PostMessageMutationVariables>;
export const UpdateMessageFromInputDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMessageFromInput"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UpdateMessageFromInputMutation, UpdateMessageFromInputMutationVariables>;
export const LinkPreviewForComposerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LinkPreviewForComposer"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"url"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"url"},"value":{"kind":"Variable","name":{"kind":"Name","value":"url"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}},{"kind":"Field","name":{"kind":"Name","value":"imageAssetId"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<LinkPreviewForComposerQuery, LinkPreviewForComposerQueryVariables>;
export const MatrixTierRolesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MatrixTierRoles"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"groupId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tierRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"groupId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"groupId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"applicablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roleName"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"override"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"inheritedAllows"}},{"kind":"Field","name":{"kind":"Name","value":"inheritedDenials"}}]}}]}}]}}]} as unknown as DocumentNode<MatrixTierRolesQuery, MatrixTierRolesQueryVariables>;
export const RolePermissionsMatrixQueryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RolePermissionsMatrixQuery"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roleName"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rolePermissionMatrix"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roleName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roleName"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roleName"}},{"kind":"Field","name":{"kind":"Name","value":"applicablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"scopes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"parentGroupId"}}]}},{"kind":"Field","name":{"kind":"Name","value":"cells"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permission"}},{"kind":"Field","name":{"kind":"Name","value":"scopeId"}},{"kind":"Field","name":{"kind":"Name","value":"override"}},{"kind":"Field","name":{"kind":"Name","value":"effective"}}]}}]}}]}}]} as unknown as DocumentNode<RolePermissionsMatrixQueryQuery, RolePermissionsMatrixQueryQueryVariables>;
export const UserPermissionsMatrixQueryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"UserPermissionsMatrixQuery"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userPermissionMatrix"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"applicablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"scopes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"parentGroupId"}}]}},{"kind":"Field","name":{"kind":"Name","value":"cells"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permission"}},{"kind":"Field","name":{"kind":"Name","value":"scopeId"}},{"kind":"Field","name":{"kind":"Name","value":"override"}},{"kind":"Field","name":{"kind":"Name","value":"effective"}}]}}]}}]}}]} as unknown as DocumentNode<UserPermissionsMatrixQueryQuery, UserPermissionsMatrixQueryQueryVariables>;
export const MatrixGrantGroupPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantGroupPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GroupPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantGroupPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantGroupPermMutation, MatrixGrantGroupPermMutationVariables>;
export const MatrixDenyGroupPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyGroupPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GroupPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyGroupPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyGroupPermMutation, MatrixDenyGroupPermMutationVariables>;
export const MatrixClearGroupPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearGroupPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GroupPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearGroupPermissionState"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearGroupPermMutation, MatrixClearGroupPermMutationVariables>;
export const MatrixGrantRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantRoomPermMutation, MatrixGrantRoomPermMutationVariables>;
export const MatrixDenyRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyRoomPermMutation, MatrixDenyRoomPermMutationVariables>;
export const MatrixClearRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearRoomPermMutation, MatrixClearRoomPermMutationVariables>;
export const MatrixGrantServerPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantServerPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantServerPermMutation, MatrixGrantServerPermMutationVariables>;
export const MatrixDenyServerPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyServerPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyServerPermMutation, MatrixDenyServerPermMutationVariables>;
export const MatrixClearServerPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearServerPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearPermissionStateInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearPermissionState"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearServerPermMutation, MatrixClearServerPermMutationVariables>;
export const MatrixGrantUserPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantUserPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantUserPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantUserPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantUserPermMutation, MatrixGrantUserPermMutationVariables>;
export const MatrixDenyUserPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyUserPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyUserPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyUserPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyUserPermMutation, MatrixDenyUserPermMutationVariables>;
export const MatrixClearUserPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearUserPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearUserPermissionStateInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearUserPermissionState"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearUserPermMutation, MatrixClearUserPermMutationVariables>;
export const StartDmDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartDM"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartDMInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startDM"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<StartDmMutation, StartDmMutationVariables>;
export const MyServerEventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"MyServerEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomCreatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerConfigUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserProfileUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"login"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerUserPreferencesUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationLevelChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"nlcRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MentionNotificationEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NewDirectMessageNotificationEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"sender"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"conversationName"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationCreatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notificationId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyToId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationDismissedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notificationId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomMarkedAsReadEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ThreadFollowChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"tfcRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"tfcThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"isFollowing"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomGroupsUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"changed"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionTerminatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HeartbeatEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"alive"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<MyServerEventsSubscription, MyServerEventsSubscriptionVariables>;
export const AddReactionFromActionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddReactionFromActions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AddReactionFromActionsMutation, AddReactionFromActionsMutationVariables>;
export const RemoveReactionFromActionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveReactionFromActions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RemoveReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RemoveReactionFromActionsMutation, RemoveReactionFromActionsMutationVariables>;
export const GetRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanPostMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanPostInThread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReact"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageOthersMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEchoMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoom"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<GetRoomQuery, GetRoomQueryVariables>;
export const GetDmRoomMembersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetDMRoomMembers"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]}}]} as unknown as DocumentNode<GetDmRoomMembersQuery, GetDmRoomMembersQueryVariables>;
export const GetRoomMembersForStoreDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoomMembersForStore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}}]}}]}}]} as unknown as DocumentNode<GetRoomMembersForStoreQuery, GetRoomMembersForStoreQueryVariables>;
export const MarkRoomAsReadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkRoomAsRead"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MarkRoomAsReadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markRoomAsRead"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previousLastReadAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastReadAt"}}]}}]}}]} as unknown as DocumentNode<MarkRoomAsReadMutation, MarkRoomAsReadMutationVariables>;
export const SendTypingIndicatorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SendTypingIndicator"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SendTypingIndicatorInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sendTypingIndicator"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SendTypingIndicatorMutation, SendTypingIndicatorMutationVariables>;
export const SubscribeToPushDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SubscribeToPush"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PushSubscriptionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"subscribeToPush"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SubscribeToPushMutation, SubscribeToPushMutationVariables>;
export const UnsubscribeFromPushDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnsubscribeFromPush"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnsubscribeFromPushInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unsubscribeFromPush"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnsubscribeFromPushMutation, UnsubscribeFromPushMutationVariables>;
export const UpdateMyPresenceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMyPresence"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMyPresenceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMyPresence"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UpdateMyPresenceMutation, UpdateMyPresenceMutationVariables>;
export const RoomMessagesLatestDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesLatest"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesLatestQuery, RoomMessagesLatestQueryVariables>;
export const RoomMessagesBeforeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesBefore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"before"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"before"},"value":{"kind":"Variable","name":{"kind":"Name","value":"before"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesBeforeQuery, RoomMessagesBeforeQueryVariables>;
export const RoomMessagesAfterDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesAfter"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesAfterQuery, RoomMessagesAfterQueryVariables>;
export const RoomMessagesAroundDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesAround"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventsAround"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"targetIndex"}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesAroundQuery, RoomMessagesAroundQueryVariables>;
export const RoomMessagesRefetchOneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesRefetchOne"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesRefetchOneQuery, RoomMessagesRefetchOneQueryVariables>;
export const ThreadMessagesAllDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ThreadMessagesAll"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"threadRootEventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"threadRootEventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"threadReplies"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ThreadMessagesAllQuery, ThreadMessagesAllQueryVariables>;
export const GetActiveCallRoomIdsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetActiveCallRoomIds"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"activeCallRoomIds"}}]}}]} as unknown as DocumentNode<GetActiveCallRoomIdsQuery, GetActiveCallRoomIdsQueryVariables>;
export const GetSidebarCallParticipantsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetSidebarCallParticipants"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callParticipants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]}}]} as unknown as DocumentNode<GetSidebarCallParticipantsQuery, GetSidebarCallParticipantsQueryVariables>;
export const GetCallParticipantsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetCallParticipants"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callParticipants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]}}]} as unknown as DocumentNode<GetCallParticipantsQuery, GetCallParticipantsQueryVariables>;
export const NotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Notifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notifications"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"50"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DMMessageNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MentionNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"mentionRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"mentionEventId"},"name":{"kind":"Name","value":"eventId"}},{"kind":"Field","alias":{"kind":"Name","value":"mentionInThread"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplyNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"replyRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"replyEventId"},"name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyToId"}},{"kind":"Field","alias":{"kind":"Name","value":"replyInThread"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomMessageNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgEventId"},"name":{"kind":"Name","value":"eventId"}}]}}]}}]}}]}}]}}]} as unknown as DocumentNode<NotificationsQuery, NotificationsQueryVariables>;
export const HasNotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"HasNotifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNotifications"}}]}}]}}]} as unknown as DocumentNode<HasNotificationsQuery, HasNotificationsQueryVariables>;
export const NotificationInstanceNameDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"NotificationInstanceName"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<NotificationInstanceNameQuery, NotificationInstanceNameQueryVariables>;
export const DismissNotificationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DismissNotification"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DismissNotificationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dismissNotification"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DismissNotificationMutation, DismissNotificationMutationVariables>;
export const DismissAllNotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DismissAllNotifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dismissAllNotifications"}}]}}]} as unknown as DocumentNode<DismissAllNotificationsMutation, DismissAllNotificationsMutationVariables>;
export const GetServerInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetServerInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"directRegistrationEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]}}]} as unknown as DocumentNode<GetServerInfoQuery, GetServerInfoQueryVariables>;
export const GetAuthenticatedServerSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetAuthenticatedServerSettings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"pushNotificationsEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"vapidPublicKey"}},{"kind":"Field","name":{"kind":"Name","value":"livekitUrl"}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessingEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"maxUploadSize"}},{"kind":"Field","name":{"kind":"Name","value":"maxVideoUploadSize"}},{"kind":"Field","name":{"kind":"Name","value":"messageEditWindowSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"motd"}}]}}]}}]}}]} as unknown as DocumentNode<GetAuthenticatedServerSettingsQuery, GetAuthenticatedServerSettingsQueryVariables>;
export const GetVoiceCallTokenDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetVoiceCallToken"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"voiceCallToken"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"token"}}]}}]}}]}}]} as unknown as DocumentNode<GetVoiceCallTokenQuery, GetVoiceCallTokenQueryVariables>;
export const GetAllRoomsInSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetAllRoomsInSpace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanJoinRoom"}}]}}]}}]}}]} as unknown as DocumentNode<GetAllRoomsInSpaceQuery, GetAllRoomsInSpaceQueryVariables>;
export const JoinRoomFromDirectoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinRoomFromDirectory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<JoinRoomFromDirectoryMutation, JoinRoomFromDirectoryMutationVariables>;
export const LeaveRoomFromDirectoryStoreDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveRoomFromDirectoryStore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveRoomFromDirectoryStoreMutation, LeaveRoomFromDirectoryStoreMutationVariables>;
export const JoinGroupFromDirectoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinGroupFromDirectory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinGroupInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinGroup"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinGroupFromDirectoryMutation, JoinGroupFromDirectoryMutationVariables>;
export const GetMyRoomsInSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMyRoomsInSpace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomGroups"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<GetMyRoomsInSpaceQuery, GetMyRoomsInSpaceQueryVariables>;
export const LeaveRoomFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveRoomFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveRoomFromModalMutation, LeaveRoomFromModalMutationVariables>;
export const DeleteMessageFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMessageFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteMessageFromModalMutation, DeleteMessageFromModalMutationVariables>;
export const DeleteLinkPreviewFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteLinkPreviewFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteLinkPreviewInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteLinkPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteLinkPreviewFromModalMutation, DeleteLinkPreviewFromModalMutationVariables>;
export const DeleteAttachmentFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteAttachmentFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteAttachmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteAttachment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteAttachmentFromModalMutation, DeleteAttachmentFromModalMutationVariables>;
export const FollowThreadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FollowThread"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"FollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<FollowThreadMutation, FollowThreadMutationVariables>;
export const UnfollowThreadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnfollowThread"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnfollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unfollowThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnfollowThreadMutation, UnfollowThreadMutationVariables>;
export const ReplyPreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ReplyPreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ReplyPreviewQuery, ReplyPreviewQueryVariables>;
export const AddReactionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddReaction"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AddReactionMutation, AddReactionMutationVariables>;
export const RemoveReactionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveReaction"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RemoveReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RemoveReactionMutation, RemoveReactionMutationVariables>;
export const FollowThreadFromPaneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FollowThreadFromPane"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"FollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<FollowThreadFromPaneMutation, FollowThreadFromPaneMutationVariables>;
export const UnfollowThreadFromPaneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnfollowThreadFromPane"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnfollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unfollowThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnfollowThreadFromPaneMutation, UnfollowThreadFromPaneMutationVariables>;
export const MarkThreadAsReadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkThreadAsRead"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MarkThreadAsReadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markThreadAsRead"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previousReadAt"}}]}}]}}]} as unknown as DocumentNode<MarkThreadAsReadMutation, MarkThreadAsReadMutationVariables>;
export const ResolveMessageLinkDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ResolveMessageLink"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}}]}}]}}]}}]}}]}}]} as unknown as DocumentNode<ResolveMessageLinkQuery, ResolveMessageLinkQueryVariables>;
export const GetSpaceNotificationPreferencesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetSpaceNotificationPreferences"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}}]}}]}}]} as unknown as DocumentNode<GetSpaceNotificationPreferencesQuery, GetSpaceNotificationPreferencesQueryVariables>;
export const SetServerNotificationLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetServerNotificationLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetServerNotificationLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setServerNotificationLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]} as unknown as DocumentNode<SetServerNotificationLevelMutation, SetServerNotificationLevelMutationVariables>;
export const SetRoomNotificationLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetRoomNotificationLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetRoomNotificationLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRoomNotificationLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]} as unknown as DocumentNode<SetRoomNotificationLevelMutation, SetRoomNotificationLevelMutationVariables>;
export const AdminEventLogDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminEventLog"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"before"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventLog"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"before"},"value":{"kind":"Variable","name":{"kind":"Name","value":"before"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"entries"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sequence"}},{"kind":"Field","name":{"kind":"Name","value":"subject"}},{"kind":"Field","name":{"kind":"Name","value":"aggregateType"}},{"kind":"Field","name":{"kind":"Name","value":"aggregateId"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]}}]} as unknown as DocumentNode<AdminEventLogQuery, AdminEventLogQueryVariables>;
export const AdminEventLogEntryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminEventLogEntry"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sequence"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventLogEntry"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sequence"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sequence"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sequence"}},{"kind":"Field","name":{"kind":"Name","value":"subject"}},{"kind":"Field","name":{"kind":"Name","value":"aggregateType"}},{"kind":"Field","name":{"kind":"Name","value":"aggregateId"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"payloadJson"}}]}}]}}]}}]} as unknown as DocumentNode<AdminEventLogEntryQuery, AdminEventLogEntryQueryVariables>;
export const SpaceMembersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceMembers"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"20"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"roles"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]}}]} as unknown as DocumentNode<SpaceMembersQuery, SpaceMembersQueryVariables>;
export const SpaceMemberDetailsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceMemberDetails"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"roles"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"user"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"availablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"member"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"roles"}}]}}]}}]}}]} as unknown as DocumentNode<SpaceMemberDetailsQuery, SpaceMemberDetailsQueryVariables>;
export const AdminUpdateUserDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateUser"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AdminUpdateUserInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateUser"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}}]}}]} as unknown as DocumentNode<AdminUpdateUserMutation, AdminUpdateUserMutationVariables>;
export const AdminClearUsernameCooldownDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminClearUsernameCooldown"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearUsernameCooldownInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearUsernameCooldown"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]}}]} as unknown as DocumentNode<AdminClearUsernameCooldownMutation, AdminClearUsernameCooldownMutationVariables>;
export const RevokeRoleFromMemberDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RevokeRoleFromMember"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RevokeRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revokeRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RevokeRoleFromMemberMutation, RevokeRoleFromMemberMutationVariables>;
export const AssignRoleToMemberDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AssignRoleToMember"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AssignRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assignRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AssignRoleToMemberMutation, AssignRoleToMemberMutationVariables>;
export const SpaceRolesGateDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRolesGate"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRolesGateQuery, SpaceRolesGateQueryVariables>;
export const SpaceRoleDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRoleDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"role"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}}]}},{"kind":"Field","name":{"kind":"Name","value":"roleUsers"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roleName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRoleDetailQuery, SpaceRoleDetailQueryVariables>;
export const UpdateRoleDetailPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRoleDetailPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateRoleDetailPageMutation, UpdateRoleDetailPageMutationVariables>;
export const DeleteRoleDetailPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteRoleDetailPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteRoleDetailPageMutation, DeleteRoleDetailPageMutationVariables>;
export const SpaceRolesNewCheckDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRolesNewCheck"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRolesNewCheckQuery, SpaceRolesNewCheckQueryVariables>;
export const CreateRoleNewPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRoleNewPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateRoleNewPageMutation, CreateRoleNewPageMutationVariables>;
export const AdminRoomGroupsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminRoomGroups"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}},{"kind":"Field","name":{"kind":"Name","value":"roomGroups"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AdminRoomGroupsQuery, AdminRoomGroupsQueryVariables>;
export const AdminCreateRoomGroupDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminCreateRoomGroup"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoomGroupInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRoomGroup"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<AdminCreateRoomGroupMutation, AdminCreateRoomGroupMutationVariables>;
export const AdminUpdateRoomGroupDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateRoomGroup"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomGroupInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoomGroup"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<AdminUpdateRoomGroupMutation, AdminUpdateRoomGroupMutationVariables>;
export const AdminDeleteRoomGroupDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminDeleteRoomGroup"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteRoomGroupInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRoomGroup"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AdminDeleteRoomGroupMutation, AdminDeleteRoomGroupMutationVariables>;
export const AdminReorderRoomGroupsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminReorderRoomGroups"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ReorderRoomGroupsInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reorderRoomGroups"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<AdminReorderRoomGroupsMutation, AdminReorderRoomGroupsMutationVariables>;
export const AdminMoveRoomToSetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminMoveRoomToSet"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MoveRoomToSetInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"moveRoomToSet"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<AdminMoveRoomToSetMutation, AdminMoveRoomToSetMutationVariables>;
export const AdminReorderRoomsInGroupDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminReorderRoomsInGroup"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ReorderRoomsInGroupInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reorderRoomsInGroup"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<AdminReorderRoomsInGroupMutation, AdminReorderRoomsInGroupMutationVariables>;
export const AdminUpdateRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<AdminUpdateRoomMutation, AdminUpdateRoomMutationVariables>;
export const ArchiveRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ArchiveRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ArchiveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"archiveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}}]}}]} as unknown as DocumentNode<ArchiveRoomMutation, ArchiveRoomMutationVariables>;
export const UnarchiveRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnarchiveRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnarchiveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unarchiveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}}]}}]} as unknown as DocumentNode<UnarchiveRoomMutation, UnarchiveRoomMutationVariables>;
export const AdminGroupPermissionsNameDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminGroupPermissionsName"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomGroups"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<AdminGroupPermissionsNameQuery, AdminGroupPermissionsNameQueryVariables>;
export const AdminRoomPermissionsNameDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminRoomPermissionsName"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<AdminRoomPermissionsNameQuery, AdminRoomPermissionsNameQueryVariables>;
export const AdminSecurityConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSecurityConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"blockedUsernames"}}]}}]}}]}}]} as unknown as DocumentNode<AdminSecurityConfigQuery, AdminSecurityConfigQueryVariables>;
export const UpdateSecurityConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateSecurityConfig"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateServerConfigInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateServerConfig"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"blockedUsernames"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateSecurityConfigMutation, UpdateSecurityConfigMutationVariables>;
export const AdminSystemInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSystemInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"systemInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"connected"}},{"kind":"Field","name":{"kind":"Name","value":"serverId"}},{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"maxPayload"}},{"kind":"Field","name":{"kind":"Name","value":"rtt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"account"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"memory"}},{"kind":"Field","name":{"kind":"Name","value":"memoryUsed"}},{"kind":"Field","name":{"kind":"Name","value":"storage"}},{"kind":"Field","name":{"kind":"Name","value":"storageUsed"}},{"kind":"Field","name":{"kind":"Name","value":"streams"}},{"kind":"Field","name":{"kind":"Name","value":"streamsUsed"}},{"kind":"Field","name":{"kind":"Name","value":"consumers"}},{"kind":"Field","name":{"kind":"Name","value":"consumersUsed"}}]}},{"kind":"Field","name":{"kind":"Name","value":"nats"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalMessages"}},{"kind":"Field","name":{"kind":"Name","value":"totalBytes"}},{"kind":"Field","name":{"kind":"Name","value":"totalConsumerPending"}},{"kind":"Field","name":{"kind":"Name","value":"totalAckPending"}},{"kind":"Field","name":{"kind":"Name","value":"streams"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"subjects"}},{"kind":"Field","name":{"kind":"Name","value":"storage"}},{"kind":"Field","name":{"kind":"Name","value":"messages"}},{"kind":"Field","name":{"kind":"Name","value":"bytes"}},{"kind":"Field","name":{"kind":"Name","value":"firstSequence"}},{"kind":"Field","name":{"kind":"Name","value":"lastSequence"}},{"kind":"Field","name":{"kind":"Name","value":"consumerCount"}},{"kind":"Field","name":{"kind":"Name","value":"replicas"}},{"kind":"Field","name":{"kind":"Name","value":"clusterLeader"}}]}},{"kind":"Field","name":{"kind":"Name","value":"consumers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"stream"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"durable"}},{"kind":"Field","name":{"kind":"Name","value":"filterSubject"}},{"kind":"Field","name":{"kind":"Name","value":"filterSubjects"}},{"kind":"Field","name":{"kind":"Name","value":"ackPolicy"}},{"kind":"Field","name":{"kind":"Name","value":"pullBased"}},{"kind":"Field","name":{"kind":"Name","value":"pushBound"}},{"kind":"Field","name":{"kind":"Name","value":"pending"}},{"kind":"Field","name":{"kind":"Name","value":"ackPending"}},{"kind":"Field","name":{"kind":"Name","value":"redelivered"}},{"kind":"Field","name":{"kind":"Name","value":"waiting"}},{"kind":"Field","name":{"kind":"Name","value":"deliveredConsumerSequence"}},{"kind":"Field","name":{"kind":"Name","value":"deliveredStreamSequence"}},{"kind":"Field","name":{"kind":"Name","value":"ackFloorConsumerSequence"}},{"kind":"Field","name":{"kind":"Name","value":"ackFloorStreamSequence"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"projections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"subjects"}},{"kind":"Field","name":{"kind":"Name","value":"started"}},{"kind":"Field","name":{"kind":"Name","value":"lastAppliedSequence"}},{"kind":"Field","name":{"kind":"Name","value":"matchingStreamSequence"}},{"kind":"Field","name":{"kind":"Name","value":"streamLastSequence"}},{"kind":"Field","name":{"kind":"Name","value":"lag"}},{"kind":"Field","name":{"kind":"Name","value":"entryCount"}},{"kind":"Field","name":{"kind":"Name","value":"estimatedBytes"}},{"kind":"Field","name":{"kind":"Name","value":"averageEntryBytes"}}]}}]}}]}}]} as unknown as DocumentNode<AdminSystemInfoQuery, AdminSystemInfoQueryVariables>;
export const GetMyLastLoginChangeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMyLastLoginChange"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}}]}}]}}]} as unknown as DocumentNode<GetMyLastLoginChangeQuery, GetMyLastLoginChangeQueryVariables>;
export const UploadAvatarDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadAvatar"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadAvatarInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadAvatar"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}}]}}]} as unknown as DocumentNode<UploadAvatarMutation, UploadAvatarMutationVariables>;
export const DeleteAvatarDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteAvatar"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteAvatarInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteAvatar"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}}]}}]} as unknown as DocumentNode<DeleteAvatarMutation, DeleteAvatarMutationVariables>;
export const UpdateProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateProfileInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}}]}}]}}]} as unknown as DocumentNode<UpdateProfileMutation, UpdateProfileMutationVariables>;
export const AccountPermissionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AccountPermissions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"user"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteAccount"}}]}}]}}]}}]} as unknown as DocumentNode<AccountPermissionsQuery, AccountPermissionsQueryVariables>;
export const RequestAccountDeletionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RequestAccountDeletion"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestAccountDeletion"}}]}}]} as unknown as DocumentNode<RequestAccountDeletionMutation, RequestAccountDeletionMutationVariables>;
export const DeleteMyAccountDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMyAccount"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteMyAccountInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMyAccount"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteMyAccountMutation, DeleteMyAccountMutationVariables>;
export const UpdateSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateSettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateSettingsInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateSettings"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}}]}}]} as unknown as DocumentNode<UpdateSettingsMutation, UpdateSettingsMutationVariables>;
export const MyFollowedThreadsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyFollowedThreads"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followedThreads"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"20"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"threads"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"rootMessage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"3"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailAssetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assetUrl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reasonCode"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Event"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageEditedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageRetractedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","alias":{"kind":"Name","value":"retractedReason"},"name":{"kind":"Name","value":"reason"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingStartedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingSucceededEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetProcessingFailedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AssetDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"deletedRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"assetId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<MyFollowedThreadsQuery, MyFollowedThreadsQueryVariables>;
export const LoginPageInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoginPageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabledAuthProviders"}},{"kind":"Field","name":{"kind":"Name","value":"directRegistrationEnabled"}}]}}]}}]} as unknown as DocumentNode<LoginPageInfoQuery, LoginPageInfoQueryVariables>;