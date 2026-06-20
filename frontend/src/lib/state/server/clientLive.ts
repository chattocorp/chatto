import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import type { Timestamp } from '@bufbuild/protobuf/wkt';
import {
  NotificationLevel,
  PresenceStatus,
  TimeFormat,
  VideoProcessingStatus
} from '$lib/gql/graphql';
import type { EventEnvelope, EventHandler } from '$lib/eventBus.svelte';
import {
  ClientLiveClientFrameSchema,
  ClientLivePongSchema,
  ClientLiveRequestSchema,
  ClientLiveServerFrameSchema,
  type ClientLiveHello,
  type ClientLiveServerFrame
} from '$lib/pb/chatto/core/v1/client_live_pb';
import type { Event } from '$lib/pb/chatto/core/v1/event_pb';
import type {
  LiveAssetURL,
  LiveAttachmentView,
  LiveEvent,
  LiveLinkPreviewView,
  LiveMessageAttachmentsUpdatedEvent,
  LiveMessageEditedEvent,
  LiveMessagePostedEvent,
  LiveMessageReactionsUpdatedEvent,
  LiveReactionSummaryView,
  LiveRoomEvent,
  LiveUserView,
  LiveVideoProcessingView
} from '$lib/pb/chatto/core/v1/live_events_pb';
import {
  NotificationLevel as ProtoNotificationLevel,
  TimeFormat as ProtoTimeFormat
} from '$lib/pb/chatto/core/v1/user_preferences_pb';
import type { LiveInfo, RegisteredServer } from './registry.svelte';

export type ClientLiveSubscription = {
  unsubscribe: () => void;
  request: ClientLiveRequestFunction;
};

const REQUIRED_CAPABILITIES = [
  'live.events.v1',
  'live.requests.v1',
  'history.room_events.v1',
  'history.thread_events.v1'
] as const;

const CLIENT_LIVE_HANDSHAKE_TIMEOUT_MS = 10_000;

export type ClientLiveRequestFunction = (
  type: string,
  payload: Uint8Array,
  options?: { timeoutMs?: number }
) => Promise<Uint8Array>;

export function startClientLiveSubscription({
  server,
  info,
  onEvent,
  onCatchUpNeeded,
  onReady,
  onEnd,
  onError
}: {
  server: RegisteredServer;
  info: LiveInfo;
  onEvent: EventHandler;
  onCatchUpNeeded: () => void;
  onReady?: () => void;
  onEnd: () => void;
  onError: (err: unknown) => void;
}): ClientLiveSubscription {
  let stopped = false;
  let ws: WebSocket | null = null;
  let lastDeliverySequence = 0n;
  let nextRequestId = 1n;
  let readySettled = false;
  let resolveReady: () => void = () => {};
  let rejectReady: (err: unknown) => void = () => {};
  let handshakeTimer: ReturnType<typeof setTimeout> | null = null;
  const ready = new Promise<void>((resolve, reject) => {
    resolveReady = resolve;
    rejectReady = reject;
  });
  void ready.catch(() => {});
  const pendingRequests = new Map<
    string,
    {
      type: string;
      resolve: (payload: Uint8Array) => void;
      reject: (err: unknown) => void;
      timer: ReturnType<typeof setTimeout>;
    }
  >();

  const settleReady = (err?: unknown) => {
    if (readySettled) return;
    readySettled = true;
    if (err) rejectReady(err);
    else resolveReady();
  };

  const rejectPending = (err: unknown) => {
    settleReady(err);
    for (const pending of pendingRequests.values()) {
      clearTimeout(pending.timer);
      pending.reject(err);
    }
    pendingRequests.clear();
  };

  const request: ClientLiveRequestFunction = (type, payload, options = {}) => {
    const requestId = nextRequestId++;
    const requestKey = requestId.toString();
    const timeoutMs = options.timeoutMs ?? 15_000;
    return new Promise<Uint8Array>((resolve, reject) => {
      const timer = setTimeout(() => {
        pendingRequests.delete(requestKey);
        reject(new Error(`Chatto live request timed out: ${type}`));
      }, timeoutMs);
      pendingRequests.set(requestKey, { type, resolve, reject, timer });

      void ready
        .then(() => {
          if (stopped) throw new Error('Chatto live transport is stopped');
          if (!ws || ws.readyState !== WebSocket.OPEN) {
            throw new Error('Chatto live transport is not connected');
          }
          ws.send(
            toBinary(
              ClientLiveClientFrameSchema,
              create(ClientLiveClientFrameSchema, {
                requestId,
                payload: {
                  case: 'request',
                  value: create(ClientLiveRequestSchema, { type, payload })
                }
              })
            )
          );
        })
        .catch((err: unknown) => {
          const pending = pendingRequests.get(requestKey);
          if (!pending) return;
          clearTimeout(pending.timer);
          pendingRequests.delete(requestKey);
          pending.reject(err);
        });
    });
  };

  const run = async () => {
    try {
      const bearerToken = server.token;
      if (!bearerToken && !isSameOriginURL(server.url)) {
        throw new Error('Chatto live transport requires a bearer token');
      }
      const ticket = await fetchLiveTicket(server, info, bearerToken);
      validateTicketProtocol(ticket, info);
      if (stopped) return;

      ws = new WebSocket(
        withTicket(ticket.url || resolveServerURL(server.url, info.url), ticket.ticket)
      );
      ws.binaryType = 'arraybuffer';

      handshakeTimer = setTimeout(() => {
        if (stopped || readySettled) return;
        const err = new Error('Chatto live WebSocket did not complete the protocol handshake');
        rejectPending(err);
        onError(err);
        ws?.close();
      }, CLIENT_LIVE_HANDSHAKE_TIMEOUT_MS);
      ws.onmessage = (message) => {
        try {
          if (!(message.data instanceof ArrayBuffer)) {
            throw new Error('Chatto live transport received a non-binary frame');
          }
          const frame = fromBinary(ClientLiveServerFrameSchema, new Uint8Array(message.data));
          if (frame.payload.case === 'hello') {
            validateHello(frame.payload.value, info);
            clearHandshakeTimer();
            settleReady();
            onReady?.();
            return;
          }
          if (!readySettled) {
            throw new Error('Chatto live transport received data before the server hello');
          }
          if (frame.deliverySequence > 0n) {
            if (lastDeliverySequence > 0n && frame.deliverySequence !== lastDeliverySequence + 1n) {
              onCatchUpNeeded();
            }
            lastDeliverySequence = frame.deliverySequence;
          }
          if (frame.payload.case === 'response' && frame.requestId > 0n) {
            const requestKey = frame.requestId.toString();
            const pending = pendingRequests.get(requestKey);
            if (!pending) return;
            pendingRequests.delete(requestKey);
            clearTimeout(pending.timer);
            pending.resolve(frame.payload.value.payload);
            return;
          }
          if (frame.payload.case === 'error' && frame.requestId > 0n) {
            const requestKey = frame.requestId.toString();
            const pending = pendingRequests.get(requestKey);
            if (!pending) return;
            pendingRequests.delete(requestKey);
            clearTimeout(pending.timer);
            const error = frame.payload.value;
            pending.reject(
              new Error(
                error.message || error.code || `Chatto live request failed: ${pending.type}`
              )
            );
            return;
          }
          const decoded = adaptWireFrame(frame);
          if (decoded.error) {
            if (decoded.error.fatal) {
              throw new Error(decoded.error.message || decoded.error.code || 'Chatto live error');
            }
            console.warn('[clientLive] server error frame', decoded.error);
            return;
          }
          if (decoded.pongNonce) {
            sendPong(ws, decoded.pongNonce);
            return;
          }
          if (decoded.needsCatchUp) {
            onCatchUpNeeded();
          }
          if (decoded.event) {
            onEvent(decoded.event);
          }
        } catch (err) {
          if (stopped) return;
          rejectPending(err);
          onError(err);
          ws?.close();
        }
      };
      ws.onerror = () => {
        if (!stopped) onError(new Error('Chatto live WebSocket failed'));
        ws?.close();
      };
      ws.onclose = () => {
        clearHandshakeTimer();
        rejectPending(new Error('Chatto live WebSocket closed'));
        if (stopped) return;
        onEnd();
      };
    } catch (err) {
      if (stopped) return;
      rejectPending(err);
      onError(err);
      onEnd();
    }
  };

  void run();

  return {
    request,
    unsubscribe: () => {
      stopped = true;
      clearHandshakeTimer();
      rejectPending(new Error('Chatto live transport stopped'));
      ws?.close();
    }
  };

  function clearHandshakeTimer() {
    if (!handshakeTimer) return;
    clearTimeout(handshakeTimer);
    handshakeTimer = null;
  }
}

type LiveTicket = {
  url?: string;
  ticket: string;
  expiresAt?: string;
  protocol?: string;
};

async function fetchLiveTicket(
  server: RegisteredServer,
  info: LiveInfo,
  bearerToken: string | null
): Promise<LiveTicket> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  };
  if (bearerToken) {
    headers.Authorization = `Bearer ${bearerToken}`;
  }
  const response = await fetch(resolveServerURL(server.url, info.tokenUrl), {
    method: 'POST',
    headers,
    credentials: bearerToken ? 'omit' : 'same-origin'
  });
  if (!response.ok) {
    throw new Error(`Chatto live token request failed with ${response.status}`);
  }
  const body = (await response.json()) as LiveTicket;
  if (typeof body.ticket !== 'string' || body.ticket === '') {
    throw new Error('Chatto live token response did not include a ticket');
  }
  return body;
}

function validateTicketProtocol(ticket: LiveTicket, info: LiveInfo) {
  if (ticket.protocol && ticket.protocol !== info.protocol) {
    throw new Error(
      `Chatto live token returned protocol ${ticket.protocol}, expected ${info.protocol}`
    );
  }
}

function validateHello(hello: ClientLiveHello, info: LiveInfo) {
  if (hello.protocol !== info.protocol) {
    throw new Error(`Chatto live protocol mismatch: ${hello.protocol || '<missing>'}`);
  }
  const capabilities = new Set(hello.capabilities);
  const missing = REQUIRED_CAPABILITIES.filter((capability) => !capabilities.has(capability));
  if (missing.length > 0) {
    throw new Error(`Chatto live server is missing capabilities: ${missing.join(', ')}`);
  }
}

type AdaptedWireFrame = {
  event: EventEnvelope | null;
  needsCatchUp: boolean;
  error?: { code: string; message: string; fatal: boolean };
  pongNonce?: string;
};

function adaptWireFrame(frame: ClientLiveServerFrame): AdaptedWireFrame {
  switch (frame.payload.case) {
    case 'hello':
    case 'response':
      return { event: null, needsCatchUp: false };
    case 'event':
      return adaptDurableEvent(frame, frame.payload.value);
    case 'liveEvent':
      return adaptLiveEvent(frame, frame.payload.value);
    case 'heartbeat':
      return {
        event: eventEnvelope(frame, { __typename: 'HeartbeatEvent', alive: true }),
        needsCatchUp: false
      };
    case 'ping':
      return { event: null, needsCatchUp: false, pongNonce: frame.payload.value.nonce };
    case 'error':
      return { event: null, needsCatchUp: false, error: frame.payload.value };
    default:
      return { event: null, needsCatchUp: true };
  }
}

function adaptDurableEvent(frame: ClientLiveServerFrame, event: Event): AdaptedWireFrame {
  const payload = event.event;
  switch (payload.case) {
    case 'roomCreated':
      return emit(frame, { __typename: 'RoomCreatedEvent', roomId: payload.value.roomId });
    case 'roomUpdated':
      return emit(frame, { __typename: 'RoomUpdatedEvent', roomId: payload.value.roomId });
    case 'roomDeleted':
      return emit(frame, { __typename: 'RoomDeletedEvent', roomId: payload.value.roomId });
    case 'roomArchived':
      return emit(frame, { __typename: 'RoomArchivedEvent', roomId: payload.value.roomId });
    case 'roomUnarchived':
      return emit(frame, { __typename: 'RoomUnarchivedEvent', roomId: payload.value.roomId });
    case 'userJoinedRoom':
      return emit(frame, { __typename: 'UserJoinedRoomEvent', roomId: payload.value.roomId });
    case 'userLeftRoom':
      return emit(frame, { __typename: 'UserLeftRoomEvent', roomId: payload.value.roomId });
    case 'roomMemberBanned':
      return emit(frame, {
        __typename: 'RoomMemberBannedEvent',
        roomId: payload.value.roomId,
        userId: payload.value.userId
      });
    case 'roomMemberUnbanned':
      return emit(frame, {
        __typename: 'RoomMemberUnbannedEvent',
        roomId: payload.value.roomId,
        userId: payload.value.userId
      });
    case 'serverMemberDeleted':
      return emit(frame, { __typename: 'ServerMemberDeletedEvent', userId: payload.value.userId });
    case 'voiceCallStarted':
      return emit(frame, {
        __typename: 'CallStartedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'voiceCallParticipantJoined':
      return emit(frame, {
        __typename: 'CallParticipantJoinedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'voiceCallParticipantLeft':
      return emit(frame, {
        __typename: 'CallParticipantLeftEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'voiceCallEnded':
      return emit(frame, {
        __typename: 'CallEndedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'messageRetracted':
      return emit(frame, {
        __typename: 'MessageRetractedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.eventId,
        retractedReason: nullIfEmpty(payload.value.reason)
      });
    case 'threadCreated':
      return emit(frame, {
        __typename: 'ThreadCreatedEvent',
        roomId: payload.value.roomId,
        threadRootEventId: payload.value.threadRootEventId
      });
    case 'reactionAdded':
      return emit(frame, {
        __typename: 'ReactionAddedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.messageEventId,
        emoji: payload.value.emoji
      });
    case 'reactionRemoved':
      return emit(frame, {
        __typename: 'ReactionRemovedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.messageEventId,
        emoji: payload.value.emoji
      });
    default:
      return { event: null, needsCatchUp: true };
  }
}

function adaptLiveEvent(frame: ClientLiveServerFrame, event: LiveEvent): AdaptedWireFrame {
  const payload = event.event;
  switch (payload.case) {
    case 'userCreated':
      return emit(frame, {
        __typename: 'UserCreatedEvent',
        userId: payload.value.userId,
        login: payload.value.login,
        displayName: payload.value.displayName
      });
    case 'userDeleted':
      return emit(frame, { __typename: 'UserDeletedEvent', userId: payload.value.userId });
    case 'userProfileUpdated':
      return emit(frame, {
        __typename: 'UserProfileUpdatedEvent',
        userId: payload.value.userId,
        displayName: payload.value.displayName,
        avatarUrl: nullIfEmpty(payload.value.avatarUrl),
        login: payload.value.login
      });
    case 'serverUserPreferencesUpdated':
      return emit(frame, {
        __typename: 'ServerUserPreferencesUpdatedEvent',
        timezone: nullIfEmpty(payload.value.timezone ?? ''),
        timeFormat: adaptTimeFormat(payload.value.timeFormat)
      });
    case 'notificationLevelChanged':
      return emit(frame, {
        __typename: 'NotificationLevelChangedEvent',
        nlcRoomId: nullIfEmpty(payload.value.roomId),
        level: adaptNotificationLevel(payload.value.level),
        effectiveLevel: adaptNotificationLevel(payload.value.effectiveLevel)
      });
    case 'threadFollowChanged':
      return emit(frame, {
        __typename: 'ThreadFollowChangedEvent',
        tfcRoomId: payload.value.roomId,
        tfcThreadRootEventId: payload.value.threadRootEventId,
        isFollowing: payload.value.isFollowing
      });
    case 'serverMemberDeleted':
      return emit(frame, { __typename: 'ServerMemberDeletedEvent', userId: payload.value.userId });
    case 'serverUpdated':
      return emit(frame, {
        __typename: 'ServerUpdatedEvent',
        name: payload.value.name,
        description: nullIfEmpty(payload.value.description),
        logoUrl: nullIfEmpty(payload.value.logoUrl),
        bannerUrl: nullIfEmpty(payload.value.bannerUrl)
      });
    case 'userTyping':
      return emit(frame, {
        __typename: 'UserTypingEvent',
        roomId: payload.value.roomId,
        typingThreadRootEventId: nullIfEmpty(payload.value.threadRootEventId ?? '')
      });
    case 'presenceChanged':
      return emit(frame, { __typename: 'PresenceChangedEvent', status: payload.value.status });
    case 'callParticipantJoined':
      return emit(frame, {
        __typename: 'CallParticipantJoinedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'callParticipantLeft':
      return emit(frame, {
        __typename: 'CallParticipantLeftEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'notificationCreated':
      return emit(frame, {
        __typename: 'NotificationCreatedEvent',
        notificationId: payload.value.notificationId,
        roomId: payload.value.roomId,
        eventId: nullIfEmpty(payload.value.eventId),
        inReplyToId: nullIfEmpty(payload.value.inReplyToId)
      });
    case 'notificationDismissed':
      return emit(frame, {
        __typename: 'NotificationDismissedEvent',
        notificationId: payload.value.notificationId
      });
    case 'roomMarkedAsRead':
      return emit(frame, { __typename: 'RoomMarkedAsReadEvent', roomId: payload.value.roomId });
    case 'mentionStatusCleared':
      return emit(frame, { __typename: 'MentionStatusClearedEvent', roomId: payload.value.roomId });
    case 'roomGroupsUpdated':
      return emit(frame, { __typename: 'RoomGroupsUpdatedEvent', changed: true });
    case 'sessionTerminated':
      return emit(frame, {
        __typename: 'SessionTerminatedEvent',
        reason: payload.value.reason
      });
    case 'roomEvent':
      return adaptLiveRoomEvent(frame, payload.value);
    case 'messageReactionsUpdated':
      return emit(frame, adaptMessageReactionsUpdated(payload.value));
    case 'messageAttachmentsUpdated':
      return emit(frame, adaptMessageAttachmentsUpdated(payload.value));
    default:
      return { event: null, needsCatchUp: false };
  }
}

function adaptLiveRoomEvent(
  frame: ClientLiveServerFrame,
  roomEvent: LiveRoomEvent
): AdaptedWireFrame {
  const event = liveRoomEventToEnvelope(roomEvent, frame);
  return { event, needsCatchUp: false };
}

export function liveRoomEventToEnvelope(
  roomEvent: LiveRoomEvent,
  frame?: ClientLiveServerFrame
): EventEnvelope | null {
  const payload = roomEvent.event;
  const envelope = (eventPayload: Record<string, unknown>): EventEnvelope =>
    liveRoomEventEnvelope(roomEvent, eventPayload, frame);

  switch (payload.case) {
    case 'messagePosted':
      return envelope(adaptMessagePosted(payload.value));
    case 'messageEdited':
      return envelope(adaptMessageEdited(payload.value));
    case 'messageRetracted':
      return envelope({
        __typename: 'MessageRetractedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.eventId,
        retractedReason: nullIfEmpty(payload.value.reason)
      });
    case 'roomCreated':
      return envelope({ __typename: 'RoomCreatedEvent', roomId: payload.value.roomId });
    case 'roomUpdated':
      return envelope({ __typename: 'RoomUpdatedEvent', roomId: payload.value.roomId });
    case 'roomDeleted':
      return envelope({ __typename: 'RoomDeletedEvent', roomId: payload.value.roomId });
    case 'roomArchived':
      return envelope({ __typename: 'RoomArchivedEvent', roomId: payload.value.roomId });
    case 'roomUnarchived':
      return envelope({ __typename: 'RoomUnarchivedEvent', roomId: payload.value.roomId });
    case 'userJoinedRoom':
      return envelope({ __typename: 'UserJoinedRoomEvent', roomId: payload.value.roomId });
    case 'userLeftRoom':
      return envelope({ __typename: 'UserLeftRoomEvent', roomId: payload.value.roomId });
    case 'roomMemberBanned':
      return envelope({
        __typename: 'RoomMemberBannedEvent',
        roomId: payload.value.roomId,
        userId: payload.value.userId
      });
    case 'roomMemberUnbanned':
      return envelope({
        __typename: 'RoomMemberUnbannedEvent',
        roomId: payload.value.roomId,
        userId: payload.value.userId
      });
    case 'serverMemberDeleted':
      return envelope({ __typename: 'ServerMemberDeletedEvent', userId: payload.value.userId });
    case 'threadCreated':
      return envelope({
        __typename: 'ThreadCreatedEvent',
        roomId: payload.value.roomId,
        threadRootEventId: payload.value.threadRootEventId
      });
    case 'callStarted':
      return envelope({
        __typename: 'CallStartedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'callParticipantJoined':
      return envelope({
        __typename: 'CallParticipantJoinedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'callParticipantLeft':
      return envelope({
        __typename: 'CallParticipantLeftEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'callEnded':
      return envelope({
        __typename: 'CallEndedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      });
    case 'assetDeleted':
      return envelope({
        __typename: 'AssetDeletedEvent',
        deletedRoomId: nullIfEmpty(roomEvent.roomId),
        assetId: payload.value.assetId
      });
    default:
      return null;
  }
}

function adaptMessagePosted(message: LiveMessagePostedEvent): Record<string, unknown> {
  return {
    __typename: 'MessagePostedEvent',
    roomId: message.roomId,
    body: message.body ?? null,
    attachments: message.attachments.map(adaptAttachment),
    linkPreview: adaptLinkPreview(message.linkPreview),
    reactions: message.reactions.map(adaptReaction),
    updatedAt: message.updatedAt ?? null,
    inReplyTo: message.inReplyTo ?? null,
    threadRootEventId: message.threadRootEventId ?? null,
    echoOfEventId: message.echoOfEventId ?? null,
    echoFromThreadRootEventId: message.echoFromThreadRootEventId ?? null,
    channelEchoEventId: message.channelEchoEventId ?? null,
    replyCount: message.replyCount,
    lastReplyAt: message.lastReplyAt ? timestampToISO(message.lastReplyAt) : null,
    threadParticipants: message.threadParticipants.map(adaptUser),
    viewerIsFollowingThread: message.viewerIsFollowingThread ?? null
  };
}

function adaptMessageEdited(message: LiveMessageEditedEvent): Record<string, unknown> {
  return {
    __typename: 'MessageEditedEvent',
    roomId: message.roomId,
    messageEventId: message.messageEventId,
    body: message.body ?? null,
    attachments: message.attachments.map(adaptAttachment),
    linkPreview: adaptLinkPreview(message.linkPreview),
    updatedAt: message.updatedAt ?? null
  };
}

function adaptMessageReactionsUpdated(
  message: LiveMessageReactionsUpdatedEvent
): Record<string, unknown> {
  return {
    __typename: 'MessageReactionsUpdatedEvent',
    roomId: message.roomId,
    messageEventId: message.messageEventId,
    reactions: message.reactions.map(adaptReaction)
  };
}

function adaptMessageAttachmentsUpdated(
  message: LiveMessageAttachmentsUpdatedEvent
): Record<string, unknown> {
  return {
    __typename: 'MessageAttachmentsUpdatedEvent',
    roomId: message.roomId,
    messageEventId: message.messageEventId,
    attachments: message.attachments.map(adaptAttachment),
    linkPreview: adaptLinkPreview(message.linkPreview),
    updatedAt: message.updatedAt ?? null
  };
}

function adaptReaction(reaction: LiveReactionSummaryView): Record<string, unknown> {
  return {
    __typename: 'ReactionSummary',
    emoji: reaction.emoji,
    count: reaction.count,
    hasReacted: reaction.hasReacted,
    users: reaction.users.map((user) => ({
      __typename: 'ReactionUser',
      id: user.id,
      displayName: user.displayName
    }))
  };
}

function adaptAttachment(attachment: LiveAttachmentView): Record<string, unknown> {
  return {
    __typename: 'Attachment',
    id: attachment.id,
    filename: attachment.filename,
    contentType: attachment.contentType,
    width: attachment.width,
    height: attachment.height,
    assetUrl: adaptAssetURL(attachment.assetUrl),
    thumbnailAssetUrl: adaptAssetURL(attachment.thumbnailAssetUrl),
    videoProcessing: adaptVideoProcessing(attachment.videoProcessing)
  };
}

function adaptVideoProcessing(video?: LiveVideoProcessingView): Record<string, unknown> | null {
  if (!video) return null;
  return {
    __typename: 'VideoProcessing',
    status: adaptVideoProcessingStatus(video.status),
    durationMs: video.durationMs == null ? null : Number(video.durationMs),
    width: video.width ?? null,
    height: video.height ?? null,
    thumbnailAssetUrl: adaptAssetURL(video.thumbnailAssetUrl),
    sourceAvailable: video.sourceAvailable,
    variants: video.variants.map((variant) => ({
      __typename: 'VideoVariant',
      quality: variant.quality,
      width: variant.width,
      height: variant.height,
      size: Number(variant.size),
      assetUrl: adaptAssetURL(variant.assetUrl)
    })),
    reasonCode: video.reasonCode ?? null
  };
}

function adaptLinkPreview(preview?: LiveLinkPreviewView): Record<string, unknown> | null {
  if (!preview) return null;
  return {
    __typename: 'LinkPreview',
    url: preview.url,
    title: preview.title,
    description: preview.description,
    imageUrl: nullIfEmpty(preview.imageUrl),
    siteName: preview.siteName,
    embedType: preview.embedType,
    embedId: preview.embedId ?? null
  };
}

function adaptAssetURL(assetURL?: LiveAssetURL): Record<string, unknown> | null {
  if (!assetURL || !assetURL.url) return null;
  return {
    __typename: 'AssetURL',
    url: assetURL.url,
    expiresAt: timestampToISO(assetURL.expiresAt)
  };
}

function adaptUser(user?: LiveUserView): Record<string, unknown> | null {
  if (!user) return null;
  return {
    __typename: 'User',
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    deleted: user.deleted,
    avatarUrl: nullIfEmpty(user.avatarUrl),
    presenceStatus: adaptPresenceStatus(user.presenceStatus)
  };
}

function emit(frame: ClientLiveServerFrame, payload: Record<string, unknown>): AdaptedWireFrame {
  return { event: eventEnvelope(frame, payload), needsCatchUp: false };
}

function eventEnvelope(
  frame: ClientLiveServerFrame,
  payload: Record<string, unknown>
): EventEnvelope {
  return {
    id: frame.id || `live-${frame.deliverySequence.toString()}`,
    createdAt: timestampToISO(frame.createdAt),
    actorId: nullIfEmpty(frame.actorId),
    actor: null,
    event: payload
  } as EventEnvelope;
}

function liveRoomEventEnvelope(
  roomEvent: LiveRoomEvent,
  payload: Record<string, unknown>,
  frame?: ClientLiveServerFrame
): EventEnvelope {
  return {
    id: roomEvent.id || frame?.id || `live-${frame?.deliverySequence.toString() ?? '0'}`,
    createdAt: timestampToISO(roomEvent.createdAt ?? frame?.createdAt),
    actorId: nullIfEmpty(roomEvent.actorId || frame?.actorId || ''),
    actor: adaptUser(roomEvent.actor),
    event: payload
  } as EventEnvelope;
}

function sendPong(ws: WebSocket | null, nonce: string) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  ws.send(
    toBinary(
      ClientLiveClientFrameSchema,
      create(ClientLiveClientFrameSchema, {
        payload: {
          case: 'pong',
          value: create(ClientLivePongSchema, { nonce })
        }
      })
    )
  );
}

function timestampToISO(timestamp?: Timestamp): string {
  if (!timestamp) return new Date().toISOString();
  const seconds =
    typeof timestamp.seconds === 'bigint'
      ? Number(timestamp.seconds)
      : Number(timestamp.seconds ?? 0);
  const millis = seconds * 1000 + Math.floor((timestamp.nanos ?? 0) / 1_000_000);
  return new Date(millis).toISOString();
}

function nullIfEmpty(value: string): string | null {
  return value === '' ? null : value;
}

function adaptNotificationLevel(level: ProtoNotificationLevel): NotificationLevel {
  switch (level) {
    case ProtoNotificationLevel.MUTED:
      return NotificationLevel.Muted;
    case ProtoNotificationLevel.ALL_MESSAGES:
      return NotificationLevel.AllMessages;
    case ProtoNotificationLevel.UNSPECIFIED:
      return NotificationLevel.Default;
    case ProtoNotificationLevel.NORMAL:
    default:
      return NotificationLevel.Normal;
  }
}

function adaptTimeFormat(format: ProtoTimeFormat): TimeFormat {
  switch (format) {
    case ProtoTimeFormat.TIME_FORMAT_12H:
      return TimeFormat.TwelveHour;
    case ProtoTimeFormat.TIME_FORMAT_24H:
      return TimeFormat.TwentyFourHour;
    case ProtoTimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}

function adaptPresenceStatus(status: string): PresenceStatus {
  switch (status) {
    case PresenceStatus.Online:
      return PresenceStatus.Online;
    case PresenceStatus.Away:
      return PresenceStatus.Away;
    case PresenceStatus.DoNotDisturb:
      return PresenceStatus.DoNotDisturb;
    case PresenceStatus.Offline:
    default:
      return PresenceStatus.Offline;
  }
}

function adaptVideoProcessingStatus(status: string): VideoProcessingStatus {
  switch (status) {
    case VideoProcessingStatus.Completed:
      return VideoProcessingStatus.Completed;
    case VideoProcessingStatus.Failed:
      return VideoProcessingStatus.Failed;
    case VideoProcessingStatus.Processing:
      return VideoProcessingStatus.Processing;
    case VideoProcessingStatus.Pending:
    default:
      return VideoProcessingStatus.Pending;
  }
}

function withTicket(rawURL: string, ticket: string): string {
  const url = new URL(rawURL);
  url.searchParams.set('ticket', ticket);
  return url.toString();
}

function resolveServerURL(baseURL: string, maybeRelative: string): string {
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(maybeRelative)) {
    return maybeRelative;
  }
  return new URL(maybeRelative, baseURL).toString();
}

function isSameOriginURL(rawURL: string): boolean {
  if (typeof window === 'undefined') return false;
  try {
    return new URL(rawURL, window.location.href).origin === window.location.origin;
  } catch {
    return false;
  }
}
