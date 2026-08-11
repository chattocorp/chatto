import type { LinkPreviewView } from './linkPreviews';
import type { MessageAttachmentView } from './messageAttachments';
import type { ReactionSummaryView } from './reactions';
import type { UserAvatarUserView } from './users';

/**
 * Renderable durable events returned by the room and thread timeline APIs.
 * These names intentionally match the generated RoomTimelineEvent oneof cases.
 */
export const TimelineEventKind = {
  MessagePosted: 'messagePosted',
  RoomArchived: 'roomArchived',
  RoomCreated: 'roomCreated',
  RoomDeleted: 'roomDeleted',
  RoomUnarchived: 'roomUnarchived',
  RoomUpdated: 'roomUpdated',
  UserJoinedRoom: 'userJoinedRoom',
  UserLeftRoom: 'userLeftRoom'
} as const;

export type TimelineEventKind = (typeof TimelineEventKind)[keyof typeof TimelineEventKind];

export type MessagePostedPayload = {
  kind: typeof TimelineEventKind.MessagePosted;
  roomId: string;
  body: string | null;
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
  pinned?: boolean;
  threadExists?: boolean;
  replyCount: number;
  lastReplyAt?: string | null;
  threadParticipantCount?: number;
  threadParticipants: UserAvatarUserView[];
  viewerIsFollowingThread?: boolean | null;
};

export type TimelineEventPayload =
  | MessagePostedPayload
  | { kind: typeof TimelineEventKind.RoomArchived; roomId: string }
  | { kind: typeof TimelineEventKind.RoomCreated; roomId: string }
  | { kind: typeof TimelineEventKind.RoomDeleted; roomId: string }
  | { kind: typeof TimelineEventKind.RoomUnarchived; roomId: string }
  | { kind: typeof TimelineEventKind.RoomUpdated; roomId: string }
  | { kind: typeof TimelineEventKind.UserJoinedRoom; roomId: string }
  | { kind: typeof TimelineEventKind.UserLeftRoom; roomId: string };

export type TimelineEventView = {
  id: string;
  createdAt: string;
  actorId?: string | null;
  actor?: UserAvatarUserView | null;
  event: TimelineEventPayload;
};

export function timelineEventKind(
  event: TimelineEventPayload | object | null | undefined
): TimelineEventKind | null {
  if (!event) return null;
  const kind = (event as { kind?: unknown }).kind;
  return typeof kind === 'string' && Object.values(TimelineEventKind).includes(kind as TimelineEventKind)
    ? (kind as TimelineEventKind)
    : null;
}

export function isMessagePostedEvent(
  event: TimelineEventPayload | object | null | undefined
): event is MessagePostedPayload {
  return timelineEventKind(event) === TimelineEventKind.MessagePosted;
}
