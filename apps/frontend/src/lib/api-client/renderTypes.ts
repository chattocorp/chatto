/**
 * Compatibility render DTOs used by the Svelte chat surface while the
 * remaining event and component models are moved to protobuf-native names.
 *
 * This file is hand-owned. Do not regenerate it from the retired legacy schema.
 */

declare const renderType: unique symbol;

export type RenderDocument<T> = {
  readonly [renderType]: (value: T) => T;
};

function renderDocument<T>(): RenderDocument<T> {
  return {} as RenderDocument<T>;
}

export enum FitMode {
  Contain = 'CONTAIN',
  Cover = 'COVER',
  Exact = 'EXACT'
}

export enum NotificationLevel {
  AllMessages = 'ALL_MESSAGES',
  Default = 'DEFAULT',
  Muted = 'MUTED',
  Normal = 'NORMAL'
}

export enum PresenceStatus {
  Away = 'AWAY',
  DoNotDisturb = 'DO_NOT_DISTURB',
  Offline = 'OFFLINE',
  Online = 'ONLINE'
}

export enum RoomType {
  Channel = 'CHANNEL',
  Dm = 'DM'
}

export enum TimeFormat {
  Auto = 'AUTO',
  TwelveHour = 'TWELVE_HOUR',
  TwentyFourHour = 'TWENTY_FOUR_HOUR'
}

export enum VideoProcessingStatus {
  Completed = 'COMPLETED',
  Failed = 'FAILED',
  Pending = 'PENDING',
  Processing = 'PROCESSING'
}

export type AssetURL = {
  url: string;
  expiresAt: string;
};

export type LinkPreviewInput = {
  previewToken: string;
};

export type LinkPreviewView = {
  url: string;
  title?: string | null;
  description?: string | null;
  imageUrl?: string | null;
  siteName?: string | null;
  embedType?: string | null;
  embedId?: string | null;
  socialPost?: SocialPostPreviewView | null;
};

export type SocialPostPreviewView = {
  provider: string;
  url?: string | null;
  author?: {
    displayName: string;
    handle: string;
    avatarUrl?: string | null;
  } | null;
  text: string;
  publishedAt?: string | null;
  externalLink?: {
    url: string;
    title?: string | null;
    description?: string | null;
    imageUrl?: string | null;
  } | null;
  contentWarning?: string | null;
  images: Array<{
    url: string;
    alt?: string | null;
    width?: number | null;
    height?: number | null;
  }>;
  quotedPost?: SocialPostPreviewView | null;
};

export type CustomUserStatusView = {
  emoji: string;
  text: string;
  expiresAt?: string | null;
};

export type UserAvatarUserView = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
  customStatus?: CustomUserStatusView | null;
};

export type VideoVariantView = {
  quality: string;
  width: number;
  height: number;
  size: number;
  assetUrl?: AssetURL | null;
};

export type VideoProcessingView = {
  status: VideoProcessingStatus;
  durationMs?: number | string | null;
  width?: number | null;
  height?: number | null;
  thumbnailAssetUrl?: AssetURL | null;
  sourceAvailable: boolean;
  variants: VideoVariantView[];
  hlsMasterPlaylistUrl?: AssetURL | null;
  reasonCode?: string | null;
};

export type MessageAttachmentView = {
  id: string;
  filename: string;
  contentType: string;
  width: number;
  height: number;
  assetUrl?: AssetURL | null;
  thumbnailAssetUrl?: AssetURL | null;
  videoProcessing?: VideoProcessingView | null;
};

export type ReactionSummaryView = {
  emoji: string;
  count: number;
  hasReacted: boolean;
  users: Array<{ id: string; displayName: string }>;
};

export type RoomEventPayload =
  | {
      kind: 'assetDeleted';
      assetId: string;
      deletedRoomId?: string | null;
    }
  | {
      kind: 'assetProcessingFailed';
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | {
      kind: 'assetProcessingStarted';
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | {
      kind: 'assetProcessingSucceeded';
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | { kind: 'callEnded'; roomId: string; callId: string }
  | {
      kind: 'callParticipantJoined';
      roomId: string;
      callId: string;
    }
  | {
      kind: 'callParticipantLeft';
      roomId: string;
      callId: string;
    }
  | { kind: 'callStarted'; roomId: string; callId: string }
  | {
      kind: 'mentionNotification';
      roomId?: string;
      room?: { name: string };
      actor?: { id: string; displayName: string } | null;
    }
  | {
      kind: 'messageEdited';
      roomId: string;
      messageEventId: string;
      body?: string | null;
      attachments: MessageAttachmentView[];
      linkPreview?: LinkPreviewView | null;
      updatedAt?: string | null;
    }
  | {
      kind: 'messagePosted';
      roomId: string;
      messageEventId?: string;
      body?: string | null;
      attachments: MessageAttachmentView[];
      linkPreview?: LinkPreviewView | null;
      reactions: ReactionSummaryView[];
      updatedAt?: string | null;
      inReplyTo?: string | null;
      threadRootEventId?: string | null;
      echoOfEventId?: string | null;
      echoFromThreadRootEventId?: string | null;
      channelEchoEventId?: string | null;
      deletedAt?: string | null;
      replyCount: number;
      lastReplyAt?: string | null;
      threadParticipantCount?: number;
      threadParticipants: UserAvatarUserView[];
      viewerIsFollowingThread?: boolean | null;
    }
  | {
      kind: 'messageRetracted';
      roomId: string;
      messageEventId: string;
      retractedReason?: string | null;
    }
  | {
      kind: 'newDirectMessageNotification';
      roomId?: string;
      conversationName?: string;
      sender?: {
        id: string;
        displayName: string;
        avatarUrl?: string | null;
      } | null;
    }
  | { kind: 'presenceChanged'; status: PresenceStatus }
  | {
      kind: 'reactionAdded';
      roomId: string;
      messageEventId: string;
      emoji: string;
    }
  | {
      kind: 'reactionRemoved';
      roomId: string;
      messageEventId: string;
      emoji: string;
    }
  | { kind: 'roomArchived'; roomId: string }
  | { kind: 'roomCreated'; roomId?: string }
  | { kind: 'roomDeleted'; roomId: string }
  | { kind: 'roomMemberBanned' }
  | { kind: 'roomMemberUnbanned' }
  | { kind: 'roomUnarchived'; roomId: string }
  | {
      kind: 'roomUniversalChanged';
      roomId?: string;
      universal?: boolean;
    }
  | { kind: 'roomUpdated'; roomId: string }
  | { kind: 'sessionTerminated'; reason?: string }
  | {
      kind: 'threadCreated';
      roomId?: string;
      threadRootEventId?: string;
    }
  | { kind: 'userCreated' }
  | {
      kind: 'userCustomStatusCleared';
      userId?: string;
    }
  | {
      kind: 'userCustomStatusSet';
      userId?: string;
      setCustomStatus?: CustomUserStatusView;
    }
  | { kind: 'userDeleted' }
  | { kind: 'userJoinedRoom'; roomId: string }
  | { kind: 'userLeftRoom'; roomId: string }
  | {
      kind: 'userTyping';
      roomId: string;
      typingThreadRootEventId?: string | null;
    };

export type RoomEventView = {
  id: string;
  createdAt: string;
  actorId?: string | null;
  actor?: UserAvatarUserView | null;
  event: RoomEventPayload | null;
};

export const UserAvatarUserViewDocument = renderDocument<UserAvatarUserView>();
export const MessageAttachmentViewDocument = renderDocument<MessageAttachmentView>();
export const LinkPreviewViewDocument = renderDocument<LinkPreviewView>();
export const RoomEventViewDocument = renderDocument<RoomEventView>();
