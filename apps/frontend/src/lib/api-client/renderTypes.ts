/**
 * Compatibility timeline DTOs used by the Svelte chat surface while the
 * remaining event model is moved to protobuf-native names.
 *
 * This file is hand-owned. Do not regenerate it from the retired legacy schema.
 */
import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { LinkPreviewView } from '$lib/render/linkPreviews';
import type { MessageAttachmentView } from '$lib/render/messageAttachments';
import type { ReactionSummaryView } from '$lib/render/reactions';
import type { CustomUserStatusView, UserAvatarUserView } from '$lib/render/users';

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
