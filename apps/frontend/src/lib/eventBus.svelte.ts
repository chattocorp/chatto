/**
 * Single realtime stream per connected server, covering everything the user
 * can receive (deployment-wide events and room-scoped events over one stream).
 *
 * The manager keeps one bus per registered server. Consumers register
 * handlers either via Svelte context (current active server) or directly
 * against a specific server's bus through the manager (used by the
 * cross-server sidebar wiring).
 */

import { createContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import { useRenderData } from './render/data';
import {
  RoomEventViewDocument,
  UserAvatarUserViewDocument,
  type LinkPreviewView,
  type MessageAttachmentView,
  type NotificationLevel,
  type PresenceStatus,
  type TimeFormat,
  type UserAvatarUserView
} from './render/types';
import { eventBusManager } from './state/server/eventBus.svelte';
import type { CustomUserStatus } from './state/userProfiles.svelte';
import type { RealtimeEventEnvelope as RealtimeProtobufEventEnvelope } from '@chatto/api-types/realtime/v1/realtime_pb';
import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';

type EventEnvelopeReactionSummary = {
  emoji: string;
  count: number;
  hasReacted: boolean;
  users: Array<{ id: string; displayName: string }>;
};

type EventEnvelopeEvent =
  | { kind: typeof RoomEventKind.AssetDeleted; assetId: string; deletedRoomId?: string | null }
  | {
      kind: typeof RoomEventKind.AssetProcessingFailed;
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | {
      kind: typeof RoomEventKind.AssetProcessingStarted;
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | {
      kind: typeof RoomEventKind.AssetProcessingSucceeded;
      assetId: string;
      processingRoomId?: string | null;
      processingMessageEventId?: string | null;
    }
  | { kind: typeof RoomEventKind.CallEnded; roomId: string; callId: string }
  | { kind: typeof RoomEventKind.CallParticipantJoined; roomId: string; callId: string }
  | { kind: typeof RoomEventKind.CallParticipantLeft; roomId: string; callId: string }
  | { kind: typeof RoomEventKind.CallStarted; roomId: string; callId: string }
  | { kind: typeof RoomEventKind.Heartbeat; alive?: boolean }
  | {
      kind: typeof RoomEventKind.MentionNotification;
      roomId: string;
      room: { name: string };
      actor?: { id: string; displayName: string } | null;
    }
  | { kind: typeof RoomEventKind.MentionStatusCleared }
  | {
      kind: typeof RoomEventKind.MessageEdited;
      roomId: string;
      messageEventId: string;
      body?: string | null;
      attachments: MessageAttachmentView[];
      linkPreview?: LinkPreviewView | null;
      updatedAt?: string | null;
    }
  | {
      kind: typeof RoomEventKind.MessagePosted;
      roomId: string;
      messageEventId?: string;
      body?: string | null;
      attachments?: MessageAttachmentView[];
      linkPreview?: LinkPreviewView | null;
      reactions?: EventEnvelopeReactionSummary[];
      updatedAt?: string | null;
      inReplyTo?: string | null;
      threadRootEventId?: string | null;
      echoOfEventId?: string | null;
      echoFromThreadRootEventId?: string | null;
      channelEchoEventId?: string | null;
      replyCount?: number;
      lastReplyAt?: string | null;
      threadParticipants?: UserAvatarUserView[];
      viewerIsFollowingThread?: boolean | null;
    }
  | {
      kind: typeof RoomEventKind.MessageRetracted;
      roomId: string;
      messageEventId: string;
      retractedReason?: string | null;
    }
  | {
      kind: typeof RoomEventKind.NewDirectMessageNotification;
      roomId: string;
      conversationName: string;
      sender?: {
        id: string;
        displayName: string;
        avatarUrl?: string | null;
      } | null;
    }
  | {
      kind: typeof RoomEventKind.NotificationCreated;
      notificationId: string;
      roomId: string;
      eventId?: string | null;
      inReplyToId?: string | null;
      silent?: boolean;
    }
  | { kind: typeof RoomEventKind.NotificationDismissed; notificationId: string }
  | {
      kind: typeof RoomEventKind.NotificationLevelChanged;
      level: NotificationLevel;
      effectiveLevel: NotificationLevel;
      nlcRoomId?: string | null;
    }
  | { kind: typeof RoomEventKind.PresenceChanged; status: PresenceStatus }
  | {
      kind: typeof RoomEventKind.ReactionAdded;
      roomId: string;
      messageEventId: string;
      emoji: string;
    }
  | {
      kind: typeof RoomEventKind.ReactionRemoved;
      roomId: string;
      messageEventId: string;
      emoji: string;
    }
  | { kind: typeof RoomEventKind.RoomArchived; roomId: string }
  | { kind: typeof RoomEventKind.RoomCreated; roomId: string }
  | { kind: typeof RoomEventKind.RoomDeleted; roomId: string }
  | { kind: typeof RoomEventKind.RoomGroupsUpdated; changed?: boolean }
  | { kind: typeof RoomEventKind.RoomMarkedAsRead; roomId: string }
  | { kind: typeof RoomEventKind.RoomMemberBanned }
  | { kind: typeof RoomEventKind.RoomMemberUnbanned }
  | { kind: typeof RoomEventKind.RoomUnarchived; roomId: string }
  | { kind: typeof RoomEventKind.RoomUniversalChanged; roomId: string; universal?: boolean }
  | { kind: typeof RoomEventKind.RoomUpdated; roomId: string }
  | { kind: typeof RoomEventKind.ServerMemberDeleted; userId: string }
  | {
      kind: typeof RoomEventKind.ServerUpdated;
      name?: string;
      description?: string | null;
      logoUrl?: string | null;
      bannerUrl?: string | null;
    }
  | {
      kind: typeof RoomEventKind.ServerUserPreferencesUpdated;
      timezone: string | null;
      timeFormat: TimeFormat;
    }
  | { kind: typeof RoomEventKind.SessionTerminated; reason: string }
  | { kind: typeof RoomEventKind.ThreadCreated; roomId?: string; threadRootEventId?: string }
  | {
      kind: typeof RoomEventKind.ThreadFollowChanged;
      isFollowing: boolean;
      tfcRoomId: string;
      tfcThreadRootEventId: string;
    }
  | { kind: typeof RoomEventKind.UserCreated }
  | { kind: typeof RoomEventKind.UserCustomStatusCleared; userId: string }
  | {
      kind: typeof RoomEventKind.UserCustomStatusSet;
      userId: string;
      setCustomStatus: CustomUserStatus;
    }
  | { kind: typeof RoomEventKind.UserDeleted }
  | { kind: typeof RoomEventKind.UserJoinedRoom; roomId: string }
  | { kind: typeof RoomEventKind.UserLeftRoom; roomId: string }
  | {
      kind: typeof RoomEventKind.UserProfileUpdated;
      userId: string;
      displayName: string;
      avatarUrl: string | null;
      login: string;
    }
  | {
      kind: typeof RoomEventKind.UserTyping;
      roomId: string;
      typingThreadRootEventId?: string | null;
    };

/** Re-export the RoomEventView render document so room-history stores can map
 * event payloads fetched from the ConnectRPC timeline compatibility DTOs. */
export { RoomEventViewDocument, useRenderData };

export type EventEnvelope = {
  id: string;
  createdAt: string;
  actorId?: string | null;
  actor?: unknown;
  event: EventEnvelopeEvent;
};

export type EventHandler = (event: EventEnvelope) => void;
export type EventBusCatchUpReason = 'subscription-ended' | 'ws-reconnected' | 'heartbeat-stalled';
export type EventBusCatchUpPhase = 'immediate' | 'projection-grace';
export type EventBusCatchUpSignal = {
  reason: EventBusCatchUpReason;
  phase: EventBusCatchUpPhase;
};
export type EventBusCatchUpHandler = (signal: EventBusCatchUpSignal) => void;

export interface EventBus {
  handlers: SvelteSet<EventHandler>;
  catchUpHandlers: SvelteSet<EventBusCatchUpHandler>;
}

const realtimeEnvelopeSymbol: unique symbol = Symbol('chattoRealtimeEventEnvelope');

type EventEnvelopeWithRealtime = EventEnvelope & {
  [realtimeEnvelopeSymbol]?: RealtimeProtobufEventEnvelope;
};

export function attachRealtimeEventEnvelope(
  event: EventEnvelope,
  realtimeEnvelope: RealtimeProtobufEventEnvelope
): EventEnvelope {
  Object.defineProperty(event, realtimeEnvelopeSymbol, {
    value: realtimeEnvelope,
    enumerable: false
  });
  return event;
}

export function getRealtimeEventEnvelope(
  event: EventEnvelope
): RealtimeProtobufEventEnvelope | undefined {
  return (event as EventEnvelopeWithRealtime)[realtimeEnvelopeSymbol];
}

// The context holds a getter — not a fixed bus — so reads from inside a
// consumer's $effect track whatever reactive state the getter touches
// (typically `page.params.serverId` via `getActiveServer`). When the URL
// `[serverId]` param changes, every `useEvent` / `onEvent` consumer
// re-subscribes against the new server's bus without needing a remount or
// a context refresh.
const [getServerBusGetter, setServerBusGetter] = createContext<() => EventBus | undefined>();

/**
 * Expose the active server's event bus to descendants via Svelte context.
 * Takes a getter so the context follows the active server reactively —
 * pass `() => activeServerId` (e.g. `getActiveServer()`) inside the
 * `[serverId]` tree, or `() => originServerId` at the top of the
 * authenticated app where the bus is fixed to the origin.
 */
export function provideEventBus(getServerId: () => string): void {
  setServerBusGetter(() => {
    const id = getServerId();
    return id ? eventBusManager.getBus(id) : undefined;
  });
}

/**
 * Register a handler against the active server's bus (resolved through
 * Svelte context). Returns a cleanup function — pair with `$effect` for
 * automatic teardown. The handler is automatically migrated to the new
 * server's bus when the active server changes, because the bus lookup
 * runs reactively inside the caller's `$effect`.
 */
export function onEvent(handler: EventHandler): () => void {
  let getBus: () => EventBus | undefined;
  try {
    getBus = getServerBusGetter();
  } catch {
    return () => {};
  }
  const bus = getBus();
  if (!bus) return () => {};
  bus.handlers.add(handler);
  return () => {
    bus.handlers.delete(handler);
  };
}

// ---------------------------------------------------------------------------
// Typed event handler helpers
// ---------------------------------------------------------------------------

// The extractor receives the inner event payload; helpers needing envelope
// fields (actorId, etc.) read them from the closure instead.

function onTypedEvent<TKind extends EventEnvelopeEvent['kind'], T>(
  kind: TKind,
  extract: (envelope: EventEnvelope, event: Extract<EventEnvelopeEvent, { kind: TKind }>) => T,
  handler: (data: T) => void
): () => void {
  let getBus: () => EventBus | undefined;
  try {
    getBus = getServerBusGetter();
  } catch {
    return () => {};
  }
  const bus = getBus();
  if (!bus) return () => {};

  const wrapper: EventHandler = (envelope) => {
    if (roomEventKind(envelope.event) === kind) {
      handler(extract(envelope, envelope.event as Extract<EventEnvelopeEvent, { kind: TKind }>));
    }
  };

  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

function onTypedEventDirect<TKind extends EventEnvelopeEvent['kind'], T>(
  bus: EventBus,
  kind: TKind,
  extract: (envelope: EventEnvelope, event: Extract<EventEnvelopeEvent, { kind: TKind }>) => T,
  handler: (data: T) => void
): () => void {
  const wrapper: EventHandler = (envelope) => {
    if (roomEventKind(envelope.event) === kind) {
      handler(extract(envelope, envelope.event as Extract<EventEnvelopeEvent, { kind: TKind }>));
    }
  };
  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

// ---------------------------------------------------------------------------
// Typed event handler exports
// ---------------------------------------------------------------------------

export type UserProfileUpdate = {
  userId: string;
  displayName: string;
  avatarUrl: string | null;
  login: string;
};

export function onUserProfileUpdate(handler: (update: UserProfileUpdate) => void): () => void {
  return onTypedEvent(
    RoomEventKind.UserProfileUpdated,
    (_env, e) => {
      return {
        userId: e.userId,
        displayName: e.displayName,
        avatarUrl: e.avatarUrl,
        login: e.login
      };
    },
    handler
  );
}

export type UserCustomStatusUpdate = {
  userId: string;
  customStatus: CustomUserStatus | null;
};

export function onUserCustomStatusUpdate(
  handler: (update: UserCustomStatusUpdate) => void
): () => void {
  const cleanupSet = onTypedEvent(
    RoomEventKind.UserCustomStatusSet,
    (_env, e) => ({ userId: e.userId, customStatus: e.setCustomStatus }),
    handler
  );
  const cleanupCleared = onTypedEvent(
    RoomEventKind.UserCustomStatusCleared,
    (_env, e) => ({ userId: e.userId, customStatus: null }),
    handler
  );
  return () => {
    cleanupSet();
    cleanupCleared();
  };
}

export type MentionNotification = {
  roomId: string;
  actorUserId: string;
  actorDisplayName: string;
  spaceName: string;
  roomName: string;
};

export function onMention(handler: (notification: MentionNotification) => void): () => void {
  return onTypedEvent(
    RoomEventKind.MentionNotification,
    (env, e) => {
      const realtime = getRealtimeEventEnvelope(env);
      if (realtime?.event.case === 'mentionNotification') {
        const mention = realtime.event.value;
        return {
          roomId: mention.roomId,
          actorUserId: mention.actorUserId ?? env.actorId ?? '',
          actorDisplayName: mention.actorDisplayName ?? 'Unknown user',
          spaceName: '',
          roomName: mention.roomName ?? ''
        };
      }

      const envelopeActor = env.actor ? useRenderData(UserAvatarUserViewDocument, env.actor) : null;
      const actor = e.actor ?? envelopeActor;

      return {
        roomId: e.roomId,
        actorUserId: actor?.id ?? env.actorId ?? '',
        actorDisplayName: actor?.displayName ?? 'Unknown user',
        spaceName: '',
        roomName: e.room.name
      };
    },
    handler
  );
}

export type DMNotification = {
  roomId: string;
  senderId: string;
  senderDisplayName: string;
  senderAvatarUrl: string;
  conversationName: string;
};

export function onNewDM(handler: (notification: DMNotification) => void): () => void {
  return onTypedEvent(
    RoomEventKind.NewDirectMessageNotification,
    (env, e) => {
      const realtime = getRealtimeEventEnvelope(env);
      if (realtime?.event.case === 'newDirectMessageNotification') {
        const dm = realtime.event.value;
        return {
          roomId: dm.roomId,
          senderId: dm.senderId ?? env.actorId ?? '',
          senderDisplayName: dm.senderDisplayName ?? 'Unknown user',
          senderAvatarUrl: dm.senderAvatarUrl ?? '',
          conversationName: dm.conversationName ?? ''
        };
      }

      const envelopeActor = env.actor ? useRenderData(UserAvatarUserViewDocument, env.actor) : null;
      const sender = e.sender ?? envelopeActor;

      return {
        roomId: e.roomId,
        senderId: sender?.id ?? env.actorId ?? '',
        senderDisplayName: sender?.displayName ?? 'Unknown user',
        senderAvatarUrl: sender?.avatarUrl ?? '',
        conversationName: e.conversationName
      };
    },
    handler
  );
}

export type NotificationCreatedInfo = {
  notificationId: string;
  spaceId?: string;
  roomId?: string;
  eventId?: string;
  inReplyToId?: string;
};

export function onNotificationCreated(
  handler: (info: NotificationCreatedInfo) => void
): () => void {
  return onTypedEvent(
    RoomEventKind.NotificationCreated,
    (env, e) => {
      const realtime = getRealtimeEventEnvelope(env);
      if (realtime?.event.case === 'notificationCreated') {
        const notification = realtime.event.value;
        return {
          notificationId: notification.notificationId,
          roomId: notification.roomId,
          eventId: notification.eventId,
          inReplyToId: notification.inReplyToId
        };
      }

      return {
        notificationId: e.notificationId,
        roomId: e.roomId ?? undefined,
        eventId: e.eventId ?? undefined,
        inReplyToId: e.inReplyToId ?? undefined
      };
    },
    handler
  );
}

export type NotificationDismissedInfo = {
  notificationId: string;
};

export function onNotificationDismissed(
  handler: (info: NotificationDismissedInfo) => void
): () => void {
  return onTypedEvent(
    RoomEventKind.NotificationDismissed,
    (_env, e) => {
      return { notificationId: e.notificationId };
    },
    handler
  );
}

export type RoomMarkedAsReadInfo = {
  roomId: string;
};

export function onRoomMarkedAsRead(handler: (info: RoomMarkedAsReadInfo) => void): () => void {
  return onTypedEvent(
    RoomEventKind.RoomMarkedAsRead,
    (_env, e) => {
      return { roomId: e.roomId };
    },
    handler
  );
}

export type UserSettingsUpdate = {
  timezone: string | null;
  timeFormat: TimeFormat;
};

export function onUserSettingsUpdate(handler: (update: UserSettingsUpdate) => void): () => void {
  return onTypedEvent(
    RoomEventKind.ServerUserPreferencesUpdated,
    (_env, e) => {
      return { timezone: e.timezone, timeFormat: e.timeFormat };
    },
    handler
  );
}

export type RoomLayoutUpdatedInfo = {
  roomId?: string;
  universal?: boolean;
};

export function onRoomLayoutUpdated(handler: (_info: RoomLayoutUpdatedInfo) => void): () => void {
  const unsubscribeGroupsUpdated = onTypedEvent(
    RoomEventKind.RoomGroupsUpdated,
    () => ({}),
    handler
  );
  const unsubscribeUniversalChanged = onTypedEvent(
    RoomEventKind.RoomUniversalChanged,
    (_env, e) => ({ roomId: e.roomId, universal: e.universal }),
    handler
  );
  return () => {
    unsubscribeGroupsUpdated();
    unsubscribeUniversalChanged();
  };
}

export type NotificationLevelChanged = {
  roomId: string | null;
  level: NotificationLevel;
  effectiveLevel: NotificationLevel;
};

export function onNotificationLevelChanged(
  handler: (update: NotificationLevelChanged) => void
): () => void {
  return onTypedEvent(
    RoomEventKind.NotificationLevelChanged,
    (_env, e) => {
      return {
        roomId: e.nlcRoomId ?? null,
        level: e.level,
        effectiveLevel: e.effectiveLevel
      };
    },
    handler
  );
}

export type ThreadFollowChanged = {
  roomId: string;
  threadRootEventId: string;
  isFollowing: boolean;
};

export function onThreadFollowChanged(handler: (update: ThreadFollowChanged) => void): () => void {
  return onTypedEvent(
    RoomEventKind.ThreadFollowChanged,
    (_env, e) => {
      return {
        roomId: e.tfcRoomId,
        threadRootEventId: e.tfcThreadRootEventId,
        isFollowing: e.isFollowing
      };
    },
    handler
  );
}

export function onSessionTerminated(handler: (reason: string) => void): () => void {
  return onTypedEvent(
    RoomEventKind.SessionTerminated,
    (_env, e) => {
      return e.reason;
    },
    handler
  );
}

// ---------------------------------------------------------------------------
// Room-scoped helpers
// ---------------------------------------------------------------------------

type PresenceHandler = (userId: string, status: PresenceStatus) => void;

export function onPresenceChange(handler: PresenceHandler): () => void {
  return onTypedEvent(
    RoomEventKind.PresenceChanged,
    (envelope, e) => {
      return { userId: envelope.actorId, status: e.status as PresenceStatus };
    },
    ({ userId, status }) => {
      if (!userId) return;
      handler(userId, status);
    }
  );
}

export interface TypingEventData {
  userId: string;
  roomId: string;
  threadRootEventId: string | null;
}

type TypingHandler = (data: TypingEventData) => void;

export function onTypingEvent(handler: TypingHandler): () => void {
  let getBus: () => EventBus | undefined;
  try {
    getBus = getServerBusGetter();
  } catch {
    return () => {};
  }
  const bus = getBus();
  if (!bus) return () => {};
  const wrapper: EventHandler = (event) => {
    if (roomEventKind(event.event) !== RoomEventKind.UserTyping) return;
    if (!event.actorId) return;
    const ev = event.event as { roomId: string; typingThreadRootEventId?: string | null };
    handler({
      userId: event.actorId,
      roomId: ev.roomId,
      threadRootEventId: ev.typingThreadRootEventId ?? null
    });
  };
  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

// ---------------------------------------------------------------------------
// Direct (cross-server) bus handler registrar
// ---------------------------------------------------------------------------

/**
 * Build a handler-registration surface bound to a specific server's bus.
 * Skips Svelte context entirely — used by sidebar wiring that needs to
 * attach handlers to every connected server's stream, not just the one
 * currently in focus.
 */
export function createEventBusHandlerRegistrar(serverId: string) {
  const bus = eventBusManager.getBus(serverId);
  if (!bus) return undefined;

  return {
    onEvent(handler: EventHandler): () => void {
      bus.handlers.add(handler);
      return () => {
        bus.handlers.delete(handler);
      };
    },
    onRoomMarkedAsRead(handler: (info: RoomMarkedAsReadInfo) => void): () => void {
      return onTypedEventDirect(
        bus,
        RoomEventKind.RoomMarkedAsRead,
        (_env, e) => {
          return { roomId: e.roomId };
        },
        handler
      );
    },
    onNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void): () => void {
      return onTypedEventDirect(
        bus,
        RoomEventKind.NotificationLevelChanged,
        (_env, e) => {
          return {
            roomId: e.nlcRoomId ?? null,
            level: e.level,
            effectiveLevel: e.effectiveLevel
          };
        },
        handler
      );
    },
    onRoomLayoutUpdated(handler: (info: RoomLayoutUpdatedInfo) => void): () => void {
      const unsubscribeGroupsUpdated = onTypedEventDirect(
        bus,
        RoomEventKind.RoomGroupsUpdated,
        () => ({}),
        handler
      );
      const unsubscribeUniversalChanged = onTypedEventDirect(
        bus,
        RoomEventKind.RoomUniversalChanged,
        (_env, e) => ({ roomId: e.roomId, universal: e.universal }),
        handler
      );
      return () => {
        unsubscribeGroupsUpdated();
        unsubscribeUniversalChanged();
      };
    }
  };
}
