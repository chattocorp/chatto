/**
 * Single GraphQL subscription per connected server, covering everything
 * the user can receive (deployment-wide events and room-scoped events
 * over one stream).
 *
 * The manager keeps one bus per registered server. Consumers register
 * handlers either via Svelte context (current active server) or directly
 * against a specific server's bus through the manager (used by the
 * cross-server sidebar wiring).
 */

import { createContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import { graphql, useFragment } from './gql';
import {
  RoomEventViewFragmentDoc,
  UserAvatarUserFragmentDoc,
  type MyServerEventsSubscription,
  type NotificationLevel,
  type PresenceStatus,
  type TimeFormat
} from './gql/graphql';
import { eventBusManager } from './state/server/eventBus.svelte';

export const MyServerEventsSubscriptionDoc = graphql(`
  subscription MyServerEvents($after: String) {
    myEvents(after: $after) {
      id
      createdAt
      actorId
      deliveryCursor
      actor {
        ...UserAvatarUser
      }
      event {
        __typename
        # Room payloads — full RoomEventView coverage for the chat surface.
        ... on MessagePostedEvent {
          roomId
          body
          attachments {
            ...MessageAttachmentView
          }
          linkPreview {
            ...LinkPreviewView
          }
          reactions {
            emoji
            count
            hasReacted
            users(first: 5) {
              id
              displayName
            }
          }
          updatedAt
          inReplyTo
          threadRootEventId
          echoOfEventId
          echoFromThreadRootEventId
          channelEchoEventId
          replyCount
          lastReplyAt
          threadParticipants(first: 5) {
            ...UserAvatarUser
          }
          viewerIsFollowingThread
        }
        ... on MessageEditedEvent {
          roomId
          messageEventId
          body
          attachments {
            ...MessageAttachmentView
          }
          linkPreview {
            ...LinkPreviewView
          }
          updatedAt
        }
        ... on MessageRetractedEvent {
          roomId
          messageEventId
          retractedReason: reason
        }
        ... on UserJoinedRoomEvent {
          roomId
        }
        ... on UserLeftRoomEvent {
          roomId
        }
        ... on RoomCreatedEvent {
          roomId
        }
        ... on RoomUpdatedEvent {
          roomId
        }
        ... on RoomDeletedEvent {
          roomId
        }
        ... on RoomArchivedEvent {
          roomId
        }
        ... on RoomUnarchivedEvent {
          roomId
        }
        ... on ReactionAddedEvent {
          roomId
          messageEventId
          emoji
        }
        ... on ReactionRemovedEvent {
          roomId
          messageEventId
          emoji
        }
        ... on PresenceChangedEvent {
          status
        }
        ... on UserTypingEvent {
          roomId
          typingThreadRootEventId: threadRootEventId
        }
        ... on AssetProcessingStartedEvent {
          processingRoomId: roomId
          assetId
          processingMessageEventId: messageEventId
        }
        ... on AssetProcessingSucceededEvent {
          processingRoomId: roomId
          assetId
          processingMessageEventId: messageEventId
        }
        ... on AssetProcessingFailedEvent {
          processingRoomId: roomId
          assetId
          processingMessageEventId: messageEventId
        }
        ... on AssetDeletedEvent {
          deletedRoomId: roomId
          assetId
        }
        ... on ServerMemberDeletedEvent {
          userId
        }
        ... on CallParticipantJoinedEvent {
          roomId
        }
        ... on CallParticipantLeftEvent {
          roomId
        }
        # Deployment-wide events.
        ... on ServerUpdatedEvent {
          name
          description
          logoUrl
          bannerUrl
        }
        ... on UserProfileUpdatedEvent {
          userId
          displayName
          avatarUrl
          login
        }
        ... on ServerUserPreferencesUpdatedEvent {
          timezone
          timeFormat
        }
        ... on NotificationLevelChangedEvent {
          nlcRoomId: roomId
          level
          effectiveLevel
        }
        ... on MentionNotificationEvent {
          roomId
          room {
            name
          }
          actor {
            id
            displayName
          }
        }
        ... on NewDirectMessageNotificationEvent {
          roomId
          sender {
            id
            displayName
            avatarUrl
          }
          conversationName
        }
        ... on NotificationCreatedEvent {
          notificationId
          roomId
          eventId
          inReplyToId
        }
        ... on NotificationDismissedEvent {
          notificationId
        }
        ... on RoomMarkedAsReadEvent {
          roomId
        }
        ... on ThreadFollowChangedEvent {
          tfcRoomId: roomId
          tfcThreadRootEventId: threadRootEventId
          isFollowing
        }
        ... on RoomGroupsUpdatedEvent {
          changed
        }
        ... on SessionTerminatedEvent {
          reason
        }
        ... on HeartbeatEvent {
          alive
        }
      }
    }
  }
`);

/** Re-export the urql RoomEventView fragment doc so the chat-event handler can
 *  mask subscription payloads when forwarding to room-history stores. */
export { RoomEventViewFragmentDoc, useFragment };

export type EventEnvelope = MyServerEventsSubscription['myEvents'];

export type EventHandler = (event: EventEnvelope) => void;

export interface EventBus {
  handlers: SvelteSet<EventHandler>;
}

// The context holds a getter — not a fixed bus — so reads from inside a
// consumer's $effect track whatever reactive state the getter touches
// (typically `page.params.serverId` via `getActiveServer`). When the URL
// `[serverId]` param changes, every `useEvent` / `onEvent` consumer
// re-subscribes against the new server's bus without needing a remount or
// a context refresh.
const [getServerBusGetter, setServerBusGetter] =
  createContext<() => EventBus | undefined>();

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
  return onTypedEvent('UserProfileUpdatedEvent', (_env, e) => {
    return { userId: e.userId, displayName: e.displayName, avatarUrl: e.avatarUrl, login: e.login };
  }, handler);
}

export type MentionNotification = {
  roomId: string;
  actorUserId: string;
  actorDisplayName: string;
  spaceName: string;
  roomName: string;
};

export function onMention(handler: (notification: MentionNotification) => void): () => void {
  return onTypedEvent('MentionNotificationEvent', (env, e) => {
    const envelopeActor = env.actor ? useFragment(UserAvatarUserFragmentDoc, env.actor) : null;
    const actor = e.actor ?? envelopeActor;

    return {
      roomId: e.roomId,
      actorUserId: actor?.id ?? env.actorId ?? '',
      actorDisplayName: actor?.displayName ?? 'Unknown user',
      spaceName: '',
      roomName: e.room.name
    };
  }, handler);
}

export type DMNotification = {
  roomId: string;
  senderId: string;
  senderDisplayName: string;
  senderAvatarUrl: string;
  conversationName: string;
};

export function onNewDM(handler: (notification: DMNotification) => void): () => void {
  return onTypedEvent('NewDirectMessageNotificationEvent', (env, e) => {
    const envelopeActor = env.actor ? useFragment(UserAvatarUserFragmentDoc, env.actor) : null;
    const sender = e.sender ?? envelopeActor;

    return {
      roomId: e.roomId,
      senderId: sender?.id ?? env.actorId ?? '',
      senderDisplayName: sender?.displayName ?? 'Unknown user',
      senderAvatarUrl: sender?.avatarUrl ?? '',
      conversationName: e.conversationName
    };
  }, handler);
}

export type NotificationCreatedInfo = {
  notificationId: string;
  spaceId?: string;
  roomId?: string;
  eventId?: string;
  inReplyToId?: string;
};

export function onNotificationCreated(handler: (info: NotificationCreatedInfo) => void): () => void {
  return onTypedEvent('NotificationCreatedEvent', (_env, e) => {
    return {
      notificationId: e.notificationId,
      roomId: e.roomId ?? undefined,
      eventId: e.eventId ?? undefined,
      inReplyToId: e.inReplyToId ?? undefined
    };
  }, handler);
}

export type NotificationDismissedInfo = {
  notificationId: string;
};

export function onNotificationDismissed(handler: (info: NotificationDismissedInfo) => void): () => void {
  return onTypedEvent('NotificationDismissedEvent', (_env, e) => {
    return { notificationId: e.notificationId };
  }, handler);
}

export type RoomMarkedAsReadInfo = {
  roomId: string;
};

export function onRoomMarkedAsRead(handler: (info: RoomMarkedAsReadInfo) => void): () => void {
  return onTypedEvent('RoomMarkedAsReadEvent', (_env, e) => {
    return { roomId: e.roomId };
  }, handler);
}

export type UserSettingsUpdate = {
  timezone: string | null;
  timeFormat: TimeFormat;
};

export function onUserSettingsUpdate(handler: (update: UserSettingsUpdate) => void): () => void {
  return onTypedEvent('ServerUserPreferencesUpdatedEvent', (_env, e) => {
    return { timezone: e.timezone, timeFormat: e.timeFormat };
  }, handler);
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

export function onNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void): () => void {
  return onTypedEvent('NotificationLevelChangedEvent', (_env, e) => {
    return {
      roomId: e.nlcRoomId ?? null,
      level: e.level,
      effectiveLevel: e.effectiveLevel
    };
  }, handler);
}

export type ThreadFollowChanged = {
  roomId: string;
  threadRootEventId: string;
  isFollowing: boolean;
};

export function onThreadFollowChanged(handler: (update: ThreadFollowChanged) => void): () => void {
  return onTypedEvent('ThreadFollowChangedEvent', (_env, e) => {
    return {
      roomId: e.tfcRoomId,
      threadRootEventId: e.tfcThreadRootEventId,
      isFollowing: e.isFollowing
    };
  }, handler);
}

export function onSessionTerminated(handler: (reason: string) => void): () => void {
  return onTypedEvent('SessionTerminatedEvent', (_env, e) => {
    return e.reason;
  }, handler);
}

// ---------------------------------------------------------------------------
// Room-scoped helpers
// ---------------------------------------------------------------------------

type PresenceHandler = (userId: string, status: PresenceStatus) => void;

export function onPresenceChange(handler: PresenceHandler): () => void {
  return onTypedEvent('PresenceChangedEvent', (envelope, e) => {
    return { userId: envelope.actorId, status: e.status as PresenceStatus };
  }, ({ userId, status }) => {
    if (!userId) return;
    handler(userId, status);
  });
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
      return onTypedEventDirect(bus, 'RoomMarkedAsReadEvent', (_env, e) => {
        return { roomId: e.roomId };
      }, handler);
    },
    onNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void): () => void {
      return onTypedEventDirect(bus, 'NotificationLevelChangedEvent', (_env, e) => {
        return {
          roomId: e.nlcRoomId ?? null,
          level: e.level,
          effectiveLevel: e.effectiveLevel
        };
      }, handler);
    },
    onRoomLayoutUpdated(handler: (info: RoomLayoutUpdatedInfo) => void): () => void {
      return onTypedEventDirect(bus, 'RoomGroupsUpdatedEvent', () => ({}), handler);
    }
  };
}
