import { notifyUserSummaries } from './hooks.js';
import { authHeaders, createChattoClient, handleAuthError } from './connect.js';
import type { RawEvent, EventConnectionPage, UserSummaryForCache } from './events.js';
import { RoomEventKind } from './eventKinds.js';
import {
  PresenceStatus,
  type RoomEventView,
  type SocialPostPreviewView
} from './renderTypes.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { createUserAPI } from './users.js';
import { RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import type { LinkPreview } from '@chatto/api-types/api/v1/link_previews_pb';
import { MessageVideoProcessingStatus } from '@chatto/api-types/api/v1/message_types_pb';
import type {
  Message,
  MessageAssetUrl,
  MessageVideoProcessing
} from '@chatto/api-types/api/v1/message_types_pb';
import type { RoomTimelineEvent } from '@chatto/api-types/api/v1/room_timeline_pb';
import type { User } from '@chatto/api-types/api/v1/users_pb';

export type RoomTimelineAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
  onUserSummaries?: (serverId: string | undefined, users: UserSummaryForCache[]) => void;
};

export type RoomTimelineAPI = {
  getRoomEvents(input: {
    roomId: string;
    limit: number;
    before?: string;
    after?: string;
  }): Promise<EventConnectionPage>;
  getRoomEventsAround(input: {
    roomId: string;
    eventId: string;
    limit: number;
  }): Promise<EventConnectionPage>;
  getMessage(input: { roomId: string; eventId: string }): Promise<RawEvent | null>;
  getThreadEvents(input: {
    roomId: string;
    threadRootEventId: string;
    limit: number;
    before?: string;
    after?: string;
  }): Promise<EventConnectionPage>;
  getThreadEventsAround(input: {
    roomId: string;
    threadRootEventId: string;
    eventId: string;
    limit: number;
  }): Promise<EventConnectionPage>;
};

export function createRoomTimelineAPI(config: RoomTimelineAPIConfig): RoomTimelineAPI {
  const messages = createChattoClient(MessageService, config);
  const rooms = createChattoClient(RoomService, config);
  const threads = createChattoClient(ThreadService, config);
  const headers = () => authHeaders(config);
  return {
    async getRoomEvents({ roomId, limit, before, after }) {
      try {
        const response = await rooms.getRoomEvents(
          {
            roomId,
            limit,
            cursor: before
              ? { case: 'before', value: before }
              : after
                ? { case: 'after', value: after }
                : { case: undefined }
          },
          { headers: headers() }
        );
        primeTimelineUserIncludes(config, response.page?.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(response.page ?? new RoomTimelinePage());
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async getRoomEventsAround({ roomId, eventId, limit }) {
      try {
        const response = await rooms.getRoomEventsAround(
          { roomId, eventId, limit },
          { headers: headers() }
        );
        if (!response.page) return emptyEventConnectionPage();
        primeTimelineUserIncludes(config, response.page.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(response.page);
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async getMessage({ roomId, eventId }) {
      try {
        const response = await messages.getMessage({ roomId, eventId }, { headers: headers() });
        const users = await timelineUsersForMessages(
          config,
          response.message ? [response.message] : []
        );
        return response.message ? messageToRawEvent(response.message, users) : null;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async getThreadEvents({ roomId, threadRootEventId, limit, before, after }) {
      try {
        const response = await threads.getThreadEvents(
          {
            roomId,
            threadRootEventId,
            limit,
            cursor: before
              ? { case: 'before', value: before }
              : after
                ? { case: 'after', value: after }
                : { case: undefined }
          },
          { headers: headers() }
        );
        primeTimelineUserIncludes(config, response.page?.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(response.page ?? new RoomTimelinePage());
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
    async getThreadEventsAround({ roomId, threadRootEventId, eventId, limit }) {
      try {
        const response = await threads.getThreadEventsAround(
          { roomId, threadRootEventId, eventId, limit },
          { headers: headers() }
        );
        if (!response.page) return emptyEventConnectionPage();
        primeTimelineUserIncludes(config, response.page.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(response.page);
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

export async function timelineUsersForEvents(
  config: RoomTimelineAPIConfig,
  events: RoomTimelineEvent[]
): Promise<Record<string, User>> {
  const userIds = messageUserIds(messagesFromTimelineEvents(events));
  return batchTimelineUsers(config, userIds);
}

export async function timelineUsersForMessages(
  config: RoomTimelineAPIConfig,
  messages: Message[]
): Promise<Record<string, User>> {
  const userIds = messageUserIds(messages);
  return batchTimelineUsers(config, userIds);
}

async function batchTimelineUsers(
  config: RoomTimelineAPIConfig,
  userIds: string[]
): Promise<Record<string, User>> {
  if (userIds.length === 0) return {};

  try {
    const summaries = await createUserAPI(config).batchGetUsers(userIds);
    const users: Record<string, User> = {};
    for (const summary of summaries) {
      users[summary.id] = {
        id: summary.id,
        login: summary.login,
        displayName: summary.displayName,
        deleted: summary.deleted,
        avatarUrl: summary.avatarUrl ?? undefined
      } as User;
    }
    primeTimelineUserIncludes(config, users);
    return users;
  } catch {
    return {};
  }
}

function messagesFromTimelineEvents(events: RoomTimelineEvent[]): Message[] {
  const messages: Message[] = [];
  for (const event of events) {
    if (event.event.case !== 'messagePosted') continue;
    if (event.event.value.message) messages.push(event.event.value.message);
  }
  return messages;
}

function messageUserIds(messages: Message[]): string[] {
  const ids = new Set<string>();
  for (const message of messages) {
    if (message.actorId) ids.add(message.actorId);
    for (const userId of message.thread?.participantPreviewUserIds ?? []) {
      if (userId) ids.add(userId);
    }
    for (const reaction of message.reactions) {
      for (const userId of reaction.previewUserIds) {
        if (userId) ids.add(userId);
      }
    }
  }
  return [...ids];
}

function primeTimelineUserIncludes(config: RoomTimelineAPIConfig, users: Record<string, User>) {
  notifyUserSummaries(
    config.serverId,
    Object.values(users).map((user) => ({
      id: user.id,
      login: user.login,
      displayName: user.displayName,
      deleted: user.deleted,
      avatarUrl: user.avatarUrl || null
    })),
    config.onUserSummaries
  );
}

function emptyEventConnectionPage(): EventConnectionPage {
  return {
    events: [],
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false
  };
}

export function roomTimelinePageToEventConnectionPage(page: RoomTimelinePage): EventConnectionPage {
  const users = page.includes?.users ?? {};
  return {
    events: page.events
      .map((event) => roomTimelineEventToRawEvent(event, users))
      .filter((event): event is RawEvent => event !== null),
    startCursor: page.startCursor || null,
    endCursor: page.endCursor || null,
    hasOlder: page.hasOlder,
    hasNewer: page.hasNewer
  };
}

export function roomTimelineEventToRawEvent(
  event: RoomTimelineEvent,
  users: Record<string, User>
): RawEvent | null {
  const payload = timelinePayload(event, users);
  if (!payload) return null;
  return {
    id: event.id,
    createdAt: timestampToISO(event.createdAt),
    actorId: event.actorId,
    actor: userView(event.actorId, users),
    event: payload
  } as unknown as RawEvent;
}

export function messageToRawEvent(message: Message, users: Record<string, User>): RawEvent | null {
  const payload = messagePostedPayload(message, users);
  if (!payload) return null;
  return {
    id: message.id,
    createdAt: timestampToISO(message.createdAt),
    actorId: message.actorId,
    actor: userView(message.actorId, users),
    event: payload
  } as unknown as RawEvent;
}

function timelinePayload(
  event: RoomTimelineEvent,
  users: Record<string, User>
): RoomEventView['event'] | null {
  switch (event.event.case) {
    case 'messagePosted':
      if (!event.event.value.message) return null;
      return messagePostedPayload(event.event.value.message, users) as RoomEventView['event'];
    case 'roomCreated':
      return {
        kind: RoomEventKind.RoomCreated,
        roomId: event.event.value.roomId
      } as never;
    case 'roomUpdated':
      return {
        kind: RoomEventKind.RoomUpdated,
        roomId: event.event.value.roomId
      } as never;
    case 'roomDeleted':
      return {
        kind: RoomEventKind.RoomDeleted,
        roomId: event.event.value.roomId
      } as never;
    case 'roomArchived':
      return {
        kind: RoomEventKind.RoomArchived,
        roomId: event.event.value.roomId
      } as never;
    case 'roomUnarchived':
      return {
        kind: RoomEventKind.RoomUnarchived,
        roomId: event.event.value.roomId
      } as never;
    case 'userJoinedRoom':
      return {
        kind: RoomEventKind.UserJoinedRoom,
        roomId: event.event.value.roomId
      } as never;
    case 'userLeftRoom':
      return {
        kind: RoomEventKind.UserLeftRoom,
        roomId: event.event.value.roomId
      } as never;
    default:
      return null;
  }
}

export function messagePostedPayload(message: Message, users: Record<string, User>) {
  const thread = message.thread;
  return {
    kind: RoomEventKind.MessagePosted,
    roomId: message.roomId,
    body: message.body !== undefined ? message.body : null,
    attachments: message.attachments.map(attachmentView),
    linkPreview: linkPreviewView(message.linkPreview),
    updatedAt: timestampToISOOrNull(message.updatedAt),
    inReplyTo: message.inReplyTo || null,
    threadRootEventId: message.threadRootEventId || null,
    echoOfEventId: message.echoOfEventId || null,
    echoFromThreadRootEventId: message.echoFromThreadRootEventId || null,
    channelEchoEventId: message.channelEchoEventId || null,
    deletedAt: timestampToISOOrNull(message.deletedAt),
    replyCount: thread?.replyCount ?? 0,
    lastReplyAt: timestampToISOOrNull(thread?.lastReplyAt),
    threadParticipantCount: thread?.participantCount ?? 0,
    threadParticipants: (thread?.participantPreviewUserIds ?? [])
      .map((id) => userView(id, users))
      .filter((user): user is NonNullable<ReturnType<typeof userView>> => user !== null),
    viewerIsFollowingThread:
      thread?.viewerState?.isFollowing !== undefined ? thread.viewerState.isFollowing : null,
    reactions: message.reactions.map((reaction) => ({
      emoji: reaction.emoji,
      count: reaction.count,
      hasReacted: reaction.hasReacted,
      users: reaction.previewUserIds
        .map((id) => userView(id, users))
        .filter((user): user is NonNullable<ReturnType<typeof userView>> => user !== null)
    }))
  };
}

function userView(userId: string, users: Record<string, User>) {
  if (!userId) return null;
  const user = users[userId];
  if (!user) {
    return {
      id: userId,
      login: '',
      displayName: 'Deleted User',
      deleted: true,
      avatarUrl: null,
      presenceStatus: PresenceStatus.Offline
    };
  }
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    deleted: user.deleted,
    avatarUrl: user.avatarUrl || null,
    presenceStatus: PresenceStatus.Offline
  };
}

function attachmentView(attachment: {
  id: string;
  filename: string;
  contentType: string;
  width: number;
  height: number;
  assetUrl?: MessageAssetUrl;
  thumbnailAssetUrl?: MessageAssetUrl;
  videoProcessing?: MessageVideoProcessing;
}) {
  return {
    id: attachment.id,
    filename: attachment.filename,
    contentType: attachment.contentType,
    width: attachment.width,
    height: attachment.height,
    assetUrl: assetUrlView(attachment.assetUrl),
    thumbnailAssetUrl: assetUrlView(attachment.thumbnailAssetUrl),
    videoProcessing: videoProcessingView(attachment.videoProcessing)
  };
}

function videoProcessingView(processing?: MessageVideoProcessing) {
  if (!processing) return null;
  const status = videoProcessingStatusView(processing.status);
  if (!status) return null;
  const durationMs = Number(processing.durationMs);
  return {
    status,
    durationMs: durationMs > 0 ? durationMs : null,
    width: processing.width > 0 ? processing.width : null,
    height: processing.height > 0 ? processing.height : null,
    sourceAvailable: processing.sourceAvailable,
    reasonCode: processing.reasonCode || null,
    thumbnailAssetUrl: assetUrlView(processing.thumbnailAssetUrl),
    variants: processing.variants.map((variant) => ({
      quality: variant.quality,
      width: variant.width,
      height: variant.height,
      size: Number(variant.size),
      assetUrl: assetUrlView(variant.assetUrl)
    }))
  };
}

function videoProcessingStatusView(status: MessageVideoProcessingStatus) {
  switch (status) {
    case MessageVideoProcessingStatus.PROCESSING:
      return 'PROCESSING';
    case MessageVideoProcessingStatus.COMPLETED:
      return 'COMPLETED';
    case MessageVideoProcessingStatus.FAILED:
      return 'FAILED';
    default:
      return null;
  }
}

function linkPreviewView(preview?: LinkPreview) {
  if (!preview) return null;
  return {
    url: preview.url,
    title: preview.title || null,
    description: preview.description || null,
    siteName: preview.siteName || null,
    imageUrl: preview.imageUrl || null,
    embedType: preview.embedType || null,
    embedId: preview.embedId || null,
    socialPost: socialPostPreviewView(preview.socialPost)
  };
}

function socialPostPreviewView(
  post?: LinkPreview['socialPost'],
  quoteDepth = 0
): SocialPostPreviewView | null {
  if (!post) return null;
  return {
    provider: post.provider,
    url: post.url || null,
    author: post.author
      ? {
          displayName: post.author.displayName,
          handle: post.author.handle,
          avatarUrl: post.author.avatarUrl || null
        }
      : null,
    text: post.text,
    publishedAt: timestampToISOOrNull(post.publishedAt),
    externalLink: post.externalLink
      ? {
          url: post.externalLink.url,
          title: post.externalLink.title || null,
          description: post.externalLink.description || null,
          imageUrl: post.externalLink.imageUrl || null
        }
      : null,
    contentWarning: post.contentWarning || null,
    images: post.images.map((image) => ({
      url: image.url,
      alt: image.alt || null,
      width: image.width || null,
      height: image.height || null
    })),
    quotedPost:
      quoteDepth === 0 ? socialPostPreviewView(post.quotedPost, quoteDepth + 1) : null
  };
}

function assetUrlView(assetUrl?: MessageAssetUrl) {
  if (!assetUrl) return null;
  return {
    url: assetUrl.url,
    expiresAt: timestampToISOOrNull(assetUrl.expiresAt)
  };
}

function timestampToISO(timestamp: { toDate(): Date } | undefined): string {
  return timestampToISOOrNull(timestamp) ?? new Date(0).toISOString();
}

function timestampToISOOrNull(timestamp: { toDate(): Date } | undefined): string | null {
  return timestamp ? timestamp.toDate().toISOString() : null;
}
