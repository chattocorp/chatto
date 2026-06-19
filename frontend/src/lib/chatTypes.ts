import { TimeFormat } from '$lib/preferences/timeFormat';

export { TimeFormat };

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

export enum VideoProcessingStatus {
  Completed = 'COMPLETED',
  Failed = 'FAILED',
  Pending = 'PENDING',
  Processing = 'PROCESSING'
}

export type UserAvatarUserFragment = {
  __typename?: 'User';
  id: string;
  login: string;
  displayName: string;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
};

export type AssetUrlView = {
  __typename?: 'AssetURL';
  url: string;
  expiresAt: string;
};

export type VideoVariantViewFragment = {
  __typename?: 'VideoVariant';
  quality: string;
  width: number;
  height: number;
  size: number;
  assetUrl: AssetUrlView;
};

export type VideoProcessingViewFragment = {
  __typename?: 'VideoProcessing';
  status: VideoProcessingStatus;
  durationMs?: number | string | bigint | null;
  width?: number | null;
  height?: number | null;
  sourceAvailable: boolean;
  reasonCode?: string | null;
  thumbnailAssetUrl?: AssetUrlView | null;
  variants: VideoVariantViewFragment[];
};

export type MessageAttachmentViewFragment = {
  __typename?: 'Attachment';
  id: string;
  filename: string;
  contentType: string;
  width: number;
  height: number;
  assetUrl: AssetUrlView;
  thumbnailAssetUrl?: AssetUrlView | null;
  videoProcessing?: VideoProcessingViewFragment | null;
};

export type LinkPreviewViewFragment = {
  __typename?: 'LinkPreview';
  url: string;
  title?: string | null;
  description?: string | null;
  imageUrl?: string | null;
  siteName?: string | null;
  embedType?: string | null;
  embedId?: string | null;
};

export type ReactionSummaryView = {
  __typename?: 'ReactionSummary';
  emoji: string;
  count: number;
  hasReacted: boolean;
  users: Array<{ __typename?: 'User'; id: string; displayName: string }>;
};

export type RoomEventPayload =
  | { __typename: 'AssetDeletedEvent'; assetId: string; deletedRoomId?: string | null }
  | {
      __typename: 'AssetProcessingFailedEvent';
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | {
      __typename: 'AssetProcessingStartedEvent';
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | {
      __typename: 'AssetProcessingSucceededEvent';
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | { __typename: 'CallEndedEvent'; roomId: string; callId: string }
  | { __typename: 'CallParticipantJoinedEvent'; roomId: string; callId: string }
  | { __typename: 'CallParticipantLeftEvent'; roomId: string; callId: string }
  | { __typename: 'CallStartedEvent'; roomId: string; callId: string }
  | { __typename: 'HeartbeatEvent'; alive?: boolean }
  | { __typename: 'MentionNotificationEvent' }
  | { __typename: 'MentionStatusClearedEvent' }
  | {
      __typename: 'MessageEditedEvent';
      roomId: string;
      messageEventId: string;
      body?: string | null;
      updatedAt?: string | null;
      attachments: MessageAttachmentViewFragment[];
      linkPreview?: LinkPreviewViewFragment | null;
    }
  | {
      __typename: 'MessagePostedEvent';
      roomId: string;
      body?: string | null;
      updatedAt?: string | null;
      inReplyTo?: string | null;
      threadRootEventId?: string | null;
      echoOfEventId?: string | null;
      echoFromThreadRootEventId?: string | null;
      channelEchoEventId?: string | null;
      replyCount: number;
      lastReplyAt?: string | null;
      viewerIsFollowingThread?: boolean | null;
      threadReplies?: {
        events: readonly RoomEventViewFragment[];
        startCursor?: string | null;
        endCursor?: string | null;
        hasOlder: boolean;
        hasNewer: boolean;
      };
      attachments: MessageAttachmentViewFragment[];
      linkPreview?: LinkPreviewViewFragment | null;
      reactions: ReactionSummaryView[];
      threadParticipants: UserAvatarUserFragment[];
    }
  | {
      __typename: 'MessageRetractedEvent';
      roomId: string;
      messageEventId: string;
      retractedReason?: string | null;
    }
  | { __typename: 'NewDirectMessageNotificationEvent' }
  | { __typename: 'NotificationCreatedEvent' }
  | { __typename: 'NotificationDismissedEvent' }
  | { __typename: 'NotificationLevelChangedEvent' }
  | { __typename: 'PresenceChangedEvent'; status: PresenceStatus }
  | { __typename: 'ReactionAddedEvent'; roomId: string; messageEventId: string; emoji: string }
  | { __typename: 'ReactionRemovedEvent'; roomId: string; messageEventId: string; emoji: string }
  | { __typename: 'RoomArchivedEvent'; roomId: string }
  | { __typename: 'RoomCreatedEvent'; roomId?: string | null }
  | { __typename: 'RoomDeletedEvent'; roomId: string }
  | { __typename: 'RoomGroupsUpdatedEvent'; changed?: boolean }
  | { __typename: 'RoomMarkedAsReadEvent'; roomId?: string | null }
  | { __typename: 'RoomMemberBannedEvent'; roomId?: string | null }
  | { __typename: 'RoomMemberUnbannedEvent'; roomId?: string | null }
  | { __typename: 'RoomUnarchivedEvent'; roomId: string }
  | { __typename: 'RoomUpdatedEvent'; roomId: string }
  | { __typename: 'ServerMemberDeletedEvent'; userId: string }
  | { __typename: 'ServerUpdatedEvent' }
  | { __typename: 'ServerUserPreferencesUpdatedEvent' }
  | { __typename: 'SessionTerminatedEvent' }
  | { __typename: 'ThreadCreatedEvent'; roomId?: string | null }
  | {
      __typename: 'ThreadFollowChangedEvent';
      tfcRoomId?: string | null;
      tfcThreadRootEventId?: string | null;
      isFollowing?: boolean;
    }
  | { __typename: 'UserCreatedEvent' }
  | { __typename: 'UserDeletedEvent' }
  | { __typename: 'UserJoinedRoomEvent'; roomId: string }
  | { __typename: 'UserLeftRoomEvent'; roomId: string }
  | { __typename: 'UserProfileUpdatedEvent' }
  | { __typename: 'UserTypingEvent'; roomId: string; typingThreadRootEventId?: string | null };

export type RoomEventViewFragment = {
  __typename?: 'Event';
  id: string;
  createdAt: string;
  actorId?: string | null;
  actor?: UserAvatarUserFragment | null;
  event: RoomEventPayload;
};
