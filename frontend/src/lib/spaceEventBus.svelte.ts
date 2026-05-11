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

// The backend emits a HeartbeatEvent every 25s on this subscription. If we
// go this long without seeing *any* event while the tab is visible, we
// consider the subscription dead and re-subscribe.
const STALE_THRESHOLD_MS = 60_000;
// How often the watchdog checks for staleness.
const WATCHDOG_INTERVAL_MS = 15_000;
// When the tab becomes visible after being hidden, re-subscribe if the
// last event is older than this. Catches laptop-wake-from-sleep cases.
const VISIBILITY_RESUBSCRIBE_AFTER_MS = 30_000;

/**
 * Start the deployment-wide event subscription. Call from within an $effect
 * for automatic cleanup. There is one subscription per connection — channel
 * and DM events flow through it together.
 *
 * A watchdog re-subscribes if no event (real or heartbeat) is received for
 * STALE_THRESHOLD_MS while the tab is visible. This catches the case where
 * the WebSocket is healthy but the server-side subscription goroutine has
 * silently died — without this, mutations succeed but new events never
 * arrive until the user hard-reloads.
 */
export function startServerSubscription(bus: SpaceEventBus, client: Client) {
  _subscriptionActive = true;
  let lastEventAt = Date.now();
  let sub: { unsubscribe: () => void } | null = null;

  const subscribe = () => {
    sub = client.subscription(MyServerEventsSubscriptionDoc, {}).subscribe((result) => {
      lastEventAt = Date.now();

      if (result.error) {
        console.error('ServerEventBus: Subscription error:', result.error);
      }
      if (!result.data) return;
      const event = useFragment(RoomEventViewFragmentDoc, result.data.myServerEvents);
      if (!event) return;
      // Heartbeats are pure liveness signals — already accounted for via
      // lastEventAt above. Don't dispatch to handlers.
      if (event.event?.__typename === 'HeartbeatEvent') return;

      bus.handlers.forEach((handler) => {
        try {
          handler(event);
        } catch (err) {
          console.error('ServerEventBus: Handler error:', err);
        }
      });
    });
  };

  const resubscribe = (reason: string) => {
    console.warn(`ServerEventBus: re-subscribing (${reason})`);
    sub?.unsubscribe();
    lastEventAt = Date.now();
    subscribe();
  };

  subscribe();

  const watchdog = setInterval(() => {
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
    if (Date.now() - lastEventAt < STALE_THRESHOLD_MS) return;
    resubscribe(`no event for ${STALE_THRESHOLD_MS}ms`);
  }, WATCHDOG_INTERVAL_MS);

  const onVisibility = () => {
    if (document.visibilityState !== 'visible') return;
    if (Date.now() - lastEventAt > VISIBILITY_RESUBSCRIBE_AFTER_MS) {
      resubscribe('tab became visible after gap');
    }
  };
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibility);
  }

  return () => {
    _subscriptionActive = false;
    clearInterval(watchdog);
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibility);
    }
    sub?.unsubscribe();
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
