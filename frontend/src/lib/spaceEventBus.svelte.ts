import { type Client } from '@urql/svelte';
import { createContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import { graphql, useFragment } from './gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment,
  type PresenceStatus
} from './gql/graphql';

const MyServerEventsSubscriptionDoc = graphql(`
  subscription ServerEventBusSubscription {
    myServerEvents {
      ...RoomEventView
    }
  }
`);

export type SpaceEvent = RoomEventViewFragment;

type EventHandler = (event: SpaceEvent) => void;

interface SpaceEventBus {
  handlers: SvelteSet<EventHandler>;
}

const [getSpaceBusCtx, setSpaceBusCtx] = createContext<SpaceEventBus>();

/**
 * Plain boolean tracking whether startServerSubscription has been called.
 * NOT reactive ($state would cause the subscription $effect to re-run when
 * the template reads it). Tests poll for this via a data attribute.
 */
let _subscriptionActive = false;

/** Check if the space event subscription is active (for SpaceEventProvider template). */
export function isSubscriptionActive(): boolean {
  return _subscriptionActive;
}

/**
 * Create the space event bus context. Must be called synchronously during
 * component initialization (not in an effect).
 */
export function createSpaceEventBus(): SpaceEventBus {
  const bus: SpaceEventBus = {
    handlers: new SvelteSet()
  };
  setSpaceBusCtx(bus);
  return bus;
}

/**
 * Start the deployment-wide event subscription. Call from within an $effect
 * for automatic cleanup. There is one subscription per connection — channel
 * and DM events flow through it together.
 */
export function startServerSubscription(bus: SpaceEventBus, client: Client) {
  _subscriptionActive = true;

  const sub = client.subscription(MyServerEventsSubscriptionDoc, {}).subscribe((result) => {
    if (result.error) {
      console.error('ServerEventBus: Subscription error:', result.error);
    }
    if (!result.data) return;
    const event = useFragment(RoomEventViewFragmentDoc, result.data.myServerEvents);
    if (event) {
      bus.handlers.forEach((handler) => {
        try {
          handler(event);
        } catch (err) {
          console.error('ServerEventBus: Handler error:', err);
        }
      });
    }
  });

  return () => {
    _subscriptionActive = false;
    sub.unsubscribe();
  };
}

/**
 * Register a space event handler. Must be called during component initialization.
 * Returns a cleanup function - use with $effect for automatic cleanup.
 */
export function onSpaceEvent(handler: EventHandler): () => void {
  const bus = getSpaceBusCtx();

  bus.handlers.add(handler);

  return () => {
    bus.handlers.delete(handler);
  };
}

// ---------------------------------------------------------------------------
// Typed event handler helper
// ---------------------------------------------------------------------------

/**
 * Create a typed space event handler that filters by __typename and extracts fields.
 * Returns no-op if bus not initialized (allows graceful fallback outside SpaceEventProvider).
 */
function onSpaceTypedEvent<T>(
  typename: string,
  extract: (event: SpaceEvent) => T,
  handler: (data: T) => void
): () => void {
  let bus: SpaceEventBus;
  try {
    bus = getSpaceBusCtx();
  } catch {
    return () => {};
  }

  const wrapper: EventHandler = (event) => {
    if (event.event?.__typename === typename) {
      handler(extract(event));
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

type PresenceHandler = (userId: string, status: PresenceStatus) => void;

/**
 * Register a handler for presence change events. Must be called during component initialization.
 * Returns a cleanup function - use with $effect for automatic cleanup.
 *
 * If the space event bus is not initialized (e.g., component is used outside
 * of a SpaceEventProvider), this returns a no-op cleanup function and the handler
 * will not receive updates. This allows UserAvatar to be used anywhere in the app.
 */
export function onPresenceChange(handler: PresenceHandler): () => void {
  return onSpaceTypedEvent('PresenceChangedEvent', (e) => {
    const ev = e.event as { status: PresenceStatus };
    return { userId: e.actorId, status: ev.status };
  }, ({ userId, status }) => handler(userId, status));
}

/**
 * Data received when a user is typing.
 */
export interface TypingEventData {
  userId: string;
  roomId: string;
  threadRootEventId: string | null;
}

type TypingHandler = (data: TypingEventData) => void;

/**
 * Register a handler for typing indicator events. Must be called during component initialization.
 * Returns a cleanup function - use with $effect for automatic cleanup.
 *
 * If the space event bus is not initialized, this returns a no-op cleanup function.
 */
export function onTypingEvent(handler: TypingHandler): () => void {
  return onSpaceTypedEvent('UserTypingEvent', (e) => {
    const ev = e.event as { roomId: string; typingThreadRootEventId?: string | null };
    return {
      userId: e.actorId,
      roomId: ev.roomId,
      threadRootEventId: ev.typingThreadRootEventId ?? null
    };
  }, handler);
}
