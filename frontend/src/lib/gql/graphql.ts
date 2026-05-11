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

/** JetStream account limits and usage. */
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
  /** Reset server configuration to defaults. Returns true on success. */
  resetServerConfig: Scalars['Boolean']['output'];
  /** Update server configuration. Returns the updated config section. */
  updateServerConfig: AdminServerConfig;
  /** Update a user's login and/or display name. Bypasses the 30-day login change cooldown but otherwise reuses the same validation as updateMyProfile. */
  updateUser: User;
};


/** Admin mutations for configuration management. */
export type AdminMutationsClearUsernameCooldownArgs = {
  userId: Scalars['ID']['input'];
};


/** Admin mutations for configuration management. */
export type AdminMutationsUpdateServerConfigArgs = {
  input: UpdateServerConfigInput;
};


/** Admin mutations for configuration management. */
export type AdminMutationsUpdateUserArgs = {
  input: AdminUpdateUserInput;
};

/** Admin-only queries. Returns null if the user is not an server admin. */
export type AdminQueries = {
  __typename?: 'AdminQueries';
  /** Get a single server role by name. */
  role?: Maybe<Role>;
  /** Get users assigned to a specific server role. */
  roleUsers: Array<User>;
  /** List all server roles with their permissions. */
  roles: Array<Role>;
  /** Get server configuration. */
  serverConfig: AdminServerConfig;
  /** List all available server permission identifiers. */
  serverPermissions: Array<Scalars['String']['output']>;
  /** Get aggregate operational metrics (NATS/JetStream connection + account-level usage). */
  systemInfo: SystemInfo;
  /**
   * Get the permissions denied via roles for a user.
   * Used for UI to show when a permission is blocked via roles.
   */
  userRoleBasedDenials: Array<Scalars['String']['output']>;
  /** Get the role-based permissions for a user. */
  userRoleBasedPermissions: Array<Scalars['String']['output']>;
  /** Get server roles assigned to a specific user. */
  userRoles: Array<Scalars['String']['output']>;
};


/** Admin-only queries. Returns null if the user is not an server admin. */
export type AdminQueriesRoleArgs = {
  name: Scalars['String']['input'];
};


/** Admin-only queries. Returns null if the user is not an server admin. */
export type AdminQueriesRoleUsersArgs = {
  roleName: Scalars['String']['input'];
};


/** Admin-only queries. Returns null if the user is not an server admin. */
export type AdminQueriesUserRoleBasedDenialsArgs = {
  userId: Scalars['ID']['input'];
};


/** Admin-only queries. Returns null if the user is not an server admin. */
export type AdminQueriesUserRoleBasedPermissionsArgs = {
  userId: Scalars['ID']['input'];
};


/** Admin-only queries. Returns null if the user is not an server admin. */
export type AdminQueriesUserRolesArgs = {
  userId: Scalars['ID']['input'];
};

/** Server configuration section. */
export type AdminServerConfig = {
  __typename?: 'AdminServerConfig';
  /** Blocked usernames (newline-separated). Users cannot register with these names. */
  blockedUsernames?: Maybe<Scalars['String']['output']>;
  /** Short description of this server, used for OG link-preview metadata and the welcome card. */
  description?: Maybe<Scalars['String']['output']>;
  /** Whether this server has been configured (has settings in KV). */
  isConfigured: Scalars['Boolean']['output'];
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

/**
 * A participant currently in a voice call.
 * Sourced from the server-side CALL_STATE KV bucket (populated by LiveKit webhooks).
 */
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
 * The user who joined is identified by the parent RoomEvent's actorId/actor.
 */
export type CallParticipantJoinedEvent = {
  __typename?: 'CallParticipantJoinedEvent';
  /** The ID of the room where the call is happening. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: A user left a voice call in a room.
 * The user who left is identified by the parent RoomEvent's actorId/actor.
 */
export type CallParticipantLeftEvent = {
  __typename?: 'CallParticipantLeftEvent';
  /** The ID of the room where the call was happening. */
  roomId: Scalars['ID']['output'];
};

/** Input for clearing a room-level permission override. */
export type ClearRoomPermissionInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear the permission for. */
  role: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/** Input for clearing permission state on an server role. */
export type ClearServerPermissionStateInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear permission state for. */
  role: Scalars['String']['input'];
};

/** Information about the NATS connection. */
export type ConnectionInfo = {
  __typename?: 'ConnectionInfo';
  /** Whether the connection to NATS is currently active. */
  connected: Scalars['Boolean']['output'];
  /** Maximum message payload size in bytes. */
  maxPayload: Scalars['Int64']['output'];
  /** Round-trip time to the NATS server (e.g., '1.234ms'). */
  rtt: Scalars['String']['output'];
  /** Unique identifier of the connected NATS server. */
  serverID: Scalars['String']['output'];
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

/** Input for creating a new room. */
export type CreateRoomInput = {
  /** Optional description of the room's purpose. */
  description?: InputMaybe<Scalars['String']['input']>;
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
  actor: User;
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

/** Input for denying a room-level permission for a role. */
export type DenyRoomPermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  role: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/** Input for denying a permission for an server role. */
export type DenyServerPermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  role: Scalars['String']['input'];
};

/** Input for dismissing a notification. */
export type DismissNotificationInput = {
  /** The ID of the notification to dismiss. */
  notificationId: Scalars['ID']['input'];
};

/** Input for editing a message. */
export type EditMessageInput = {
  /** The new message content. */
  body: Scalars['String']['input'];
  /** The event ID of the message to edit. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
};

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
  rootMessage?: Maybe<RoomEvent>;
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

/** Input for granting a room-level permission to a role. */
export type GrantRoomPermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  role: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
};

/** Input for granting a permission to an server role. */
export type GrantServerPermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  role: Scalars['String']['input'];
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
 * the data in the postMessage mutation so the server stores it directly.
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
};

/** Result of marking a room as read. */
export type MarkRoomAsReadResult = {
  __typename?: 'MarkRoomAsReadResult';
  /** The timestamp of the last-read event (null if no messages in room). */
  lastReadAt?: Maybe<Scalars['Time']['output']>;
  /** The timestamp of the previously-read event (null if first time reading this room). */
  previousLastReadAt?: Maybe<Scalars['Time']['output']>;
};

/** Input for marking a thread as opened. */
export type MarkThreadAsOpenedInput = {
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['input'];
  /** The event ID of the thread root message. */
  threadRootEventId: Scalars['ID']['input'];
};

/** Result of marking a thread as opened. */
export type MarkThreadAsOpenedResult = {
  __typename?: 'MarkThreadAsOpenedResult';
  /** The timestamp when the thread was previously opened (null if never opened before). */
  previousOpenedAt?: Maybe<Scalars['Time']['output']>;
};

/**
 * Notification: A user was mentioned in a message.
 * This is a live-only notification event for toast displays.
 * Persistent state is tracked separately via Room.hasMention.
 */
export type MentionNotificationEvent = {
  __typename?: 'MentionNotificationEvent';
  /** The user who mentioned you. */
  actor: User;
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
  actor: User;
  /** When the notification was created */
  createdAt: Scalars['Time']['output'];
  /** Event ID of the message containing the mention */
  eventId: Scalars['ID']['output'];
  /** Unique notification ID */
  id: Scalars['ID']['output'];
  /** Thread root event ID if the mention is on a message inside a thread. Null for room-level messages. */
  inThread?: Maybe<Scalars['ID']['output']>;
  /** Room where the mention occurred */
  room: Room;
  /** Human-readable summary for display */
  summary: Scalars['String']['output'];
};

/** Event: A message was deleted */
export type MessageDeletedEvent = {
  __typename?: 'MessageDeletedEvent';
  /** The event ID of the message that was deleted. */
  messageEventId: Scalars['ID']['output'];
  /** The ID of the room where the message was deleted. */
  roomId: Scalars['ID']['output'];
};

/** Event: A message was posted */
export type MessagePostedEvent = {
  __typename?: 'MessagePostedEvent';
  /** Attachments for this message. Lazy-loaded from KV bucket on demand. */
  attachments: Array<Attachment>;
  /** The message content. Lazy-loaded from KV bucket on demand. Null if deleted. */
  body?: Maybe<Scalars['String']['output']>;
  /** The thread this echo originates from (null for non-echo messages). */
  echoFromThreadRootEventId?: Maybe<Scalars['ID']['output']>;
  /** Event ID of the original thread reply this echoes (null for non-echo messages). */
  echoOfEventId?: Maybe<Scalars['ID']['output']>;
  /** Sequence ID of the message this is replying to (null for top-level messages). */
  inReplyTo?: Maybe<Scalars['ID']['output']>;
  /** Sequence ID of the thread root message (null for top-level messages). For direct replies, equals inReplyTo. For nested replies, references the original root. */
  inThread?: Maybe<Scalars['ID']['output']>;
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
  /** When the message was last updated (null if never edited). Lazy-loaded from body. */
  updatedAt?: Maybe<Scalars['Time']['output']>;
  /** Whether the current viewer is following this thread. Null for non-root messages or messages without replies. */
  viewerIsFollowingThread?: Maybe<Scalars['Boolean']['output']>;
};


/** Event: A message was posted */
export type MessagePostedEventThreadParticipantsArgs = {
  first?: InputMaybe<Scalars['Int']['input']>;
};

/**
 * Event: A message was updated.
 * This is a live-only notification event — clients should refetch the affected message
 * to get the updated content. Only identifies which message changed.
 */
export type MessageUpdatedEvent = {
  __typename?: 'MessageUpdatedEvent';
  /** The event ID of the message that was updated. */
  messageEventId: Scalars['ID']['output'];
  /** The ID of the room where the message was updated. */
  roomId: Scalars['ID']['output'];
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
  /** Archive a room. Hides it from sidebar and Browse Rooms. Requires rooms.manage permission. */
  archiveRoom: Room;
  /**
   * Assign an server role to a user. Idempotent - assigning an already-assigned
   * role succeeds silently. Returns true on success.
   * Note: The 'everyone' role is implicit for all users and cannot be assigned.
   * Requires: admin.users.manage permission.
   * Errors: If role doesn't exist or is 'everyone'.
   */
  assignRole: Scalars['Boolean']['output'];
  /**
   * Clear room-level grant and denial for a permission on a role.
   * Returns the permission to neutral (inherit from server defaults).
   * Requires: admin.roles.manage permission.
   */
  clearRoomPermission: Scalars['Boolean']['output'];
  /**
   * Clear any grant or denial state for a permission on an server role, restoring neutral state.
   * Idempotent - clearing when no state exists succeeds silently. Returns true on success.
   * After clearing, this role neither grants nor denies the permission.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  clearServerPermissionState: Scalars['Boolean']['output'];
  /**
   * Create a new custom server role. Returns the created role with empty permissions.
   * System role names ('owner', 'admin', 'moderator', 'everyone') cannot be used.
   * Requires: admin.roles.manage permission.
   * Errors: If role name already exists or is a system role name.
   */
  createRole: Role;
  /** Create a new room. */
  createRoom: Room;
  /**
   * Delete an attachment from a message. Only the message author can delete their attachments.
   * Removes the attachment from the message and deletes the file from storage.
   * Returns true on success.
   */
  deleteAttachment: Scalars['Boolean']['output'];
  /**
   * Delete a link preview from a message. Only the message author can delete their link previews.
   * Returns true on success.
   */
  deleteLinkPreview: Scalars['Boolean']['output'];
  /**
   * Delete a message body for GDPR compliance.
   * The message event remains in the stream for audit trail, but the content is removed.
   * Requires either delete_any_message permission (moderator) or delete_own_message permission
   * and ownership of the message.
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
   * Delete the current user's avatar.
   * Returns the updated user.
   */
  deleteMyAvatar: User;
  /**
   * Delete a custom server role and all associated data. Returns true on success.
   * Deletes: role definition, all permission grants, and all user role assignments.
   * System roles ('owner', 'admin', 'moderator', 'everyone') cannot be deleted.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or is a system role.
   */
  deleteRole: Scalars['Boolean']['output'];
  /** Delete the server banner. Requires admin.instance.manage permission. */
  deleteServerBanner: Server;
  /** Delete the server logo. Requires admin.instance.manage permission. */
  deleteServerLogo: Server;
  /**
   * Deny a permission for a role at room level. Overrides server-level state for this room.
   * Clears any existing grant for the same permission in this room.
   * Requires: admin.roles.manage permission.
   */
  denyRoomPermission: Scalars['Boolean']['output'];
  /**
   * Deny a permission for an server role. Users with this role will be blocked from this
   * permission, regardless of what other roles grant it (deny-override pattern).
   * Clears any existing grant for the same permission. Returns true on success.
   * Note: Admin role is immune to role denials; denying a permission on admin has no effect.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  denyServerPermission: Scalars['Boolean']['output'];
  /** Dismiss all notifications for the current user. Returns count of dismissed notifications. */
  dismissAllNotifications: Scalars['Int']['output'];
  /** Dismiss a single notification. Returns true if it existed and was dismissed. */
  dismissNotification: Scalars['Boolean']['output'];
  /**
   * Edit a message body. Only the message author can edit their own messages,
   * within 3 hours of posting. The edit window may be configurable in the future.
   * Returns true on success.
   */
  editMessage: Scalars['Boolean']['output'];
  /** Follow a thread to receive notifications on new replies. Requires room membership. */
  followThread: Scalars['Boolean']['output'];
  /**
   * Grant a permission to a role at room level. Overrides server-level state for this room.
   * Clears any existing denial for the same permission in this room.
   * Requires: admin.roles.manage permission.
   */
  grantRoomPermission: Scalars['Boolean']['output'];
  /**
   * Grant a permission to an server role. Idempotent - granting an already-granted
   * permission succeeds silently. Returns true on success.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  grantServerPermission: Scalars['Boolean']['output'];
  /** Join the specified room. */
  joinRoom: Scalars['Boolean']['output'];
  /** Leave the specified room. */
  leaveRoom: Scalars['Boolean']['output'];
  /**
   * Mark a room as read for the current user.
   * Stores the room's current last root message event ID as the user's read marker.
   * Returns the timestamps of the new and previous last-read events.
   */
  markRoomAsRead: MarkRoomAsReadResult;
  /**
   * Mark a thread as opened by the current user.
   * Stores the current timestamp and returns the previous timestamp.
   * Used for showing unread separators in thread panes.
   */
  markThreadAsOpened: MarkThreadAsOpenedResult;
  /** Post a message to a room. Automatically marks the room as read since the user is viewing it. */
  postMessage: RoomEvent;
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
   * Requires: admin.roles.manage permission.
   * Returns: All server roles, sorted by position.
   */
  reorderRoles: Array<Role>;
  /**
   * Request account deletion by generating a confirmation token.
   * The token is valid for 15 minutes and must be passed to deleteMyAccount.
   * This two-step process protects against XSS attacks.
   * Returns the confirmation token.
   */
  requestAccountDeletion: Scalars['String']['output'];
  /**
   * Revoke an server role from a user. Idempotent - revoking a non-assigned
   * role succeeds silently. Returns true on success.
   * Note: Users cannot revoke their own admin role (prevents self-lockout).
   * Note: The 'everyone' role is implicit and cannot be revoked.
   * Requires: admin.users.manage permission.
   * Errors: If role doesn't exist, is 'everyone', or user tries to revoke own admin role.
   */
  revokeRole: Scalars['Boolean']['output'];
  /**
   * Revoke a permission grant from an server role. Idempotent - revoking a non-granted
   * permission succeeds silently. Returns true on success.
   * Note: This only removes grants, not denials. Use clearServerPermissionState to remove both.
   * Note: Admin role has all permissions implicitly; revoking from admin has no effect.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  revokeServerPermission: Scalars['Boolean']['output'];
  /**
   * Send a typing indicator to other users in the room.
   * This is a live-only event (not stored). Clients should call this every ~2 seconds
   * while typing and implement 6-second timeout-based clearing.
   * Returns true on success.
   */
  sendTypingIndicator: Scalars['Boolean']['output'];
  /** Set whether new members automatically join a room. Requires rooms.manage permission. */
  setRoomAutoJoin: Room;
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
  /** Unarchive a previously archived room. Requires rooms.manage permission. */
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
   * Update the current user's presence status.
   * Status persists until changed or the user disconnects (TTL expiry).
   * OFFLINE is not a valid input — to go offline, simply disconnect.
   */
  updateMyPresence: Scalars['Boolean']['output'];
  /**
   * Update the current user's profile.
   * Supports updating display name and/or login (username).
   * At least one field must be provided.
   * Login changes are subject to a 30-day cooldown.
   * Returns the updated user.
   */
  updateMyProfile: User;
  /** Update the current user's display settings. Returns the updated settings. */
  updateMySettings: UserSettings;
  /**
   * Update an server role's display name and description. Returns the updated role.
   * Role name cannot be changed after creation. System roles cannot be edited.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist.
   */
  updateRole: Role;
  /** Update an existing room's name and description. Requires rooms.manage permission. */
  updateRoom: Room;
  /** Update the room layout for the server. Requires room.manage permission. */
  updateRoomLayout: RoomLayout;
  /** Update the server's name. Requires admin.instance.manage permission. */
  updateServer: Server;
  /**
   * Upload an avatar for the current user.
   * Image will be resized to 256x256 max and converted to WebP.
   * Returns the updated user.
   */
  uploadMyAvatar: User;
  /** Upload a banner for the server. Requires admin.instance.manage permission. */
  uploadServerBanner: Server;
  /** Upload a logo for the server. Requires admin.instance.manage permission. */
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
export type MutationClearRoomPermissionArgs = {
  input: ClearRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationClearServerPermissionStateArgs = {
  input: ClearServerPermissionStateInput;
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
export type MutationDeleteAttachmentArgs = {
  input: DeleteAttachmentInput;
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
export type MutationDenyRoomPermissionArgs = {
  input: DenyRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyServerPermissionArgs = {
  input: DenyServerPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDismissNotificationArgs = {
  input: DismissNotificationInput;
};


/** Root mutation type for modifying data. */
export type MutationEditMessageArgs = {
  input: EditMessageInput;
};


/** Root mutation type for modifying data. */
export type MutationFollowThreadArgs = {
  input: FollowThreadInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantRoomPermissionArgs = {
  input: GrantRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantServerPermissionArgs = {
  input: GrantServerPermissionInput;
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
export type MutationMarkThreadAsOpenedArgs = {
  input: MarkThreadAsOpenedInput;
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
export type MutationRevokeRoleArgs = {
  input: RevokeRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokeServerPermissionArgs = {
  input: RevokeServerPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationSendTypingIndicatorArgs = {
  input: SendTypingIndicatorInput;
};


/** Root mutation type for modifying data. */
export type MutationSetRoomAutoJoinArgs = {
  input: SetRoomAutoJoinInput;
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
export type MutationUpdateMyPresenceArgs = {
  input: UpdateMyPresenceInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateMyProfileArgs = {
  input: UpdateMyProfileInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateMySettingsArgs = {
  input: UpdateUserSettingsInput;
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
export type MutationUpdateRoomLayoutArgs = {
  input: UpdateRoomLayoutInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateServerArgs = {
  input: UpdateServerInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadMyAvatarArgs = {
  input: UploadMyAvatarInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadServerBannerArgs = {
  input: UploadServerBannerInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadServerLogoArgs = {
  input: UploadServerLogoInput;
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
  sender: User;
};

/**
 * Event: A new message was posted on the server.
 * Published to all members (except the author) when a message is posted.
 * Enables real-time unread indicators in the sidebar without per-room subscriptions.
 */
export type NewMessageInServerEvent = {
  __typename?: 'NewMessageInServerEvent';
  /** The ID of the room where the message was posted. */
  roomId: Scalars['ID']['output'];
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
  /** Use inherited default (instance default for rooms, NORMAL for instance). */
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

/** The kind of decision a role contributed at a given level. */
export enum PermissionDecisionKind {
  /** The role's KV grants the permission. */
  Allow = 'ALLOW',
  /** The role's KV denies the permission. */
  Deny = 'DENY',
  /** Used only for overall State; the resolver found no allow or deny anywhere. */
  None = 'NONE'
}

/**
 * The complete explanation for one permission for one user at one scope.
 * Mirrors the algorithm of HasInstance/Space/RoomPermission: the first trace
 * entry is the winning decision; subsequent entries are also-saw context.
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
  /** Decision came from an server role acting in the server KV bucket. */
  Instance = 'INSTANCE',
  /** Decision came from a per-room override (objectId=roomId). */
  Room = 'ROOM',
  /** Decision came from a role acting at space scope (objectId='any'). */
  Space = 'SPACE'
}

/**
 * A single step in the permission resolution trace.
 * Only entries actually backed by a KV value are emitted (allow or deny);
 * roles with no entry at the level being checked are silent.
 */
export type PermissionTraceEntry = {
  __typename?: 'PermissionTraceEntry';
  /** Whether this entry is the winning decision (matches the trace head). */
  applied: Scalars['Boolean']['output'];
  /** Whether the role's KV said allow or deny at this level. */
  decision: PermissionDecisionKind;
  /** The level at which this decision was observed. */
  level: PermissionLevel;
  /** The role whose KV produced this decision. */
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
  /** Event ID of the thread root message. Determines thread membership and controls permission check (message.start_thread vs message.post_in_thread vs message.post). */
  inThread?: InputMaybe<Scalars['ID']['input']>;
  /** Link preview data from the composer. Server stores this directly without fetching. */
  linkPreview?: InputMaybe<LinkPreviewInput>;
  /** The ID of the room to post to. */
  roomId: Scalars['ID']['input'];
};

/**
 * Event: A user's presence status changed.
 * The user whose presence changed is identified by the parent RoomEvent's actorId/actor.
 * Presence is instance-wide.
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
   * Get the participants currently in a voice call in a room.
   * Returns empty list if no call is active or LiveKit is not configured.
   * Requires room membership.
   */
  callParticipants: Array<CallParticipant>;
  /** Check if the current user has any notifications (for bell icon indicator) */
  hasNotifications: Scalars['Boolean']['output'];
  /**
   * Check if the current user has any unread followed threads.
   * Lightweight query for sidebar unread indicators.
   * Requires server membership.
   */
  hasUnreadFollowedThreads: Scalars['Boolean']['output'];
  /**
   * Fetch link preview metadata for a URL.
   * Results are cached server-side. Returns null if the URL cannot be previewed.
   * Requires authentication.
   */
  linkPreview?: Maybe<LinkPreview>;
  /** Get the currently authenticated user. */
  me?: Maybe<User>;
  /**
   * List threads the current user is following on the server.
   * Returns threads sorted by last activity (newest first).
   * Requires server membership.
   */
  myFollowedThreads: Array<FollowedThread>;
  /** Get all notifications for the current user, newest first */
  notifications: Array<NotificationItem>;
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
  /** Fetch a single room event by event ID (O(1) subject lookup). Returns null if not found. */
  roomEventByEventId?: Maybe<RoomEvent>;
  /**
   * Fetch historical events for a specific room (default limit: 50). Use the
   * opaque `before` cursor for backward pagination and `after` for forward
   * pagination — pass the `startCursor` / `endCursor` from a previous
   * `RoomEventsConnection` response. Cursors are opaque strings; clients
   * must not attempt to parse them.
   */
  roomEvents: RoomEventsConnection;
  /**
   * Fetch room events centered around a specific event.
   * Returns a window of events with the target event roughly in the middle.
   * Used for "jump to message" when clicking reply links to messages not in the loaded range.
   */
  roomEventsAround: RoomEventsAroundResult;
  /** Get information about this Chatto server. No authentication required. */
  server: Server;
  /**
   * Fetch thread messages for a specific thread.
   * Returns the root message plus all replies in chronological order.
   */
  threadEvents: Array<RoomEvent>;
  /**
   * Return the full permission matrix at a tier: every applicable role
   * with its override and inherited baseline. Authorization mirrors
   * rolePermissions.
   */
  tierRoles?: Maybe<TierRoles>;
  /** Get a specific user by ID. */
  user?: Maybe<User>;
  /** Get a specific user by login. Returns null if not found. */
  userByLogin?: Maybe<User>;
  /** List all users on this server. Requires server admin. */
  users: Array<User>;
  /** The current authenticated user's server-level permissions. Null if not authenticated. */
  viewer?: Maybe<Viewer>;
  /**
   * Get a LiveKit join token for a voice call in a room.
   * Returns null if LiveKit is not configured on this server.
   * Requires room membership.
   */
  voiceCallToken?: Maybe<VoiceCallToken>;
};


/** Root query type for fetching data. */
export type QueryCallParticipantsArgs = {
  roomId: Scalars['ID']['input'];
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
export type QueryRolePermissionsArgs = {
  roleName: Scalars['String']['input'];
  roomId?: InputMaybe<Scalars['ID']['input']>;
};


/** Root query type for fetching data. */
export type QueryRoomArgs = {
  roomId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomEventByEventIdArgs = {
  eventId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomEventsArgs = {
  after?: InputMaybe<Scalars['String']['input']>;
  before?: InputMaybe<Scalars['String']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  roomId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomEventsAroundArgs = {
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  roomId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryThreadEventsArgs = {
  roomId: Scalars['ID']['input'];
  threadRootEventId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryTierRolesArgs = {
  roomId?: InputMaybe<Scalars['ID']['input']>;
};


/** Root query type for fetching data. */
export type QueryUserArgs = {
  id: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryUserByLoginArgs = {
  login: Scalars['String']['input'];
};


/** Root query type for fetching data. */
export type QueryVoiceCallTokenArgs = {
  roomId: Scalars['ID']['input'];
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
 * Notification for replies to your messages.
 * Created when someone replies to one of your messages.
 */
export type ReplyNotificationItem = {
  __typename?: 'ReplyNotificationItem';
  /** User who triggered the notification */
  actor: User;
  /** When the notification was created */
  createdAt: Scalars['Time']['output'];
  /** Event ID of the reply message */
  eventId: Scalars['ID']['output'];
  /** Unique notification ID */
  id: Scalars['ID']['output'];
  /** Event ID of your original message that was replied to */
  inReplyToId: Scalars['ID']['output'];
  /** Thread root event ID if this is a thread reply. Null for room-level replies. */
  inThread?: Maybe<Scalars['ID']['output']>;
  /** Room where the reply occurred */
  room: Room;
  /** Human-readable summary for display */
  summary: Scalars['String']['output'];
};

/** Input for revoking an server role from a user. */
export type RevokeRoleInput = {
  /** The name of the role to revoke. */
  roleName: Scalars['String']['input'];
  /** The ID of the user to revoke the role from. */
  userId: Scalars['ID']['input'];
};

/** Input for revoking a permission from an server role. */
export type RevokeServerPermissionInput = {
  /** The permission identifier to revoke. */
  permission: Scalars['String']['input'];
  /** The role to revoke the permission from. */
  role: Scalars['String']['input'];
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
  /** Hierarchy position: lower = higher rank. Owner=0, everyone=MAX_INT. */
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
  /** Hierarchy position; lower means higher rank. */
  position: Scalars['Int']['output'];
  /** Internal role name (e.g. 'admin', 'moderator'). */
  roleName: Scalars['String']['output'];
  /** Permission state at room scope (null when roomId not provided). */
  room?: Maybe<TierPermissions>;
  /** Permission state at server scope (the role's defaults everywhere). */
  server: TierPermissions;
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
  /** Hierarchy position (lower = higher rank) */
  position: Scalars['Int']['output'];
  /** Role identifier */
  roleName: Scalars['String']['output'];
};

/** A Room is a chat channel on the server where users can exchange messages. */
export type Room = {
  __typename?: 'Room';
  /** Whether this room is archived. Archived rooms are hidden from sidebar and Browse Rooms. */
  archived: Scalars['Boolean']['output'];
  /** Whether new server members automatically join this room. */
  autoJoin: Scalars['Boolean']['output'];
  /** Permissions configurable at room scope. */
  availableRoomPermissions: Array<Scalars['String']['output']>;
  /** Optional description of the room's purpose. */
  description?: Maybe<Scalars['String']['output']>;
  /**
   * Whether the current user has an unread @mention in this room.
   * Returns false if user is not a member or has no unread mentions.
   * More prominent than hasUnread - indicates user was directly addressed.
   */
  hasMention: Scalars['Boolean']['output'];
  /**
   * Whether the room has unread messages for the current user.
   * Returns false if user is not a member or room has no new messages.
   */
  hasUnread: Scalars['Boolean']['output'];
  /** The room's unique ID. */
  id: Scalars['ID']['output'];
  /** List of members in this room. */
  members: Array<User>;
  /** The room's name. Empty for DM rooms — clients derive the display name from `members`. */
  name: Scalars['String']['output'];
  /** Room-level permission overrides for all roles. */
  roomPermissionOverrides: Array<RoleRoomPermissions>;
  /** Kind of room — distinguishes regular channels from direct-message conversations. */
  type: RoomType;
  /** Whether the current user can delete any user's messages in this room. */
  viewerCanDeleteAnyMessage: Scalars['Boolean']['output'];
  /** Whether the current user can delete their own messages in this room. */
  viewerCanDeleteOwnMessage: Scalars['Boolean']['output'];
  /** Whether the current user can echo thread replies to the main channel. */
  viewerCanEchoMessage: Scalars['Boolean']['output'];
  /** Whether the current user can edit any user's messages in this room. */
  viewerCanEditAnyMessage: Scalars['Boolean']['output'];
  /** Whether the current user can edit their own messages in this room. */
  viewerCanEditOwnMessage: Scalars['Boolean']['output'];
  /** Whether the current user can join this room (has room.join permission). */
  viewerCanJoinRoom: Scalars['Boolean']['output'];
  /** Whether the current user can post messages in threads in this room. */
  viewerCanPostInThread: Scalars['Boolean']['output'];
  /** Whether the current user can post new root messages in this room. */
  viewerCanPostMessage: Scalars['Boolean']['output'];
  /** Whether the current user can add/remove reactions in this room. */
  viewerCanReact: Scalars['Boolean']['output'];
  /** Whether the current user can use reply attribution on room-level messages. */
  viewerCanReply: Scalars['Boolean']['output'];
  /** Whether the current user can use reply attribution on thread messages. */
  viewerCanReplyInThread: Scalars['Boolean']['output'];
  /** The current user's notification preference for this room. Null if not authenticated. */
  viewerNotificationPreference?: Maybe<ViewerNotificationPreference>;
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
 * RoomEvent wraps all room-scoped events.
 *
 * Events are either:
 * - Stored in JetStream streams — persisted events
 * - Published to NATS Core for real-time updates — live events
 *
 * Authorization is determined by NATS subject:
 * - Room events: room.{roomId}.> (JetStream) or live.room.{roomId}.> (NATS Core)
 */
export type RoomEvent = {
  __typename?: 'RoomEvent';
  /** The user who triggered this event. May be null if user was deleted. */
  actor?: Maybe<User>;
  /** The ID of the user who triggered this event. */
  actorId: Scalars['ID']['output'];
  /** When this event was created. */
  createdAt: Scalars['Time']['output'];
  /** The concrete event data. */
  event: RoomEventType;
  /** Universal event identifier. */
  id: Scalars['ID']['output'];
};

/** Union of all room-scoped event types (both persisted and live). */
export type RoomEventType = CallParticipantJoinedEvent | CallParticipantLeftEvent | MessageDeletedEvent | MessagePostedEvent | MessageUpdatedEvent | PresenceChangedEvent | ReactionAddedEvent | ReactionRemovedEvent | RoomArchivedEvent | RoomCreatedEvent | RoomDeletedEvent | RoomUnarchivedEvent | RoomUpdatedEvent | ServerMemberDeletedEvent | UserJoinedRoomEvent | UserLeftRoomEvent | UserTypingEvent | VideoProcessingCompletedEvent;

/**
 * Result of fetching events around a specific target event. `startCursor`
 * and `endCursor` are opaque pagination cursors usable on `roomEvents`.
 */
export type RoomEventsAroundResult = {
  __typename?: 'RoomEventsAroundResult';
  /** Opaque cursor of the last event in this window (null if empty). */
  endCursor?: Maybe<Scalars['String']['output']>;
  /** The events in the window, in chronological order. */
  events: Array<RoomEvent>;
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
 * cursors — pass them as `before` / `after` on a subsequent `roomEvents`
 * call. Both are null when `events` is empty.
 */
export type RoomEventsConnection = {
  __typename?: 'RoomEventsConnection';
  /** Opaque cursor of the last event in this page (null if empty). */
  endCursor?: Maybe<Scalars['String']['output']>;
  /** The events in chronological order. */
  events: Array<RoomEvent>;
  /** Whether there are newer events after this page. */
  hasNewer: Scalars['Boolean']['output'];
  /** Whether there are older events before this page. */
  hasOlder: Scalars['Boolean']['output'];
  /** Opaque cursor of the first event in this page (null if empty). */
  startCursor?: Maybe<Scalars['String']['output']>;
};

/**
 * The visual organization of rooms in the sidebar.
 * Defines sections and room ordering. Rooms not in any section appear as unsectioned.
 */
export type RoomLayout = {
  __typename?: 'RoomLayout';
  /** Ordered sections of rooms. */
  sections: Array<RoomLayoutSection>;
  /** Rooms not assigned to any section (appear at bottom of sidebar). */
  unsectioned: Array<Room>;
  /** Ordered list of unsectioned room IDs. When present, unsectioned rooms are displayed in this order. */
  unsectionedRoomIds: Array<Scalars['ID']['output']>;
};

/**
 * A named section of rooms in the sidebar layout.
 * Sections allow server admins to organize rooms into visual groups.
 */
export type RoomLayoutSection = {
  __typename?: 'RoomLayoutSection';
  /** Unique ID for this section. */
  id: Scalars['ID']['output'];
  /** Display name for this section (e.g., 'General', 'Projects'). */
  name: Scalars['String']['output'];
  /** Ordered list of rooms in this section. */
  rooms: Array<Room>;
};

/** Input for a room layout section. */
export type RoomLayoutSectionInput = {
  /** Section ID (use existing ID to update, or a new NanoID to create). */
  id: Scalars['ID']['input'];
  /** Display name for this section. */
  name: Scalars['String']['input'];
  /** Ordered list of room IDs in this section. */
  roomIds: Array<Scalars['ID']['input']>;
};

/**
 * Event: The room layout was updated.
 * Clients should refetch the room layout to get the new order/sections.
 * This is a live-only event (not stored in JetStream).
 */
export type RoomLayoutUpdatedEvent = {
  __typename?: 'RoomLayoutUpdatedEvent';
  /** Always true. Vestigial — clients only need the event arrival to trigger a refetch of the layout. */
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
 * Notification for a new message in a room (for users with ALL_MESSAGES level).
 * Created for every root message posted in a room the user is watching.
 */
export type RoomMessageNotificationItem = {
  __typename?: 'RoomMessageNotificationItem';
  /** User who posted the message. */
  actor: User;
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
  /** A direct-message conversation — derives its display name from its participants and uses fixed DM permissions instead of RBAC. */
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
  /** Runtime-editable configuration settings. */
  config: ServerConfig;
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
  /** ID of the deployment's server space. Internal migration bridge — frontend should treat this as opaque and prefer top-level Server fields. */
  primarySpaceId: Scalars['String']['output'];
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
  /** Room layout for the sidebar. Null if no custom layout is configured. */
  roomLayout?: Maybe<RoomLayout>;
  /**
   * List of rooms on this server.
   *
   * When `type` is null or `CHANNEL`, the result includes regular channels. When
   * `type` is null or `DM`, the caller's direct-message conversations are merged
   * in (subject to `dm.view`); the unified sidebar uses the null default to
   * render channels and DMs together. Pass `type: CHANNEL` for channels-only
   * consumers (e.g. the admin room-management UI); pass `type: DM` for DMs-only
   * consumers.
   */
  rooms: Array<Room>;
  /**
   * Get permissions denied for the user via their roles.
   * Used for UI to show when a permission is blocked via roles.
   */
  userRoleBasedDenials: Array<Scalars['String']['output']>;
  /**
   * Get permissions the user would have via roles.
   * Implements deny-override: if ANY role denies, permission is blocked regardless of grants.
   */
  userRoleBasedPermissions: Array<Scalars['String']['output']>;
  /** VAPID public key for Web Push subscriptions. Null if push is disabled. */
  vapidPublicKey?: Maybe<Scalars['String']['output']>;
  /** The application version. */
  version: Scalars['String']['output'];
  /** Whether the current user can assign roles to users (has admin.roles.assign permission). */
  viewerCanAssignRoles: Scalars['Boolean']['output'];
  /** Whether the current user can browse rooms (has rooms.browse permission). */
  viewerCanBrowseRooms: Scalars['Boolean']['output'];
  /** Whether the current user can create rooms (has rooms.create permission). */
  viewerCanCreateRoom: Scalars['Boolean']['output'];
  /** Whether the current user can invite new members (has admin.members.invite permission). */
  viewerCanInviteMembers: Scalars['Boolean']['output'];
  /** Whether the current user can manage this server (has admin.instance.manage permission). */
  viewerCanManageInstance: Scalars['Boolean']['output'];
  /** Whether the current user can manage roles (has admin.roles.manage permission). */
  viewerCanManageRoles: Scalars['Boolean']['output'];
  /** Whether the current user can manage rooms (has room.manage permission). */
  viewerCanManageRooms: Scalars['Boolean']['output'];
  /**
   * Check if the viewer can manage a specific user based on role hierarchy.
   * Returns true if the viewer's highest role outranks the target user's highest role.
   */
  viewerCanManageUser: Scalars['Boolean']['output'];
  /** Whether the current user has any admin.* permission (for showing the Admin link). */
  viewerHasAnyAdminPermission: Scalars['Boolean']['output'];
  /** Whether the current user has any unread messages in rooms they've joined. */
  viewerHasUnreadRooms: Scalars['Boolean']['output'];
  /** The current user's server-level notification preference. Null if not authenticated. */
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
export type ServerUserRoleBasedDenialsArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * Information about this Chatto server.
 * Some fields don't require authentication and are available on the login page.
 */
export type ServerUserRoleBasedPermissionsArgs = {
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
 * Runtime-editable server configuration.
 * These are settings that can be changed by admins at runtime.
 */
export type ServerConfig = {
  __typename?: 'ServerConfig';
  /** URL to the server banner image, if set. Pass width and height for a resized thumbnail. */
  bannerUrl?: Maybe<Scalars['String']['output']>;
  /** Short description of this server, used for OG link-preview metadata and the welcome card. Null if not configured. */
  description?: Maybe<Scalars['String']['output']>;
  /** URL to the server logo, if set. Pass width and height for a resized thumbnail. */
  logoUrl?: Maybe<Scalars['String']['output']>;
  /** Message of the Day, displayed in the header bar. Null if not configured. */
  motd?: Maybe<Scalars['String']['output']>;
  /** Server name, displayed in page titles. Defaults to 'Chatto'. */
  serverName: Scalars['String']['output'];
  /** Welcome message to display on the login screen (Markdown). Null if not configured. */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};


/**
 * Runtime-editable server configuration.
 * These are settings that can be changed by admins at runtime.
 */
export type ServerConfigBannerUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/**
 * Runtime-editable server configuration.
 * These are settings that can be changed by admins at runtime.
 */
export type ServerConfigLogoUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};

/**
 * Event: Server configuration was updated.
 * Clients should refetch instance info to get the new values.
 */
export type ServerConfigUpdatedEvent = {
  __typename?: 'ServerConfigUpdatedEvent';
  /** The updated blocked usernames (null if cleared). */
  blockedUsernames?: Maybe<Scalars['String']['output']>;
  /** The updated MOTD (null if cleared). */
  motd?: Maybe<Scalars['String']['output']>;
  /** The updated instance name. */
  serverName: Scalars['String']['output'];
  /** The updated welcome message (null if cleared). */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};

/**
 * ServerEvent wraps all server-scoped events.
 *
 * All server events are live-only (published to NATS Core, never persisted).
 * Authorization is determined by NATS subject:
 * - Server events: live.instance.{scope}.{id}.{eventType}
 */
export type ServerEvent = {
  __typename?: 'ServerEvent';
  /** The user who triggered this event. May be null if user was deleted. */
  actor?: Maybe<User>;
  /** The ID of the user who triggered this event. */
  actorId: Scalars['ID']['output'];
  /** When this event was created. */
  createdAt: Scalars['Time']['output'];
  /** The concrete event data. */
  event: ServerEventType;
  /** Universal event identifier. */
  id: Scalars['ID']['output'];
};

/** Union of all server-scoped event types. */
export type ServerEventType = MentionNotificationEvent | NewDirectMessageNotificationEvent | NewMessageInServerEvent | NotificationCreatedEvent | NotificationDismissedEvent | NotificationLevelChangedEvent | RoomLayoutUpdatedEvent | RoomMarkedAsReadEvent | ServerConfigUpdatedEvent | ServerUpdatedEvent | ServerUserPreferencesUpdatedEvent | SessionTerminatedEvent | ThreadFollowChangedEvent | UserCreatedEvent | UserDeletedEvent | UserJoinedServerEvent | UserLeftServerEvent | UserProfileUpdatedEvent;

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

/** Paginated list of instance members with metadata. */
export type ServerMembersConnection = {
  __typename?: 'ServerMembersConnection';
  /** Whether there are more members beyond this page. */
  hasMore: Scalars['Boolean']['output'];
  /** Total count of members matching the search (before pagination). */
  totalCount: Scalars['Int']['output'];
  /** The users who are members of this server. */
  users: Array<User>;
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
 * Published on logout or admin boot. The subscription stream closes after this event,
 * tearing down the WebSocket connection server-side.
 */
export type SessionTerminatedEvent = {
  __typename?: 'SessionTerminatedEvent';
  /** Why the session was terminated (logout, admin_boot, account_deleted). */
  reason: Scalars['String']['output'];
};

/** Input for setting whether new members automatically join a room. */
export type SetRoomAutoJoinInput = {
  /** Whether new members should automatically join this room. */
  autoJoin: Scalars['Boolean']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
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
   * Subscribe to server-level events, filtered for the current user.
   *
   * Requires authentication. Events are server-side filtered based on scope:
   *
   * **Config events** (instance name, MOTD, welcome message changes):
   * Delivered to all authenticated users.
   *
   * **User events** — two categories based on event type:
   * - Profile updates (avatar, display name): broadcast to all authenticated
   *   users, since user profiles are public.
   * - Private events (notification sync, preference changes, session
   *   termination, server membership changes): delivered only to the target user.
   *   These enable cross-tab/cross-device sync.
   *
   * **Server events** (server updated, new message indicators):
   * Delivered only to server members. For new-message-in-server events, an
   * additional room membership check ensures unread indicators only appear
   * for rooms the user has joined.
   *
   * **Side effects:**
   * - Subscribing sets the user's presence status to ONLINE.
   * - Presence is refreshed every 30s while active (60s TTL).
   * - When the subscription ends, presence expires via TTL.
   * - Receiving a session termination event closes the stream server-side.
   *
   * Only streams new events — no replay of historical events.
   */
  myInstanceEvents: ServerEvent;
  /**
   * Subscribe to events on this deployment, filtered for the current user.
   *
   * Sources from the single SERVER_EVENTS stream — channel and DM rooms flow
   * through the same subscription. Authentication is the only gate; per-event
   * authorization is applied server-side.
   *
   * **Room events** (messages, room creation/deletion, user join/leave):
   * Delivered only for rooms the user is a member of. The membership set is
   * tracked in real-time — when the user joins or leaves a room during the
   * subscription, filtering updates immediately without reconnecting.
   *
   * **Transient events** (reactions, message edits/deletes, typing indicators,
   * video processing, call participant changes):
   * Delivered only for rooms the user is a member of. These are live-only
   * (not stored in JetStream) and have no sequence ID. The user's own typing
   * indicators are suppressed server-side.
   *
   * **Server-level events** (member removed):
   * Delivered to all subscribers.
   *
   * **Presence changes** (online/offline/away/DND status):
   * Delivered for all authenticated users on the deployment.
   *
   * **DM-room events** are additionally gated by the `dm.view` permission;
   * users without it never receive them through this subscription.
   *
   * Only streams new events — no replay of historical events.
   */
  myServerEvents: RoomEvent;
};

/** Aggregate operational metrics. Intentionally excludes per-stream / per-bucket / per-object-store breakdowns: those leak structural information (room IDs, user IDs, bucket names) without serving an operator use case the chatto CLI doesn't already cover. */
export type SystemInfo = {
  __typename?: 'SystemInfo';
  /** JetStream account limits and usage (aggregate totals). */
  account: AccountInfo;
  /** NATS connection status and server info. */
  connection: ConnectionInfo;
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
  /** Hierarchy position; lower means higher rank. */
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
  /** 12-hour format (e.g., 2:30 PM). */
  TwelveHour = 'TWELVE_HOUR',
  /** 24-hour format (e.g., 14:30). */
  TwentyFourHour = 'TWENTY_FOUR_HOUR',
  /** Use browser/locale default. */
  Unspecified = 'UNSPECIFIED'
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

/** Input for updating the current user's presence status. */
export type UpdateMyPresenceInput = {
  /** The presence status to set. */
  status: PresenceStatus;
};

/** Input for updating the current user's profile. */
export type UpdateMyProfileInput = {
  /** New display name. Omit to leave unchanged. */
  displayName?: InputMaybe<Scalars['String']['input']>;
  /** New login/username. Omit to leave unchanged. Subject to 30-day cooldown. */
  login?: InputMaybe<Scalars['String']['input']>;
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

/** Input for updating an existing room. */
export type UpdateRoomInput = {
  /** The new description for the room. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** The new name for the room. */
  name: Scalars['String']['input'];
  /** The ID of the room to update. */
  roomId: Scalars['ID']['input'];
};

/** Input for updating the room layout. */
export type UpdateRoomLayoutInput = {
  /** The new layout sections in display order. */
  sections: Array<RoomLayoutSectionInput>;
  /** Ordered list of unsectioned room IDs. When provided, unsectioned rooms are displayed in this order. */
  unsectionedRoomIds?: InputMaybe<Array<Scalars['ID']['input']>>;
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
 * Input for updating user settings. All fields are optional.
 * Only provided fields will be updated; omitted fields are left unchanged.
 */
export type UpdateUserSettingsInput = {
  /** Time display format. Set to UNSPECIFIED to use browser locale default. */
  timeFormat?: InputMaybe<TimeFormat>;
  /** IANA timezone name. Set to null to clear (revert to browser default). */
  timezone?: InputMaybe<Scalars['String']['input']>;
};

/** Input for uploading a user avatar. */
export type UploadMyAvatarInput = {
  /** The avatar image file to upload. */
  file: Scalars['Upload']['input'];
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
  /** URL to the user's avatar image. Pass width and height for a resized thumbnail. */
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
  /** The user's verified email addresses. Only visible to admins and the user themselves. */
  verifiedEmails: Array<Scalars['String']['output']>;
  /** Whether the currently authenticated user can delete this account. */
  viewerCanDeleteAccount: Scalars['Boolean']['output'];
};


/** A Chatto User. */
export type UserAvatarUrlArgs = {
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

/**
 * Event: A user joined the server.
 * The actor is the user who joined.
 */
export type UserJoinedServerEvent = {
  __typename?: 'UserJoinedServerEvent';
  /** The ID of the user who joined (mirrors the parent ServerEvent.actorId). */
  userId: Scalars['ID']['output'];
};

/** Event: A user left a room */
export type UserLeftRoomEvent = {
  __typename?: 'UserLeftRoomEvent';
  /** The ID of the room the user left. */
  roomId: Scalars['ID']['output'];
};

/**
 * Event: A user left the server.
 * The actor is the user who left.
 */
export type UserLeftServerEvent = {
  __typename?: 'UserLeftServerEvent';
  /** The ID of the user who left (mirrors the parent ServerEvent.actorId). */
  userId: Scalars['ID']['output'];
};

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
 * Stored server-side so preferences persist across devices.
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
 * This is a live-only event (not stored in JetStream).
 * Clients should implement timeout-based clearing (e.g., 6 seconds of inactivity).
 * The user who is typing is identified by the parent RoomEvent's actorId/actor.
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
  /** Error message if processing failed. */
  errorMessage?: Maybe<Scalars['String']['output']>;
  /** Original video height in pixels. */
  height?: Maybe<Scalars['Int']['output']>;
  /** Current processing status. */
  status: VideoProcessingStatus;
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
  Completed = 'COMPLETED',
  Failed = 'FAILED',
  Pending = 'PENDING',
  Processing = 'PROCESSING'
}

/** A transcoded quality variant of a video. */
export type VideoVariant = {
  __typename?: 'VideoVariant';
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
 * The current authenticated user's server-level permissions.
 * Null if not authenticated.
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
  /** Whether the viewer can access the admin panel (includes config-admin check). */
  canViewAdmin: Scalars['Boolean']['output'];
  /** Whether the viewer can access direct messages. */
  canViewDMs: Scalars['Boolean']['output'];
  /** Whether the viewer can start DM conversations and send messages. */
  canWriteDMs: Scalars['Boolean']['output'];
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


export type JoinRoomMutation = { __typename?: 'Mutation', joinRoom: boolean };

export type ServerSettingsModalQueryVariables = Exact<{ [key: string]: never; }>;


export type ServerSettingsModalQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerCanManageInstance: boolean, config: { __typename?: 'ServerConfig', serverName: string, description?: string | null, motd?: string | null, welcomeMessage?: string | null, logoUrl?: string | null, bannerUrl?: string | null } } };

export type UpdateServerSettingsModalMutationVariables = Exact<{
  input: UpdateServerInput;
}>;


export type UpdateServerSettingsModalMutation = { __typename?: 'Mutation', updateServer: { __typename?: 'Server', config: { __typename?: 'ServerConfig', serverName: string, description?: string | null, motd?: string | null, welcomeMessage?: string | null } } };

export type UploadInstanceLogoMutationVariables = Exact<{
  input: UploadServerLogoInput;
}>;


export type UploadInstanceLogoMutation = { __typename?: 'Mutation', uploadServerLogo: { __typename?: 'Server', config: { __typename?: 'ServerConfig', logoUrl?: string | null } } };

export type DeleteInstanceLogoMutationVariables = Exact<{ [key: string]: never; }>;


export type DeleteInstanceLogoMutation = { __typename?: 'Mutation', deleteServerLogo: { __typename?: 'Server', config: { __typename?: 'ServerConfig', logoUrl?: string | null } } };

export type UploadInstanceBannerMutationVariables = Exact<{
  input: UploadServerBannerInput;
}>;


export type UploadInstanceBannerMutation = { __typename?: 'Mutation', uploadServerBanner: { __typename?: 'Server', config: { __typename?: 'ServerConfig', bannerUrl?: string | null } } };

export type DeleteInstanceBannerMutationVariables = Exact<{ [key: string]: never; }>;


export type DeleteInstanceBannerMutation = { __typename?: 'Mutation', deleteServerBanner: { __typename?: 'Server', config: { __typename?: 'ServerConfig', bannerUrl?: string | null } } };

export type InstanceInitQueryVariables = Exact<{ [key: string]: never; }>;


export type InstanceInitQuery = { __typename?: 'Query', server: { __typename?: 'Server', primarySpaceId: string, viewerHasUnreadRooms: boolean, config: { __typename?: 'ServerConfig', serverName: string, logoUrl?: string | null }, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null, rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> }, me?: { __typename?: 'User', roomNotificationPreferences: Array<{ __typename?: 'RoomNotificationPreferenceItem', roomId: string, level: NotificationLevel, effectiveLevel: NotificationLevel }> } | null, viewer?: { __typename?: 'Viewer', canViewAdmin: boolean, canViewDMs: boolean, canWriteDMs: boolean, canAdminViewUsers: boolean, canAdminManageUsers: boolean, canAdminViewRoles: boolean, canAdminManageRoles: boolean, canAdminViewSystem: boolean, canAdminViewAudit: boolean } | null };

export type InstanceIconRefreshQueryVariables = Exact<{ [key: string]: never; }>;


export type InstanceIconRefreshQuery = { __typename?: 'Query', server: { __typename?: 'Server', config: { __typename?: 'ServerConfig', serverName: string, logoUrl?: string | null } } };

export type FirstUnreadRoomQueryVariables = Exact<{ [key: string]: never; }>;


export type FirstUnreadRoomQuery = { __typename?: 'Query', server: { __typename?: 'Server', rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean }> } };

export type RoomSettingsDataQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type RoomSettingsDataQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string, description?: string | null } | null, server: { __typename?: 'Server', viewerCanManageRooms: boolean } };

export type UpdateRoomSettingsMutationVariables = Exact<{
  input: UpdateRoomInput;
}>;


export type UpdateRoomSettingsMutation = { __typename?: 'Mutation', updateRoom: { __typename?: 'Room', id: string, name: string, description?: string | null } };

export type LoadCurrentUserQueryVariables = Exact<{ [key: string]: never; }>;


export type LoadCurrentUserQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus, hasVerifiedEmail: boolean, settings?: { __typename?: 'UserSettings', timezone?: string | null, timeFormat: TimeFormat } | null } | null };

export type LinkPreviewViewFragment = { __typename?: 'LinkPreview', url: string, title?: string | null, description?: string | null, imageUrl?: string | null, siteName?: string | null, embedType?: string | null, embedId?: string | null } & { ' $fragmentName'?: 'LinkPreviewViewFragment' };

export type MessagePreviewQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type MessagePreviewQuery = { __typename?: 'Query', roomEventByEventId?: { __typename?: 'RoomEvent', id: string, createdAt: any, actor?: (
      { __typename?: 'User' }
      & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
    ) | null, event:
      | { __typename: 'CallParticipantJoinedEvent' }
      | { __typename: 'CallParticipantLeftEvent' }
      | { __typename: 'MessageDeletedEvent' }
      | { __typename: 'MessagePostedEvent', body?: string | null, attachments: Array<{ __typename?: 'Attachment', id: string, filename: string, contentType: string, thumbnailUrl?: string | null }> }
      | { __typename: 'MessageUpdatedEvent' }
      | { __typename: 'PresenceChangedEvent' }
      | { __typename: 'ReactionAddedEvent' }
      | { __typename: 'ReactionRemovedEvent' }
      | { __typename: 'RoomArchivedEvent' }
      | { __typename: 'RoomCreatedEvent' }
      | { __typename: 'RoomDeletedEvent' }
      | { __typename: 'RoomUnarchivedEvent' }
      | { __typename: 'RoomUpdatedEvent' }
      | { __typename: 'ServerMemberDeletedEvent' }
      | { __typename: 'UserJoinedRoomEvent' }
      | { __typename: 'UserLeftRoomEvent' }
      | { __typename: 'UserTypingEvent' }
      | { __typename: 'VideoProcessingCompletedEvent' }
     } | null, server: { __typename?: 'Server', config: { __typename?: 'ServerConfig', serverName: string } }, room?: { __typename?: 'Room', id: string, name: string } | null };

export type QuickSwitcherInstanceQueryVariables = Exact<{ [key: string]: never; }>;


export type QuickSwitcherInstanceQuery = { __typename?: 'Query', server: { __typename?: 'Server', primarySpaceId: string, config: { __typename?: 'ServerConfig', serverName: string, logoUrl?: string | null } } };

export type QuickSwitcherRoomsQueryVariables = Exact<{ [key: string]: never; }>;


export type QuickSwitcherRoomsQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, rooms: Array<{ __typename?: 'Room', id: string, name: string, type: RoomType, members: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> }> } | null };

export type UserAvatarUserFragment = { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } & { ' $fragmentName'?: 'UserAvatarUserFragment' };

export type PostMessageMutationVariables = Exact<{
  input: PostMessageInput;
}>;


export type PostMessageMutation = { __typename?: 'Mutation', postMessage: { __typename?: 'RoomEvent', id: string } };

export type EditMessageFromInputMutationVariables = Exact<{
  input: EditMessageInput;
}>;


export type EditMessageFromInputMutation = { __typename?: 'Mutation', editMessage: boolean };

export type LinkPreviewForComposerQueryVariables = Exact<{
  url: Scalars['String']['input'];
}>;


export type LinkPreviewForComposerQuery = { __typename?: 'Query', linkPreview?: (
    { __typename?: 'LinkPreview', imageAssetId?: string | null }
    & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
  ) | null };

export type PermissionInspectorQueryVariables = Exact<{
  userId: Scalars['ID']['input'];
  roomId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type PermissionInspectorQuery = { __typename?: 'Query', permissionExplanation: Array<{ __typename?: 'PermissionExplanation', permission: string, state: PermissionDecisionKind, decidedAt?: PermissionLevel | null, decidedByRole?: string | null, trace: Array<{ __typename?: 'PermissionTraceEntry', level: PermissionLevel, roleName: string, decision: PermissionDecisionKind, applied: boolean }> }> };

export type MatrixTierRolesQueryVariables = Exact<{
  roomId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type MatrixTierRolesQuery = { __typename?: 'Query', tierRoles?: { __typename?: 'TierRoles', applicablePermissions: Array<string>, roles: Array<{ __typename?: 'TierRole', roleName: string, displayName: string, description: string, isSystem: boolean, position: number, inheritedAllows: Array<string>, inheritedDenials: Array<string>, override: { __typename?: 'TierPermissions', permissions: Array<string>, permissionDenials: Array<string> } }> } | null };

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
  input: GrantServerPermissionInput;
}>;


export type MatrixGrantServerPermMutation = { __typename?: 'Mutation', grantServerPermission: boolean };

export type MatrixDenyServerPermMutationVariables = Exact<{
  input: DenyServerPermissionInput;
}>;


export type MatrixDenyServerPermMutation = { __typename?: 'Mutation', denyServerPermission: boolean };

export type MatrixClearServerPermMutationVariables = Exact<{
  input: ClearServerPermissionStateInput;
}>;


export type MatrixClearServerPermMutation = { __typename?: 'Mutation', clearServerPermissionState: boolean };

export type StartDmMutationVariables = Exact<{
  input: StartDmInput;
}>;


export type StartDmMutation = { __typename?: 'Mutation', startDM: { __typename?: 'Room', id: string } };

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


export type GetRoomQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string, type: RoomType, viewerCanPostMessage: boolean, viewerCanPostInThread: boolean, viewerCanReply: boolean, viewerCanReplyInThread: boolean, viewerCanReact: boolean, viewerCanEditOwnMessage: boolean, viewerCanEditAnyMessage: boolean, viewerCanDeleteOwnMessage: boolean, viewerCanDeleteAnyMessage: boolean, viewerCanEchoMessage: boolean, members: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } | null, server: { __typename?: 'Server', viewerCanManageRooms: boolean, config: { __typename?: 'ServerConfig', serverName: string } } };

export type GetDmRoomMembersQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetDmRoomMembersQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, members: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } | null, me?: { __typename?: 'User', id: string } | null };

export type GetRoomMembersForStoreQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetRoomMembersForStoreQuery = { __typename?: 'Query', room?: { __typename?: 'Room', members: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } | null };

export type MarkRoomAsReadMutationVariables = Exact<{
  input: MarkRoomAsReadInput;
}>;


export type MarkRoomAsReadMutation = { __typename?: 'Mutation', markRoomAsRead: { __typename?: 'MarkRoomAsReadResult', previousLastReadAt?: any | null, lastReadAt?: any | null } };

export type SendTypingIndicatorMutationVariables = Exact<{
  input: SendTypingIndicatorInput;
}>;


export type SendTypingIndicatorMutation = { __typename?: 'Mutation', sendTypingIndicator: boolean };

export type MyInstanceEventsSubscriptionVariables = Exact<{ [key: string]: never; }>;


export type MyInstanceEventsSubscription = { __typename?: 'Subscription', myInstanceEvents: { __typename?: 'ServerEvent', actorId: string, event:
      | { __typename: 'MentionNotificationEvent', roomId: string, room: { __typename?: 'Room', name: string }, actor: { __typename?: 'User', id: string, displayName: string } }
      | { __typename: 'NewDirectMessageNotificationEvent', roomId: string, conversationName: string, sender: { __typename?: 'User', id: string, displayName: string, avatarUrl?: string | null } }
      | { __typename: 'NewMessageInServerEvent', roomId: string }
      | { __typename: 'NotificationCreatedEvent', notificationId: string, roomId: string, eventId?: string | null, inReplyToId?: string | null }
      | { __typename: 'NotificationDismissedEvent', notificationId: string }
      | { __typename: 'NotificationLevelChangedEvent', level: NotificationLevel, effectiveLevel: NotificationLevel, nlcRoomId?: string | null }
      | { __typename: 'RoomLayoutUpdatedEvent', changed: boolean }
      | { __typename: 'RoomMarkedAsReadEvent', roomId: string }
      | { __typename: 'ServerConfigUpdatedEvent', serverName: string, motd?: string | null, welcomeMessage?: string | null }
      | { __typename: 'ServerUpdatedEvent', name: string, description: string, logoUrl: string, bannerUrl: string }
      | { __typename: 'ServerUserPreferencesUpdatedEvent', timezone: string, timeFormat: TimeFormat }
      | { __typename: 'SessionTerminatedEvent', reason: string }
      | { __typename: 'ThreadFollowChangedEvent', threadRootEventId: string, isFollowing: boolean, tfcRoomId: string }
      | { __typename: 'UserCreatedEvent' }
      | { __typename: 'UserDeletedEvent' }
      | { __typename: 'UserJoinedServerEvent', userId: string }
      | { __typename: 'UserLeftServerEvent', userId: string }
      | { __typename: 'UserProfileUpdatedEvent', userId: string, displayName: string, avatarUrl: string, login: string }
     } };

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

export type ServerEventBusSubscriptionSubscriptionVariables = Exact<{ [key: string]: never; }>;


export type ServerEventBusSubscriptionSubscription = { __typename?: 'Subscription', myServerEvents: (
    { __typename?: 'RoomEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  ) };

export type GetActiveCallRoomIdsQueryVariables = Exact<{ [key: string]: never; }>;


export type GetActiveCallRoomIdsQuery = { __typename?: 'Query', activeCallRoomIds: Array<string> };

export type GetSidebarCallParticipantsQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetSidebarCallParticipantsQuery = { __typename?: 'Query', callParticipants: Array<{ __typename?: 'CallParticipant', userId: string, displayName: string, login: string, avatarUrl?: string | null, joinedAt: number }> };

export type GetCallParticipantsQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetCallParticipantsQuery = { __typename?: 'Query', callParticipants: Array<{ __typename?: 'CallParticipant', userId: string, displayName: string, login: string, avatarUrl?: string | null, joinedAt: number }> };

export type NotificationsQueryVariables = Exact<{ [key: string]: never; }>;


export type NotificationsQuery = { __typename?: 'Query', notifications: Array<
    | { __typename: 'DMMessageNotificationItem', id: string, createdAt: any, summary: string, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, room: { __typename?: 'Room', id: string } }
    | { __typename: 'MentionNotificationItem', id: string, createdAt: any, summary: string, mentionEventId: string, mentionInThread?: string | null, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, mentionRoom: { __typename?: 'Room', id: string, name: string } }
    | { __typename: 'ReplyNotificationItem', id: string, createdAt: any, summary: string, inReplyToId: string, replyEventId: string, replyInThread?: string | null, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, replyRoom: { __typename?: 'Room', id: string, name: string } }
    | { __typename: 'RoomMessageNotificationItem', id: string, createdAt: any, summary: string, roomMsgEventId: string, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, roomMsgRoom: { __typename?: 'Room', id: string, name: string } }
  > };

export type HasNotificationsQueryVariables = Exact<{ [key: string]: never; }>;


export type HasNotificationsQuery = { __typename?: 'Query', hasNotifications: boolean };

export type NotificationInstanceNameQueryVariables = Exact<{ [key: string]: never; }>;


export type NotificationInstanceNameQuery = { __typename?: 'Query', server: { __typename?: 'Server', config: { __typename?: 'ServerConfig', serverName: string } } };

export type DismissNotificationMutationVariables = Exact<{
  input: DismissNotificationInput;
}>;


export type DismissNotificationMutation = { __typename?: 'Mutation', dismissNotification: boolean };

export type DismissAllNotificationsMutationVariables = Exact<{ [key: string]: never; }>;


export type DismissAllNotificationsMutation = { __typename?: 'Mutation', dismissAllNotifications: number };

export type GetInstanceInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type GetInstanceInfoQuery = { __typename?: 'Query', server: { __typename?: 'Server', directRegistrationEnabled: boolean, pushNotificationsEnabled: boolean, vapidPublicKey?: string | null, livekitUrl?: string | null, maxUploadSize: number, maxVideoUploadSize: number, messageEditWindowSeconds: number, primarySpaceId: string, config: { __typename?: 'ServerConfig', serverName: string, motd?: string | null, welcomeMessage?: string | null, description?: string | null, logoUrl?: string | null, bannerUrl?: string | null } } };

export type GetVoiceCallTokenQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetVoiceCallTokenQuery = { __typename?: 'Query', voiceCallToken?: { __typename?: 'VoiceCallToken', token: string } | null };

export type RoomMessagesLatestQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
}>;


export type RoomMessagesLatestQuery = { __typename?: 'Query', roomEvents: { __typename?: 'RoomEventsConnection', startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'RoomEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesBeforeQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  before?: InputMaybe<Scalars['String']['input']>;
}>;


export type RoomMessagesBeforeQuery = { __typename?: 'Query', roomEvents: { __typename?: 'RoomEventsConnection', startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'RoomEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesAfterQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  after?: InputMaybe<Scalars['String']['input']>;
}>;


export type RoomMessagesAfterQuery = { __typename?: 'Query', roomEvents: { __typename?: 'RoomEventsConnection', startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'RoomEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesAroundQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
}>;


export type RoomMessagesAroundQuery = { __typename?: 'Query', roomEventsAround: { __typename?: 'RoomEventsAroundResult', targetIndex: number, startCursor?: string | null, endCursor?: string | null, hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'RoomEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesRefetchOneQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type RoomMessagesRefetchOneQuery = { __typename?: 'Query', roomEventByEventId?: (
    { __typename?: 'RoomEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  ) | null };

export type ThreadMessagesAllQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  threadRootEventId: Scalars['ID']['input'];
}>;


export type ThreadMessagesAllQuery = { __typename?: 'Query', threadEvents: Array<(
    { __typename?: 'RoomEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  )> };

export type GetAllRoomsInSpaceQueryVariables = Exact<{ [key: string]: never; }>;


export type GetAllRoomsInSpaceQuery = { __typename?: 'Query', server: { __typename?: 'Server', rooms: Array<{ __typename?: 'Room', id: string, name: string, description?: string | null, archived: boolean, viewerCanJoinRoom: boolean }> } };

export type JoinRoomFromDirectoryMutationVariables = Exact<{
  input: JoinRoomInput;
}>;


export type JoinRoomFromDirectoryMutation = { __typename?: 'Mutation', joinRoom: boolean };

export type LeaveRoomFromDirectoryStoreMutationVariables = Exact<{
  input: LeaveRoomInput;
}>;


export type LeaveRoomFromDirectoryStoreMutation = { __typename?: 'Mutation', leaveRoom: boolean };

export type GetMyRoomsInSpaceQueryVariables = Exact<{ [key: string]: never; }>;


export type GetMyRoomsInSpaceQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, rooms: Array<{ __typename?: 'Room', id: string, name: string, type: RoomType, hasUnread: boolean, hasMention: boolean, archived: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null, members: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> }> } | null, server: { __typename?: 'Server', roomLayout?: { __typename?: 'RoomLayout', unsectionedRoomIds: Array<string>, sections: Array<{ __typename?: 'RoomLayoutSection', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } | null } };

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

export type ValidateSpaceAccessQueryVariables = Exact<{ [key: string]: never; }>;


export type ValidateSpaceAccessQuery = { __typename?: 'Query', server: { __typename?: 'Server', primarySpaceId: string, viewerHasAnyAdminPermission: boolean, viewerCanManageInstance: boolean, viewerCanBrowseRooms: boolean, viewerCanManageRooms: boolean, viewerCanManageRoles: boolean, viewerCanAssignRoles: boolean, viewerCanInviteMembers: boolean, config: { __typename?: 'ServerConfig', serverName: string, bannerUrl?: string | null } } };

export type GetRoomForSettingsQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
}>;


export type GetRoomForSettingsQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string } | null, server: { __typename?: 'Server', viewerCanManageRooms: boolean } };

export type MessageAttachmentViewFragment = { __typename?: 'Attachment', id: string, filename: string, contentType: string, width: number, height: number, url: string, thumbnailUrl?: string | null, videoProcessing?: { __typename?: 'VideoProcessing', status: VideoProcessingStatus, durationMs?: any | null, width?: number | null, height?: number | null, thumbnailUrl?: string | null, errorMessage?: string | null, variants: Array<{ __typename?: 'VideoVariant', url: string, quality: string, width: number, height: number, size: any }> } | null } & { ' $fragmentName'?: 'MessageAttachmentViewFragment' };

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


export type ReplyPreviewQuery = { __typename?: 'Query', roomEventByEventId?: (
    { __typename?: 'RoomEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  ) | null };

export type AddReactionMutationVariables = Exact<{
  input: AddReactionInput;
}>;


export type AddReactionMutation = { __typename?: 'Mutation', addReaction: boolean };

export type RemoveReactionMutationVariables = Exact<{
  input: RemoveReactionInput;
}>;


export type RemoveReactionMutation = { __typename?: 'Mutation', removeReaction: boolean };

export type RoomEventViewFragment = { __typename?: 'RoomEvent', id: string, createdAt: any, actorId: string, actor?: (
    { __typename?: 'User' }
    & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
  ) | null, event:
    | { __typename: 'CallParticipantJoinedEvent', roomId: string }
    | { __typename: 'CallParticipantLeftEvent', roomId: string }
    | { __typename: 'MessageDeletedEvent', roomId: string, messageEventId: string }
    | { __typename: 'MessagePostedEvent', roomId: string, body?: string | null, updatedAt?: any | null, inReplyTo?: string | null, inThread?: string | null, echoOfEventId?: string | null, echoFromThreadRootEventId?: string | null, replyCount: number, lastReplyAt?: any | null, viewerIsFollowingThread?: boolean | null, attachments: Array<(
        { __typename?: 'Attachment' }
        & { ' $fragmentRefs'?: { 'MessageAttachmentViewFragment': MessageAttachmentViewFragment } }
      )>, linkPreview?: (
        { __typename?: 'LinkPreview' }
        & { ' $fragmentRefs'?: { 'LinkPreviewViewFragment': LinkPreviewViewFragment } }
      ) | null, reactions: Array<{ __typename?: 'Reaction', emoji: string, count: number, hasReacted: boolean, users: Array<{ __typename?: 'User', id: string, displayName: string }> }>, threadParticipants: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> }
    | { __typename: 'MessageUpdatedEvent', roomId: string, messageEventId: string }
    | { __typename: 'PresenceChangedEvent', status: PresenceStatus }
    | { __typename: 'ReactionAddedEvent', roomId: string, messageEventId: string, emoji: string }
    | { __typename: 'ReactionRemovedEvent', roomId: string, messageEventId: string, emoji: string }
    | { __typename: 'RoomArchivedEvent', roomId: string }
    | { __typename: 'RoomCreatedEvent' }
    | { __typename: 'RoomDeletedEvent', roomId: string }
    | { __typename: 'RoomUnarchivedEvent', roomId: string }
    | { __typename: 'RoomUpdatedEvent', roomId: string }
    | { __typename: 'ServerMemberDeletedEvent', userId: string }
    | { __typename: 'UserJoinedRoomEvent', roomId: string }
    | { __typename: 'UserLeftRoomEvent', roomId: string }
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

export type MarkThreadAsOpenedMutationVariables = Exact<{
  input: MarkThreadAsOpenedInput;
}>;


export type MarkThreadAsOpenedMutation = { __typename?: 'Mutation', markThreadAsOpened: { __typename?: 'MarkThreadAsOpenedResult', previousOpenedAt?: any | null } };

export type ResolveMessageLinkQueryVariables = Exact<{
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type ResolveMessageLinkQuery = { __typename?: 'Query', roomEventByEventId?: { __typename?: 'RoomEvent', id: string, event:
      | { __typename: 'CallParticipantJoinedEvent' }
      | { __typename: 'CallParticipantLeftEvent' }
      | { __typename: 'MessageDeletedEvent' }
      | { __typename: 'MessagePostedEvent', inThread?: string | null }
      | { __typename: 'MessageUpdatedEvent' }
      | { __typename: 'PresenceChangedEvent' }
      | { __typename: 'ReactionAddedEvent' }
      | { __typename: 'ReactionRemovedEvent' }
      | { __typename: 'RoomArchivedEvent' }
      | { __typename: 'RoomCreatedEvent' }
      | { __typename: 'RoomDeletedEvent' }
      | { __typename: 'RoomUnarchivedEvent' }
      | { __typename: 'RoomUpdatedEvent' }
      | { __typename: 'ServerMemberDeletedEvent' }
      | { __typename: 'UserJoinedRoomEvent' }
      | { __typename: 'UserLeftRoomEvent' }
      | { __typename: 'UserTypingEvent' }
      | { __typename: 'VideoProcessingCompletedEvent' }
     } | null };

export type GetSpaceNotificationPreferencesQueryVariables = Exact<{ [key: string]: never; }>;


export type GetSpaceNotificationPreferencesQuery = { __typename?: 'Query', server: { __typename?: 'Server', viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }, me?: { __typename?: 'User', rooms: Array<{ __typename?: 'Room', id: string, name: string, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> } | null };

export type SetServerNotificationLevelMutationVariables = Exact<{
  input: SetServerNotificationLevelInput;
}>;


export type SetServerNotificationLevelMutation = { __typename?: 'Mutation', setServerNotificationLevel: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } };

export type SetRoomNotificationLevelMutationVariables = Exact<{
  input: SetRoomNotificationLevelInput;
}>;


export type SetRoomNotificationLevelMutation = { __typename?: 'Mutation', setRoomNotificationLevel: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } };

export type AdminDashboardUsersQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminDashboardUsersQuery = { __typename?: 'Query', users: Array<{ __typename?: 'User', id: string }> };

export type SpaceMembersQueryVariables = Exact<{
  search?: InputMaybe<Scalars['String']['input']>;
}>;


export type SpaceMembersQuery = { __typename?: 'Query', server: { __typename?: 'Server', roles: Array<{ __typename?: 'Role', name: string, displayName: string }>, members: { __typename?: 'ServerMembersConnection', totalCount: number, users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, roles: Array<string>, createdAt?: any | null }> } } };

export type SpaceMemberDetailsQueryVariables = Exact<{
  userId: Scalars['ID']['input'];
}>;


export type SpaceMemberDetailsQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, roles: Array<string> } | null, user?: { __typename?: 'User', lastLoginChange?: any | null } | null, server: { __typename?: 'Server', viewerCanAssignRoles: boolean, viewerCanManageRoles: boolean, availablePermissions: Array<string>, roles: Array<{ __typename?: 'Role', name: string, displayName: string, position: number, permissions: Array<string>, permissionDenials: Array<string> }>, member?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, roles: Array<string> } | null } };

export type AdminUpdateUserMutationVariables = Exact<{
  input: AdminUpdateUserInput;
}>;


export type AdminUpdateUserMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', updateUser: { __typename?: 'User', id: string, login: string, displayName: string } } | null };

export type AdminClearUsernameCooldownMutationVariables = Exact<{
  userId: Scalars['ID']['input'];
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

export type AdminRoomLayoutQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminRoomLayoutQuery = { __typename?: 'Query', server: { __typename?: 'Server', rooms: Array<{ __typename?: 'Room', id: string, name: string, description?: string | null, archived: boolean, autoJoin: boolean }>, roomLayout?: { __typename?: 'RoomLayout', unsectionedRoomIds: Array<string>, sections: Array<{ __typename?: 'RoomLayoutSection', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } | null } };

export type UpdateRoomLayoutMutationVariables = Exact<{
  input: UpdateRoomLayoutInput;
}>;


export type UpdateRoomLayoutMutation = { __typename?: 'Mutation', updateRoomLayout: { __typename?: 'RoomLayout', unsectionedRoomIds: Array<string>, sections: Array<{ __typename?: 'RoomLayoutSection', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } };

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

export type SetRoomAutoJoinMutationVariables = Exact<{
  input: SetRoomAutoJoinInput;
}>;


export type SetRoomAutoJoinMutation = { __typename?: 'Mutation', setRoomAutoJoin: { __typename?: 'Room', id: string, autoJoin: boolean } };

export type AdminSecurityConfigQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSecurityConfigQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', serverConfig: { __typename?: 'AdminServerConfig', isConfigured: boolean, blockedUsernames?: string | null } } | null };

export type UpdateSecurityConfigMutationVariables = Exact<{
  input: UpdateServerConfigInput;
}>;


export type UpdateSecurityConfigMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', updateServerConfig: { __typename?: 'AdminServerConfig', isConfigured: boolean, blockedUsernames?: string | null } } | null };

export type AdminSystemInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSystemInfoQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', systemInfo: { __typename?: 'SystemInfo', connection: { __typename?: 'ConnectionInfo', connected: boolean, serverID: string, serverName: string, version: string, maxPayload: any, rtt: string }, account: { __typename?: 'AccountInfo', memory: any, memoryUsed: any, storage: any, storageUsed: any, streams: number, streamsUsed: number, consumers: number, consumersUsed: number } } } | null };

export type MyFollowedThreadsQueryVariables = Exact<{ [key: string]: never; }>;


export type MyFollowedThreadsQuery = { __typename?: 'Query', myFollowedThreads: Array<{ __typename?: 'FollowedThread', roomId: string, threadRootEventId: string, replyCount: number, lastReplyAt?: any | null, hasUnread: boolean, room: { __typename?: 'Room', name: string }, rootMessage?: (
      { __typename?: 'RoomEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    ) | null, threadParticipants: Array<(
      { __typename?: 'User' }
      & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
    )> }> };

export type GetMyLastLoginChangeQueryVariables = Exact<{ [key: string]: never; }>;


export type GetMyLastLoginChangeQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, lastLoginChange?: any | null } | null };

export type UploadMyAvatarMutationVariables = Exact<{
  input: UploadMyAvatarInput;
}>;


export type UploadMyAvatarMutation = { __typename?: 'Mutation', uploadMyAvatar: { __typename?: 'User', id: string, avatarUrl?: string | null } };

export type DeleteMyAvatarMutationVariables = Exact<{ [key: string]: never; }>;


export type DeleteMyAvatarMutation = { __typename?: 'Mutation', deleteMyAvatar: { __typename?: 'User', id: string, avatarUrl?: string | null } };

export type UpdateMyProfileMutationVariables = Exact<{
  input: UpdateMyProfileInput;
}>;


export type UpdateMyProfileMutation = { __typename?: 'Mutation', updateMyProfile: { __typename?: 'User', id: string, displayName: string, login: string } };

export type AccountPermissionsQueryVariables = Exact<{ [key: string]: never; }>;


export type AccountPermissionsQuery = { __typename?: 'Query', me?: { __typename?: 'User', viewerCanDeleteAccount: boolean } | null };

export type RequestAccountDeletionMutationVariables = Exact<{ [key: string]: never; }>;


export type RequestAccountDeletionMutation = { __typename?: 'Mutation', requestAccountDeletion: string };

export type DeleteMyAccountMutationVariables = Exact<{
  input: DeleteMyAccountInput;
}>;


export type DeleteMyAccountMutation = { __typename?: 'Mutation', deleteMyAccount: boolean };

export type UpdateMySettingsMutationVariables = Exact<{
  input: UpdateUserSettingsInput;
}>;


export type UpdateMySettingsMutation = { __typename?: 'Mutation', updateMySettings: { __typename?: 'UserSettings', timezone?: string | null, timeFormat: TimeFormat } };

export type LoginPageInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type LoginPageInfoQuery = { __typename?: 'Query', server: { __typename?: 'Server', enabledAuthProviders: Array<string>, directRegistrationEnabled: boolean } };

export const UserAvatarUserFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<UserAvatarUserFragment, unknown>;
export const MessageAttachmentViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}}]} as unknown as DocumentNode<MessageAttachmentViewFragment, unknown>;
export const LinkPreviewViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<LinkPreviewViewFragment, unknown>;
export const RoomEventViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<RoomEventViewFragment, unknown>;
export const CreateRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateRoomMutation, CreateRoomMutationVariables>;
export const JoinRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinRoomMutation, JoinRoomMutationVariables>;
export const ServerSettingsModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ServerSettingsModal"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageInstance"}}]}}]}}]} as unknown as DocumentNode<ServerSettingsModalQuery, ServerSettingsModalQueryVariables>;
export const UpdateServerSettingsModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateServerSettingsModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateServerInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateServer"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateServerSettingsModalMutation, UpdateServerSettingsModalMutationVariables>;
export const UploadInstanceLogoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadInstanceLogo"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadServerLogoInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadServerLogo"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]}}]} as unknown as DocumentNode<UploadInstanceLogoMutation, UploadInstanceLogoMutationVariables>;
export const DeleteInstanceLogoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteInstanceLogo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteServerLogo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]}}]} as unknown as DocumentNode<DeleteInstanceLogoMutation, DeleteInstanceLogoMutationVariables>;
export const UploadInstanceBannerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadInstanceBanner"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadServerBannerInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadServerBanner"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]}}]} as unknown as DocumentNode<UploadInstanceBannerMutation, UploadInstanceBannerMutationVariables>;
export const DeleteInstanceBannerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteInstanceBanner"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteServerBanner"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]}}]} as unknown as DocumentNode<DeleteInstanceBannerMutation, DeleteInstanceBannerMutationVariables>;
export const InstanceInitDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InstanceInit"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"primarySpaceId"}},{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerHasUnreadRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"DM"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomNotificationPreferences"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"canViewAdmin"}},{"kind":"Field","name":{"kind":"Name","value":"canViewDMs"}},{"kind":"Field","name":{"kind":"Name","value":"canWriteDMs"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewUsers"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminManageUsers"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewRoles"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewSystem"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewAudit"}}]}}]}}]} as unknown as DocumentNode<InstanceInitQuery, InstanceInitQueryVariables>;
export const InstanceIconRefreshDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InstanceIconRefresh"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}}]}}]}}]} as unknown as DocumentNode<InstanceIconRefreshQuery, InstanceIconRefreshQueryVariables>;
export const FirstUnreadRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FirstUnreadRoom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}}]}}]}}]}}]} as unknown as DocumentNode<FirstUnreadRoomQuery, FirstUnreadRoomQueryVariables>;
export const RoomSettingsDataDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomSettingsData"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<RoomSettingsDataQuery, RoomSettingsDataQueryVariables>;
export const UpdateRoomSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRoomSettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateRoomSettingsMutation, UpdateRoomSettingsMutationVariables>;
export const LoadCurrentUserDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoadCurrentUser"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}},{"kind":"Field","name":{"kind":"Name","value":"hasVerifiedEmail"}},{"kind":"Field","name":{"kind":"Name","value":"settings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}}]}}]}}]} as unknown as DocumentNode<LoadCurrentUserQuery, LoadCurrentUserQueryVariables>;
export const MessagePreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MessagePreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"120"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"120"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"COVER"}}]}]}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<MessagePreviewQuery, MessagePreviewQueryVariables>;
export const QuickSwitcherInstanceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherInstance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"primarySpaceId"}},{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}}]}}]}}]} as unknown as DocumentNode<QuickSwitcherInstanceQuery, QuickSwitcherInstanceQueryVariables>;
export const QuickSwitcherRoomsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherRooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<QuickSwitcherRoomsQuery, QuickSwitcherRoomsQueryVariables>;
export const PostMessageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PostMessage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PostMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"postMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<PostMessageMutation, PostMessageMutationVariables>;
export const EditMessageFromInputDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EditMessageFromInput"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EditMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"editMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<EditMessageFromInputMutation, EditMessageFromInputMutationVariables>;
export const LinkPreviewForComposerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LinkPreviewForComposer"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"url"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"url"},"value":{"kind":"Variable","name":{"kind":"Name","value":"url"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}},{"kind":"Field","name":{"kind":"Name","value":"imageAssetId"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<LinkPreviewForComposerQuery, LinkPreviewForComposerQueryVariables>;
export const PermissionInspectorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PermissionInspector"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permissionExplanation"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permission"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"decidedAt"}},{"kind":"Field","name":{"kind":"Name","value":"decidedByRole"}},{"kind":"Field","name":{"kind":"Name","value":"trace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"roleName"}},{"kind":"Field","name":{"kind":"Name","value":"decision"}},{"kind":"Field","name":{"kind":"Name","value":"applied"}}]}}]}}]}}]} as unknown as DocumentNode<PermissionInspectorQuery, PermissionInspectorQueryVariables>;
export const MatrixTierRolesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MatrixTierRoles"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tierRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"applicablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roleName"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"override"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"inheritedAllows"}},{"kind":"Field","name":{"kind":"Name","value":"inheritedDenials"}}]}}]}}]}}]} as unknown as DocumentNode<MatrixTierRolesQuery, MatrixTierRolesQueryVariables>;
export const MatrixGrantRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantRoomPermMutation, MatrixGrantRoomPermMutationVariables>;
export const MatrixDenyRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyRoomPermMutation, MatrixDenyRoomPermMutationVariables>;
export const MatrixClearRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearRoomPermMutation, MatrixClearRoomPermMutationVariables>;
export const MatrixGrantServerPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantServerPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantServerPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantServerPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantServerPermMutation, MatrixGrantServerPermMutationVariables>;
export const MatrixDenyServerPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyServerPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyServerPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyServerPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyServerPermMutation, MatrixDenyServerPermMutationVariables>;
export const MatrixClearServerPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearServerPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearServerPermissionStateInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearServerPermissionState"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearServerPermMutation, MatrixClearServerPermMutationVariables>;
export const StartDmDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartDM"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartDMInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startDM"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<StartDmMutation, StartDmMutationVariables>;
export const AddReactionFromActionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddReactionFromActions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AddReactionFromActionsMutation, AddReactionFromActionsMutationVariables>;
export const RemoveReactionFromActionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveReactionFromActions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RemoveReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RemoveReactionFromActionsMutation, RemoveReactionFromActionsMutationVariables>;
export const GetRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanPostMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanPostInThread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReply"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReplyInThread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReact"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEditOwnMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEditAnyMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteOwnMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteAnyMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEchoMessage"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<GetRoomQuery, GetRoomQueryVariables>;
export const GetDmRoomMembersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetDMRoomMembers"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<GetDmRoomMembersQuery, GetDmRoomMembersQueryVariables>;
export const GetRoomMembersForStoreDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoomMembersForStore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}}]}}]} as unknown as DocumentNode<GetRoomMembersForStoreQuery, GetRoomMembersForStoreQueryVariables>;
export const MarkRoomAsReadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkRoomAsRead"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MarkRoomAsReadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markRoomAsRead"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previousLastReadAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastReadAt"}}]}}]}}]} as unknown as DocumentNode<MarkRoomAsReadMutation, MarkRoomAsReadMutationVariables>;
export const SendTypingIndicatorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SendTypingIndicator"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SendTypingIndicatorInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sendTypingIndicator"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SendTypingIndicatorMutation, SendTypingIndicatorMutationVariables>;
export const MyInstanceEventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"MyInstanceEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myInstanceEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerConfigUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedServerEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftServerEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserProfileUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"login"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerUserPreferencesUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationLevelChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"nlcRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MentionNotificationEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NewDirectMessageNotificationEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"sender"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"conversationName"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationCreatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notificationId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyToId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationDismissedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notificationId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NewMessageInServerEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomMarkedAsReadEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ThreadFollowChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"tfcRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"isFollowing"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomLayoutUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"changed"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionTerminatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reason"}}]}}]}}]}}]}}]} as unknown as DocumentNode<MyInstanceEventsSubscription, MyInstanceEventsSubscriptionVariables>;
export const SubscribeToPushDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SubscribeToPush"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PushSubscriptionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"subscribeToPush"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SubscribeToPushMutation, SubscribeToPushMutationVariables>;
export const UnsubscribeFromPushDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnsubscribeFromPush"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnsubscribeFromPushInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unsubscribeFromPush"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnsubscribeFromPushMutation, UnsubscribeFromPushMutationVariables>;
export const UpdateMyPresenceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMyPresence"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMyPresenceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMyPresence"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UpdateMyPresenceMutation, UpdateMyPresenceMutationVariables>;
export const ServerEventBusSubscriptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"ServerEventBusSubscription"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myServerEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ServerEventBusSubscriptionSubscription, ServerEventBusSubscriptionSubscriptionVariables>;
export const GetActiveCallRoomIdsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetActiveCallRoomIds"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"activeCallRoomIds"}}]}}]} as unknown as DocumentNode<GetActiveCallRoomIdsQuery, GetActiveCallRoomIdsQueryVariables>;
export const GetSidebarCallParticipantsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetSidebarCallParticipants"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]} as unknown as DocumentNode<GetSidebarCallParticipantsQuery, GetSidebarCallParticipantsQueryVariables>;
export const GetCallParticipantsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetCallParticipants"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]} as unknown as DocumentNode<GetCallParticipantsQuery, GetCallParticipantsQueryVariables>;
export const NotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Notifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DMMessageNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MentionNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"mentionRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"mentionEventId"},"name":{"kind":"Name","value":"eventId"}},{"kind":"Field","alias":{"kind":"Name","value":"mentionInThread"},"name":{"kind":"Name","value":"inThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplyNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"replyRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"replyEventId"},"name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyToId"}},{"kind":"Field","alias":{"kind":"Name","value":"replyInThread"},"name":{"kind":"Name","value":"inThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomMessageNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgEventId"},"name":{"kind":"Name","value":"eventId"}}]}}]}}]}}]} as unknown as DocumentNode<NotificationsQuery, NotificationsQueryVariables>;
export const HasNotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"HasNotifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNotifications"}}]}}]} as unknown as DocumentNode<HasNotificationsQuery, HasNotificationsQueryVariables>;
export const NotificationInstanceNameDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"NotificationInstanceName"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}}]}}]}}]}}]} as unknown as DocumentNode<NotificationInstanceNameQuery, NotificationInstanceNameQueryVariables>;
export const DismissNotificationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DismissNotification"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DismissNotificationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dismissNotification"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DismissNotificationMutation, DismissNotificationMutationVariables>;
export const DismissAllNotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DismissAllNotifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dismissAllNotifications"}}]}}]} as unknown as DocumentNode<DismissAllNotificationsMutation, DismissAllNotificationsMutationVariables>;
export const GetInstanceInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetInstanceInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"directRegistrationEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"pushNotificationsEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"vapidPublicKey"}},{"kind":"Field","name":{"kind":"Name","value":"livekitUrl"}},{"kind":"Field","name":{"kind":"Name","value":"maxUploadSize"}},{"kind":"Field","name":{"kind":"Name","value":"maxVideoUploadSize"}},{"kind":"Field","name":{"kind":"Name","value":"messageEditWindowSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"primarySpaceId"}},{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"256"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"256"}}]},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"1200"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"630"}}]}]}}]}}]}}]} as unknown as DocumentNode<GetInstanceInfoQuery, GetInstanceInfoQueryVariables>;
export const GetVoiceCallTokenDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetVoiceCallToken"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"voiceCallToken"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"token"}}]}}]}}]} as unknown as DocumentNode<GetVoiceCallTokenQuery, GetVoiceCallTokenQueryVariables>;
export const RoomMessagesLatestDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesLatest"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesLatestQuery, RoomMessagesLatestQueryVariables>;
export const RoomMessagesBeforeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesBefore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"before"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"before"},"value":{"kind":"Variable","name":{"kind":"Name","value":"before"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesBeforeQuery, RoomMessagesBeforeQueryVariables>;
export const RoomMessagesAfterDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesAfter"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesAfterQuery, RoomMessagesAfterQueryVariables>;
export const RoomMessagesAroundDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesAround"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventsAround"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"targetIndex"}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesAroundQuery, RoomMessagesAroundQueryVariables>;
export const RoomMessagesRefetchOneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesRefetchOne"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesRefetchOneQuery, RoomMessagesRefetchOneQueryVariables>;
export const ThreadMessagesAllDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ThreadMessagesAll"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"threadRootEventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"threadEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"threadRootEventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"threadRootEventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ThreadMessagesAllQuery, ThreadMessagesAllQueryVariables>;
export const GetAllRoomsInSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetAllRoomsInSpace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanJoinRoom"}}]}}]}}]}}]} as unknown as DocumentNode<GetAllRoomsInSpaceQuery, GetAllRoomsInSpaceQueryVariables>;
export const JoinRoomFromDirectoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinRoomFromDirectory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinRoomFromDirectoryMutation, JoinRoomFromDirectoryMutationVariables>;
export const LeaveRoomFromDirectoryStoreDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveRoomFromDirectoryStore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveRoomFromDirectoryStoreMutation, LeaveRoomFromDirectoryStoreMutationVariables>;
export const GetMyRoomsInSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMyRoomsInSpace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"hasMention"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomLayout"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"unsectionedRoomIds"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<GetMyRoomsInSpaceQuery, GetMyRoomsInSpaceQueryVariables>;
export const LeaveRoomFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveRoomFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveRoomFromModalMutation, LeaveRoomFromModalMutationVariables>;
export const DeleteMessageFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMessageFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteMessageFromModalMutation, DeleteMessageFromModalMutationVariables>;
export const DeleteLinkPreviewFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteLinkPreviewFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteLinkPreviewInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteLinkPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteLinkPreviewFromModalMutation, DeleteLinkPreviewFromModalMutationVariables>;
export const DeleteAttachmentFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteAttachmentFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteAttachmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteAttachment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteAttachmentFromModalMutation, DeleteAttachmentFromModalMutationVariables>;
export const ValidateSpaceAccessDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ValidateSpaceAccess"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"primarySpaceId"}},{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"480"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"252"}}]}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerHasAnyAdminPermission"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageInstance"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanBrowseRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanInviteMembers"}}]}}]}}]} as unknown as DocumentNode<ValidateSpaceAccessQuery, ValidateSpaceAccessQueryVariables>;
export const GetRoomForSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoomForSettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<GetRoomForSettingsQuery, GetRoomForSettingsQueryVariables>;
export const FollowThreadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FollowThread"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"FollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<FollowThreadMutation, FollowThreadMutationVariables>;
export const UnfollowThreadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnfollowThread"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnfollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unfollowThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnfollowThreadMutation, UnfollowThreadMutationVariables>;
export const ReplyPreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ReplyPreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ReplyPreviewQuery, ReplyPreviewQueryVariables>;
export const AddReactionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddReaction"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AddReactionMutation, AddReactionMutationVariables>;
export const RemoveReactionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveReaction"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RemoveReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RemoveReactionMutation, RemoveReactionMutationVariables>;
export const FollowThreadFromPaneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FollowThreadFromPane"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"FollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<FollowThreadFromPaneMutation, FollowThreadFromPaneMutationVariables>;
export const UnfollowThreadFromPaneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnfollowThreadFromPane"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnfollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unfollowThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnfollowThreadFromPaneMutation, UnfollowThreadFromPaneMutationVariables>;
export const MarkThreadAsOpenedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkThreadAsOpened"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MarkThreadAsOpenedInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markThreadAsOpened"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previousOpenedAt"}}]}}]}}]} as unknown as DocumentNode<MarkThreadAsOpenedMutation, MarkThreadAsOpenedMutationVariables>;
export const ResolveMessageLinkDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ResolveMessageLink"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inThread"}}]}}]}}]}}]}}]} as unknown as DocumentNode<ResolveMessageLinkQuery, ResolveMessageLinkQueryVariables>;
export const GetSpaceNotificationPreferencesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetSpaceNotificationPreferences"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}}]}}]} as unknown as DocumentNode<GetSpaceNotificationPreferencesQuery, GetSpaceNotificationPreferencesQueryVariables>;
export const SetServerNotificationLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetServerNotificationLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetServerNotificationLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setServerNotificationLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]} as unknown as DocumentNode<SetServerNotificationLevelMutation, SetServerNotificationLevelMutationVariables>;
export const SetRoomNotificationLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetRoomNotificationLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetRoomNotificationLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRoomNotificationLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]} as unknown as DocumentNode<SetRoomNotificationLevelMutation, SetRoomNotificationLevelMutationVariables>;
export const AdminDashboardUsersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminDashboardUsers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<AdminDashboardUsersQuery, AdminDashboardUsersQueryVariables>;
export const SpaceMembersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceMembers"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"20"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"roles"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]}}]} as unknown as DocumentNode<SpaceMembersQuery, SpaceMembersQueryVariables>;
export const SpaceMemberDetailsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceMemberDetails"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"roles"}}]}},{"kind":"Field","name":{"kind":"Name","value":"user"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}},{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"availablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"member"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"roles"}}]}}]}}]}}]} as unknown as DocumentNode<SpaceMemberDetailsQuery, SpaceMemberDetailsQueryVariables>;
export const AdminUpdateUserDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateUser"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AdminUpdateUserInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateUser"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}}]}}]} as unknown as DocumentNode<AdminUpdateUserMutation, AdminUpdateUserMutationVariables>;
export const AdminClearUsernameCooldownDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminClearUsernameCooldown"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearUsernameCooldown"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}]}]}}]}}]} as unknown as DocumentNode<AdminClearUsernameCooldownMutation, AdminClearUsernameCooldownMutationVariables>;
export const RevokeRoleFromMemberDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RevokeRoleFromMember"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RevokeRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revokeRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RevokeRoleFromMemberMutation, RevokeRoleFromMemberMutationVariables>;
export const AssignRoleToMemberDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AssignRoleToMember"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AssignRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assignRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AssignRoleToMemberMutation, AssignRoleToMemberMutationVariables>;
export const SpaceRolesGateDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRolesGate"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRolesGateQuery, SpaceRolesGateQueryVariables>;
export const SpaceRoleDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRoleDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"role"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}}]}},{"kind":"Field","name":{"kind":"Name","value":"roleUsers"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roleName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRoleDetailQuery, SpaceRoleDetailQueryVariables>;
export const UpdateRoleDetailPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRoleDetailPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateRoleDetailPageMutation, UpdateRoleDetailPageMutationVariables>;
export const DeleteRoleDetailPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteRoleDetailPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteRoleDetailPageMutation, DeleteRoleDetailPageMutationVariables>;
export const SpaceRolesNewCheckDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRolesNewCheck"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRolesNewCheckQuery, SpaceRolesNewCheckQueryVariables>;
export const CreateRoleNewPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRoleNewPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateRoleNewPageMutation, CreateRoleNewPageMutationVariables>;
export const AdminRoomLayoutDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminRoomLayout"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"type"},"value":{"kind":"EnumValue","value":"CHANNEL"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"autoJoin"}}]}},{"kind":"Field","name":{"kind":"Name","value":"roomLayout"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"unsectionedRoomIds"}}]}}]}}]}}]} as unknown as DocumentNode<AdminRoomLayoutQuery, AdminRoomLayoutQueryVariables>;
export const UpdateRoomLayoutDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRoomLayout"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomLayoutInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoomLayout"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"unsectionedRoomIds"}}]}}]}}]} as unknown as DocumentNode<UpdateRoomLayoutMutation, UpdateRoomLayoutMutationVariables>;
export const AdminUpdateRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<AdminUpdateRoomMutation, AdminUpdateRoomMutationVariables>;
export const ArchiveRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ArchiveRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ArchiveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"archiveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}}]}}]} as unknown as DocumentNode<ArchiveRoomMutation, ArchiveRoomMutationVariables>;
export const UnarchiveRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnarchiveRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnarchiveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unarchiveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}}]}}]} as unknown as DocumentNode<UnarchiveRoomMutation, UnarchiveRoomMutationVariables>;
export const SetRoomAutoJoinDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetRoomAutoJoin"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetRoomAutoJoinInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRoomAutoJoin"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"autoJoin"}}]}}]}}]} as unknown as DocumentNode<SetRoomAutoJoinMutation, SetRoomAutoJoinMutationVariables>;
export const AdminSecurityConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSecurityConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"isConfigured"}},{"kind":"Field","name":{"kind":"Name","value":"blockedUsernames"}}]}}]}}]}}]} as unknown as DocumentNode<AdminSecurityConfigQuery, AdminSecurityConfigQueryVariables>;
export const UpdateSecurityConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateSecurityConfig"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateServerConfigInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateServerConfig"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"isConfigured"}},{"kind":"Field","name":{"kind":"Name","value":"blockedUsernames"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateSecurityConfigMutation, UpdateSecurityConfigMutationVariables>;
export const AdminSystemInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSystemInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"systemInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"connected"}},{"kind":"Field","name":{"kind":"Name","value":"serverID"}},{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"maxPayload"}},{"kind":"Field","name":{"kind":"Name","value":"rtt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"account"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"memory"}},{"kind":"Field","name":{"kind":"Name","value":"memoryUsed"}},{"kind":"Field","name":{"kind":"Name","value":"storage"}},{"kind":"Field","name":{"kind":"Name","value":"storageUsed"}},{"kind":"Field","name":{"kind":"Name","value":"streams"}},{"kind":"Field","name":{"kind":"Name","value":"streamsUsed"}},{"kind":"Field","name":{"kind":"Name","value":"consumers"}},{"kind":"Field","name":{"kind":"Name","value":"consumersUsed"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AdminSystemInfoQuery, AdminSystemInfoQueryVariables>;
export const MyFollowedThreadsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyFollowedThreads"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myFollowedThreads"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"rootMessage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"3"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ServerMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<MyFollowedThreadsQuery, MyFollowedThreadsQueryVariables>;
export const GetMyLastLoginChangeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMyLastLoginChange"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}}]}}]} as unknown as DocumentNode<GetMyLastLoginChangeQuery, GetMyLastLoginChangeQueryVariables>;
export const UploadMyAvatarDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadMyAvatar"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadMyAvatarInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadMyAvatar"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}}]}}]} as unknown as DocumentNode<UploadMyAvatarMutation, UploadMyAvatarMutationVariables>;
export const DeleteMyAvatarDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMyAvatar"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMyAvatar"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}}]}}]} as unknown as DocumentNode<DeleteMyAvatarMutation, DeleteMyAvatarMutationVariables>;
export const UpdateMyProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMyProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMyProfileInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMyProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}}]}}]}}]} as unknown as DocumentNode<UpdateMyProfileMutation, UpdateMyProfileMutationVariables>;
export const AccountPermissionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AccountPermissions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteAccount"}}]}}]}}]} as unknown as DocumentNode<AccountPermissionsQuery, AccountPermissionsQueryVariables>;
export const RequestAccountDeletionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RequestAccountDeletion"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestAccountDeletion"}}]}}]} as unknown as DocumentNode<RequestAccountDeletionMutation, RequestAccountDeletionMutationVariables>;
export const DeleteMyAccountDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMyAccount"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteMyAccountInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMyAccount"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteMyAccountMutation, DeleteMyAccountMutationVariables>;
export const UpdateMySettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMySettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateUserSettingsInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMySettings"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}}]}}]} as unknown as DocumentNode<UpdateMySettingsMutation, UpdateMySettingsMutationVariables>;
export const LoginPageInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoginPageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"server"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabledAuthProviders"}},{"kind":"Field","name":{"kind":"Name","value":"directRegistrationEnabled"}}]}}]}}]} as unknown as DocumentNode<LoginPageInfoQuery, LoginPageInfoQueryVariables>;