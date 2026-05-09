import { type Client } from '@urql/svelte';
import { createContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import { graphql } from './gql';
import type { MyInstanceEventsSubscription, NotificationLevel, TimeFormat } from './gql/graphql';
import { instanceEventBusManager } from './state/instance/eventBus.svelte';

export const MyInstanceEventsSubscriptionDoc = graphql(`
  subscription MyInstanceEvents {
    myInstanceEvents {
      actorId
      event {
        __typename
        ... on InstanceConfigUpdatedEvent {
          instanceName
          motd
          welcomeMessage
        }
        ... on SpaceCreatedEvent {
          spaceId
        }
        ... on SpaceUpdatedEvent {
          spaceId
          name
          description
          logoUrl
          bannerUrl
        }
        ... on SpaceDeletedEvent {
          spaceId
        }
        ... on UserJoinedSpaceEvent {
          spaceId
        }
        ... on UserLeftSpaceEvent {
          spaceId
        }
        ... on UserProfileUpdatedEvent {
          userId
          displayName
          avatarUrl
          login
        }
        ... on InstanceUserPreferencesUpdatedEvent {
          timezone
          timeFormat
        }
        ... on NotificationLevelChangedEvent {
          nlcSpaceId: spaceId
          nlcRoomId: roomId
          level
          effectiveLevel
        }
        ... on MentionNotificationEvent {
          spaceId
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
          spaceId
          roomId
          eventId
          inReplyToId
        }
        ... on NotificationDismissedEvent {
          notificationId
        }
        ... on NewMessageInSpaceEvent {
          spaceId
          roomId
        }
        ... on RoomMarkedAsReadEvent {
          spaceId
          roomId
        }
        ... on ThreadFollowChangedEvent {
          tfcSpaceId: spaceId
          tfcRoomId: roomId
          threadRootEventId
          isFollowing
        }
        ... on RoomLayoutUpdatedEvent {
          rluSpaceId: spaceId
        }
        ... on SessionTerminatedEvent {
          reason
        }
      }
    }
  }
`);

export type InstanceEvent = MyInstanceEventsSubscription['myInstanceEvents'];

export type EventHandler = (event: InstanceEvent) => void;

export interface InstanceEventBus {
  handlers: SvelteSet<EventHandler>;
}

const [getInstanceBusCtx, setInstanceBusCtx] = createContext<InstanceEventBus>();

/**
 * Initialize the instance event bus for the given instance.
 * Creates/starts a bus via the manager and sets the Svelte context so that
 * child components can subscribe via the `on*` hooks.
 *
 * @param client - The urql client for this instance
 * @param instanceId - The instance ID (defaults to 'home' for backward compatibility)
 * @returns Cleanup function that stops the subscription
 */
export function initInstanceEventBus(client: Client, instanceId: string = 'home') {
  const cleanup = instanceEventBusManager.startBus(instanceId, client);

  const bus = instanceEventBusManager.getBus(instanceId)!;
  setInstanceBusCtx(bus);

  return () => {
    cleanup();
  };
}

/**
 * Provide an already-started instance event bus to child components via Svelte context.
 * Use this in layouts that need to expose a specific instance's bus without starting it.
 * The bus must already be running via instanceEventBusManager.startBus().
 *
 * @param instanceId - The instance whose bus to provide
 * @returns true if the bus was found and context was set, false otherwise
 */
export function provideInstanceEventBus(instanceId: string): boolean {
  const bus = instanceEventBusManager.getBus(instanceId);
  if (!bus) return false;
  setInstanceBusCtx(bus);
  return true;
}

/**
 * Register an instance event handler. Must be called during component initialization.
 * Returns a cleanup function - use with $effect for automatic cleanup.
 */
export function onInstanceEvent(handler: EventHandler): () => void {
  const bus = getInstanceBusCtx();

  bus.handlers.add(handler);

  return () => {
    bus.handlers.delete(handler);
  };
}

// ---------------------------------------------------------------------------
// Typed event handler helpers
// ---------------------------------------------------------------------------

/**
 * Create a typed event handler that filters by __typename and extracts fields.
 * Registers on the bus from Svelte context. Returns no-op if bus not initialized.
 */
function onTypedEvent<T>(
  typename: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  extract: (event: any) => T,
  handler: (data: T) => void
): () => void {
  let bus: InstanceEventBus;
  try {
    bus = getInstanceBusCtx();
  } catch {
    return () => {};
  }

  const wrapper: EventHandler = (event) => {
    if (event.event?.__typename === typename) {
      handler(extract(event.event));
    }
  };

  bus.handlers.add(wrapper);
  return () => {
    bus.handlers.delete(wrapper);
  };
}

/**
 * Like onTypedEvent but registers directly on a bus (bypassing Svelte context).
 */
function onTypedEventDirect<T>(
  bus: InstanceEventBus,
  typename: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  extract: (event: any) => T,
  handler: (data: T) => void
): () => void {
  const wrapper: EventHandler = (event) => {
    if (event.event?.__typename === typename) {
      handler(extract(event.event));
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
  avatarUrl: string;
  login: string;
};

export function onUserProfileUpdate(handler: (update: UserProfileUpdate) => void): () => void {
  return onTypedEvent('UserProfileUpdatedEvent', (e) => {
    return { userId: e.userId, displayName: e.displayName, avatarUrl: e.avatarUrl, login: e.login };
  }, handler);
}

export type MentionNotification = {
  spaceId: string;
  roomId: string;
  actorUserId: string;
  actorDisplayName: string;
  spaceName: string;
  roomName: string;
};

export function onMention(handler: (notification: MentionNotification) => void): () => void {
  return onTypedEvent('MentionNotificationEvent', (e) => {
    return {
      spaceId: e.spaceId, roomId: e.roomId,
      actorUserId: e.actor.id, actorDisplayName: e.actor.displayName,
      spaceName: '', roomName: e.room.name
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
  return onTypedEvent('NewDirectMessageNotificationEvent', (e) => {
    return {
      roomId: e.roomId, senderId: e.sender.id,
      senderDisplayName: e.sender.displayName,
      senderAvatarUrl: e.sender.avatarUrl ?? '',
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
  return onTypedEvent('NotificationCreatedEvent', (e) => {
    return {
      notificationId: e.notificationId,
      spaceId: e.spaceId ?? undefined, roomId: e.roomId ?? undefined,
      eventId: e.eventId ?? undefined, inReplyToId: e.inReplyToId ?? undefined
    };
  }, handler);
}

export type NotificationDismissedInfo = {
  notificationId: string;
};

export function onNotificationDismissed(handler: (info: NotificationDismissedInfo) => void): () => void {
  return onTypedEvent('NotificationDismissedEvent', (e) => {
    return { notificationId: e.notificationId };
  }, handler);
}

export type RoomMarkedAsReadInfo = {
  spaceId: string;
  roomId: string;
};

export function onRoomMarkedAsRead(handler: (info: RoomMarkedAsReadInfo) => void): () => void {
  return onTypedEvent('RoomMarkedAsReadEvent', (e) => {
    return { spaceId: e.spaceId, roomId: e.roomId };
  }, handler);
}

export type UserSettingsUpdate = {
  timezone: string;
  timeFormat: TimeFormat;
};

export function onUserSettingsUpdate(handler: (update: UserSettingsUpdate) => void): () => void {
  return onTypedEvent('InstanceUserPreferencesUpdatedEvent', (e) => {
    return { timezone: e.timezone, timeFormat: e.timeFormat };
  }, handler);
}

export type RoomLayoutUpdatedInfo = {
  spaceId: string;
};

export function onRoomLayoutUpdated(handler: (info: RoomLayoutUpdatedInfo) => void): () => void {
  return onTypedEvent('RoomLayoutUpdatedEvent', (e) => {
    return { spaceId: e.rluSpaceId };
  }, handler);
}

export type NotificationLevelChanged = {
  spaceId: string;
  roomId: string | null;
  level: NotificationLevel;
  effectiveLevel: NotificationLevel;
};

export function onNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void): () => void {
  return onTypedEvent('NotificationLevelChangedEvent', (e) => {
    return {
      spaceId: e.nlcSpaceId, roomId: e.nlcRoomId ?? null,
      level: e.level, effectiveLevel: e.effectiveLevel
    };
  }, handler);
}

export type ThreadFollowChanged = {
  spaceId: string;
  roomId: string;
  threadRootEventId: string;
  isFollowing: boolean;
};

export function onThreadFollowChanged(handler: (update: ThreadFollowChanged) => void): () => void {
  return onTypedEvent('ThreadFollowChangedEvent', (e) => {
    return {
      spaceId: e.tfcSpaceId, roomId: e.tfcRoomId,
      threadRootEventId: e.threadRootEventId, isFollowing: e.isFollowing
    };
  }, handler);
}

export function onSessionTerminated(handler: (reason: string) => void): () => void {
  return onTypedEvent('SessionTerminatedEvent', (e) => {
    return e.reason;
  }, handler);
}

// ---------------------------------------------------------------------------
// Direct bus handler registrar (bypasses Svelte context)
// ---------------------------------------------------------------------------

/**
 * Create a handler registrar for a specific instance's event bus.
 * This bypasses Svelte context and registers directly on the manager's bus,
 * allowing sidebar components to subscribe to any instance's events.
 *
 * @param instanceId - The instance to register handlers on
 * @returns Object with registration methods, or undefined if the bus isn't started
 */
export function createInstanceEventBusHandlerRegistrar(instanceId: string) {
  const bus = instanceEventBusManager.getBus(instanceId);
  if (!bus) return undefined;

  return {
    onInstanceEvent(handler: EventHandler): () => void {
      bus.handlers.add(handler);
      return () => {
        bus.handlers.delete(handler);
      };
    },
    onRoomMarkedAsRead(handler: (info: RoomMarkedAsReadInfo) => void): () => void {
      return onTypedEventDirect(bus, 'RoomMarkedAsReadEvent', (e) => {
        return { spaceId: e.spaceId, roomId: e.roomId };
      }, handler);
    },
    onNotificationLevelChanged(handler: (update: NotificationLevelChanged) => void): () => void {
      return onTypedEventDirect(bus, 'NotificationLevelChangedEvent', (e) => {
        return {
          spaceId: e.nlcSpaceId, roomId: e.nlcRoomId ?? null,
          level: e.level, effectiveLevel: e.effectiveLevel
        };
      }, handler);
    },
    onRoomLayoutUpdated(handler: (info: RoomLayoutUpdatedInfo) => void): () => void {
      return onTypedEventDirect(bus, 'RoomLayoutUpdatedEvent', (e) => {
        return { spaceId: e.rluSpaceId };
      }, handler);
    }
  };
}
