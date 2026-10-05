import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { MessageAttachmentView } from '@chatto/client/timeline/messageAttachments';
import { TimelineEventKind, type TimelineEventView } from '@chatto/client/timeline/timelineEvents';

export type MessageOverrides = Partial<{
  id: string;
  actorId: string;
  body: string;
  attachments: MessageAttachmentView[];
  threadRootEventId: string | null;
  echoOfEventId: string | null;
  echoFromThreadRootEventId: string | null;
  channelEchoEventId: string | null;
  threadExists: boolean;
  replyCount: number;
}>;

/** A posted message with one 👍 reaction by the viewer, for message row specs. */
export function messageEvent(overrides: MessageOverrides = {}): TimelineEventView {
  const actorId = overrides.actorId ?? 'viewer';
  return {
    id: overrides.id ?? 'regular-message',
    actorId,
    actor: {
      id: actorId,
      login: actorId,
      displayName: actorId,
      deleted: false,
      avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE
    },
    createdAt: new Date().toISOString(),
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room-1',
      body: overrides.body ?? 'Hello from this message',
      attachments: overrides.attachments ?? [],
      linkPreview: null,
      reactions: [
        {
          emoji: 'thumbsup',
          count: 1,
          hasReacted: true,
          users: [{ id: 'viewer', displayName: 'viewer' }]
        }
      ],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: overrides.threadRootEventId ?? null,
      echoOfEventId: overrides.echoOfEventId ?? null,
      echoFromThreadRootEventId: overrides.echoFromThreadRootEventId ?? null,
      channelEchoEventId: overrides.channelEchoEventId ?? null,
      replyCount: overrides.replyCount ?? 0,
      lastReplyAt: null,
      threadParticipants: [],
      threadExists: overrides.threadExists ?? false,
      viewerIsFollowingThread: false
    }
  } as TimelineEventView;
}
