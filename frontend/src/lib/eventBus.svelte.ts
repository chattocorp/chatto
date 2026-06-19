/**
 * Single protobuf wire event bus per connected server, covering everything
 * the user can receive over one stream.
 *
 * The manager keeps one bus per registered server. Consumers register handlers
 * either via Svelte context (current active server) or directly against a
 * specific server's bus through the manager (used by cross-server sidebar
 * wiring).
 */

import { createContext } from 'svelte';
import { SvelteDate, SvelteSet } from 'svelte/reactivity';
import type { Event as DurableEvent } from '$lib/pb/chatto/core/v1/event_pb';
import type { LiveEvent } from '$lib/pb/chatto/core/v1/live_events_pb';
import type { PresenceStatus, RoomEventViewFragment } from '$lib/chatTypes';
import { NotificationLevel } from '$lib/preferences/notificationLevel';
import { TimeFormat } from '$lib/preferences/timeFormat';
import {
  NotificationLevel as WireNotificationLevel,
  TimeFormat as WireTimeFormat
} from '$lib/pb/chatto/core/v1/user_preferences_pb';
import type { StreamEvent } from '$lib/pb/chatto/wire/v1/protocol_pb';
import { wireDurableEvent, wireLiveEvent, wireRoomTimelineId } from '$lib/wire/events';
import { eventBusManager } from './state/server/eventBus.svelte';

type ViewRoomEventPayload = NonNullable<RoomEventViewFragment['event']>;
type LiveEventTypename = LiveEventPayload['__typename'];
type RoomEventPayload = Exclude<ViewRoomEventPayload, { __typename: LiveEventTypename }>;

type EventUser = NonNullable<RoomEventViewFragment['actor']>;

type LiveEventPayload =
  | { __typename: 'HeartbeatEvent'; alive: boolean }
  | {
      __typename: 'ServerUpdatedEvent';
      name: string;
      description: string | null;
      logoUrl: string | null;
      bannerUrl: string | null;
    }
  | {
      __typename: 'UserProfileUpdatedEvent';
      userId: string;
      displayName: string;
      avatarUrl: string | null;
      login: string;
    }
  | {
      __typename: 'ServerUserPreferencesUpdatedEvent';
      timezone: string | null;
      timeFormat: TimeFormat;
    }
  | {
      __typename: 'NotificationLevelChangedEvent';
      nlcRoomId: string | null;
      level: NotificationLevel;
      effectiveLevel: NotificationLevel;
    }
  | {
      __typename: 'MentionNotificationEvent';
      roomId: string;
      room: { name: string };
      actor: { id: string; displayName: string } | null;
    }
  | {
      __typename: 'NewDirectMessageNotificationEvent';
      roomId: string;
      sender: { id: string; displayName: string; avatarUrl: string | null } | null;
      conversationName: string;
    }
  | {
      __typename: 'NotificationCreatedEvent';
      notificationId: string;
      roomId: string | null;
      eventId: string | null;
      inReplyToId: string | null;
    }
  | { __typename: 'NotificationDismissedEvent'; notificationId: string }
  | { __typename: 'RoomMarkedAsReadEvent'; roomId: string }
  | {
      __typename: 'ThreadFollowChangedEvent';
      tfcRoomId: string;
      tfcThreadRootEventId: string;
      isFollowing: boolean;
    }
  | { __typename: 'RoomGroupsUpdatedEvent'; changed: boolean }
  | { __typename: 'SessionTerminatedEvent'; reason: string }
  | { __typename: 'PresenceChangedEvent'; status: PresenceStatus }
  | { __typename: 'UserTypingEvent'; roomId: string; typingThreadRootEventId: string | null };

export type EventPayload = RoomEventPayload | LiveEventPayload;

export type EventEnvelope = Omit<RoomEventViewFragment, 'actor' | 'actorId' | 'event'> & {
  actorId: string | null;
  actor: EventUser | null;
  event: EventPayload | null;
};

export function streamEventToEventEnvelope(streamEvent: StreamEvent): EventEnvelope | null {
  if (streamEvent.payload.case === 'heartbeat') {
    return {
      __typename: 'Event',
      id: streamEvent.eventId || 'heartbeat',
      createdAt: nowIso(),
      actorId: null,
      actor: null,
      event: { __typename: 'HeartbeatEvent', alive: true }
    };
  }

  const durable = wireDurableEvent(streamEvent);
  if (durable) return durableEventToEnvelope(durable, streamEvent);

  const live = wireLiveEvent(streamEvent);
  if (live) return liveEventToEnvelope(live);

  return null;
}

function durableEventToEnvelope(durable: DurableEvent, streamEvent: StreamEvent): EventEnvelope {
  return {
    __typename: 'Event',
    id: durable.id,
    createdAt: timestampToIso(durable.createdAt),
    actorId: durable.actorId || null,
    actor: null,
    event: durablePayloadToEvent(durable, streamEvent)
  };
}

function durablePayloadToEvent(
  durable: DurableEvent,
  streamEvent: StreamEvent
): EventPayload | null {
  const payload = durable.event;
  if (!payload.case) return null;

  switch (payload.case) {
    case 'messagePosted': {
      const value = payload.value;
      return {
        __typename: 'MessagePostedEvent',
        roomId: value.roomId,
        body: null,
        attachments: [],
        linkPreview: null,
        reactions: [],
        updatedAt: null,
        inReplyTo: value.inReplyTo || null,
        threadRootEventId: value.inThread || null,
        echoOfEventId: value.echoOfEventId || null,
        echoFromThreadRootEventId: value.echoFromThreadRootEventId || null,
        channelEchoEventId: null,
        replyCount: 0,
        lastReplyAt: null,
        threadParticipants: [],
        viewerIsFollowingThread: null
      } as RoomEventPayload;
    }
    case 'messageEdited':
      return {
        __typename: 'MessageEditedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.eventId,
        body: null,
        attachments: [],
        linkPreview: null,
        updatedAt: null
      } as RoomEventPayload;
    case 'messageRetracted':
      return {
        __typename: 'MessageRetractedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.eventId,
        retractedReason: payload.value.reason || null
      } as RoomEventPayload;
    case 'roomCreated':
      return { __typename: 'RoomCreatedEvent', roomId: payload.value.roomId } as RoomEventPayload;
    case 'roomUpdated':
      return { __typename: 'RoomUpdatedEvent', roomId: payload.value.roomId } as RoomEventPayload;
    case 'roomDeleted':
      return { __typename: 'RoomDeletedEvent', roomId: payload.value.roomId } as RoomEventPayload;
    case 'roomArchived':
      return { __typename: 'RoomArchivedEvent', roomId: payload.value.roomId } as RoomEventPayload;
    case 'roomUnarchived':
      return {
        __typename: 'RoomUnarchivedEvent',
        roomId: payload.value.roomId
      } as RoomEventPayload;
    case 'userJoinedRoom':
      return {
        __typename: 'UserJoinedRoomEvent',
        roomId: payload.value.roomId
      } as RoomEventPayload;
    case 'userLeftRoom':
      return { __typename: 'UserLeftRoomEvent', roomId: payload.value.roomId } as RoomEventPayload;
    case 'reactionAdded':
      return {
        __typename: 'ReactionAddedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.messageEventId,
        emoji: payload.value.emoji
      } as RoomEventPayload;
    case 'reactionRemoved':
      return {
        __typename: 'ReactionRemovedEvent',
        roomId: payload.value.roomId,
        messageEventId: payload.value.messageEventId,
        emoji: payload.value.emoji
      } as RoomEventPayload;
    case 'assetProcessingStarted':
      return assetProcessingPayload(
        'AssetProcessingStartedEvent',
        payload.value,
        wireRoomTimelineId(streamEvent)
      );
    case 'assetProcessingSucceeded':
      return assetProcessingPayload(
        'AssetProcessingSucceededEvent',
        payload.value,
        wireRoomTimelineId(streamEvent)
      );
    case 'assetProcessingFailed':
      return assetProcessingPayload(
        'AssetProcessingFailedEvent',
        payload.value,
        wireRoomTimelineId(streamEvent)
      );
    case 'assetDeleted':
      return {
        __typename: 'AssetDeletedEvent',
        deletedRoomId: wireRoomTimelineId(streamEvent),
        assetId: payload.value.assetId
      } as RoomEventPayload;
    case 'serverMemberDeleted':
      return {
        __typename: 'ServerMemberDeletedEvent',
        userId: payload.value.userId
      } as RoomEventPayload;
    case 'voiceCallStarted':
      return {
        __typename: 'CallStartedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      } as RoomEventPayload;
    case 'voiceCallEnded':
      return {
        __typename: 'CallEndedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      } as RoomEventPayload;
    case 'voiceCallParticipantJoined':
      return {
        __typename: 'CallParticipantJoinedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      } as RoomEventPayload;
    case 'voiceCallParticipantLeft':
      return {
        __typename: 'CallParticipantLeftEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      } as RoomEventPayload;
    default:
      return null;
  }
}

function liveEventToEnvelope(live: LiveEvent): EventEnvelope {
  return {
    __typename: 'Event',
    id: live.id,
    createdAt: timestampToIso(live.createdAt),
    actorId: live.actorId || null,
    actor: null,
    event: livePayloadToEvent(live)
  };
}

function livePayloadToEvent(live: LiveEvent): EventPayload | null {
  const payload = live.event;
  if (!payload.case) return null;

  switch (payload.case) {
    case 'userProfileUpdated':
      return {
        __typename: 'UserProfileUpdatedEvent',
        userId: payload.value.userId,
        displayName: payload.value.displayName,
        avatarUrl: payload.value.avatarUrl || null,
        login: payload.value.login
      };
    case 'serverUserPreferencesUpdated':
      return {
        __typename: 'ServerUserPreferencesUpdatedEvent',
        timezone: payload.value.timezone || null,
        timeFormat: graphQLTimeFormat(payload.value.timeFormat)
      };
    case 'notificationLevelChanged':
      return {
        __typename: 'NotificationLevelChangedEvent',
        nlcRoomId: payload.value.roomId || null,
        level: graphQLNotificationLevel(payload.value.level),
        effectiveLevel: graphQLNotificationLevel(payload.value.effectiveLevel)
      };
    case 'threadFollowChanged':
      return {
        __typename: 'ThreadFollowChangedEvent',
        tfcRoomId: payload.value.roomId,
        tfcThreadRootEventId: payload.value.threadRootEventId,
        isFollowing: payload.value.isFollowing
      };
    case 'serverMemberDeleted':
      return {
        __typename: 'ServerMemberDeletedEvent',
        userId: payload.value.userId
      } as RoomEventPayload;
    case 'serverUpdated':
      return {
        __typename: 'ServerUpdatedEvent',
        name: payload.value.name,
        description: payload.value.description || null,
        logoUrl: payload.value.logoUrl || null,
        bannerUrl: payload.value.bannerUrl || null
      };
    case 'userTyping':
      return {
        __typename: 'UserTypingEvent',
        roomId: payload.value.roomId,
        typingThreadRootEventId: payload.value.threadRootEventId || null
      };
    case 'presenceChanged':
      return { __typename: 'PresenceChangedEvent', status: payload.value.status as PresenceStatus };
    case 'mentionNotification':
      return {
        __typename: 'MentionNotificationEvent',
        roomId: payload.value.roomId,
        room: { name: '' },
        actor: payload.value.mentionedByUserId
          ? { id: payload.value.mentionedByUserId, displayName: 'Unknown user' }
          : null
      };
    case 'newDirectMessageNotification':
      return {
        __typename: 'NewDirectMessageNotificationEvent',
        roomId: payload.value.roomId,
        sender: payload.value.senderId
          ? { id: payload.value.senderId, displayName: 'Unknown user', avatarUrl: null }
          : null,
        conversationName: ''
      };
    case 'callParticipantJoined':
      return {
        __typename: 'CallParticipantJoinedEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      } as RoomEventPayload;
    case 'callParticipantLeft':
      return {
        __typename: 'CallParticipantLeftEvent',
        roomId: payload.value.roomId,
        callId: payload.value.callId
      } as RoomEventPayload;
    case 'notificationCreated':
      return {
        __typename: 'NotificationCreatedEvent',
        notificationId: payload.value.notificationId,
        roomId: payload.value.roomId || null,
        eventId: payload.value.eventId || null,
        inReplyToId: payload.value.inReplyToId || null
      };
    case 'notificationDismissed':
      return {
        __typename: 'NotificationDismissedEvent',
        notificationId: payload.value.notificationId
      };
    case 'roomMarkedAsRead':
      return { __typename: 'RoomMarkedAsReadEvent', roomId: payload.value.roomId };
    case 'roomGroupsUpdated':
      return { __typename: 'RoomGroupsUpdatedEvent', changed: true };
    case 'sessionTerminated':
      return { __typename: 'SessionTerminatedEvent', reason: payload.value.reason };
    default:
      return null;
  }
}

function assetProcessingPayload(
  typename:
    | 'AssetProcessingStartedEvent'
    | 'AssetProcessingSucceededEvent'
    | 'AssetProcessingFailedEvent',
  value: { assetId: string; messageEventId: string },
  roomId: string | null
): RoomEventPayload {
  return {
    __typename: typename,
    processingRoomId: roomId,
    assetId: value.assetId,
    processingMessageEventId: value.messageEventId || null
  } as RoomEventPayload;
}

function graphQLNotificationLevel(level: WireNotificationLevel): NotificationLevel {
  switch (level) {
    case WireNotificationLevel.MUTED:
      return NotificationLevel.Muted;
    case WireNotificationLevel.ALL_MESSAGES:
      return NotificationLevel.AllMessages;
    case WireNotificationLevel.UNSPECIFIED:
      return NotificationLevel.Default;
    case WireNotificationLevel.NORMAL:
    default:
      return NotificationLevel.Normal;
  }
}

function graphQLTimeFormat(format: WireTimeFormat | undefined): TimeFormat {
  switch (format) {
    case WireTimeFormat.TIME_FORMAT_12H:
      return TimeFormat.TwelveHour;
    case WireTimeFormat.TIME_FORMAT_24H:
      return TimeFormat.TwentyFourHour;
    case WireTimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}

function timestampToIso(timestamp: { toDate(): Date } | undefined): string {
  return timestamp?.toDate().toISOString() ?? nowIso();
}

function nowIso(): string {
  return new SvelteDate().toISOString();
}

export type EventHandler = (event: EventEnvelope) => void;
export type EventBusCatchUpReason = 'subscription-ended' | 'ws-reconnected' | 'heartbeat-stalled';
export type EventBusCatchUpHandler = (reason: EventBusCatchUpReason) => void;

export interface EventBus {
  handlers: SvelteSet<EventHandler>;
  catchUpHandlers: SvelteSet<EventBusCatchUpHandler>;
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

function onTypedEvent<T>(
  typename: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  extract: (envelope: EventEnvelope, event: any) => T,
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
    if (envelope.event?.__typename === typename) {
      handler(extract(envelope, envelope.event));
    }
  };

  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

function onTypedEventDirect<T>(
  bus: EventBus,
  typename: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  extract: (envelope: EventEnvelope, event: any) => T,
  handler: (data: T) => void
): () => void {
  const wrapper: EventHandler = (envelope) => {
    if (envelope.event?.__typename === typename) {
      handler(extract(envelope, envelope.event));
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
    'UserProfileUpdatedEvent',
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

export type MentionNotification = {
  roomId: string;
  actorUserId: string;
  actorDisplayName: string;
  spaceName: string;
  roomName: string;
};

export function onMention(handler: (notification: MentionNotification) => void): () => void {
  return onTypedEvent(
    'MentionNotificationEvent',
    (env, e) => {
      const actor = e.actor ?? env.actor;

      return {
        roomId: e.roomId,
        actorUserId: actor?.id ?? env.actorId ?? '',
        actorDisplayName: actor?.displayName ?? 'Unknown user',
        spaceName: '',
        roomName: e.room?.name ?? ''
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
    'NewDirectMessageNotificationEvent',
    (env, e) => {
      const sender = e.sender ?? env.actor;

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
    'NotificationCreatedEvent',
    (_env, e) => {
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
    'NotificationDismissedEvent',
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
    'RoomMarkedAsReadEvent',
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
    'ServerUserPreferencesUpdatedEvent',
    (_env, e) => {
      return { timezone: e.timezone, timeFormat: e.timeFormat };
    },
    handler
  );
}

export type RoomLayoutUpdatedInfo = Record<string, never>;

export function onRoomLayoutUpdated(handler: (_info: RoomLayoutUpdatedInfo) => void): () => void {
  return onTypedEvent('RoomGroupsUpdatedEvent', () => ({}), handler);
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
    'NotificationLevelChangedEvent',
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
    'ThreadFollowChangedEvent',
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
    'SessionTerminatedEvent',
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
    'PresenceChangedEvent',
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
    if (event.event?.__typename !== 'UserTypingEvent') return;
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
        'RoomMarkedAsReadEvent',
        (_env, e) => {
          return { roomId: e.roomId };
        },
        handler
      );
    },
    onNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void): () => void {
      return onTypedEventDirect(
        bus,
        'NotificationLevelChangedEvent',
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
      return onTypedEventDirect(bus, 'RoomGroupsUpdatedEvent', () => ({}), handler);
    }
  };
}
