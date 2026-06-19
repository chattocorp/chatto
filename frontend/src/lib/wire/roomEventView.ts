import {
  type AssetUrl,
  type AttachmentView,
  type LinkPreviewView,
  type RoomEventsPage,
  type RoomEventView as WireRoomEventView,
  type VideoProcessingView,
  VideoProcessingStatus as WireVideoProcessingStatus,
  type VideoVariantView
} from '$lib/pb/chatto/api/v1/chat_pb';
import type { User } from '$lib/pb/chatto/core/v1/models_pb';
import {
  PresenceStatus,
  type RoomEventViewFragment,
  VideoProcessingStatus
} from '$lib/chatTypes';
import type { Timestamp } from '@bufbuild/protobuf';

export type WireRoomEventConnectionPage = {
  events: RoomEventViewFragment[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export function wireRoomEventsPageToConnection(
  page: RoomEventsPage | undefined
): WireRoomEventConnectionPage | null {
  if (!page) return null;
  return {
    events: page.events.map(wireRoomEventViewToFragment).filter(isRoomEventView),
    startCursor: sequenceToCursor(page.startSequence),
    endCursor: sequenceToCursor(page.endSequence),
    hasOlder: page.hasOlder,
    hasNewer: page.hasNewer
  };
}

export function wireRoomEventViewToFragment(
  view: WireRoomEventView | undefined
): RoomEventViewFragment | null {
  if (!view) return null;
  const event = wirePayloadToFragment(view);
  if (!event) return null;

  return {
    __typename: 'Event',
    id: view.id,
    createdAt: timestampToISO(view.createdAt),
    actorId: view.actorId || null,
    actor: userToFragment(view.actor),
    event
  } as RoomEventViewFragment;
}

export function cursorToSequence(cursor: string | null | undefined): bigint {
  if (!cursor) return 0n;
  const trimmed = cursor.trim();
  const value = trimmed.startsWith('seq:') ? trimmed.slice(4) : trimmed;
  if (!/^\d+$/.test(value)) return 0n;
  return BigInt(value);
}

export function sequenceToCursor(sequence: bigint | number | string): string | null {
  const value = BigInt(sequence);
  return value > 0n ? `seq:${value}` : null;
}

function wirePayloadToFragment(view: WireRoomEventView): RoomEventViewFragment['event'] | null {
  const payload = view.event?.payload;
  if (!payload || !payload.case) return null;

  switch (payload.case) {
    case 'messagePosted': {
      const value = payload.value;
      return {
        __typename: 'MessagePostedEvent',
        roomId: value.roomId,
        body: value.body ?? null,
        attachments: value.attachments.map(attachmentToFragment),
        linkPreview: linkPreviewToFragment(value.linkPreview),
        reactions: value.reactions.map((reaction) => ({
          __typename: 'ReactionSummary',
          emoji: reaction.emoji,
          count: reaction.count,
          hasReacted: reaction.hasReacted,
          users: reaction.users.map(userToReactionUser).filter(isReactionUser)
        })),
        updatedAt: timestampToNullableISO(value.updatedAt),
        inReplyTo: value.inReplyTo ?? null,
        threadRootEventId: value.threadRootEventId ?? null,
        echoOfEventId: value.echoOfEventId ?? null,
        echoFromThreadRootEventId: value.echoFromThreadRootEventId ?? null,
        channelEchoEventId: value.channelEchoEventId ?? null,
        replyCount: value.replyCount,
        lastReplyAt: timestampToNullableISO(value.lastReplyAt),
        threadParticipants: value.threadParticipants.map(userToFragment).filter(isUserFragment),
        viewerIsFollowingThread: value.viewerIsFollowingThread ?? null
      } as RoomEventViewFragment['event'];
    }
    case 'messageEdited': {
      const value = payload.value;
      return {
        __typename: 'MessageEditedEvent',
        roomId: value.roomId,
        messageEventId: value.messageEventId,
        body: value.body ?? null,
        attachments: value.attachments.map(attachmentToFragment),
        linkPreview: linkPreviewToFragment(value.linkPreview),
        updatedAt: timestampToNullableISO(value.updatedAt)
      } as RoomEventViewFragment['event'];
    }
    case 'messageRetracted': {
      const value = payload.value;
      return {
        __typename: 'MessageRetractedEvent',
        roomId: value.roomId,
        messageEventId: value.messageEventId,
        retractedReason: value.reason ?? null
      } as RoomEventViewFragment['event'];
    }
    case 'roomCreated':
      return { __typename: 'RoomCreatedEvent' };
    case 'roomUpdated': {
      const value = payload.value;
      return { __typename: 'RoomUpdatedEvent', roomId: value.roomId };
    }
    case 'roomDeleted': {
      const value = payload.value;
      return { __typename: 'RoomDeletedEvent', roomId: value.roomId };
    }
    case 'roomArchived': {
      const value = payload.value;
      return { __typename: 'RoomArchivedEvent', roomId: value.roomId };
    }
    case 'roomUnarchived': {
      const value = payload.value;
      return { __typename: 'RoomUnarchivedEvent', roomId: value.roomId };
    }
    case 'userJoinedRoom': {
      const value = payload.value;
      return { __typename: 'UserJoinedRoomEvent', roomId: value.roomId };
    }
    case 'userLeftRoom': {
      const value = payload.value;
      return { __typename: 'UserLeftRoomEvent', roomId: value.roomId };
    }
    case 'reactionAdded': {
      const value = payload.value;
      return {
        __typename: 'ReactionAddedEvent',
        roomId: value.roomId,
        messageEventId: value.messageEventId,
        emoji: value.emoji
      };
    }
    case 'reactionRemoved': {
      const value = payload.value;
      return {
        __typename: 'ReactionRemovedEvent',
        roomId: value.roomId,
        messageEventId: value.messageEventId,
        emoji: value.emoji
      };
    }
    case 'assetProcessingStarted': {
      const value = payload.value;
      return {
        __typename: 'AssetProcessingStartedEvent',
        processingRoomId: value.roomId || null,
        assetId: value.assetId,
        processingMessageEventId: value.messageEventId || null
      };
    }
    case 'assetProcessingSucceeded': {
      const value = payload.value;
      return {
        __typename: 'AssetProcessingSucceededEvent',
        processingRoomId: value.roomId || null,
        assetId: value.assetId,
        processingMessageEventId: value.messageEventId || null
      };
    }
    case 'assetProcessingFailed': {
      const value = payload.value;
      return {
        __typename: 'AssetProcessingFailedEvent',
        processingRoomId: value.roomId || null,
        assetId: value.assetId,
        processingMessageEventId: value.messageEventId || null
      };
    }
    case 'assetDeleted': {
      const value = payload.value;
      return {
        __typename: 'AssetDeletedEvent',
        deletedRoomId: value.roomId || null,
        assetId: value.assetId
      };
    }
    case 'serverMemberDeleted': {
      const value = payload.value;
      return { __typename: 'ServerMemberDeletedEvent', userId: value.userId };
    }
    case 'callStarted': {
      const value = payload.value;
      return { __typename: 'CallStartedEvent', roomId: value.roomId, callId: value.callId };
    }
    case 'callParticipantJoined': {
      const value = payload.value;
      return {
        __typename: 'CallParticipantJoinedEvent',
        roomId: value.roomId,
        callId: value.callId
      };
    }
    case 'callParticipantLeft': {
      const value = payload.value;
      return {
        __typename: 'CallParticipantLeftEvent',
        roomId: value.roomId,
        callId: value.callId
      };
    }
    case 'callEnded': {
      const value = payload.value;
      return { __typename: 'CallEndedEvent', roomId: value.roomId, callId: value.callId };
    }
    case 'threadCreated':
      return { __typename: 'ThreadCreatedEvent' };
    case 'roomMemberBanned':
      return { __typename: 'RoomMemberBannedEvent' };
    case 'roomMemberUnbanned':
      return { __typename: 'RoomMemberUnbannedEvent' };
  }
}

function attachmentToFragment(attachment: AttachmentView) {
  return {
    __typename: 'Attachment',
    id: attachment.id,
    filename: attachment.filename,
    contentType: attachment.contentType,
    width: attachment.width,
    height: attachment.height,
    assetUrl: assetUrlToFragment(attachment.assetUrl) ?? emptyAssetUrl(),
    thumbnailAssetUrl: assetUrlToFragment(attachment.thumbnailAssetUrl),
    videoProcessing: videoProcessingToFragment(attachment.videoProcessing)
  };
}

function videoProcessingToFragment(video: VideoProcessingView | undefined) {
  if (!video) return null;
  return {
    __typename: 'VideoProcessing',
    status: videoStatusToGraphQL(video.status),
    durationMs: bigintToNullableNumber(video.durationMs),
    width: video.width ?? null,
    height: video.height ?? null,
    thumbnailAssetUrl: assetUrlToFragment(video.thumbnailAssetUrl),
    sourceAvailable: video.sourceAvailable,
    variants: video.variants.map(videoVariantToFragment),
    reasonCode: video.reasonCode ?? null
  };
}

function videoVariantToFragment(variant: VideoVariantView) {
  return {
    __typename: 'VideoVariant',
    quality: variant.quality,
    width: variant.width,
    height: variant.height,
    size: bigintToNumber(variant.size),
    assetUrl: assetUrlToFragment(variant.assetUrl) ?? emptyAssetUrl()
  };
}

function videoStatusToGraphQL(status: WireVideoProcessingStatus): VideoProcessingStatus {
  switch (status) {
    case WireVideoProcessingStatus.PROCESSING:
      return VideoProcessingStatus.Processing;
    case WireVideoProcessingStatus.COMPLETED:
      return VideoProcessingStatus.Completed;
    case WireVideoProcessingStatus.FAILED:
      return VideoProcessingStatus.Failed;
    default:
      return VideoProcessingStatus.Pending;
  }
}

function linkPreviewToFragment(preview: LinkPreviewView | undefined) {
  if (!preview || !preview.url) return null;
  return {
    __typename: 'LinkPreview',
    url: preview.url,
    title: preview.title,
    description: preview.description,
    imageUrl: preview.imageUrl ?? null,
    siteName: preview.siteName,
    embedType: preview.embedType,
    embedId: preview.embedId ?? null
  };
}

function assetUrlToFragment(value: AssetUrl | undefined) {
  if (!value || !value.url) return null;
  return {
    __typename: 'AssetURL',
    url: value.url,
    expiresAt: timestampToISO(value.expiresAt)
  };
}

function emptyAssetUrl() {
  return {
    __typename: 'AssetURL',
    url: '',
    expiresAt: new Date(0).toISOString()
  };
}

function userToFragment(user: User | undefined) {
  if (!user) return null;
  return {
    __typename: 'User',
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

function userToReactionUser(user: User) {
  if (!user.id) return null;
  return {
    __typename: 'User',
    id: user.id,
    displayName: user.displayName
  };
}

function timestampToISO(value: Timestamp | undefined): string {
  return value ? value.toDate().toISOString() : new Date(0).toISOString();
}

function timestampToNullableISO(value: Timestamp | undefined): string | null {
  return value ? value.toDate().toISOString() : null;
}

function bigintToNumber(value: bigint | number | string): number {
  return Number(value);
}

function bigintToNullableNumber(value: bigint | undefined): number | null {
  return value === undefined ? null : Number(value);
}

function isRoomEventView(value: RoomEventViewFragment | null): value is RoomEventViewFragment {
  return value !== null;
}

function isUserFragment(
  value: ReturnType<typeof userToFragment>
): value is NonNullable<ReturnType<typeof userToFragment>> {
  return value !== null;
}

function isReactionUser(
  value: ReturnType<typeof userToReactionUser>
): value is NonNullable<ReturnType<typeof userToReactionUser>> {
  return value !== null;
}
