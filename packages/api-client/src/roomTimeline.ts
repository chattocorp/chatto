import { notifyAuthenticationRequired, notifyUserSummaries } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import type {
  RawEvent,
  EventConnectionPage,
  UserSummaryForCache,
} from "./events.js";
import { RoomEventKind } from "./eventKinds.js";
import { PresenceStatus, type RoomEventView } from "./renderTypes.js";
import { RoomTimelineService } from "@chatto/api-types/api/v1/room_timeline_connect";
import {
  RoomTimelinePage,
  RoomTimelineVideoProcessingStatus,
} from "@chatto/api-types/api/v1/room_timeline_pb";
import type { LinkPreview } from "@chatto/api-types/api/v1/link_previews_pb";
import type {
  RoomTimelineAssetUrl,
  RoomTimelineEvent,
  RoomTimelineMessagePosted,
  RoomTimelineVideoProcessing,
} from "@chatto/api-types/api/v1/room_timeline_pb";
import type { User } from "@chatto/api-types/api/v1/users_pb";

export type RoomTimelineAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
  onUserSummaries?: (
    serverId: string | undefined,
    users: UserSummaryForCache[],
  ) => void;
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
  resolveMessageLinkTarget(input: {
    roomId: string;
    eventId: string;
  }): Promise<{ event: RawEvent | null; threadRootEventId: string | null }>;
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

export function createRoomTimelineAPI(
  config: RoomTimelineAPIConfig,
): RoomTimelineAPI {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(RoomTimelineService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      config.serverId
    ) {
      notifyAuthenticationRequired(
        config.serverId,
        config.onAuthenticationRequired,
      );
    }
    throw err;
  }

  return {
    async getRoomEvents({ roomId, limit, before, after }) {
      try {
        const response = await client.getRoomEvents(
          {
            roomId,
            limit,
            cursor: before
              ? { case: "before", value: before }
              : after
                ? { case: "after", value: after }
                : { case: undefined },
          },
          { headers: headers() },
        );
        primeTimelineUserIncludes(config, response.page?.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(
          response.page ?? new RoomTimelinePage(),
        );
      } catch (err) {
        return handleAuthError(err);
      }
    },
    async getRoomEventsAround({ roomId, eventId, limit }) {
      try {
        const response = await client.getRoomEventsAround(
          { roomId, eventId, limit },
          { headers: headers() },
        );
        if (!response.page) return emptyEventConnectionPage();
        primeTimelineUserIncludes(config, response.page.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(response.page);
      } catch (err) {
        return handleAuthError(err);
      }
    },
    async resolveMessageLinkTarget({ roomId, eventId }) {
      try {
        const response = await client.resolveMessageLinkTarget(
          { roomId, eventId },
          { headers: headers() },
        );
        primeTimelineUserIncludes(config, response.includes?.users ?? {});
        return {
          event: response.event
            ? roomTimelineEventToRawEvent(
                response.event,
                response.includes?.users ?? {},
              )
            : null,
          threadRootEventId: response.threadRootEventId || null,
        };
      } catch (err) {
        return handleAuthError(err);
      }
    },
    async getThreadEvents({ roomId, threadRootEventId, limit, before, after }) {
      try {
        const response = await client.getThreadEvents(
          {
            roomId,
            threadRootEventId,
            limit,
            cursor: before
              ? { case: "before", value: before }
              : after
                ? { case: "after", value: after }
                : { case: undefined },
          },
          { headers: headers() },
        );
        primeTimelineUserIncludes(config, response.page?.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(
          response.page ?? new RoomTimelinePage(),
        );
      } catch (err) {
        return handleAuthError(err);
      }
    },
    async getThreadEventsAround({ roomId, threadRootEventId, eventId, limit }) {
      try {
        const response = await client.getThreadEventsAround(
          { roomId, threadRootEventId, eventId, limit },
          { headers: headers() },
        );
        if (!response.page) return emptyEventConnectionPage();
        primeTimelineUserIncludes(config, response.page.includes?.users ?? {});
        return roomTimelinePageToEventConnectionPage(response.page);
      } catch (err) {
        return handleAuthError(err);
      }
    },
  };
}

function primeTimelineUserIncludes(
  config: RoomTimelineAPIConfig,
  users: Record<string, User>,
) {
  notifyUserSummaries(
    config.serverId,
    Object.values(users).map((user) => ({
      id: user.id,
      login: user.login,
      displayName: user.displayName,
      deleted: user.deleted,
      avatarUrl: user.avatarUrl || null,
    })),
    config.onUserSummaries,
  );
}

function emptyEventConnectionPage(): EventConnectionPage {
  return {
    events: [],
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false,
  };
}

export function roomTimelinePageToEventConnectionPage(
  page: RoomTimelinePage,
): EventConnectionPage {
  const users = page.includes?.users ?? {};
  return {
    events: page.events
      .map((event) => roomTimelineEventToRawEvent(event, users))
      .filter((event): event is RawEvent => event !== null),
    startCursor: page.startCursor || null,
    endCursor: page.endCursor || null,
    hasOlder: page.hasOlder,
    hasNewer: page.hasNewer,
  };
}

export function roomTimelineEventToRawEvent(
  event: RoomTimelineEvent,
  users: Record<string, User>,
): RawEvent | null {
  const payload = timelinePayload(event, users);
  if (!payload) return null;
  return {
    id: event.id,
    createdAt: timestampToISO(event.createdAt),
    actorId: event.actorId,
    actor: userView(event.actorId, users),
    event: payload,
  } as unknown as RawEvent;
}

function timelinePayload(
  event: RoomTimelineEvent,
  users: Record<string, User>,
): RoomEventView["event"] | null {
  switch (event.event.case) {
    case "messagePosted":
      return messagePostedPayload(
        event.event.value,
        users,
      ) as RoomEventView["event"];
    case "roomCreated":
      return {
        kind: RoomEventKind.RoomCreated,
        roomId: event.event.value.roomId,
      } as never;
    case "roomUpdated":
      return {
        kind: RoomEventKind.RoomUpdated,
        roomId: event.event.value.roomId,
      } as never;
    case "roomDeleted":
      return {
        kind: RoomEventKind.RoomDeleted,
        roomId: event.event.value.roomId,
      } as never;
    case "roomArchived":
      return {
        kind: RoomEventKind.RoomArchived,
        roomId: event.event.value.roomId,
      } as never;
    case "roomUnarchived":
      return {
        kind: RoomEventKind.RoomUnarchived,
        roomId: event.event.value.roomId,
      } as never;
    case "userJoinedRoom":
      return {
        kind: RoomEventKind.UserJoinedRoom,
        roomId: event.event.value.roomId,
      } as never;
    case "userLeftRoom":
      return {
        kind: RoomEventKind.UserLeftRoom,
        roomId: event.event.value.roomId,
      } as never;
    default:
      return null;
  }
}

function messagePostedPayload(
  message: RoomTimelineMessagePosted,
  users: Record<string, User>,
) {
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
    replyCount: message.replyCount,
    lastReplyAt: timestampToISOOrNull(message.lastReplyAt),
    threadParticipants: message.threadParticipantUserIds
      .map((id) => userView(id, users))
      .filter(
        (user): user is NonNullable<ReturnType<typeof userView>> =>
          user !== null,
      ),
    viewerIsFollowingThread:
      message.viewerIsFollowingThread !== undefined
        ? message.viewerIsFollowingThread
        : null,
    reactions: message.reactions.map((reaction) => ({
      emoji: reaction.emoji,
      count: reaction.count,
      hasReacted: reaction.hasReacted,
      users: reaction.userIds
        .map((id) => userView(id, users))
        .filter(
          (user): user is NonNullable<ReturnType<typeof userView>> =>
            user !== null,
        ),
    })),
  };
}

function userView(userId: string, users: Record<string, User>) {
  if (!userId) return null;
  const user = users[userId];
  if (!user) {
    return {
      id: userId,
      login: "",
      displayName: "Deleted User",
      deleted: true,
      avatarUrl: null,
      presenceStatus: PresenceStatus.Offline,
    };
  }
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    deleted: user.deleted,
    avatarUrl: user.avatarUrl || null,
    presenceStatus: PresenceStatus.Offline,
  };
}

function attachmentView(attachment: {
  id: string;
  filename: string;
  contentType: string;
  width: number;
  height: number;
  assetUrl?: RoomTimelineAssetUrl;
  thumbnailAssetUrl?: RoomTimelineAssetUrl;
  videoProcessing?: RoomTimelineVideoProcessing;
}) {
  return {
    id: attachment.id,
    filename: attachment.filename,
    contentType: attachment.contentType,
    width: attachment.width,
    height: attachment.height,
    assetUrl: assetUrlView(attachment.assetUrl),
    thumbnailAssetUrl: assetUrlView(attachment.thumbnailAssetUrl),
    videoProcessing: videoProcessingView(attachment.videoProcessing),
  };
}

function videoProcessingView(processing?: RoomTimelineVideoProcessing) {
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
      assetUrl: assetUrlView(variant.assetUrl),
    })),
  };
}

function videoProcessingStatusView(status: RoomTimelineVideoProcessingStatus) {
  switch (status) {
    case RoomTimelineVideoProcessingStatus.PROCESSING:
      return "PROCESSING";
    case RoomTimelineVideoProcessingStatus.COMPLETED:
      return "COMPLETED";
    case RoomTimelineVideoProcessingStatus.FAILED:
      return "FAILED";
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
  };
}

function assetUrlView(assetUrl?: RoomTimelineAssetUrl) {
  if (!assetUrl) return null;
  return {
    url: assetUrl.url,
    expiresAt: timestampToISOOrNull(assetUrl.expiresAt),
  };
}

function timestampToISO(timestamp: { toDate(): Date } | undefined): string {
  return timestampToISOOrNull(timestamp) ?? new Date(0).toISOString();
}

function timestampToISOOrNull(
  timestamp: { toDate(): Date } | undefined,
): string | null {
  return timestamp ? timestamp.toDate().toISOString() : null;
}
