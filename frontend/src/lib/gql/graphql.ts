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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Instance configuration section. */
export type AdminInstanceConfig = {
  __typename?: 'AdminInstanceConfig';
  /** Blocked usernames (newline-separated). Users cannot register with these names. */
  blockedUsernames?: Maybe<Scalars['String']['output']>;
  /** Instance name, displayed in page titles. Defaults to 'Chatto' if not set. */
  instanceName: Scalars['String']['output'];
  /** Whether this instance has been configured (has settings in KV). */
  isConfigured: Scalars['Boolean']['output'];
  /** Message of the Day, displayed in the header bar. */
  motd?: Maybe<Scalars['String']['output']>;
  /** OpenGraph description for link previews. */
  ogDescription?: Maybe<Scalars['String']['output']>;
  /** URL of the OpenGraph image for link previews. */
  ogImageUrl?: Maybe<Scalars['String']['output']>;
  /** OpenGraph title for link previews. Falls back to instance name if not set. */
  ogTitle?: Maybe<Scalars['String']['output']>;
  /** Welcome message shown on the login page (markdown supported). */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};

/** Admin mutations for configuration management. */
export type AdminMutations = {
  __typename?: 'AdminMutations';
  /** Clear the 30-day login change cooldown for a user, allowing them to immediately rename themselves. Idempotent. */
  clearUsernameCooldown: Scalars['Boolean']['output'];
  /** Delete the OpenGraph image. Returns the updated config section. */
  deleteInstanceOGImage: AdminInstanceConfig;
  /** Reset instance configuration to defaults. Returns true on success. */
  resetInstanceConfig: Scalars['Boolean']['output'];
  /** Update instance configuration. Returns the updated config section. */
  updateInstanceConfig: AdminInstanceConfig;
  /** Update a user's login and/or display name. Bypasses the 30-day login change cooldown but otherwise reuses the same validation as updateMyProfile. */
  updateUser: User;
  /** Upload an OpenGraph image for link previews. Returns the updated config section. */
  uploadInstanceOGImage: AdminInstanceConfig;
};


/** Admin mutations for configuration management. */
export type AdminMutationsClearUsernameCooldownArgs = {
  userId: Scalars['ID']['input'];
};


/** Admin mutations for configuration management. */
export type AdminMutationsUpdateInstanceConfigArgs = {
  input: UpdateInstanceConfigInput;
};


/** Admin mutations for configuration management. */
export type AdminMutationsUpdateUserArgs = {
  input: AdminUpdateUserInput;
};


/** Admin mutations for configuration management. */
export type AdminMutationsUploadInstanceOgImageArgs = {
  input: UploadInstanceOgImageInput;
};

/** Admin-only queries. Returns null if the user is not an instance admin. */
export type AdminQueries = {
  __typename?: 'AdminQueries';
  /** Get instance configuration. */
  instanceConfig: AdminInstanceConfig;
  /** List all available instance permission identifiers. */
  instancePermissions: Array<Scalars['String']['output']>;
  /** Get users assigned to a specific instance role. */
  instanceRoleUsers: Array<User>;
  /** Get a single instance role by name. */
  role?: Maybe<Role>;
  /** List all instance roles with their permissions. */
  roles: Array<Role>;
  /** Get aggregate operational metrics (NATS/JetStream connection + account-level usage). */
  systemInfo: SystemInfo;
  /** Get instance roles assigned to a specific user. */
  userInstanceRoles: Array<Scalars['String']['output']>;
  /**
   * Get the permissions denied via roles for a user.
   * Used for UI to show when a permission is blocked via roles.
   */
  userRoleBasedDenials: Array<Scalars['String']['output']>;
  /** Get the role-based permissions for a user. */
  userRoleBasedPermissions: Array<Scalars['String']['output']>;
};


/** Admin-only queries. Returns null if the user is not an instance admin. */
export type AdminQueriesInstanceRoleUsersArgs = {
  roleName: Scalars['String']['input'];
};


/** Admin-only queries. Returns null if the user is not an instance admin. */
export type AdminQueriesRoleArgs = {
  name: Scalars['String']['input'];
};


/** Admin-only queries. Returns null if the user is not an instance admin. */
export type AdminQueriesUserInstanceRolesArgs = {
  userId: Scalars['ID']['input'];
};


/** Admin-only queries. Returns null if the user is not an instance admin. */
export type AdminQueriesUserRoleBasedDenialsArgs = {
  userId: Scalars['ID']['input'];
};


/** Admin-only queries. Returns null if the user is not an instance admin. */
export type AdminQueriesUserRoleBasedPermissionsArgs = {
  userId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for assigning an instance role to a user. */
export type AssignInstanceRoleInput = {
  /** The name of the role to assign. */
  roleName: Scalars['String']['input'];
  /** The ID of the user to assign the role to. */
  userId: Scalars['ID']['input'];
};

/** Input for assigning a space role to a user. */
export type AssignSpaceRoleInput = {
  /** The name of the role to assign. */
  roleName: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
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
  /** The space ID where this attachment is stored. */
  spaceId: Scalars['ID']['output'];
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
 * The user who joined is identified by the parent SpaceEvent's actorId/actor.
 */
export type CallParticipantJoinedEvent = {
  __typename?: 'CallParticipantJoinedEvent';
  /** The ID of the room where the call is happening. */
  roomId: Scalars['ID']['output'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
};

/**
 * Event: A user left a voice call in a room.
 * The user who left is identified by the parent SpaceEvent's actorId/actor.
 */
export type CallParticipantLeftEvent = {
  __typename?: 'CallParticipantLeftEvent';
  /** The ID of the room where the call was happening. */
  roomId: Scalars['ID']['output'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
};

/** Input for clearing permission state on an instance role. */
export type ClearInstancePermissionStateInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear permission state for. */
  role: Scalars['String']['input'];
};

/** Input for clearing a space permission on an instance role. */
export type ClearInstanceRoleSpacePermissionInput = {
  /** The instance role to clear the permission for. */
  instanceRole: Scalars['String']['input'];
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for clearing a room-level permission override. */
export type ClearRoomPermissionInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear the permission for. */
  role: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for clearing permission state on a space role. */
export type ClearSpacePermissionStateInput = {
  /** The permission identifier to clear. */
  permission: Scalars['String']['input'];
  /** The role to clear permission state for. */
  role: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space where the room will be created. */
  spaceId: Scalars['ID']['input'];
};

/** Input for creating a new space. */
export type CreateSpaceInput = {
  /** Optional description of the space's purpose. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** The name of the new space. */
  name: Scalars['String']['input'];
};

/** Input for creating a new space role. */
export type CreateSpaceRoleInput = {
  /** Role description. */
  description: Scalars['String']['input'];
  /** Human-readable display name. */
  displayName: Scalars['String']['input'];
  /** Role identifier (lowercase alphanumeric + dashes, max 32 chars). */
  name: Scalars['String']['input'];
  /** Space ID where the role will be created. */
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for deleting a link preview from a message. */
export type DeleteLinkPreviewInput = {
  /** The event ID of the message containing the link preview. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
  /** The URL of the link preview to delete. */
  url: Scalars['String']['input'];
};

/** Input for deleting a message. */
export type DeleteMessageInput = {
  /** The event ID of the message to delete. */
  eventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for deleting the current user's account. */
export type DeleteMyAccountInput = {
  /** Confirmation token obtained from requestAccountDeletion. */
  confirmationToken: Scalars['String']['input'];
};

/** Input for deleting an instance role. */
export type DeleteRoleInput = {
  /** The name of the role to delete. */
  name: Scalars['String']['input'];
};

/** Input for deleting a space banner. */
export type DeleteSpaceBannerInput = {
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for deleting a space logo. */
export type DeleteSpaceLogoInput = {
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for deleting a space role. */
export type DeleteSpaceRoleInput = {
  /** The name of the role to delete. */
  name: Scalars['String']['input'];
  /** The ID of the space containing the role. */
  spaceId: Scalars['ID']['input'];
};

/** Input for denying a permission for an instance role. */
export type DenyInstancePermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  role: Scalars['String']['input'];
};

/** Input for denying a space permission for an instance role. */
export type DenyInstanceRoleSpacePermissionInput = {
  /** The instance role to deny the permission for. */
  instanceRole: Scalars['String']['input'];
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for denying a room-level permission for a role. */
export type DenyRoomPermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  role: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for denying a permission for a space role. */
export type DenySpacePermissionInput = {
  /** The permission identifier to deny. */
  permission: Scalars['String']['input'];
  /** The role to deny the permission for. */
  role: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
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
  rootMessage?: Maybe<SpaceEvent>;
  /** The ID of the space containing the thread. */
  spaceId: Scalars['ID']['output'];
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

/** Input for granting a permission to an instance role. */
export type GrantInstancePermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  role: Scalars['String']['input'];
};

/** Input for granting a space permission to an instance role. */
export type GrantInstanceRoleSpacePermissionInput = {
  /** The instance role to grant the permission to. */
  instanceRole: Scalars['String']['input'];
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for granting a room-level permission to a role. */
export type GrantRoomPermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  role: Scalars['String']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for granting a permission to a space role. */
export type GrantSpacePermissionInput = {
  /** The permission identifier to grant. */
  permission: Scalars['String']['input'];
  /** The role to grant the permission to. */
  role: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/**
 * Information about this Chatto instance.
 * These fields don't require authentication and are available on the login page.
 */
export type Instance = {
  __typename?: 'Instance';
  /** Runtime-editable configuration settings. */
  config: InstanceConfig;
  /** True if direct (email/password) registration is enabled on this instance. */
  directRegistrationEnabled: Scalars['Boolean']['output'];
  /** List of enabled SSO provider names (e.g., 'google', 'github'). */
  enabledAuthProviders: Array<Scalars['String']['output']>;
  /** LiveKit WebSocket URL for voice calls. Null if voice calls are disabled. */
  livekitUrl?: Maybe<Scalars['String']['output']>;
  /** Maximum upload size for regular attachments (images, files) in bytes. */
  maxUploadSize: Scalars['Int']['output'];
  /** Maximum upload size for video attachments in bytes. Same as maxUploadSize when video processing is disabled. */
  maxVideoUploadSize: Scalars['Int']['output'];
  /** ID of the space treated as this deployment's primary (future Server). Empty on fresh installs with no user-facing space yet. Migration bridge for ADR-027 / #330; goes away when Instance and Space have collapsed into a single Server entity. */
  primarySpaceId: Scalars['String']['output'];
  /** True if Web Push notifications are enabled on this instance. */
  pushNotificationsEnabled: Scalars['Boolean']['output'];
  /** VAPID public key for Web Push subscriptions. Null if push is disabled. */
  vapidPublicKey?: Maybe<Scalars['String']['output']>;
  /** The application version. */
  version: Scalars['String']['output'];
};

/**
 * Runtime-editable instance configuration.
 * These are settings that can be changed by admins at runtime.
 */
export type InstanceConfig = {
  __typename?: 'InstanceConfig';
  /** Instance name, displayed in page titles. Defaults to 'Chatto'. */
  instanceName: Scalars['String']['output'];
  /** Message of the Day, displayed in the header bar. Null if not configured. */
  motd?: Maybe<Scalars['String']['output']>;
  /** OpenGraph description for link previews. Falls back to default if not set. */
  ogDescription?: Maybe<Scalars['String']['output']>;
  /** OpenGraph image URL for link previews and the login page. Null if not set. Pass width and height to get a resized version. */
  ogImageUrl?: Maybe<Scalars['String']['output']>;
  /** OpenGraph title for link previews. Falls back to instance name if not set. */
  ogTitle?: Maybe<Scalars['String']['output']>;
  /** Welcome message to display on the login screen (Markdown). Null if not configured. */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};


/**
 * Runtime-editable instance configuration.
 * These are settings that can be changed by admins at runtime.
 */
export type InstanceConfigOgImageUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};

/**
 * Event: Instance configuration was updated.
 * Clients should refetch instance info to get the new values.
 */
export type InstanceConfigUpdatedEvent = {
  __typename?: 'InstanceConfigUpdatedEvent';
  /** The updated blocked usernames (null if cleared). */
  blockedUsernames?: Maybe<Scalars['String']['output']>;
  /** The updated instance name. */
  instanceName: Scalars['String']['output'];
  /** The updated MOTD (null if cleared). */
  motd?: Maybe<Scalars['String']['output']>;
  /** The updated welcome message (null if cleared). */
  welcomeMessage?: Maybe<Scalars['String']['output']>;
};

/**
 * InstanceEvent wraps all instance-scoped events.
 *
 * All instance events are live-only (published to NATS Core, never persisted).
 * Authorization is determined by NATS subject:
 * - Instance events: live.instance.{scope}.{id}.{eventType}
 */
export type InstanceEvent = {
  __typename?: 'InstanceEvent';
  /** The user who triggered this event. May be null if user was deleted. */
  actor?: Maybe<User>;
  /** The ID of the user who triggered this event. */
  actorId: Scalars['ID']['output'];
  /** When this event was created. */
  createdAt: Scalars['Time']['output'];
  /** The concrete event data. */
  event: InstanceEventType;
  /** Universal event identifier. */
  id: Scalars['ID']['output'];
};

/** Union of all instance-scoped event types. */
export type InstanceEventType = InstanceConfigUpdatedEvent | InstanceUserPreferencesUpdatedEvent | MentionNotificationEvent | NewDirectMessageNotificationEvent | NewMessageInSpaceEvent | NotificationCreatedEvent | NotificationDismissedEvent | NotificationLevelChangedEvent | RoomLayoutUpdatedEvent | RoomMarkedAsReadEvent | SessionTerminatedEvent | SpaceCreatedEvent | SpaceDeletedEvent | SpaceUpdatedEvent | ThreadFollowChangedEvent | UserCreatedEvent | UserDeletedEvent | UserJoinedSpaceEvent | UserLeftSpaceEvent | UserProfileUpdatedEvent;

/**
 * Configuration of an instance role's space-level permissions.
 * Space admins can grant/deny space permissions to users based on their instance roles.
 */
export type InstanceRoleSpaceConfig = {
  __typename?: 'InstanceRoleSpaceConfig';
  /** Space permissions denied for users with this instance role */
  permissionDenials: Array<Scalars['String']['output']>;
  /** Space permissions granted to users with this instance role */
  permissions: Array<Scalars['String']['output']>;
  /** The instance role information (name, display name, description, etc.) */
  role: Role;
};

/**
 * Event: The current user's display preferences were updated.
 * Published to the user across all sessions for multi-tab sync.
 */
export type InstanceUserPreferencesUpdatedEvent = {
  __typename?: 'InstanceUserPreferencesUpdatedEvent';
  /** Time display format. */
  timeFormat: TimeFormat;
  /** IANA timezone name (empty string = browser default). */
  timezone: Scalars['String']['output'];
};

/** Input for joining a room. */
export type JoinRoomInput = {
  /** The ID of the room to join. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for joining a space. */
export type JoinSpaceInput = {
  /** The ID of the space to join. */
  spaceId: Scalars['ID']['input'];
};

/** Input for leaving a room. */
export type LeaveRoomInput = {
  /** The ID of the room to leave. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for leaving a space. */
export type LeaveSpaceInput = {
  /** The ID of the space to leave. */
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
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
  /** The space where the mention occurred (for display). */
  space: Space;
  /** The ID of the space where the mention occurred. */
  spaceId: Scalars['ID']['output'];
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
  /** Space where the mention occurred */
  space: Space;
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
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
   * Assign an instance role to a user. Idempotent - assigning an already-assigned
   * role succeeds silently. Returns true on success.
   * Note: The 'everyone' role is implicit for all users and cannot be assigned.
   * Requires: admin.users.manage permission.
   * Errors: If role doesn't exist or is 'everyone'.
   */
  assignInstanceRole: Scalars['Boolean']['output'];
  /**
   * Assign a space role to a user. Idempotent - assigning an already-assigned
   * role succeeds silently. Returns true on success.
   * Requires: admin.roles.assign permission in the space.
   * Errors: If role doesn't exist.
   */
  assignSpaceRole: Scalars['Boolean']['output'];
  /**
   * Clear any grant or denial state for a permission on an instance role, restoring neutral state.
   * Idempotent - clearing when no state exists succeeds silently. Returns true on success.
   * After clearing, this role neither grants nor denies the permission.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  clearInstancePermissionState: Scalars['Boolean']['output'];
  /**
   * Clear any grant or denial for a space permission on an instance role.
   * Returns the permission to neutral state (no space-level configuration).
   * Requires: admin.roles.manage permission in the space.
   */
  clearInstanceRoleSpacePermission: Scalars['Boolean']['output'];
  /**
   * Clear room-level grant and denial for a permission on a role.
   * Returns the permission to neutral (inherit from space/instance defaults).
   * Requires: admin.roles.manage permission in the space.
   */
  clearRoomPermission: Scalars['Boolean']['output'];
  /**
   * Clear any grant or denial state for a permission on a space role, restoring neutral state.
   * Idempotent - clearing when no state exists succeeds silently. Returns true on success.
   * After clearing, this role neither grants nor denies the permission.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role doesn't exist or permission is invalid.
   */
  clearSpacePermissionState: Scalars['Boolean']['output'];
  /**
   * Create a new custom instance role. Returns the created role with empty permissions.
   * System role names ('instance-owner', 'instance-admin', 'instance-moderator', 'everyone') cannot be used.
   * Requires: admin.roles.manage permission.
   * Errors: If role name already exists or is a system role name.
   */
  createRole: Role;
  /** Create a new room in a space. */
  createRoom: Room;
  /** Create a new space with the given name and optional description. */
  createSpace: Space;
  /**
   * Create a new role in a space. Returns the created role with empty permissions.
   * System role names ('owner', 'moderator', 'everyone') cannot be used.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role name already exists or is a system role name.
   */
  createSpaceRole: Role;
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
   * - Removes the user from all spaces and rooms
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
   * Delete a custom instance role and all associated data. Returns true on success.
   * Deletes: role definition, all permission grants, and all user role assignments.
   * System roles ('instance-owner', 'instance-admin', 'instance-moderator', 'everyone') cannot be deleted.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or is a system role.
   */
  deleteRole: Scalars['Boolean']['output'];
  /** Delete the banner for a space. Requires manage_space permission. */
  deleteSpaceBanner: Space;
  /** Delete the logo for a space. Requires manage_space permission. */
  deleteSpaceLogo: Space;
  /**
   * Delete a custom space role and all associated data. Returns true on success.
   * Deletes: role definition, all permission grants, and all user role assignments.
   * System roles ('owner', 'moderator', 'everyone') cannot be deleted.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role doesn't exist or is a system role.
   */
  deleteSpaceRole: Scalars['Boolean']['output'];
  /**
   * Deny a permission for an instance role. Users with this role will be blocked from this
   * permission, regardless of what other roles grant it (deny-override pattern).
   * Clears any existing grant for the same permission. Returns true on success.
   * Note: Admin role is immune to role denials; denying a permission on admin has no effect.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  denyInstancePermission: Scalars['Boolean']['output'];
  /**
   * Deny a space permission for an instance role.
   * Users with this instance role will be blocked from this permission in this space.
   * Example: Deny rooms.join to instance:member to create a staff-only space.
   * Clears any existing grant for the same permission.
   * Requires: admin.roles.manage permission in the space.
   */
  denyInstanceRoleSpacePermission: Scalars['Boolean']['output'];
  /**
   * Deny a permission for a role at room level. Overrides space-level state for this room.
   * Clears any existing grant for the same permission in this room.
   * Requires: admin.roles.manage permission in the space.
   */
  denyRoomPermission: Scalars['Boolean']['output'];
  /**
   * Deny a permission for a space role. Users with this role will be blocked from this
   * permission, regardless of what other roles grant it (deny-override pattern).
   * Clears any existing grant for the same permission. Returns true on success.
   * Note: Admin role is immune to role denials; denying a permission on admin has no effect.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role doesn't exist or permission is invalid.
   */
  denySpacePermission: Scalars['Boolean']['output'];
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
   * Grant a permission to an instance role. Idempotent - granting an already-granted
   * permission succeeds silently. Returns true on success.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  grantInstancePermission: Scalars['Boolean']['output'];
  /**
   * Grant a space permission to an instance role.
   * Users with this instance role will receive this permission in this space.
   * Example: Grant rooms.create to instance:staff so staff can create rooms.
   * Clears any existing denial for the same permission.
   * Requires: admin.roles.manage permission in the space.
   */
  grantInstanceRoleSpacePermission: Scalars['Boolean']['output'];
  /**
   * Grant a permission to a role at room level. Overrides space-level state for this room.
   * Clears any existing denial for the same permission in this room.
   * Requires: admin.roles.manage permission in the space.
   */
  grantRoomPermission: Scalars['Boolean']['output'];
  /**
   * Grant a permission to a space role. Idempotent - granting an already-granted
   * permission succeeds silently. Returns true on success.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role doesn't exist or permission is invalid.
   */
  grantSpacePermission: Scalars['Boolean']['output'];
  /** Join the specified room. User must be a member of the space. */
  joinRoom: Scalars['Boolean']['output'];
  /** Join the specified space. */
  joinSpace: Scalars['Boolean']['output'];
  /** Leave the specified room. */
  leaveRoom: Scalars['Boolean']['output'];
  /** Leave the specified space. */
  leaveSpace: Scalars['Boolean']['output'];
  /**
   * Mark a room as read for the current user.
   * Stores the current stream lastSeq as the last read sequence.
   * Returns the previous and new last-read sequence IDs.
   */
  markRoomAsRead: MarkRoomAsReadResult;
  /**
   * Mark a thread as opened by the current user.
   * Stores the current timestamp and returns the previous timestamp.
   * Used for showing unread separators in thread panes.
   */
  markThreadAsOpened: MarkThreadAsOpenedResult;
  /** Post a message to a room. Automatically marks the room as read since the user is viewing it. */
  postMessage: SpaceEvent;
  /**
   * Remove an emoji reaction from a message.
   * The emoji parameter must be a shortcode name (e.g., "thumbsup", "heart").
   * Returns true if the reaction was removed, false if it didn't exist.
   */
  removeReaction: Scalars['Boolean']['output'];
  /**
   * Reorder instance roles. Accepts an ordered list of custom role names.
   * System roles (instance-owner, instance-admin, instance-moderator, everyone) maintain fixed positions and should not be included.
   * Positions are assigned based on array index (first role = position 1, second = 2, etc).
   * Requires: admin.roles.manage permission.
   * Returns: All instance roles, sorted by position.
   */
  reorderInstanceRoles: Array<Role>;
  /**
   * Reorder space roles. Accepts an ordered list of custom role names.
   * System roles (owner, moderator, everyone) maintain fixed positions and should not be included.
   * Positions are assigned based on array index (first role = position 1, second = 2, etc).
   * Owner always stays at position 0, everyone always stays at max position.
   * Requires: admin.roles.manage permission in the space.
   * Returns: All roles in the space, sorted by position.
   */
  reorderSpaceRoles: Array<Role>;
  /**
   * Request account deletion by generating a confirmation token.
   * The token is valid for 15 minutes and must be passed to deleteMyAccount.
   * This two-step process protects against XSS attacks.
   * Returns the confirmation token.
   */
  requestAccountDeletion: Scalars['String']['output'];
  /**
   * Revoke a permission grant from an instance role. Idempotent - revoking a non-granted
   * permission succeeds silently. Returns true on success.
   * Note: This only removes grants, not denials. Use clearInstancePermissionState to remove both.
   * Note: Admin role has all permissions implicitly; revoking from admin has no effect.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist or permission is invalid.
   */
  revokeInstancePermission: Scalars['Boolean']['output'];
  /**
   * Revoke an instance role from a user. Idempotent - revoking a non-assigned
   * role succeeds silently. Returns true on success.
   * Note: Users cannot revoke their own admin role (prevents self-lockout).
   * Note: The 'everyone' role is implicit and cannot be revoked.
   * Requires: admin.users.manage permission.
   * Errors: If role doesn't exist, is 'everyone', or user tries to revoke own admin role.
   */
  revokeInstanceRole: Scalars['Boolean']['output'];
  /**
   * Revoke a permission grant from a space role. Idempotent - revoking a non-granted
   * permission succeeds silently. Returns true on success.
   * Note: This only removes grants, not denials. Use clearSpacePermissionState to remove both.
   * Note: Admin role has all permissions implicitly; revoking from admin has no effect.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role doesn't exist or permission is invalid.
   */
  revokeSpacePermission: Scalars['Boolean']['output'];
  /**
   * Revoke a space role from a user. Idempotent - revoking a non-assigned
   * role succeeds silently. Returns true on success.
   * Requires: admin.roles.assign permission in the space.
   * Errors: If role doesn't exist.
   */
  revokeSpaceRole: Scalars['Boolean']['output'];
  /**
   * Send a typing indicator to other users in the room.
   * This is a live-only event (not stored). Clients should call this every ~2 seconds
   * while typing and implement 6-second timeout-based clearing.
   * Returns true on success.
   */
  sendTypingIndicator: Scalars['Boolean']['output'];
  /** Set whether new space members automatically join a room. Requires rooms.manage permission. */
  setRoomAutoJoin: Room;
  /** Set the current user's notification level for a room. Pass DEFAULT to clear. */
  setRoomNotificationLevel: ViewerNotificationPreference;
  /** Set the current user's notification level for a space. Pass DEFAULT to clear. */
  setSpaceNotificationLevel: ViewerNotificationPreference;
  /**
   * Start a DM conversation with the given participants.
   * If a conversation already exists with exactly these participants, returns the existing one.
   * The current user is automatically included as a participant.
   *
   * DMs exist in the system space "DM" - use space(id: "DM").rooms to list conversations.
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
   * Update an instance role's display name and description. Returns the updated role.
   * Role name cannot be changed after creation. System roles cannot be edited.
   * Requires: admin.roles.manage permission.
   * Errors: If role doesn't exist.
   */
  updateRole: Role;
  /** Update an existing room's name and description. Requires rooms.manage permission. */
  updateRoom: Room;
  /** Update the room layout for a space. Requires room.manage permission. */
  updateRoomLayout: RoomLayout;
  /** Update an existing space's name and description. */
  updateSpace: Space;
  /**
   * Update a space role's display name and description. Returns the updated role.
   * Role name cannot be changed after creation.
   * Requires: admin.roles.manage permission in the space.
   * Errors: If role doesn't exist.
   */
  updateSpaceRole: Role;
  /**
   * Upload an avatar for the current user.
   * Image will be resized to 256x256 max and converted to WebP.
   * Returns the updated user.
   */
  uploadMyAvatar: User;
  /** Upload a banner for a space. Requires manage_space permission. */
  uploadSpaceBanner: Space;
  /** Upload a logo for a space. Requires manage_space permission. */
  uploadSpaceLogo: Space;
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
export type MutationAssignInstanceRoleArgs = {
  input: AssignInstanceRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationAssignSpaceRoleArgs = {
  input: AssignSpaceRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationClearInstancePermissionStateArgs = {
  input: ClearInstancePermissionStateInput;
};


/** Root mutation type for modifying data. */
export type MutationClearInstanceRoleSpacePermissionArgs = {
  input: ClearInstanceRoleSpacePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationClearRoomPermissionArgs = {
  input: ClearRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationClearSpacePermissionStateArgs = {
  input: ClearSpacePermissionStateInput;
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
export type MutationCreateSpaceArgs = {
  input: CreateSpaceInput;
};


/** Root mutation type for modifying data. */
export type MutationCreateSpaceRoleArgs = {
  input: CreateSpaceRoleInput;
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
export type MutationDeleteSpaceBannerArgs = {
  input: DeleteSpaceBannerInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteSpaceLogoArgs = {
  input: DeleteSpaceLogoInput;
};


/** Root mutation type for modifying data. */
export type MutationDeleteSpaceRoleArgs = {
  input: DeleteSpaceRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyInstancePermissionArgs = {
  input: DenyInstancePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyInstanceRoleSpacePermissionArgs = {
  input: DenyInstanceRoleSpacePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenyRoomPermissionArgs = {
  input: DenyRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationDenySpacePermissionArgs = {
  input: DenySpacePermissionInput;
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
export type MutationGrantInstancePermissionArgs = {
  input: GrantInstancePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantInstanceRoleSpacePermissionArgs = {
  input: GrantInstanceRoleSpacePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantRoomPermissionArgs = {
  input: GrantRoomPermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationGrantSpacePermissionArgs = {
  input: GrantSpacePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationJoinRoomArgs = {
  input: JoinRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationJoinSpaceArgs = {
  input: JoinSpaceInput;
};


/** Root mutation type for modifying data. */
export type MutationLeaveRoomArgs = {
  input: LeaveRoomInput;
};


/** Root mutation type for modifying data. */
export type MutationLeaveSpaceArgs = {
  input: LeaveSpaceInput;
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
export type MutationReorderInstanceRolesArgs = {
  input: ReorderInstanceRolesInput;
};


/** Root mutation type for modifying data. */
export type MutationReorderSpaceRolesArgs = {
  input: ReorderSpaceRolesInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokeInstancePermissionArgs = {
  input: RevokeInstancePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokeInstanceRoleArgs = {
  input: RevokeInstanceRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokeSpacePermissionArgs = {
  input: RevokeSpacePermissionInput;
};


/** Root mutation type for modifying data. */
export type MutationRevokeSpaceRoleArgs = {
  input: RevokeSpaceRoleInput;
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
export type MutationSetSpaceNotificationLevelArgs = {
  input: SetSpaceNotificationLevelInput;
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
export type MutationUpdateSpaceArgs = {
  input: UpdateSpaceInput;
};


/** Root mutation type for modifying data. */
export type MutationUpdateSpaceRoleArgs = {
  input: UpdateSpaceRoleInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadMyAvatarArgs = {
  input: UploadMyAvatarInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadSpaceBannerArgs = {
  input: UploadSpaceBannerInput;
};


/** Root mutation type for modifying data. */
export type MutationUploadSpaceLogoArgs = {
  input: UploadSpaceLogoInput;
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
 * Event: A new message was posted in a space.
 * Published to all space members (except the author) when a message is posted.
 * Enables real-time unread indicators on space icons without per-space subscriptions.
 */
export type NewMessageInSpaceEvent = {
  __typename?: 'NewMessageInSpaceEvent';
  /** The ID of the room where the message was posted. */
  roomId: Scalars['ID']['output'];
  /** The ID of the space where the message was posted. */
  spaceId: Scalars['ID']['output'];
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
  /** Space ID for navigation (DM for DMs) */
  spaceId: Scalars['ID']['output'];
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

/** Controls how a user receives notifications for a space or room. */
export enum NotificationLevel {
  /** Like NORMAL, plus a notification for every new root message. */
  AllMessages = 'ALL_MESSAGES',
  /** Use inherited default (space default for rooms, NORMAL for spaces). */
  Default = 'DEFAULT',
  /** Suppress all notifications and unread markers. */
  Muted = 'MUTED',
  /** Standard behavior: unread markers + notifications for mentions/DMs/threads. */
  Normal = 'NORMAL'
}

/**
 * Event: The user's notification level for a space or room was changed.
 * Published to the user for multi-tab/multi-device sync.
 */
export type NotificationLevelChangedEvent = {
  __typename?: 'NotificationLevelChangedEvent';
  /** The effective level after inheritance. */
  effectiveLevel: NotificationLevel;
  /** The new notification level. */
  level: NotificationLevel;
  /** The room ID (null for space-level changes). */
  roomId?: Maybe<Scalars['ID']['output']>;
  /** The space ID. */
  spaceId: Scalars['ID']['output'];
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
  /** Decision came from an instance role acting in the instance KV bucket. */
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/**
 * Event: A user's presence status changed.
 * The user whose presence changed is identified by the parent SpaceEvent's actorId/actor.
 * Presence is instance-wide; the space context is implicit in the subscription.
 */
export type PresenceChangedEvent = {
  __typename?: 'PresenceChangedEvent';
  /** The user's new presence status. */
  status: PresenceStatus;
};

/** User presence status in a space. */
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
   * Get room IDs in a space that currently have active voice calls.
   * Returns empty list if LiveKit is not configured.
   * Requires space membership.
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
   * Check if the current user has any unread followed threads in a space.
   * Lightweight query for sidebar unread indicators.
   * Requires space membership.
   */
  hasUnreadFollowedThreads: Scalars['Boolean']['output'];
  /** Get information about this Chatto instance. No authentication required. */
  instance: Instance;
  /**
   * Fetch link preview metadata for a URL.
   * Results are cached server-side. Returns null if the URL cannot be previewed.
   * Requires authentication.
   */
  linkPreview?: Maybe<LinkPreview>;
  /** Get the currently authenticated user. */
  me?: Maybe<User>;
  /**
   * List threads the current user is following in a specific space.
   * Returns threads sorted by last activity (newest first).
   * Requires space membership.
   */
  myFollowedThreads: Array<FollowedThread>;
  /** Get all notifications for the current user, newest first */
  notifications: Array<NotificationItem>;
  /**
   * Explain every applicable permission for a user at the given scope.
   * - userId only → instance-scoped permissions.
   * - userId + spaceId → space-scoped permissions.
   * - userId + spaceId + roomId → room-scoped permissions.
   * Authorization: The viewer must be either the target user (self-inspection at
   * any scope they are a member of) or an admin at the requested scope:
   * instance-admin for instance scope, space admin (roles.manage in spaceId) or
   * instance admin for space/room scope.
   */
  permissionExplanation: Array<PermissionExplanation>;
  /**
   * Resolve a single role's permission state across every applicable tier
   * in one round-trip. Useful for permission editors that need to show
   * values inherited from the tiers above the one being edited.
   *
   * Authorization mirrors the per-tier resolvers: instance scope requires
   * instance admin; space and room scopes require role.manage in spaceId
   * or instance admin.
   */
  rolePermissions?: Maybe<RoleAcrossTiers>;
  /** Get a specific room by space and room ID. */
  room?: Maybe<Room>;
  /** Fetch a single room event by event ID (O(1) subject lookup). Returns null if not found. */
  roomEventByEventId?: Maybe<SpaceEvent>;
  /** Fetch historical events for a specific room (default limit: 50). Use 'before' for backward pagination, 'after' for forward pagination. */
  roomEvents: RoomEventsConnection;
  /**
   * Fetch room events centered around a specific event.
   * Returns a window of events with the target event roughly in the middle.
   * Used for "jump to message" when clicking reply links to messages not in the loaded range.
   */
  roomEventsAround: RoomEventsAroundResult;
  /** Get a specific space by ID. Does not require authentication for discovery. */
  space?: Maybe<Space>;
  /** List all spaces the current user has access to. */
  spaces: Array<Space>;
  /**
   * Fetch thread messages for a specific thread.
   * Returns the root message plus all replies in chronological order.
   */
  threadEvents: Array<SpaceEvent>;
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
  /** List all users on this instance. Requires instance admin. */
  users: Array<User>;
  /** The current authenticated user's instance-level permissions. Null if not authenticated. */
  viewer?: Maybe<Viewer>;
  /**
   * Get a LiveKit join token for a voice call in a room.
   * Returns null if LiveKit is not configured on this instance.
   * Requires room membership.
   */
  voiceCallToken?: Maybe<VoiceCallToken>;
};


/** Root query type for fetching data. */
export type QueryActiveCallRoomIdsArgs = {
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryCallParticipantsArgs = {
  roomId: Scalars['ID']['input'];
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryHasUnreadFollowedThreadsArgs = {
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryLinkPreviewArgs = {
  url: Scalars['String']['input'];
};


/** Root query type for fetching data. */
export type QueryMyFollowedThreadsArgs = {
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryPermissionExplanationArgs = {
  roomId?: InputMaybe<Scalars['ID']['input']>;
  spaceId?: InputMaybe<Scalars['ID']['input']>;
  userId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRolePermissionsArgs = {
  roleName: Scalars['String']['input'];
  roomId?: InputMaybe<Scalars['ID']['input']>;
  spaceId?: InputMaybe<Scalars['ID']['input']>;
};


/** Root query type for fetching data. */
export type QueryRoomArgs = {
  roomId: Scalars['ID']['input'];
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomEventByEventIdArgs = {
  eventId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomEventsArgs = {
  after?: InputMaybe<Scalars['Time']['input']>;
  before?: InputMaybe<Scalars['Time']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  roomId: Scalars['ID']['input'];
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryRoomEventsAroundArgs = {
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  roomId: Scalars['ID']['input'];
  spaceId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QuerySpaceArgs = {
  id: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryThreadEventsArgs = {
  roomId: Scalars['ID']['input'];
  spaceId: Scalars['ID']['input'];
  threadRootEventId: Scalars['ID']['input'];
};


/** Root query type for fetching data. */
export type QueryTierRolesArgs = {
  roomId?: InputMaybe<Scalars['ID']['input']>;
  spaceId?: InputMaybe<Scalars['ID']['input']>;
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
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the message. */
  spaceId: Scalars['ID']['output'];
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
  /** The ID of the space containing the message. */
  spaceId: Scalars['ID']['output'];
};

/** Input for removing an emoji reaction from a message. */
export type RemoveReactionInput = {
  /** The emoji shortcode name (e.g., 'thumbsup', 'heart'). */
  emoji: Scalars['String']['input'];
  /** The event ID of the message to remove the reaction from. */
  messageEventId: Scalars['ID']['input'];
  /** The ID of the room containing the message. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for reordering instance roles. */
export type ReorderInstanceRolesInput = {
  /** Ordered list of custom role names. System roles should not be included. */
  roleNames: Array<Scalars['String']['input']>;
};

/** Input for reordering space roles. */
export type ReorderSpaceRolesInput = {
  /** Ordered list of custom role names. System roles should not be included. */
  roleNames: Array<Scalars['String']['input']>;
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
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
  /** Space where the reply occurred */
  space: Space;
  /** Human-readable summary for display */
  summary: Scalars['String']['output'];
};

/** Input for revoking a permission from an instance role. */
export type RevokeInstancePermissionInput = {
  /** The permission identifier to revoke. */
  permission: Scalars['String']['input'];
  /** The role to revoke the permission from. */
  role: Scalars['String']['input'];
};

/** Input for revoking an instance role from a user. */
export type RevokeInstanceRoleInput = {
  /** The name of the role to revoke. */
  roleName: Scalars['String']['input'];
  /** The ID of the user to revoke the role from. */
  userId: Scalars['ID']['input'];
};

/** Input for revoking a permission from a space role. */
export type RevokeSpacePermissionInput = {
  /** The permission identifier to revoke. */
  permission: Scalars['String']['input'];
  /** The role to revoke the permission from. */
  role: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for revoking a space role from a user. */
export type RevokeSpaceRoleInput = {
  /** The name of the role to revoke. */
  roleName: Scalars['String']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
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
  /** Role identifier (e.g., 'admin', 'member'). */
  name: Scalars['String']['output'];
  /** List of permission identifiers denied by this role. Denials override grants from other roles. */
  permissionDenials: Array<Scalars['String']['output']>;
  /** List of permission identifiers granted (allowed) by this role. */
  permissions: Array<Scalars['String']['output']>;
  /** Hierarchy position: lower = higher rank. Admin=0, Member=MAX_INT. */
  position: Scalars['Int']['output'];
};

/**
 * A single role's permission state at every applicable tier.
 *
 * Tiers are populated broadest-first based on which scope was requested:
 * - rolePermissions(roleName, spaceId: null, roomId: null) → instance only.
 * - rolePermissions(roleName, spaceId) → instance (if instance role) + space.
 * - rolePermissions(roleName, spaceId, roomId) → instance (if instance role) + space + room.
 *
 * space roles never have an instance tier (the resolver returns null there).
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
  /** Permission state at instance scope (null for space roles). */
  instance?: Maybe<TierPermissions>;
  /** Whether this is an instance role (false for space roles). */
  isInstanceRole: Scalars['Boolean']['output'];
  /** Whether this is a system role and cannot be deleted. */
  isSystem: Scalars['Boolean']['output'];
  /** Hierarchy position; lower means higher rank. */
  position: Scalars['Int']['output'];
  /** Internal role name (e.g. 'admin', 'instance-admin'). */
  roleName: Scalars['String']['output'];
  /** Permission state at room scope (null when roomId not provided). */
  room?: Maybe<TierPermissions>;
  /** Permission state at space scope (null when spaceId not provided). */
  space?: Maybe<TierPermissions>;
};

/**
 * Room-level permission configuration for a single role.
 * Shows grants and denials that are specific to this room (not inherited from space).
 */
export type RoleRoomPermissions = {
  __typename?: 'RoleRoomPermissions';
  /** Human-readable display name */
  displayName: Scalars['String']['output'];
  /** Whether this is an instance role (vs space role) */
  isInstanceRole: Scalars['Boolean']['output'];
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

/** A Room is a chat channel within a Space where users can exchange messages. */
export type Room = {
  __typename?: 'Room';
  /** Whether this room is archived. Archived rooms are hidden from sidebar and Browse Rooms. */
  archived: Scalars['Boolean']['output'];
  /** Whether new space members automatically join this room. */
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
  /** The room's name. */
  name: Scalars['String']['output'];
  /** Room-level permission overrides for all roles (space + instance). */
  roomPermissionOverrides: Array<RoleRoomPermissions>;
  /** The ID of the space this room belongs to. */
  spaceId: Scalars['ID']['output'];
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

/** Result of fetching events around a specific target event. */
export type RoomEventsAroundResult = {
  __typename?: 'RoomEventsAroundResult';
  /** The events in the window, in chronological order. */
  events: Array<SpaceEvent>;
  /** Whether there are newer events after this window. */
  hasNewer: Scalars['Boolean']['output'];
  /** Whether there are older events before this window. */
  hasOlder: Scalars['Boolean']['output'];
  /** The index of the target event within the events array. */
  targetIndex: Scalars['Int']['output'];
};

/** Paginated room events with metadata indicating whether more events exist in either direction. */
export type RoomEventsConnection = {
  __typename?: 'RoomEventsConnection';
  /** The events in chronological order. */
  events: Array<SpaceEvent>;
  /** Whether there are newer events after this page. */
  hasNewer: Scalars['Boolean']['output'];
  /** Whether there are older events before this page. */
  hasOlder: Scalars['Boolean']['output'];
};

/**
 * The visual organization of rooms in a space's sidebar.
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
 * Sections allow space admins to organize rooms into visual groups.
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
 * Event: The room layout for a space was updated.
 * Clients should refetch the room layout to get the new order/sections.
 * This is a live-only event (not stored in JetStream).
 */
export type RoomLayoutUpdatedEvent = {
  __typename?: 'RoomLayoutUpdatedEvent';
  /** The ID of the space whose room layout was updated. */
  spaceId: Scalars['ID']['output'];
};

/**
 * Event: A room was marked as read by the current user.
 * Published to the user when they mark a room as read (e.g., by entering it).
 * Enables real-time updates to space unread indicators.
 */
export type RoomMarkedAsReadEvent = {
  __typename?: 'RoomMarkedAsReadEvent';
  /** The ID of the room that was marked as read. */
  roomId: Scalars['ID']['output'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
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
  /** Space where the message was posted. */
  space: Space;
  /** Human-readable summary for display. */
  summary: Scalars['String']['output'];
};

/**
 * A user's notification preference for a specific room, including space context.
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
  /** The space containing the room. */
  spaceId: Scalars['ID']['output'];
};

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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
  /** The event ID of the thread root message, if typing in a thread. */
  threadRootEventId?: InputMaybe<Scalars['ID']['input']>;
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

/** Input for setting whether new space members automatically join a room. */
export type SetRoomAutoJoinInput = {
  /** Whether new space members should automatically join this room. */
  autoJoin: Scalars['Boolean']['input'];
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for setting the notification level for a room. */
export type SetRoomNotificationLevelInput = {
  /** The notification level to set. */
  level: NotificationLevel;
  /** The ID of the room. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for setting the notification level for a space. */
export type SetSpaceNotificationLevelInput = {
  /** The notification level to set. */
  level: NotificationLevel;
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type Space = {
  __typename?: 'Space';
  /** Number of assets (attachments) uploaded to this space. */
  assetCount: Scalars['Int']['output'];
  /** List all available space permission identifiers. */
  availablePermissions: Array<Scalars['String']['output']>;
  /** URL to the space's banner image, if set. Pass width and height for a resized thumbnail. */
  bannerUrl?: Maybe<Scalars['String']['output']>;
  /** Optional description of the space's purpose. */
  description?: Maybe<Scalars['String']['output']>;
  /** The space's unique ID. */
  id: Scalars['ID']['output'];
  /**
   * List all instance roles with their space-level permission configurations.
   * Allows space admins to see and modify space permissions for instance roles.
   * Requires: admin.roles.manage permission in the space.
   */
  instanceRoleConfigs: Array<InstanceRoleSpaceConfig>;
  /** URL to the space's logo image, if set. Pass width and height for a resized thumbnail. */
  logoUrl?: Maybe<Scalars['String']['output']>;
  /**
   * Get a single member of this space by user ID.
   * Requires space membership to view.
   * Returns null if the user is not a member of the space.
   */
  member?: Maybe<User>;
  /** Number of members in this space. */
  memberCount: Scalars['Int']['output'];
  /**
   * List members of this space with optional search and pagination.
   * Requires space membership to view.
   * Search matches login and display name (case-insensitive partial match).
   */
  members: SpaceMembersConnection;
  /** The space's name. */
  name: Scalars['String']['output'];
  /** Get a single role by name. Returns null if not found. */
  role?: Maybe<Role>;
  /** Get users assigned to a specific space role. */
  roleUsers: Array<User>;
  /** List all roles in this space. Requires space membership. */
  roles: Array<Role>;
  /** Number of rooms in this space. */
  roomCount: Scalars['Int']['output'];
  /** Room layout for the sidebar. Null if no custom layout is configured. */
  roomLayout?: Maybe<RoomLayout>;
  /** List of rooms within this space. */
  rooms: Array<Room>;
  /**
   * Get permissions denied for the user via their roles.
   * Used for UI to show when a permission is blocked via roles.
   * Requires space membership.
   */
  userRoleBasedDenials: Array<Scalars['String']['output']>;
  /**
   * Get permissions the user would have via roles.
   * Implements deny-override: if ANY role denies, permission is blocked regardless of grants.
   * Requires space membership.
   */
  userRoleBasedPermissions: Array<Scalars['String']['output']>;
  /** Whether the current user can assign roles to users in this space (has admin.roles.assign permission). */
  viewerCanAssignRoles: Scalars['Boolean']['output'];
  /** Whether the current user can browse rooms in this space (has rooms.browse permission). */
  viewerCanBrowseRooms: Scalars['Boolean']['output'];
  /** Whether the current user can create rooms in this space (has rooms.create permission). */
  viewerCanCreateRoom: Scalars['Boolean']['output'];
  /** Whether the current user can invite new members to this space (has admin.members.invite permission). */
  viewerCanInviteMembers: Scalars['Boolean']['output'];
  /** Whether the current user can join this space (has space.join permission at instance or space level). */
  viewerCanJoinSpace: Scalars['Boolean']['output'];
  /** Whether the current user can manage roles in this space (has admin.roles.manage permission). */
  viewerCanManageRoles: Scalars['Boolean']['output'];
  /** Whether the current user can manage rooms in this space (has room.manage permission). */
  viewerCanManageRooms: Scalars['Boolean']['output'];
  /** Whether the current user can manage this space (has admin.space.manage permission). */
  viewerCanManageSpace: Scalars['Boolean']['output'];
  /**
   * Check if the viewer can manage a specific user based on role hierarchy.
   * Returns true if the viewer's highest role outranks the target user's highest role.
   */
  viewerCanManageUser: Scalars['Boolean']['output'];
  /** Whether the current user has any admin.* permission in this space (for showing Space Admin link). */
  viewerHasAnyAdminPermission: Scalars['Boolean']['output'];
  /** Whether the current user has any unread messages in rooms they've joined in this space. */
  viewerHasUnreadRooms: Scalars['Boolean']['output'];
  /** Whether the current user is a member of this space. */
  viewerIsMember: Scalars['Boolean']['output'];
  /** The current user's notification preference for this space. Null if not authenticated. */
  viewerNotificationPreference?: Maybe<ViewerNotificationPreference>;
  /** Get the current user's permissions in this space. */
  viewerPermissions: Array<Scalars['String']['output']>;
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceBannerUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceLogoUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceMemberArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceMembersArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceRoleArgs = {
  name: Scalars['String']['input'];
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceRoleUsersArgs = {
  roleName: Scalars['String']['input'];
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceUserRoleBasedDenialsArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceUserRoleBasedPermissionsArgs = {
  userId: Scalars['ID']['input'];
};


/**
 * A Space represents a single Chatto workspace containing one or more
 * chat rooms.
 */
export type SpaceViewerCanManageUserArgs = {
  userId: Scalars['ID']['input'];
};

/** Event: A space was created (instance-level for discovery) */
export type SpaceCreatedEvent = {
  __typename?: 'SpaceCreatedEvent';
  /** The space's description. */
  description: Scalars['String']['output'];
  /** The space's name. */
  name: Scalars['String']['output'];
  /** The ID of the newly created space. */
  spaceId: Scalars['ID']['output'];
};

/** Event: A space was deleted */
export type SpaceDeletedEvent = {
  __typename?: 'SpaceDeletedEvent';
  /** The ID of the deleted space. */
  spaceId: Scalars['ID']['output'];
};

/**
 * SpaceEvent wraps all space-scoped events.
 *
 * Events are either:
 * - Stored in JetStream streams — persisted events
 * - Published to NATS Core for real-time updates — live events
 *
 * Authorization is determined by NATS subject:
 * - Space events: space.{spaceId}.> (JetStream) or live.space.{spaceId}.> (NATS Core)
 */
export type SpaceEvent = {
  __typename?: 'SpaceEvent';
  /** The user who triggered this event. May be null if user was deleted. */
  actor?: Maybe<User>;
  /** The ID of the user who triggered this event. */
  actorId: Scalars['ID']['output'];
  /** When this event was created. */
  createdAt: Scalars['Time']['output'];
  /** The concrete event data. */
  event: SpaceEventType;
  /** Universal event identifier. */
  id: Scalars['ID']['output'];
};

/** Union of all space-scoped event types (both persisted and live). */
export type SpaceEventType = CallParticipantJoinedEvent | CallParticipantLeftEvent | MessageDeletedEvent | MessagePostedEvent | MessageUpdatedEvent | PresenceChangedEvent | ReactionAddedEvent | ReactionRemovedEvent | RoomArchivedEvent | RoomCreatedEvent | RoomDeletedEvent | RoomUnarchivedEvent | RoomUpdatedEvent | SpaceMemberDeletedEvent | UserJoinedRoomEvent | UserLeftRoomEvent | UserTypingEvent | VideoProcessingCompletedEvent;

/**
 * Event: A space member's account was deleted.
 * Published to notify clients to update member lists and refresh messages
 * to show "Deleted User" and unavailable content.
 */
export type SpaceMemberDeletedEvent = {
  __typename?: 'SpaceMemberDeletedEvent';
  /** The ID of the space. */
  spaceId: Scalars['ID']['output'];
  /** The ID of the deleted user. */
  userId: Scalars['ID']['output'];
};

/** Paginated list of space members with metadata. */
export type SpaceMembersConnection = {
  __typename?: 'SpaceMembersConnection';
  /** Whether there are more members beyond this page. */
  hasMore: Scalars['Boolean']['output'];
  /** Total count of members matching the search (before pagination). */
  totalCount: Scalars['Int']['output'];
  /** The users who are members of this space. */
  users: Array<User>;
};

/** Event: A space was updated */
export type SpaceUpdatedEvent = {
  __typename?: 'SpaceUpdatedEvent';
  /** The space's banner URL (empty if no banner). */
  bannerUrl: Scalars['String']['output'];
  /** The space's updated description. */
  description: Scalars['String']['output'];
  /** The space's logo URL (empty if no logo). */
  logoUrl: Scalars['String']['output'];
  /** The space's updated name. */
  name: Scalars['String']['output'];
  /** The ID of the updated space. */
  spaceId: Scalars['ID']['output'];
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
   * Subscribe to instance-level events, filtered for the current user.
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
   *   termination, space membership changes): delivered only to the target user.
   *   These enable cross-tab/cross-device sync.
   *
   * **Space events** (space created/updated, new message indicators):
   * Delivered only to members of the affected space. For new-message-in-space
   * events, an additional room membership check ensures unread indicators
   * only appear for rooms the user has joined.
   *
   * **Side effects:**
   * - Subscribing sets the user's presence status to ONLINE.
   * - Presence is refreshed every 30s while active (60s TTL).
   * - When the subscription ends, presence expires via TTL.
   * - Receiving a session termination event closes the stream server-side.
   *
   * Only streams new events — no replay of historical events.
   */
  myInstanceEvents: InstanceEvent;
  /**
   * Subscribe to events within a specific space, filtered for the current user.
   *
   * Requires space membership. Events are server-side filtered so clients only
   * receive what they are authorized to see.
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
   * **Space-level events** (member removed):
   * Delivered to all space members regardless of room membership.
   *
   * **Presence changes** (online/offline/away/DND status):
   * Delivered for users who are members of this space. Non-members' presence
   * changes are filtered out server-side using a lazy membership cache.
   *
   * Only streams new events — no replay of historical events.
   */
  mySpaceEvents: SpaceEvent;
};


/** Root subscription type. */
export type SubscriptionMySpaceEventsArgs = {
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the thread. */
  spaceId: Scalars['ID']['output'];
  /** The root event ID of the thread. */
  threadRootEventId: Scalars['ID']['output'];
};

/**
 * A role's permission state at a single tier (instance, space, or room).
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
   * Permissions allowed by inheritance from the tiers above this one. For
   * instance scope this is always empty; for space scope it reflects the
   * instance tier (instance roles only); for room scope it reflects the
   * resolved space + instance state for this role.
   */
  inheritedAllows: Array<Scalars['String']['output']>;
  /** Permissions denied by inheritance from the tiers above this one. */
  inheritedDenials: Array<Scalars['String']['output']>;
  /** Whether this is an instance role (false for space roles). */
  isInstanceRole: Scalars['Boolean']['output'];
  /** Whether this is a system role and cannot be deleted. */
  isSystem: Scalars['Boolean']['output'];
  /**
   * Explicit allow/deny at the requested tier. Allow and deny lists may
   * both be empty for a role with no override at this tier.
   */
  override: TierPermissions;
  /** Hierarchy position; lower means higher rank. */
  position: Scalars['Int']['output'];
  /** Internal role name (e.g. 'admin', 'instance-admin'). */
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
  /**
   * Roles applicable at this tier, ordered for display: at instance scope
   * all instance roles by position; at space and room scope, space roles
   * by position followed by instance roles by position.
   */
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for unfollowing a thread. */
export type UnfollowThreadInput = {
  /** The ID of the room containing the thread. */
  roomId: Scalars['ID']['input'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
  /** The event ID of the thread root message. */
  threadRootEventId: Scalars['ID']['input'];
};

/** Input for unsubscribing from push notifications. */
export type UnsubscribeFromPushInput = {
  /** The push service endpoint URL to unsubscribe. */
  endpoint: Scalars['String']['input'];
};

/** Input for updating instance configuration. */
export type UpdateInstanceConfigInput = {
  /** Blocked usernames (newline-separated). Set to empty string to clear. */
  blockedUsernames?: InputMaybe<Scalars['String']['input']>;
  /** Instance name for page titles. Set to empty string to use default. */
  instanceName?: InputMaybe<Scalars['String']['input']>;
  /** Message of the Day for the header. Set to empty string to clear. */
  motd?: InputMaybe<Scalars['String']['input']>;
  /** OpenGraph description for link previews. Set to empty string to use default. */
  ogDescription?: InputMaybe<Scalars['String']['input']>;
  /** OpenGraph title for link previews. Set to empty string to use instance name. */
  ogTitle?: InputMaybe<Scalars['String']['input']>;
  /** Welcome message shown on the login page. Set to empty string to clear. */
  welcomeMessage?: InputMaybe<Scalars['String']['input']>;
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['input'];
};

/** Input for updating the room layout of a space. */
export type UpdateRoomLayoutInput = {
  /** The new layout sections in display order. */
  sections: Array<RoomLayoutSectionInput>;
  /** The ID of the space to update. */
  spaceId: Scalars['ID']['input'];
  /** Ordered list of unsectioned room IDs. When provided, unsectioned rooms are displayed in this order. */
  unsectionedRoomIds?: InputMaybe<Array<Scalars['ID']['input']>>;
};

/** Input for updating an existing space. */
export type UpdateSpaceInput = {
  /** The new description for the space. */
  description?: InputMaybe<Scalars['String']['input']>;
  /** The ID of the space to update. */
  id: Scalars['ID']['input'];
  /** The new name for the space. */
  name: Scalars['String']['input'];
};

/** Input for updating an existing space role. */
export type UpdateSpaceRoleInput = {
  /** Role description. */
  description: Scalars['String']['input'];
  /** Human-readable display name. */
  displayName: Scalars['String']['input'];
  /** The name of the role to update. */
  name: Scalars['String']['input'];
  /** The ID of the space containing the role. */
  spaceId: Scalars['ID']['input'];
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

/** Input for uploading an OpenGraph image. */
export type UploadInstanceOgImageInput = {
  /** The OG image file to upload. */
  file: Scalars['Upload']['input'];
};

/** Input for uploading a user avatar. */
export type UploadMyAvatarInput = {
  /** The avatar image file to upload. */
  file: Scalars['Upload']['input'];
};

/** Input for uploading a space banner. */
export type UploadSpaceBannerInput = {
  /** The banner image file. */
  file: Scalars['Upload']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/** Input for uploading a space logo. */
export type UploadSpaceLogoInput = {
  /** The logo image file. */
  file: Scalars['Upload']['input'];
  /** The ID of the space. */
  spaceId: Scalars['ID']['input'];
};

/**
 * A Chatto User. Users can be members of any number of Spaces within the
 * same Chatto instance.
 */
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
  /** Instance roles assigned to this user (e.g., admin). Visible to any authenticated user. */
  instanceRoles: Array<Scalars['String']['output']>;
  /** When the user last changed their login/username. Null if never changed. Visible to the user themselves and to instance admins. */
  lastLoginChange?: Maybe<Scalars['Time']['output']>;
  /** The user's login name (unique identifier for authentication). */
  login: Scalars['String']['output'];
  /** Get user's presence status. Returns OFFLINE if not present. */
  presenceStatus: PresenceStatus;
  /**
   * All room notification preferences for rooms the user has joined, across all spaces.
   * Returns one entry per joined room with its notification level.
   * Self-only: only the user themselves can query this.
   */
  roomNotificationPreferences: Array<RoomNotificationPreferenceItem>;
  /** Rooms the user is a member of in a specific space. Only visible to the user themselves. */
  rooms: Array<Room>;
  /** The user's display preferences. Self-only: returns null for other users. */
  settings?: Maybe<UserSettings>;
  /**
   * Roles assigned to this user in a specific space.
   * Requires space membership to view (caller must be a member of the space).
   * Returns empty array if user is not a member of the space.
   */
  spaceRoles: Array<Scalars['String']['output']>;
  /** Spaces the user is a member of. Only visible to the user themselves. */
  spaces: Array<Space>;
  /** The user's verified email addresses. Only visible to admins and the user themselves. */
  verifiedEmails: Array<Scalars['String']['output']>;
  /** Whether the currently authenticated user can delete this account. */
  viewerCanDeleteAccount: Scalars['Boolean']['output'];
};


/**
 * A Chatto User. Users can be members of any number of Spaces within the
 * same Chatto instance.
 */
export type UserAvatarUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


/**
 * A Chatto User. Users can be members of any number of Spaces within the
 * same Chatto instance.
 */
export type UserRoomsArgs = {
  spaceId: Scalars['ID']['input'];
};


/**
 * A Chatto User. Users can be members of any number of Spaces within the
 * same Chatto instance.
 */
export type UserSpaceRolesArgs = {
  spaceId: Scalars['ID']['input'];
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
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
};

/** Event: A user joined a space */
export type UserJoinedSpaceEvent = {
  __typename?: 'UserJoinedSpaceEvent';
  /** The ID of the space the user joined. */
  spaceId: Scalars['ID']['output'];
};

/** Event: A user left a room */
export type UserLeftRoomEvent = {
  __typename?: 'UserLeftRoomEvent';
  /** The ID of the room the user left. */
  roomId: Scalars['ID']['output'];
  /** The ID of the space containing the room. */
  spaceId: Scalars['ID']['output'];
};

/** Event: A user left a space */
export type UserLeftSpaceEvent = {
  __typename?: 'UserLeftSpaceEvent';
  /** The ID of the space the user left. */
  spaceId: Scalars['ID']['output'];
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
 * The user who is typing is identified by the parent SpaceEvent's actorId/actor.
 */
export type UserTypingEvent = {
  __typename?: 'UserTypingEvent';
  /** The ID of the room where the user is typing. */
  roomId: Scalars['ID']['output'];
  /** The ID of the space where the user is typing. */
  spaceId: Scalars['ID']['output'];
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
  /** The space ID. */
  spaceId: Scalars['ID']['output'];
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
 * The current authenticated user's instance-level permissions.
 * Null if not authenticated.
 */
export type Viewer = {
  __typename?: 'Viewer';
  /** Whether the viewer can create and edit instance roles. */
  canAdminManageRoles: Scalars['Boolean']['output'];
  /** Whether the viewer can manage user role assignments. */
  canAdminManageUsers: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin audit log. */
  canAdminViewAudit: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin roles page. */
  canAdminViewRoles: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin spaces page. */
  canAdminViewSpaces: Scalars['Boolean']['output'];
  /** Whether the viewer can view admin system and data pages. */
  canAdminViewSystem: Scalars['Boolean']['output'];
  /** Whether the viewer can view the admin users page. */
  canAdminViewUsers: Scalars['Boolean']['output'];
  /** Whether the viewer can create new spaces. */
  canCreateSpace: Scalars['Boolean']['output'];
  /** Whether the viewer can browse the space directory. */
  canListSpaces: Scalars['Boolean']['output'];
  /** Whether the viewer can access the admin panel (includes config-admin check). */
  canViewAdmin: Scalars['Boolean']['output'];
  /** Whether the viewer can access direct messages. */
  canViewDMs: Scalars['Boolean']['output'];
  /** Whether the viewer can start DM conversations and send messages. */
  canWriteDMs: Scalars['Boolean']['output'];
};

/**
 * The viewer's notification preference for a space or room.
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

export type CreateSpaceMutationVariables = Exact<{
  input: CreateSpaceInput;
}>;


export type CreateSpaceMutation = { __typename?: 'Mutation', createSpace: { __typename?: 'Space', id: string, name: string, description?: string | null } };

export type SpaceListInitQueryVariables = Exact<{ [key: string]: never; }>;


export type SpaceListInitQuery = { __typename?: 'Query', me?: { __typename?: 'User', spaces: Array<(
      { __typename?: 'Space', viewerHasUnreadRooms: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }
      & { ' $fragmentRefs'?: { 'SpaceIconSpaceFragment': SpaceIconSpaceFragment } }
    )>, roomNotificationPreferences: Array<{ __typename?: 'RoomNotificationPreferenceItem', spaceId: string, roomId: string, level: NotificationLevel, effectiveLevel: NotificationLevel }> } | null, dmSpace?: { __typename?: 'Space', rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> } | null, viewer?: { __typename?: 'Viewer', canViewAdmin: boolean, canCreateSpace: boolean, canListSpaces: boolean, canViewDMs: boolean, canWriteDMs: boolean, canAdminViewUsers: boolean, canAdminManageUsers: boolean, canAdminViewSpaces: boolean, canAdminViewRoles: boolean, canAdminManageRoles: boolean, canAdminViewSystem: boolean, canAdminViewAudit: boolean } | null };

export type GetAllSpacesQueryVariables = Exact<{ [key: string]: never; }>;


export type GetAllSpacesQuery = { __typename?: 'Query', me?: { __typename?: 'User', spaces: Array<(
      { __typename?: 'Space' }
      & { ' $fragmentRefs'?: { 'SpaceIconSpaceFragment': SpaceIconSpaceFragment } }
    )> } | null };

export type FirstUnreadRoomQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type FirstUnreadRoomQuery = { __typename?: 'Query', space?: { __typename?: 'Space', rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean }> } | null };

export type RoomSettingsDataQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type RoomSettingsDataQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string, description?: string | null } | null, space?: { __typename?: 'Space', viewerCanManageRooms: boolean } | null };

export type UpdateRoomSettingsMutationVariables = Exact<{
  input: UpdateRoomInput;
}>;


export type UpdateRoomSettingsMutation = { __typename?: 'Mutation', updateRoom: { __typename?: 'Room', id: string, name: string, description?: string | null } };

export type SpaceIconSpaceFragment = { __typename?: 'Space', id: string, name: string, logoUrl?: string | null } & { ' $fragmentName'?: 'SpaceIconSpaceFragment' };

export type SpaceSettingsModalQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type SpaceSettingsModalQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, name: string, description?: string | null, logoUrl?: string | null, bannerUrl?: string | null, viewerCanManageSpace: boolean } | null };

export type UpdateSpaceSettingsModalMutationVariables = Exact<{
  input: UpdateSpaceInput;
}>;


export type UpdateSpaceSettingsModalMutation = { __typename?: 'Mutation', updateSpace: { __typename?: 'Space', id: string, name: string, description?: string | null } };

export type UploadSpaceLogoMutationVariables = Exact<{
  input: UploadSpaceLogoInput;
}>;


export type UploadSpaceLogoMutation = { __typename?: 'Mutation', uploadSpaceLogo: { __typename?: 'Space', id: string, logoUrl?: string | null } };

export type DeleteSpaceLogoMutationVariables = Exact<{
  input: DeleteSpaceLogoInput;
}>;


export type DeleteSpaceLogoMutation = { __typename?: 'Mutation', deleteSpaceLogo: { __typename?: 'Space', id: string, logoUrl?: string | null } };

export type UploadSpaceBannerMutationVariables = Exact<{
  input: UploadSpaceBannerInput;
}>;


export type UploadSpaceBannerMutation = { __typename?: 'Mutation', uploadSpaceBanner: { __typename?: 'Space', id: string, bannerUrl?: string | null } };

export type DeleteSpaceBannerMutationVariables = Exact<{
  input: DeleteSpaceBannerInput;
}>;


export type DeleteSpaceBannerMutation = { __typename?: 'Mutation', deleteSpaceBanner: { __typename?: 'Space', id: string, bannerUrl?: string | null } };

export type LoadCurrentUserQueryVariables = Exact<{ [key: string]: never; }>;


export type LoadCurrentUserQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus, hasVerifiedEmail: boolean, settings?: { __typename?: 'UserSettings', timezone?: string | null, timeFormat: TimeFormat } | null } | null };

export type LinkPreviewViewFragment = { __typename?: 'LinkPreview', url: string, title?: string | null, description?: string | null, imageUrl?: string | null, siteName?: string | null, embedType?: string | null, embedId?: string | null } & { ' $fragmentName'?: 'LinkPreviewViewFragment' };

export type MessagePreviewQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type MessagePreviewQuery = { __typename?: 'Query', roomEventByEventId?: { __typename?: 'SpaceEvent', id: string, createdAt: any, actor?: (
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
      | { __typename: 'SpaceMemberDeletedEvent' }
      | { __typename: 'UserJoinedRoomEvent' }
      | { __typename: 'UserLeftRoomEvent' }
      | { __typename: 'UserTypingEvent' }
      | { __typename: 'VideoProcessingCompletedEvent' }
     } | null, space?: { __typename?: 'Space', id: string, name: string } | null, room?: { __typename?: 'Room', id: string, name: string } | null };

export type QuickSwitcherSpacesQueryVariables = Exact<{ [key: string]: never; }>;


export type QuickSwitcherSpacesQuery = { __typename?: 'Query', me?: { __typename?: 'User', spaces: Array<{ __typename?: 'Space', id: string, name: string, logoUrl?: string | null }> } | null, viewer?: { __typename?: 'Viewer', canListSpaces: boolean, canViewDMs: boolean } | null };

export type QuickSwitcherRoomsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type QuickSwitcherRoomsQuery = { __typename?: 'Query', me?: { __typename?: 'User', rooms: Array<{ __typename?: 'Room', id: string, name: string }> } | null };

export type QuickSwitcherDMsQueryVariables = Exact<{ [key: string]: never; }>;


export type QuickSwitcherDMsQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string } | null, space?: { __typename?: 'Space', rooms: Array<{ __typename?: 'Room', id: string, members: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> }> } | null };

export type QuickSwitcherSpaceMembersSearchQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  search: Scalars['String']['input'];
  limit: Scalars['Int']['input'];
}>;


export type QuickSwitcherSpaceMembersSearchQuery = { __typename?: 'Query', space?: { __typename?: 'Space', members: { __typename?: 'SpaceMembersConnection', users: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> } } | null };

export type SpaceCardSpaceFragment = { __typename?: 'Space', id: string, name: string, description?: string | null, logoUrl?: string | null, bannerUrl?: string | null, memberCount: number, viewerCanJoinSpace: boolean, viewerIsMember: boolean } & { ' $fragmentName'?: 'SpaceCardSpaceFragment' };

export type UserAvatarUserFragment = { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus } & { ' $fragmentName'?: 'UserAvatarUserFragment' };

export type PostMessageMutationVariables = Exact<{
  input: PostMessageInput;
}>;


export type PostMessageMutation = { __typename?: 'Mutation', postMessage: { __typename?: 'SpaceEvent', id: string } };

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
  spaceId?: InputMaybe<Scalars['ID']['input']>;
  roomId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type PermissionInspectorQuery = { __typename?: 'Query', permissionExplanation: Array<{ __typename?: 'PermissionExplanation', permission: string, state: PermissionDecisionKind, decidedAt?: PermissionLevel | null, decidedByRole?: string | null, trace: Array<{ __typename?: 'PermissionTraceEntry', level: PermissionLevel, roleName: string, decision: PermissionDecisionKind, applied: boolean }> }> };

export type MatrixTierRolesQueryVariables = Exact<{
  spaceId?: InputMaybe<Scalars['ID']['input']>;
  roomId?: InputMaybe<Scalars['ID']['input']>;
}>;


export type MatrixTierRolesQuery = { __typename?: 'Query', tierRoles?: { __typename?: 'TierRoles', applicablePermissions: Array<string>, roles: Array<{ __typename?: 'TierRole', roleName: string, displayName: string, description: string, isInstanceRole: boolean, isSystem: boolean, position: number, inheritedAllows: Array<string>, inheritedDenials: Array<string>, override: { __typename?: 'TierPermissions', permissions: Array<string>, permissionDenials: Array<string> } }> } | null };

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

export type MatrixGrantInstanceRoleSpacePermMutationVariables = Exact<{
  input: GrantInstanceRoleSpacePermissionInput;
}>;


export type MatrixGrantInstanceRoleSpacePermMutation = { __typename?: 'Mutation', grantInstanceRoleSpacePermission: boolean };

export type MatrixDenyInstanceRoleSpacePermMutationVariables = Exact<{
  input: DenyInstanceRoleSpacePermissionInput;
}>;


export type MatrixDenyInstanceRoleSpacePermMutation = { __typename?: 'Mutation', denyInstanceRoleSpacePermission: boolean };

export type MatrixClearInstanceRoleSpacePermMutationVariables = Exact<{
  input: ClearInstanceRoleSpacePermissionInput;
}>;


export type MatrixClearInstanceRoleSpacePermMutation = { __typename?: 'Mutation', clearInstanceRoleSpacePermission: boolean };

export type MatrixGrantSpacePermMutationVariables = Exact<{
  input: GrantSpacePermissionInput;
}>;


export type MatrixGrantSpacePermMutation = { __typename?: 'Mutation', grantSpacePermission: boolean };

export type MatrixDenySpacePermMutationVariables = Exact<{
  input: DenySpacePermissionInput;
}>;


export type MatrixDenySpacePermMutation = { __typename?: 'Mutation', denySpacePermission: boolean };

export type MatrixClearSpacePermMutationVariables = Exact<{
  input: ClearSpacePermissionStateInput;
}>;


export type MatrixClearSpacePermMutation = { __typename?: 'Mutation', clearSpacePermissionState: boolean };

export type MatrixGrantInstancePermMutationVariables = Exact<{
  input: GrantInstancePermissionInput;
}>;


export type MatrixGrantInstancePermMutation = { __typename?: 'Mutation', grantInstancePermission: boolean };

export type MatrixDenyInstancePermMutationVariables = Exact<{
  input: DenyInstancePermissionInput;
}>;


export type MatrixDenyInstancePermMutation = { __typename?: 'Mutation', denyInstancePermission: boolean };

export type MatrixClearInstancePermMutationVariables = Exact<{
  input: ClearInstancePermissionStateInput;
}>;


export type MatrixClearInstancePermMutation = { __typename?: 'Mutation', clearInstancePermissionState: boolean };

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
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type GetRoomQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string, viewerCanPostMessage: boolean, viewerCanPostInThread: boolean, viewerCanReply: boolean, viewerCanReplyInThread: boolean, viewerCanReact: boolean, viewerCanEditOwnMessage: boolean, viewerCanEditAnyMessage: boolean, viewerCanDeleteOwnMessage: boolean, viewerCanDeleteAnyMessage: boolean, viewerCanEchoMessage: boolean, members: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } | null, space?: { __typename?: 'Space', id: string, name: string, viewerCanManageRooms: boolean } | null };

export type GetDmRoomMembersQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type GetDmRoomMembersQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, members: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }> } | null, me?: { __typename?: 'User', id: string } | null };

export type GetRoomMembersForStoreQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
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


export type MyInstanceEventsSubscription = { __typename?: 'Subscription', myInstanceEvents: { __typename?: 'InstanceEvent', actorId: string, event:
      | { __typename: 'InstanceConfigUpdatedEvent', instanceName: string, motd?: string | null, welcomeMessage?: string | null }
      | { __typename: 'InstanceUserPreferencesUpdatedEvent', timezone: string, timeFormat: TimeFormat }
      | { __typename: 'MentionNotificationEvent', spaceId: string, roomId: string, space: { __typename?: 'Space', name: string }, room: { __typename?: 'Room', name: string }, actor: { __typename?: 'User', id: string, displayName: string } }
      | { __typename: 'NewDirectMessageNotificationEvent', roomId: string, conversationName: string, sender: { __typename?: 'User', id: string, displayName: string, avatarUrl?: string | null } }
      | { __typename: 'NewMessageInSpaceEvent', spaceId: string, roomId: string }
      | { __typename: 'NotificationCreatedEvent', notificationId: string, spaceId: string, roomId: string, eventId?: string | null, inReplyToId?: string | null }
      | { __typename: 'NotificationDismissedEvent', notificationId: string }
      | { __typename: 'NotificationLevelChangedEvent', level: NotificationLevel, effectiveLevel: NotificationLevel, nlcSpaceId: string, nlcRoomId?: string | null }
      | { __typename: 'RoomLayoutUpdatedEvent', rluSpaceId: string }
      | { __typename: 'RoomMarkedAsReadEvent', spaceId: string, roomId: string }
      | { __typename: 'SessionTerminatedEvent', reason: string }
      | { __typename: 'SpaceCreatedEvent', spaceId: string }
      | { __typename: 'SpaceDeletedEvent', spaceId: string }
      | { __typename: 'SpaceUpdatedEvent', spaceId: string, name: string, description: string, logoUrl: string, bannerUrl: string }
      | { __typename: 'ThreadFollowChangedEvent', threadRootEventId: string, isFollowing: boolean, tfcSpaceId: string, tfcRoomId: string }
      | { __typename: 'UserCreatedEvent' }
      | { __typename: 'UserDeletedEvent' }
      | { __typename: 'UserJoinedSpaceEvent', spaceId: string }
      | { __typename: 'UserLeftSpaceEvent', spaceId: string }
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

export type SpaceEventBusSubscriptionSubscriptionVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type SpaceEventBusSubscriptionSubscription = { __typename?: 'Subscription', mySpaceEvents: (
    { __typename?: 'SpaceEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  ) };

export type GetDmConversationsForListQueryVariables = Exact<{ [key: string]: never; }>;


export type GetDmConversationsForListQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string } | null, space?: { __typename?: 'Space', rooms: Array<{ __typename?: 'Room', id: string, hasUnread: boolean, members: Array<(
        { __typename?: 'User' }
        & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
      )> }> } | null };

export type GetActiveCallRoomIdsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type GetActiveCallRoomIdsQuery = { __typename?: 'Query', activeCallRoomIds: Array<string> };

export type GetSidebarCallParticipantsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type GetSidebarCallParticipantsQuery = { __typename?: 'Query', callParticipants: Array<{ __typename?: 'CallParticipant', userId: string, displayName: string, login: string, avatarUrl?: string | null, joinedAt: number }> };

export type GetCallParticipantsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type GetCallParticipantsQuery = { __typename?: 'Query', callParticipants: Array<{ __typename?: 'CallParticipant', userId: string, displayName: string, login: string, avatarUrl?: string | null, joinedAt: number }> };

export type NotificationsQueryVariables = Exact<{ [key: string]: never; }>;


export type NotificationsQuery = { __typename?: 'Query', notifications: Array<
    | { __typename: 'DMMessageNotificationItem', id: string, createdAt: any, summary: string, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, room: { __typename?: 'Room', id: string } }
    | { __typename: 'MentionNotificationItem', id: string, createdAt: any, summary: string, mentionEventId: string, mentionInThread?: string | null, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, mentionSpace: { __typename?: 'Space', id: string, name: string }, mentionRoom: { __typename?: 'Room', id: string, name: string } }
    | { __typename: 'ReplyNotificationItem', id: string, createdAt: any, summary: string, inReplyToId: string, replyEventId: string, replyInThread?: string | null, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, replySpace: { __typename?: 'Space', id: string, name: string }, replyRoom: { __typename?: 'Room', id: string, name: string } }
    | { __typename: 'RoomMessageNotificationItem', id: string, createdAt: any, summary: string, roomMsgEventId: string, actor: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, presenceStatus: PresenceStatus }, roomMsgSpace: { __typename?: 'Space', id: string, name: string }, roomMsgRoom: { __typename?: 'Room', id: string, name: string } }
  > };

export type HasNotificationsQueryVariables = Exact<{ [key: string]: never; }>;


export type HasNotificationsQuery = { __typename?: 'Query', hasNotifications: boolean };

export type DismissNotificationMutationVariables = Exact<{
  input: DismissNotificationInput;
}>;


export type DismissNotificationMutation = { __typename?: 'Mutation', dismissNotification: boolean };

export type DismissAllNotificationsMutationVariables = Exact<{ [key: string]: never; }>;


export type DismissAllNotificationsMutation = { __typename?: 'Mutation', dismissAllNotifications: number };

export type GetInstanceInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type GetInstanceInfoQuery = { __typename?: 'Query', instance: { __typename?: 'Instance', directRegistrationEnabled: boolean, pushNotificationsEnabled: boolean, vapidPublicKey?: string | null, livekitUrl?: string | null, maxUploadSize: number, maxVideoUploadSize: number, primarySpaceId: string, config: { __typename?: 'InstanceConfig', instanceName: string, motd?: string | null, welcomeMessage?: string | null, ogImageUrl?: string | null } } };

export type GetVoiceCallTokenQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type GetVoiceCallTokenQuery = { __typename?: 'Query', voiceCallToken?: { __typename?: 'VoiceCallToken', token: string } | null };

export type RoomMessagesLatestQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
}>;


export type RoomMessagesLatestQuery = { __typename?: 'Query', roomEvents: { __typename?: 'RoomEventsConnection', hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'SpaceEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesBeforeQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  before?: InputMaybe<Scalars['Time']['input']>;
}>;


export type RoomMessagesBeforeQuery = { __typename?: 'Query', roomEvents: { __typename?: 'RoomEventsConnection', hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'SpaceEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesAfterQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  after?: InputMaybe<Scalars['Time']['input']>;
}>;


export type RoomMessagesAfterQuery = { __typename?: 'Query', roomEvents: { __typename?: 'RoomEventsConnection', hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'SpaceEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesAroundQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
}>;


export type RoomMessagesAroundQuery = { __typename?: 'Query', roomEventsAround: { __typename?: 'RoomEventsAroundResult', targetIndex: number, hasOlder: boolean, hasNewer: boolean, events: Array<(
      { __typename?: 'SpaceEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    )> } };

export type RoomMessagesRefetchOneQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type RoomMessagesRefetchOneQuery = { __typename?: 'Query', roomEventByEventId?: (
    { __typename?: 'SpaceEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  ) | null };

export type ThreadMessagesAllQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  threadRootEventId: Scalars['ID']['input'];
}>;


export type ThreadMessagesAllQuery = { __typename?: 'Query', threadEvents: Array<(
    { __typename?: 'SpaceEvent' }
    & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
  )> };

export type GetAllRoomsInSpaceQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type GetAllRoomsInSpaceQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, rooms: Array<{ __typename?: 'Room', id: string, name: string, description?: string | null, archived: boolean, viewerCanJoinRoom: boolean }> } | null };

export type JoinRoomFromDirectoryMutationVariables = Exact<{
  input: JoinRoomInput;
}>;


export type JoinRoomFromDirectoryMutation = { __typename?: 'Mutation', joinRoom: boolean };

export type LeaveRoomFromDirectoryStoreMutationVariables = Exact<{
  input: LeaveRoomInput;
}>;


export type LeaveRoomFromDirectoryStoreMutation = { __typename?: 'Mutation', leaveRoom: boolean };

export type GetMyRoomsInSpaceQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type GetMyRoomsInSpaceQuery = { __typename?: 'Query', me?: { __typename?: 'User', rooms: Array<{ __typename?: 'Room', id: string, name: string, hasUnread: boolean, hasMention: boolean, archived: boolean, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> } | null, space?: { __typename?: 'Space', roomLayout?: { __typename?: 'RoomLayout', unsectionedRoomIds: Array<string>, sections: Array<{ __typename?: 'RoomLayoutSection', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } | null } | null };

export type LoadInstanceSpacesQueryVariables = Exact<{ [key: string]: never; }>;


export type LoadInstanceSpacesQuery = { __typename?: 'Query', spaces: Array<(
    { __typename?: 'Space' }
    & { ' $fragmentRefs'?: { 'SpaceCardSpaceFragment': SpaceCardSpaceFragment } }
  )>, viewer?: { __typename?: 'Viewer', canListSpaces: boolean } | null };

export type JoinSpaceFromDirectoryMutationVariables = Exact<{
  input: JoinSpaceInput;
}>;


export type JoinSpaceFromDirectoryMutation = { __typename?: 'Mutation', joinSpace: boolean };

export type LeaveRoomFromModalMutationVariables = Exact<{
  input: LeaveRoomInput;
}>;


export type LeaveRoomFromModalMutation = { __typename?: 'Mutation', leaveRoom: boolean };

export type LeaveSpaceFromModalMutationVariables = Exact<{
  input: LeaveSpaceInput;
}>;


export type LeaveSpaceFromModalMutation = { __typename?: 'Mutation', leaveSpace: boolean };

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

export type ValidateSpaceAccessQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type ValidateSpaceAccessQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, name: string, bannerUrl?: string | null, viewerIsMember: boolean, viewerHasAnyAdminPermission: boolean, viewerCanManageSpace: boolean, viewerCanBrowseRooms: boolean, viewerCanManageRooms: boolean, viewerCanManageRoles: boolean, viewerCanAssignRoles: boolean, viewerCanInviteMembers: boolean } | null };

export type GetRoomForSettingsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
}>;


export type GetRoomForSettingsQuery = { __typename?: 'Query', room?: { __typename?: 'Room', id: string, name: string } | null, space?: { __typename?: 'Space', viewerCanManageRooms: boolean } | null };

export type MessageAttachmentViewFragment = { __typename?: 'Attachment', id: string, spaceId: string, filename: string, contentType: string, width: number, height: number, url: string, thumbnailUrl?: string | null, videoProcessing?: { __typename?: 'VideoProcessing', status: VideoProcessingStatus, durationMs?: any | null, width?: number | null, height?: number | null, thumbnailUrl?: string | null, errorMessage?: string | null, variants: Array<{ __typename?: 'VideoVariant', url: string, quality: string, width: number, height: number, size: any }> } | null } & { ' $fragmentName'?: 'MessageAttachmentViewFragment' };

export type FollowThreadMutationVariables = Exact<{
  input: FollowThreadInput;
}>;


export type FollowThreadMutation = { __typename?: 'Mutation', followThread: boolean };

export type UnfollowThreadMutationVariables = Exact<{
  input: UnfollowThreadInput;
}>;


export type UnfollowThreadMutation = { __typename?: 'Mutation', unfollowThread: boolean };

export type ReplyPreviewQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type ReplyPreviewQuery = { __typename?: 'Query', roomEventByEventId?: (
    { __typename?: 'SpaceEvent' }
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

export type RoomEventViewFragment = { __typename?: 'SpaceEvent', id: string, createdAt: any, actorId: string, actor?: (
    { __typename?: 'User' }
    & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
  ) | null, event:
    | { __typename: 'CallParticipantJoinedEvent', spaceId: string, roomId: string }
    | { __typename: 'CallParticipantLeftEvent', spaceId: string, roomId: string }
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
    | { __typename: 'ReactionAddedEvent', spaceId: string, roomId: string, messageEventId: string, emoji: string }
    | { __typename: 'ReactionRemovedEvent', spaceId: string, roomId: string, messageEventId: string, emoji: string }
    | { __typename: 'RoomArchivedEvent', roomId: string }
    | { __typename: 'RoomCreatedEvent' }
    | { __typename: 'RoomDeletedEvent', roomId: string }
    | { __typename: 'RoomUnarchivedEvent', roomId: string }
    | { __typename: 'RoomUpdatedEvent', roomId: string }
    | { __typename: 'SpaceMemberDeletedEvent', spaceId: string, userId: string }
    | { __typename: 'UserJoinedRoomEvent', spaceId: string, roomId: string }
    | { __typename: 'UserLeftRoomEvent', spaceId: string, roomId: string }
    | { __typename: 'UserTypingEvent', spaceId: string, roomId: string, typingThreadRootEventId?: string | null }
    | { __typename: 'VideoProcessingCompletedEvent', spaceId: string, roomId: string, attachmentId: string, messageEventId: string }
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
  spaceId: Scalars['ID']['input'];
  roomId: Scalars['ID']['input'];
  eventId: Scalars['ID']['input'];
}>;


export type ResolveMessageLinkQuery = { __typename?: 'Query', roomEventByEventId?: { __typename?: 'SpaceEvent', id: string, event:
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
      | { __typename: 'SpaceMemberDeletedEvent' }
      | { __typename: 'UserJoinedRoomEvent' }
      | { __typename: 'UserLeftRoomEvent' }
      | { __typename: 'UserTypingEvent' }
      | { __typename: 'VideoProcessingCompletedEvent' }
     } | null };

export type GetSpaceNotificationPreferencesQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type GetSpaceNotificationPreferencesQuery = { __typename?: 'Query', space?: { __typename?: 'Space', viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null } | null, me?: { __typename?: 'User', rooms: Array<{ __typename?: 'Room', id: string, name: string, viewerNotificationPreference?: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } | null }> } | null };

export type SetSpaceNotificationLevelMutationVariables = Exact<{
  input: SetSpaceNotificationLevelInput;
}>;


export type SetSpaceNotificationLevelMutation = { __typename?: 'Mutation', setSpaceNotificationLevel: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } };

export type SetRoomNotificationLevelMutationVariables = Exact<{
  input: SetRoomNotificationLevelInput;
}>;


export type SetRoomNotificationLevelMutation = { __typename?: 'Mutation', setRoomNotificationLevel: { __typename?: 'ViewerNotificationPreference', level: NotificationLevel, effectiveLevel: NotificationLevel } };

export type SpaceMembersQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  search?: InputMaybe<Scalars['String']['input']>;
}>;


export type SpaceMembersQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, roles: Array<{ __typename?: 'Role', name: string, displayName: string }>, members: { __typename?: 'SpaceMembersConnection', totalCount: number, users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, spaceRoles: Array<string>, createdAt?: any | null }> } } | null };

export type SpaceMemberDetailsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  userId: Scalars['ID']['input'];
}>;


export type SpaceMemberDetailsQuery = { __typename?: 'Query', me?: { __typename?: 'User', id: string, spaceRoles: Array<string> } | null, space?: { __typename?: 'Space', id: string, viewerCanAssignRoles: boolean, viewerCanManageRoles: boolean, availablePermissions: Array<string>, roles: Array<{ __typename?: 'Role', name: string, displayName: string, position: number, permissions: Array<string>, permissionDenials: Array<string> }>, member?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, instanceRoles: Array<string>, spaceRoles: Array<string> } | null } | null };

export type RevokeSpaceRoleFromMemberMutationVariables = Exact<{
  input: RevokeSpaceRoleInput;
}>;


export type RevokeSpaceRoleFromMemberMutation = { __typename?: 'Mutation', revokeSpaceRole: boolean };

export type AssignSpaceRoleToMemberMutationVariables = Exact<{
  input: AssignSpaceRoleInput;
}>;


export type AssignSpaceRoleToMemberMutation = { __typename?: 'Mutation', assignSpaceRole: boolean };

export type SpaceRolesGateQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type SpaceRolesGateQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, viewerCanManageRoles: boolean } | null };

export type SpaceRoleDetailQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
  name: Scalars['String']['input'];
}>;


export type SpaceRoleDetailQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, name: string, viewerCanManageRoles: boolean, viewerCanAssignRoles: boolean, role?: { __typename?: 'Role', name: string, displayName: string, description: string, permissions: Array<string>, permissionDenials: Array<string>, isSystem: boolean, position: number } | null, roleUsers: Array<{ __typename?: 'User', id: string, login: string, displayName: string }> } | null };

export type UpdateSpaceRoleMutationVariables = Exact<{
  input: UpdateSpaceRoleInput;
}>;


export type UpdateSpaceRoleMutation = { __typename?: 'Mutation', updateSpaceRole: { __typename?: 'Role', name: string, displayName: string, description: string } };

export type DeleteSpaceRoleMutationVariables = Exact<{
  input: DeleteSpaceRoleInput;
}>;


export type DeleteSpaceRoleMutation = { __typename?: 'Mutation', deleteSpaceRole: boolean };

export type SpaceRolesNewCheckQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type SpaceRolesNewCheckQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, viewerCanManageRoles: boolean } | null };

export type CreateSpaceRoleMutationVariables = Exact<{
  input: CreateSpaceRoleInput;
}>;


export type CreateSpaceRoleMutation = { __typename?: 'Mutation', createSpaceRole: { __typename?: 'Role', name: string, displayName: string, description: string } };

export type AdminRoomLayoutQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type AdminRoomLayoutQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, rooms: Array<{ __typename?: 'Room', id: string, name: string, description?: string | null, archived: boolean, autoJoin: boolean }>, roomLayout?: { __typename?: 'RoomLayout', unsectionedRoomIds: Array<string>, sections: Array<{ __typename?: 'RoomLayoutSection', id: string, name: string, rooms: Array<{ __typename?: 'Room', id: string }> }> } | null } | null };

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

export type MyFollowedThreadsQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type MyFollowedThreadsQuery = { __typename?: 'Query', myFollowedThreads: Array<{ __typename?: 'FollowedThread', spaceId: string, roomId: string, threadRootEventId: string, replyCount: number, lastReplyAt?: any | null, hasUnread: boolean, room: { __typename?: 'Room', name: string }, rootMessage?: (
      { __typename?: 'SpaceEvent' }
      & { ' $fragmentRefs'?: { 'RoomEventViewFragment': RoomEventViewFragment } }
    ) | null, threadParticipants: Array<(
      { __typename?: 'User' }
      & { ' $fragmentRefs'?: { 'UserAvatarUserFragment': UserAvatarUserFragment } }
    )> }> };

export type AdminDashboardUsersQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminDashboardUsersQuery = { __typename?: 'Query', users: Array<{ __typename?: 'User', id: string }> };

export type AdminDashboardSpacesQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminDashboardSpacesQuery = { __typename?: 'Query', spaces: Array<{ __typename?: 'Space', id: string }> };

export type AdminRoleQueryVariables = Exact<{
  name: Scalars['String']['input'];
}>;


export type AdminRoleQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', role?: { __typename?: 'Role', name: string, displayName: string, description: string, permissions: Array<string>, permissionDenials: Array<string>, isSystem: boolean, position: number } | null, instanceRoleUsers: Array<{ __typename?: 'User', id: string, login: string, displayName: string }> } | null };

export type UpdateRoleMutationVariables = Exact<{
  input: UpdateRoleInput;
}>;


export type UpdateRoleMutation = { __typename?: 'Mutation', updateRole: { __typename?: 'Role', name: string, displayName: string, description: string } };

export type DeleteRoleMutationVariables = Exact<{
  input: DeleteRoleInput;
}>;


export type DeleteRoleMutation = { __typename?: 'Mutation', deleteRole: boolean };

export type CreateRoleMutationVariables = Exact<{
  input: CreateRoleInput;
}>;


export type CreateRoleMutation = { __typename?: 'Mutation', createRole: { __typename?: 'Role', name: string } };

export type AdminInstanceConfigQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminInstanceConfigQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', instanceConfig: { __typename?: 'AdminInstanceConfig', isConfigured: boolean, instanceName: string, ogTitle?: string | null, ogDescription?: string | null, ogImageUrl?: string | null, motd?: string | null, welcomeMessage?: string | null, blockedUsernames?: string | null } } | null };

export type UpdateInstanceConfigMutationVariables = Exact<{
  input: UpdateInstanceConfigInput;
}>;


export type UpdateInstanceConfigMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', updateInstanceConfig: { __typename?: 'AdminInstanceConfig', isConfigured: boolean, instanceName: string, ogTitle?: string | null, ogDescription?: string | null, motd?: string | null, welcomeMessage?: string | null, blockedUsernames?: string | null } } | null };

export type ResetInstanceConfigMutationVariables = Exact<{ [key: string]: never; }>;


export type ResetInstanceConfigMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', resetInstanceConfig: boolean } | null };

export type UploadInstanceOgImageMutationVariables = Exact<{
  input: UploadInstanceOgImageInput;
}>;


export type UploadInstanceOgImageMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', uploadInstanceOGImage: { __typename?: 'AdminInstanceConfig', ogImageUrl?: string | null } } | null };

export type DeleteInstanceOgImageMutationVariables = Exact<{ [key: string]: never; }>;


export type DeleteInstanceOgImageMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', deleteInstanceOGImage: { __typename?: 'AdminInstanceConfig', ogImageUrl?: string | null } } | null };

export type AdminSpacesListQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSpacesListQuery = { __typename?: 'Query', spaces: Array<{ __typename?: 'Space', id: string, name: string, description?: string | null, memberCount: number, roomCount: number, assetCount: number }> };

export type AdminSystemInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSystemInfoQuery = { __typename?: 'Query', admin?: { __typename?: 'AdminQueries', systemInfo: { __typename?: 'SystemInfo', connection: { __typename?: 'ConnectionInfo', connected: boolean, serverID: string, serverName: string, version: string, maxPayload: any, rtt: string }, account: { __typename?: 'AccountInfo', memory: any, memoryUsed: any, storage: any, storageUsed: any, streams: number, streamsUsed: number, consumers: number, consumersUsed: number } } } | null };

export type AdminUsersListQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminUsersListQuery = { __typename?: 'Query', users: Array<{ __typename?: 'User', id: string, login: string, displayName: string, hasVerifiedEmail: boolean, verifiedEmails: Array<string> }> };

export type AdminUserDetailsQueryVariables = Exact<{
  userId: Scalars['ID']['input'];
}>;


export type AdminUserDetailsQuery = { __typename?: 'Query', me?: { __typename?: 'User', instanceRoles: Array<string> } | null, user?: { __typename?: 'User', id: string, login: string, displayName: string, avatarUrl?: string | null, verifiedEmails: Array<string>, createdAt?: any | null, lastLoginChange?: any | null } | null, admin?: { __typename?: 'AdminQueries', instancePermissions: Array<string>, userInstanceRoles: Array<string>, userRoleBasedPermissions: Array<string>, userRoleBasedDenials: Array<string>, roles: Array<{ __typename?: 'Role', name: string, displayName: string, description: string, isSystem: boolean, position: number, permissions: Array<string>, permissionDenials: Array<string> }> } | null };

export type AssignInstanceRoleMutationVariables = Exact<{
  input: AssignInstanceRoleInput;
}>;


export type AssignInstanceRoleMutation = { __typename?: 'Mutation', assignInstanceRole: boolean };

export type RevokeInstanceRoleMutationVariables = Exact<{
  input: RevokeInstanceRoleInput;
}>;


export type RevokeInstanceRoleMutation = { __typename?: 'Mutation', revokeInstanceRole: boolean };

export type AdminUpdateUserMutationVariables = Exact<{
  input: AdminUpdateUserInput;
}>;


export type AdminUpdateUserMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', updateUser: { __typename?: 'User', id: string, login: string, displayName: string, lastLoginChange?: any | null } } | null };

export type AdminClearUsernameCooldownMutationVariables = Exact<{
  userId: Scalars['ID']['input'];
}>;


export type AdminClearUsernameCooldownMutation = { __typename?: 'Mutation', admin?: { __typename?: 'AdminMutations', clearUsernameCooldown: boolean } | null };

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

export type CreateSpacePageMutationVariables = Exact<{
  input: CreateSpaceInput;
}>;


export type CreateSpacePageMutation = { __typename?: 'Mutation', createSpace: { __typename?: 'Space', id: string, name: string, description?: string | null } };

export type SpaceJoinPageQueryVariables = Exact<{
  spaceId: Scalars['ID']['input'];
}>;


export type SpaceJoinPageQuery = { __typename?: 'Query', space?: { __typename?: 'Space', id: string, name: string, description?: string | null, memberCount: number, viewerIsMember: boolean } | null, me?: { __typename?: 'User', id: string } | null };

export type JoinSpaceFromInviteMutationVariables = Exact<{
  input: JoinSpaceInput;
}>;


export type JoinSpaceFromInviteMutation = { __typename?: 'Mutation', joinSpace: boolean };

export type LoginPageInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type LoginPageInfoQuery = { __typename?: 'Query', instance: { __typename?: 'Instance', enabledAuthProviders: Array<string>, directRegistrationEnabled: boolean } };

export const SpaceIconSpaceFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SpaceIconSpace"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Space"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}}]} as unknown as DocumentNode<SpaceIconSpaceFragment, unknown>;
export const SpaceCardSpaceFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SpaceCardSpace"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Space"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"384"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"288"}}]},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanJoinSpace"}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsMember"}}]}}]} as unknown as DocumentNode<SpaceCardSpaceFragment, unknown>;
export const UserAvatarUserFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<UserAvatarUserFragment, unknown>;
export const MessageAttachmentViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}}]} as unknown as DocumentNode<MessageAttachmentViewFragment, unknown>;
export const LinkPreviewViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<LinkPreviewViewFragment, unknown>;
export const RoomEventViewFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<RoomEventViewFragment, unknown>;
export const CreateRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateRoomMutation, CreateRoomMutationVariables>;
export const JoinRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinRoomMutation, JoinRoomMutationVariables>;
export const CreateSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateSpace"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateSpaceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createSpace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateSpaceMutation, CreateSpaceMutationVariables>;
export const SpaceListInitDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceListInit"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SpaceIconSpace"}},{"kind":"Field","name":{"kind":"Name","value":"viewerHasUnreadRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"roomNotificationPreferences"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","alias":{"kind":"Name","value":"dmSpace"},"name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"StringValue","value":"DM","block":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"canViewAdmin"}},{"kind":"Field","name":{"kind":"Name","value":"canCreateSpace"}},{"kind":"Field","name":{"kind":"Name","value":"canListSpaces"}},{"kind":"Field","name":{"kind":"Name","value":"canViewDMs"}},{"kind":"Field","name":{"kind":"Name","value":"canWriteDMs"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewUsers"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminManageUsers"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewSpaces"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewRoles"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewSystem"}},{"kind":"Field","name":{"kind":"Name","value":"canAdminViewAudit"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SpaceIconSpace"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Space"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}}]} as unknown as DocumentNode<SpaceListInitQuery, SpaceListInitQueryVariables>;
export const GetAllSpacesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetAllSpaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SpaceIconSpace"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SpaceIconSpace"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Space"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}}]} as unknown as DocumentNode<GetAllSpacesQuery, GetAllSpacesQueryVariables>;
export const FirstUnreadRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FirstUnreadRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}}]}}]}}]}}]} as unknown as DocumentNode<FirstUnreadRoomQuery, FirstUnreadRoomQueryVariables>;
export const RoomSettingsDataDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomSettingsData"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<RoomSettingsDataQuery, RoomSettingsDataQueryVariables>;
export const UpdateRoomSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRoomSettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateRoomSettingsMutation, UpdateRoomSettingsMutationVariables>;
export const SpaceSettingsModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceSettingsModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageSpace"}}]}}]}}]} as unknown as DocumentNode<SpaceSettingsModalQuery, SpaceSettingsModalQueryVariables>;
export const UpdateSpaceSettingsModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateSpaceSettingsModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateSpaceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateSpace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateSpaceSettingsModalMutation, UpdateSpaceSettingsModalMutationVariables>;
export const UploadSpaceLogoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadSpaceLogo"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadSpaceLogoInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadSpaceLogo"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]} as unknown as DocumentNode<UploadSpaceLogoMutation, UploadSpaceLogoMutationVariables>;
export const DeleteSpaceLogoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteSpaceLogo"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteSpaceLogoInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteSpaceLogo"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}}]}}]}}]} as unknown as DocumentNode<DeleteSpaceLogoMutation, DeleteSpaceLogoMutationVariables>;
export const UploadSpaceBannerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadSpaceBanner"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadSpaceBannerInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadSpaceBanner"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]} as unknown as DocumentNode<UploadSpaceBannerMutation, UploadSpaceBannerMutationVariables>;
export const DeleteSpaceBannerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteSpaceBanner"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteSpaceBannerInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteSpaceBanner"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}}]}}]} as unknown as DocumentNode<DeleteSpaceBannerMutation, DeleteSpaceBannerMutationVariables>;
export const LoadCurrentUserDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoadCurrentUser"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}},{"kind":"Field","name":{"kind":"Name","value":"hasVerifiedEmail"}},{"kind":"Field","name":{"kind":"Name","value":"settings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}}]}}]}}]} as unknown as DocumentNode<LoadCurrentUserQuery, LoadCurrentUserQueryVariables>;
export const MessagePreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MessagePreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"120"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"120"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"COVER"}}]}]}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<MessagePreviewQuery, MessagePreviewQueryVariables>;
export const QuickSwitcherSpacesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherSpaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"canListSpaces"}},{"kind":"Field","name":{"kind":"Name","value":"canViewDMs"}}]}}]}}]} as unknown as DocumentNode<QuickSwitcherSpacesQuery, QuickSwitcherSpacesQueryVariables>;
export const QuickSwitcherRoomsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherRooms"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<QuickSwitcherRoomsQuery, QuickSwitcherRoomsQueryVariables>;
export const QuickSwitcherDMsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherDMs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"StringValue","value":"DM","block":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<QuickSwitcherDMsQuery, QuickSwitcherDMsQueryVariables>;
export const QuickSwitcherSpaceMembersSearchDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"QuickSwitcherSpaceMembersSearch"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<QuickSwitcherSpaceMembersSearchQuery, QuickSwitcherSpaceMembersSearchQueryVariables>;
export const PostMessageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PostMessage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PostMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"postMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<PostMessageMutation, PostMessageMutationVariables>;
export const EditMessageFromInputDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EditMessageFromInput"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EditMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"editMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<EditMessageFromInputMutation, EditMessageFromInputMutationVariables>;
export const LinkPreviewForComposerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LinkPreviewForComposer"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"url"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"url"},"value":{"kind":"Variable","name":{"kind":"Name","value":"url"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}},{"kind":"Field","name":{"kind":"Name","value":"imageAssetId"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}}]} as unknown as DocumentNode<LinkPreviewForComposerQuery, LinkPreviewForComposerQueryVariables>;
export const PermissionInspectorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PermissionInspector"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permissionExplanation"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}},{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permission"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"decidedAt"}},{"kind":"Field","name":{"kind":"Name","value":"decidedByRole"}},{"kind":"Field","name":{"kind":"Name","value":"trace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"roleName"}},{"kind":"Field","name":{"kind":"Name","value":"decision"}},{"kind":"Field","name":{"kind":"Name","value":"applied"}}]}}]}}]}}]} as unknown as DocumentNode<PermissionInspectorQuery, PermissionInspectorQueryVariables>;
export const MatrixTierRolesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MatrixTierRoles"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tierRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"applicablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roleName"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isInstanceRole"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"override"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"inheritedAllows"}},{"kind":"Field","name":{"kind":"Name","value":"inheritedDenials"}}]}}]}}]}}]} as unknown as DocumentNode<MatrixTierRolesQuery, MatrixTierRolesQueryVariables>;
export const MatrixGrantRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantRoomPermMutation, MatrixGrantRoomPermMutationVariables>;
export const MatrixDenyRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyRoomPermMutation, MatrixDenyRoomPermMutationVariables>;
export const MatrixClearRoomPermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearRoomPerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearRoomPermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearRoomPermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearRoomPermMutation, MatrixClearRoomPermMutationVariables>;
export const MatrixGrantInstanceRoleSpacePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantInstanceRoleSpacePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantInstanceRoleSpacePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantInstanceRoleSpacePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantInstanceRoleSpacePermMutation, MatrixGrantInstanceRoleSpacePermMutationVariables>;
export const MatrixDenyInstanceRoleSpacePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyInstanceRoleSpacePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyInstanceRoleSpacePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyInstanceRoleSpacePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyInstanceRoleSpacePermMutation, MatrixDenyInstanceRoleSpacePermMutationVariables>;
export const MatrixClearInstanceRoleSpacePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearInstanceRoleSpacePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearInstanceRoleSpacePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearInstanceRoleSpacePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearInstanceRoleSpacePermMutation, MatrixClearInstanceRoleSpacePermMutationVariables>;
export const MatrixGrantSpacePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantSpacePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantSpacePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantSpacePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantSpacePermMutation, MatrixGrantSpacePermMutationVariables>;
export const MatrixDenySpacePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenySpacePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenySpacePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denySpacePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenySpacePermMutation, MatrixDenySpacePermMutationVariables>;
export const MatrixClearSpacePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearSpacePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearSpacePermissionStateInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearSpacePermissionState"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearSpacePermMutation, MatrixClearSpacePermMutationVariables>;
export const MatrixGrantInstancePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixGrantInstancePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GrantInstancePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"grantInstancePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixGrantInstancePermMutation, MatrixGrantInstancePermMutationVariables>;
export const MatrixDenyInstancePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixDenyInstancePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DenyInstancePermissionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"denyInstancePermission"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixDenyInstancePermMutation, MatrixDenyInstancePermMutationVariables>;
export const MatrixClearInstancePermDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MatrixClearInstancePerm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClearInstancePermissionStateInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearInstancePermissionState"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<MatrixClearInstancePermMutation, MatrixClearInstancePermMutationVariables>;
export const StartDmDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartDM"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartDMInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startDM"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<StartDmMutation, StartDmMutationVariables>;
export const AddReactionFromActionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddReactionFromActions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AddReactionFromActionsMutation, AddReactionFromActionsMutationVariables>;
export const RemoveReactionFromActionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveReactionFromActions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RemoveReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RemoveReactionFromActionsMutation, RemoveReactionFromActionsMutationVariables>;
export const GetRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanPostMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanPostInThread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReply"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReplyInThread"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanReact"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEditOwnMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEditAnyMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteOwnMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteAnyMessage"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanEchoMessage"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<GetRoomQuery, GetRoomQueryVariables>;
export const GetDmRoomMembersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetDMRoomMembers"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<GetDmRoomMembersQuery, GetDmRoomMembersQueryVariables>;
export const GetRoomMembersForStoreDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoomMembersForStore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]}}]}}]} as unknown as DocumentNode<GetRoomMembersForStoreQuery, GetRoomMembersForStoreQueryVariables>;
export const MarkRoomAsReadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkRoomAsRead"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MarkRoomAsReadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markRoomAsRead"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previousLastReadAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastReadAt"}}]}}]}}]} as unknown as DocumentNode<MarkRoomAsReadMutation, MarkRoomAsReadMutationVariables>;
export const SendTypingIndicatorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SendTypingIndicator"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SendTypingIndicatorInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sendTypingIndicator"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SendTypingIndicatorMutation, SendTypingIndicatorMutationVariables>;
export const MyInstanceEventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"MyInstanceEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myInstanceEvents"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"InstanceConfigUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"instanceName"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceCreatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedSpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftSpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserProfileUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"login"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"InstanceUserPreferencesUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationLevelChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"nlcSpaceId"},"name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","alias":{"kind":"Name","value":"nlcRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MentionNotificationEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"space"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NewDirectMessageNotificationEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"sender"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}},{"kind":"Field","name":{"kind":"Name","value":"conversationName"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationCreatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notificationId"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyToId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NotificationDismissedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notificationId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"NewMessageInSpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomMarkedAsReadEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ThreadFollowChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"tfcSpaceId"},"name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","alias":{"kind":"Name","value":"tfcRoomId"},"name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"isFollowing"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomLayoutUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"rluSpaceId"},"name":{"kind":"Name","value":"spaceId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionTerminatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reason"}}]}}]}}]}}]}}]} as unknown as DocumentNode<MyInstanceEventsSubscription, MyInstanceEventsSubscriptionVariables>;
export const SubscribeToPushDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SubscribeToPush"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PushSubscriptionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"subscribeToPush"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SubscribeToPushMutation, SubscribeToPushMutationVariables>;
export const UnsubscribeFromPushDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnsubscribeFromPush"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnsubscribeFromPushInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unsubscribeFromPush"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnsubscribeFromPushMutation, UnsubscribeFromPushMutationVariables>;
export const UpdateMyPresenceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMyPresence"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMyPresenceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMyPresence"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UpdateMyPresenceMutation, UpdateMyPresenceMutationVariables>;
export const SpaceEventBusSubscriptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"SpaceEventBusSubscription"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mySpaceEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<SpaceEventBusSubscriptionSubscription, SpaceEventBusSubscriptionSubscriptionVariables>;
export const GetDmConversationsForListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetDmConversationsForList"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"StringValue","value":"DM","block":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}}]} as unknown as DocumentNode<GetDmConversationsForListQuery, GetDmConversationsForListQueryVariables>;
export const GetActiveCallRoomIdsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetActiveCallRoomIds"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"activeCallRoomIds"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}]}]}}]} as unknown as DocumentNode<GetActiveCallRoomIdsQuery, GetActiveCallRoomIdsQueryVariables>;
export const GetSidebarCallParticipantsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetSidebarCallParticipants"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]} as unknown as DocumentNode<GetSidebarCallParticipantsQuery, GetSidebarCallParticipantsQueryVariables>;
export const GetCallParticipantsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetCallParticipants"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"callParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"userId"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]} as unknown as DocumentNode<GetCallParticipantsQuery, GetCallParticipantsQueryVariables>;
export const NotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Notifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"notifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DMMessageNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MentionNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"mentionSpace"},"name":{"kind":"Name","value":"space"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"mentionRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"mentionEventId"},"name":{"kind":"Name","value":"eventId"}},{"kind":"Field","alias":{"kind":"Name","value":"mentionInThread"},"name":{"kind":"Name","value":"inThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplyNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"replySpace"},"name":{"kind":"Name","value":"space"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"replyRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"replyEventId"},"name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyToId"}},{"kind":"Field","alias":{"kind":"Name","value":"replyInThread"},"name":{"kind":"Name","value":"inThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomMessageNotificationItem"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgSpace"},"name":{"kind":"Name","value":"space"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgRoom"},"name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"roomMsgEventId"},"name":{"kind":"Name","value":"eventId"}}]}}]}}]}}]} as unknown as DocumentNode<NotificationsQuery, NotificationsQueryVariables>;
export const HasNotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"HasNotifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNotifications"}}]}}]} as unknown as DocumentNode<HasNotificationsQuery, HasNotificationsQueryVariables>;
export const DismissNotificationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DismissNotification"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DismissNotificationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dismissNotification"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DismissNotificationMutation, DismissNotificationMutationVariables>;
export const DismissAllNotificationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DismissAllNotifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dismissAllNotifications"}}]}}]} as unknown as DocumentNode<DismissAllNotificationsMutation, DismissAllNotificationsMutationVariables>;
export const GetInstanceInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetInstanceInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"instance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"directRegistrationEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"pushNotificationsEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"vapidPublicKey"}},{"kind":"Field","name":{"kind":"Name","value":"livekitUrl"}},{"kind":"Field","name":{"kind":"Name","value":"maxUploadSize"}},{"kind":"Field","name":{"kind":"Name","value":"maxVideoUploadSize"}},{"kind":"Field","name":{"kind":"Name","value":"primarySpaceId"}},{"kind":"Field","name":{"kind":"Name","value":"config"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"instanceName"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"ogImageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"768"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"402"}}]}]}}]}}]}}]} as unknown as DocumentNode<GetInstanceInfoQuery, GetInstanceInfoQueryVariables>;
export const GetVoiceCallTokenDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetVoiceCallToken"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"voiceCallToken"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"token"}}]}}]}}]} as unknown as DocumentNode<GetVoiceCallTokenQuery, GetVoiceCallTokenQueryVariables>;
export const RoomMessagesLatestDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesLatest"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesLatestQuery, RoomMessagesLatestQueryVariables>;
export const RoomMessagesBeforeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesBefore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"before"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Time"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"before"},"value":{"kind":"Variable","name":{"kind":"Name","value":"before"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesBeforeQuery, RoomMessagesBeforeQueryVariables>;
export const RoomMessagesAfterDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesAfter"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Time"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesAfterQuery, RoomMessagesAfterQueryVariables>;
export const RoomMessagesAroundDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesAround"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventsAround"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"targetIndex"}},{"kind":"Field","name":{"kind":"Name","value":"hasOlder"}},{"kind":"Field","name":{"kind":"Name","value":"hasNewer"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesAroundQuery, RoomMessagesAroundQueryVariables>;
export const RoomMessagesRefetchOneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RoomMessagesRefetchOne"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<RoomMessagesRefetchOneQuery, RoomMessagesRefetchOneQueryVariables>;
export const ThreadMessagesAllDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ThreadMessagesAll"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"threadRootEventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"threadEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"threadRootEventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"threadRootEventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ThreadMessagesAllQuery, ThreadMessagesAllQueryVariables>;
export const GetAllRoomsInSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetAllRoomsInSpace"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanJoinRoom"}}]}}]}}]}}]} as unknown as DocumentNode<GetAllRoomsInSpaceQuery, GetAllRoomsInSpaceQueryVariables>;
export const JoinRoomFromDirectoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinRoomFromDirectory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinRoomFromDirectoryMutation, JoinRoomFromDirectoryMutationVariables>;
export const LeaveRoomFromDirectoryStoreDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveRoomFromDirectoryStore"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveRoomFromDirectoryStoreMutation, LeaveRoomFromDirectoryStoreMutationVariables>;
export const GetMyRoomsInSpaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMyRoomsInSpace"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}},{"kind":"Field","name":{"kind":"Name","value":"hasMention"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomLayout"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"unsectionedRoomIds"}}]}}]}}]}}]} as unknown as DocumentNode<GetMyRoomsInSpaceQuery, GetMyRoomsInSpaceQueryVariables>;
export const LoadInstanceSpacesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoadInstanceSpaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SpaceCardSpace"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"canListSpaces"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SpaceCardSpace"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Space"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"logoUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"384"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"288"}}]},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanJoinSpace"}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsMember"}}]}}]} as unknown as DocumentNode<LoadInstanceSpacesQuery, LoadInstanceSpacesQueryVariables>;
export const JoinSpaceFromDirectoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinSpaceFromDirectory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinSpaceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinSpace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinSpaceFromDirectoryMutation, JoinSpaceFromDirectoryMutationVariables>;
export const LeaveRoomFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveRoomFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveRoomFromModalMutation, LeaveRoomFromModalMutationVariables>;
export const LeaveSpaceFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"LeaveSpaceFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LeaveSpaceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"leaveSpace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<LeaveSpaceFromModalMutation, LeaveSpaceFromModalMutationVariables>;
export const DeleteMessageFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMessageFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteMessageFromModalMutation, DeleteMessageFromModalMutationVariables>;
export const DeleteLinkPreviewFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteLinkPreviewFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteLinkPreviewInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteLinkPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteLinkPreviewFromModalMutation, DeleteLinkPreviewFromModalMutationVariables>;
export const DeleteAttachmentFromModalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteAttachmentFromModal"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteAttachmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteAttachment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteAttachmentFromModalMutation, DeleteAttachmentFromModalMutationVariables>;
export const ValidateSpaceAccessDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ValidateSpaceAccess"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"bannerUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"512"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"384"}}]},{"kind":"Field","name":{"kind":"Name","value":"viewerIsMember"}},{"kind":"Field","name":{"kind":"Name","value":"viewerHasAnyAdminPermission"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageSpace"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanBrowseRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanInviteMembers"}}]}}]}}]} as unknown as DocumentNode<ValidateSpaceAccessQuery, ValidateSpaceAccessQueryVariables>;
export const GetRoomForSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetRoomForSettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"room"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRooms"}}]}}]}}]} as unknown as DocumentNode<GetRoomForSettingsQuery, GetRoomForSettingsQueryVariables>;
export const FollowThreadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FollowThread"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"FollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<FollowThreadMutation, FollowThreadMutationVariables>;
export const UnfollowThreadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnfollowThread"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnfollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unfollowThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnfollowThreadMutation, UnfollowThreadMutationVariables>;
export const ReplyPreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ReplyPreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<ReplyPreviewQuery, ReplyPreviewQueryVariables>;
export const AddReactionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddReaction"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AddReactionMutation, AddReactionMutationVariables>;
export const RemoveReactionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveReaction"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RemoveReactionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeReaction"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RemoveReactionMutation, RemoveReactionMutationVariables>;
export const FollowThreadFromPaneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FollowThreadFromPane"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"FollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<FollowThreadFromPaneMutation, FollowThreadFromPaneMutationVariables>;
export const UnfollowThreadFromPaneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnfollowThreadFromPane"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnfollowThreadInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unfollowThread"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<UnfollowThreadFromPaneMutation, UnfollowThreadFromPaneMutationVariables>;
export const MarkThreadAsOpenedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkThreadAsOpened"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MarkThreadAsOpenedInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markThreadAsOpened"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previousOpenedAt"}}]}}]}}]} as unknown as DocumentNode<MarkThreadAsOpenedMutation, MarkThreadAsOpenedMutationVariables>;
export const ResolveMessageLinkDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ResolveMessageLink"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomEventByEventId"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}},{"kind":"Argument","name":{"kind":"Name","value":"roomId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"roomId"}}},{"kind":"Argument","name":{"kind":"Name","value":"eventId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"eventId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inThread"}}]}}]}}]}}]}}]} as unknown as DocumentNode<ResolveMessageLinkQuery, ResolveMessageLinkQueryVariables>;
export const GetSpaceNotificationPreferencesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetSpaceNotificationPreferences"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rooms"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"viewerNotificationPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]}}]}}]} as unknown as DocumentNode<GetSpaceNotificationPreferencesQuery, GetSpaceNotificationPreferencesQueryVariables>;
export const SetSpaceNotificationLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetSpaceNotificationLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetSpaceNotificationLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setSpaceNotificationLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]} as unknown as DocumentNode<SetSpaceNotificationLevelMutation, SetSpaceNotificationLevelMutationVariables>;
export const SetRoomNotificationLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetRoomNotificationLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetRoomNotificationLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRoomNotificationLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"level"}},{"kind":"Field","name":{"kind":"Name","value":"effectiveLevel"}}]}}]}}]} as unknown as DocumentNode<SetRoomNotificationLevelMutation, SetRoomNotificationLevelMutationVariables>;
export const SpaceMembersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceMembers"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"20"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"spaceRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}]},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]}}]} as unknown as DocumentNode<SpaceMembersQuery, SpaceMembersQueryVariables>;
export const SpaceMemberDetailsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceMemberDetails"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}]}]}},{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"availablePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"member"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"instanceRoles"}},{"kind":"Field","name":{"kind":"Name","value":"spaceRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}]}]}}]}}]}}]} as unknown as DocumentNode<SpaceMemberDetailsQuery, SpaceMemberDetailsQueryVariables>;
export const RevokeSpaceRoleFromMemberDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RevokeSpaceRoleFromMember"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RevokeSpaceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revokeSpaceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RevokeSpaceRoleFromMemberMutation, RevokeSpaceRoleFromMemberMutationVariables>;
export const AssignSpaceRoleToMemberDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AssignSpaceRoleToMember"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AssignSpaceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assignSpaceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AssignSpaceRoleToMemberMutation, AssignSpaceRoleToMemberMutationVariables>;
export const SpaceRolesGateDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRolesGate"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRolesGateQuery, SpaceRolesGateQueryVariables>;
export const SpaceRoleDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRoleDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"role"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}}]}},{"kind":"Field","name":{"kind":"Name","value":"roleUsers"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roleName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanAssignRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRoleDetailQuery, SpaceRoleDetailQueryVariables>;
export const UpdateSpaceRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateSpaceRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateSpaceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateSpaceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateSpaceRoleMutation, UpdateSpaceRoleMutationVariables>;
export const DeleteSpaceRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteSpaceRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteSpaceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteSpaceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteSpaceRoleMutation, DeleteSpaceRoleMutationVariables>;
export const SpaceRolesNewCheckDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceRolesNewCheck"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"viewerCanManageRoles"}}]}}]}}]} as unknown as DocumentNode<SpaceRolesNewCheckQuery, SpaceRolesNewCheckQueryVariables>;
export const CreateSpaceRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateSpaceRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateSpaceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createSpaceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateSpaceRoleMutation, CreateSpaceRoleMutationVariables>;
export const AdminRoomLayoutDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminRoomLayout"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"autoJoin"}}]}},{"kind":"Field","name":{"kind":"Name","value":"roomLayout"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"unsectionedRoomIds"}}]}}]}}]}}]} as unknown as DocumentNode<AdminRoomLayoutQuery, AdminRoomLayoutQueryVariables>;
export const UpdateRoomLayoutDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRoomLayout"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomLayoutInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoomLayout"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rooms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"unsectionedRoomIds"}}]}}]}}]} as unknown as DocumentNode<UpdateRoomLayoutMutation, UpdateRoomLayoutMutationVariables>;
export const AdminUpdateRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<AdminUpdateRoomMutation, AdminUpdateRoomMutationVariables>;
export const ArchiveRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ArchiveRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ArchiveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"archiveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}}]}}]} as unknown as DocumentNode<ArchiveRoomMutation, ArchiveRoomMutationVariables>;
export const UnarchiveRoomDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UnarchiveRoom"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UnarchiveRoomInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"unarchiveRoom"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}}]}}]}}]} as unknown as DocumentNode<UnarchiveRoomMutation, UnarchiveRoomMutationVariables>;
export const SetRoomAutoJoinDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetRoomAutoJoin"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetRoomAutoJoinInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRoomAutoJoin"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"autoJoin"}}]}}]}}]} as unknown as DocumentNode<SetRoomAutoJoinMutation, SetRoomAutoJoinMutationVariables>;
export const MyFollowedThreadsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyFollowedThreads"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myFollowedThreads"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"spaceId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"room"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"threadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"rootMessage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RoomEventView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"3"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasUnread"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"UserAvatarUser"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"User"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"96"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"96"}}]},{"kind":"Field","name":{"kind":"Name","value":"presenceStatus"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MessageAttachmentView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Attachment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"contentType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"960"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"800"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"videoProcessing"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"thumbnailUrl"}},{"kind":"Field","name":{"kind":"Name","value":"variants"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quality"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}},{"kind":"Field","name":{"kind":"Name","value":"size"}}]}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"LinkPreviewView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LinkPreview"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"imageUrl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"width"},"value":{"kind":"IntValue","value":"600"}},{"kind":"Argument","name":{"kind":"Name","value":"height"},"value":{"kind":"IntValue","value":"314"}},{"kind":"Argument","name":{"kind":"Name","value":"fit"},"value":{"kind":"EnumValue","value":"CONTAIN"}}]},{"kind":"Field","name":{"kind":"Name","value":"siteName"}},{"kind":"Field","name":{"kind":"Name","value":"embedType"}},{"kind":"Field","name":{"kind":"Name","value":"embedId"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RoomEventView"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"actor"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessagePostedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"attachments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MessageAttachmentView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"linkPreview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"LinkPreviewView"}}]}},{"kind":"Field","name":{"kind":"Name","value":"reactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emoji"}},{"kind":"Field","name":{"kind":"Name","value":"count"}},{"kind":"Field","name":{"kind":"Name","value":"hasReacted"}},{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"inReplyTo"}},{"kind":"Field","name":{"kind":"Name","value":"inThread"}},{"kind":"Field","name":{"kind":"Name","value":"echoOfEventId"}},{"kind":"Field","name":{"kind":"Name","value":"echoFromThreadRootEventId"}},{"kind":"Field","name":{"kind":"Name","value":"replyCount"}},{"kind":"Field","name":{"kind":"Name","value":"lastReplyAt"}},{"kind":"Field","name":{"kind":"Name","value":"threadParticipants"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"5"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"UserAvatarUser"}}]}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsFollowingThread"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MessageDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserJoinedRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserLeftRoomEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUpdatedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomArchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RoomUnarchivedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionAddedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReactionRemovedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}},{"kind":"Field","name":{"kind":"Name","value":"emoji"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PresenceChangedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UserTypingEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","alias":{"kind":"Name","value":"typingThreadRootEventId"},"name":{"kind":"Name","value":"threadRootEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"VideoProcessingCompletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}},{"kind":"Field","name":{"kind":"Name","value":"attachmentId"}},{"kind":"Field","name":{"kind":"Name","value":"messageEventId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SpaceMemberDeletedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"userId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantJoinedEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CallParticipantLeftEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaceId"}},{"kind":"Field","name":{"kind":"Name","value":"roomId"}}]}}]}}]}}]} as unknown as DocumentNode<MyFollowedThreadsQuery, MyFollowedThreadsQueryVariables>;
export const AdminDashboardUsersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminDashboardUsers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<AdminDashboardUsersQuery, AdminDashboardUsersQueryVariables>;
export const AdminDashboardSpacesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminDashboardSpaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<AdminDashboardSpacesQuery, AdminDashboardSpacesQueryVariables>;
export const AdminRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"role"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}}]}},{"kind":"Field","name":{"kind":"Name","value":"instanceRoleUsers"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"roleName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}}]}}]}}]}}]} as unknown as DocumentNode<AdminRoleQuery, AdminRoleQueryVariables>;
export const UpdateRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<UpdateRoleMutation, UpdateRoleMutationVariables>;
export const DeleteRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteRoleMutation, DeleteRoleMutationVariables>;
export const CreateRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<CreateRoleMutation, CreateRoleMutationVariables>;
export const AdminInstanceConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminInstanceConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"instanceConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"isConfigured"}},{"kind":"Field","name":{"kind":"Name","value":"instanceName"}},{"kind":"Field","name":{"kind":"Name","value":"ogTitle"}},{"kind":"Field","name":{"kind":"Name","value":"ogDescription"}},{"kind":"Field","name":{"kind":"Name","value":"ogImageUrl"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"blockedUsernames"}}]}}]}}]}}]} as unknown as DocumentNode<AdminInstanceConfigQuery, AdminInstanceConfigQueryVariables>;
export const UpdateInstanceConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateInstanceConfig"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateInstanceConfigInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateInstanceConfig"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"isConfigured"}},{"kind":"Field","name":{"kind":"Name","value":"instanceName"}},{"kind":"Field","name":{"kind":"Name","value":"ogTitle"}},{"kind":"Field","name":{"kind":"Name","value":"ogDescription"}},{"kind":"Field","name":{"kind":"Name","value":"motd"}},{"kind":"Field","name":{"kind":"Name","value":"welcomeMessage"}},{"kind":"Field","name":{"kind":"Name","value":"blockedUsernames"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateInstanceConfigMutation, UpdateInstanceConfigMutationVariables>;
export const ResetInstanceConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ResetInstanceConfig"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"resetInstanceConfig"}}]}}]}}]} as unknown as DocumentNode<ResetInstanceConfigMutation, ResetInstanceConfigMutationVariables>;
export const UploadInstanceOgImageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadInstanceOGImage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadInstanceOGImageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadInstanceOGImage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ogImageUrl"}}]}}]}}]}}]} as unknown as DocumentNode<UploadInstanceOgImageMutation, UploadInstanceOgImageMutationVariables>;
export const DeleteInstanceOgImageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteInstanceOGImage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteInstanceOGImage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ogImageUrl"}}]}}]}}]}}]} as unknown as DocumentNode<DeleteInstanceOgImageMutation, DeleteInstanceOgImageMutationVariables>;
export const AdminSpacesListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSpacesList"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"spaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}},{"kind":"Field","name":{"kind":"Name","value":"roomCount"}},{"kind":"Field","name":{"kind":"Name","value":"assetCount"}}]}}]}}]} as unknown as DocumentNode<AdminSpacesListQuery, AdminSpacesListQueryVariables>;
export const AdminSystemInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSystemInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"systemInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"connected"}},{"kind":"Field","name":{"kind":"Name","value":"serverID"}},{"kind":"Field","name":{"kind":"Name","value":"serverName"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"maxPayload"}},{"kind":"Field","name":{"kind":"Name","value":"rtt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"account"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"memory"}},{"kind":"Field","name":{"kind":"Name","value":"memoryUsed"}},{"kind":"Field","name":{"kind":"Name","value":"storage"}},{"kind":"Field","name":{"kind":"Name","value":"storageUsed"}},{"kind":"Field","name":{"kind":"Name","value":"streams"}},{"kind":"Field","name":{"kind":"Name","value":"streamsUsed"}},{"kind":"Field","name":{"kind":"Name","value":"consumers"}},{"kind":"Field","name":{"kind":"Name","value":"consumersUsed"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AdminSystemInfoQuery, AdminSystemInfoQueryVariables>;
export const AdminUsersListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminUsersList"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"users"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"hasVerifiedEmail"}},{"kind":"Field","name":{"kind":"Name","value":"verifiedEmails"}}]}}]}}]} as unknown as DocumentNode<AdminUsersListQuery, AdminUsersListQueryVariables>;
export const AdminUserDetailsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminUserDetails"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"instanceRoles"}}]}},{"kind":"Field","name":{"kind":"Name","value":"user"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}},{"kind":"Field","name":{"kind":"Name","value":"verifiedEmails"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}},{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isSystem"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"permissions"}},{"kind":"Field","name":{"kind":"Name","value":"permissionDenials"}}]}},{"kind":"Field","name":{"kind":"Name","value":"instancePermissions"}},{"kind":"Field","name":{"kind":"Name","value":"userInstanceRoles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}]},{"kind":"Field","name":{"kind":"Name","value":"userRoleBasedPermissions"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}]},{"kind":"Field","name":{"kind":"Name","value":"userRoleBasedDenials"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}]}]}}]}}]} as unknown as DocumentNode<AdminUserDetailsQuery, AdminUserDetailsQueryVariables>;
export const AssignInstanceRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AssignInstanceRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AssignInstanceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assignInstanceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<AssignInstanceRoleMutation, AssignInstanceRoleMutationVariables>;
export const RevokeInstanceRoleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RevokeInstanceRole"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RevokeInstanceRoleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revokeInstanceRole"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RevokeInstanceRoleMutation, RevokeInstanceRoleMutationVariables>;
export const AdminUpdateUserDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminUpdateUser"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AdminUpdateUserInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateUser"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"login"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}}]}}]}}]} as unknown as DocumentNode<AdminUpdateUserMutation, AdminUpdateUserMutationVariables>;
export const AdminClearUsernameCooldownDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminClearUsernameCooldown"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"userId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearUsernameCooldown"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"userId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"userId"}}}]}]}}]}}]} as unknown as DocumentNode<AdminClearUsernameCooldownMutation, AdminClearUsernameCooldownMutationVariables>;
export const GetMyLastLoginChangeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMyLastLoginChange"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"lastLoginChange"}}]}}]}}]} as unknown as DocumentNode<GetMyLastLoginChangeQuery, GetMyLastLoginChangeQueryVariables>;
export const UploadMyAvatarDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadMyAvatar"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadMyAvatarInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadMyAvatar"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}}]}}]} as unknown as DocumentNode<UploadMyAvatarMutation, UploadMyAvatarMutationVariables>;
export const DeleteMyAvatarDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMyAvatar"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMyAvatar"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"avatarUrl"}}]}}]}}]} as unknown as DocumentNode<DeleteMyAvatarMutation, DeleteMyAvatarMutationVariables>;
export const UpdateMyProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMyProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMyProfileInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMyProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"login"}}]}}]}}]} as unknown as DocumentNode<UpdateMyProfileMutation, UpdateMyProfileMutationVariables>;
export const AccountPermissionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AccountPermissions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"viewerCanDeleteAccount"}}]}}]}}]} as unknown as DocumentNode<AccountPermissionsQuery, AccountPermissionsQueryVariables>;
export const RequestAccountDeletionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RequestAccountDeletion"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestAccountDeletion"}}]}}]} as unknown as DocumentNode<RequestAccountDeletionMutation, RequestAccountDeletionMutationVariables>;
export const DeleteMyAccountDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMyAccount"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeleteMyAccountInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMyAccount"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<DeleteMyAccountMutation, DeleteMyAccountMutationVariables>;
export const UpdateMySettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMySettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateUserSettingsInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMySettings"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}}]}}]}}]} as unknown as DocumentNode<UpdateMySettingsMutation, UpdateMySettingsMutationVariables>;
export const CreateSpacePageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateSpacePage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateSpaceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createSpace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}}]} as unknown as DocumentNode<CreateSpacePageMutation, CreateSpacePageMutationVariables>;
export const SpaceJoinPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SpaceJoinPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"space"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"spaceId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}},{"kind":"Field","name":{"kind":"Name","value":"viewerIsMember"}}]}},{"kind":"Field","name":{"kind":"Name","value":"me"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<SpaceJoinPageQuery, SpaceJoinPageQueryVariables>;
export const JoinSpaceFromInviteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"JoinSpaceFromInvite"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JoinSpaceInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"joinSpace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<JoinSpaceFromInviteMutation, JoinSpaceFromInviteMutationVariables>;
export const LoginPageInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoginPageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"instance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabledAuthProviders"}},{"kind":"Field","name":{"kind":"Name","value":"directRegistrationEnabled"}}]}}]}}]} as unknown as DocumentNode<LoginPageInfoQuery, LoginPageInfoQueryVariables>;