/**
 * Manages per-server event bus subscriptions. One client-live WebSocket stream
 * per registered server — the bus holds the handler set, the manager stores the
 * subscription handle for teardown.
 *
 * The sidebar wires handlers against any connected server's bus through
 * the manager (not just the one in URL focus), which is how cross-server
 * notification indicators work without each server holding its own subscription
 * context.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { EventBusCatchUpReason, EventHandler, EventBus } from '$lib/eventBus.svelte';
import type { GraphQLClient } from './graphqlClient.svelte';
import { serverRegistry } from './registry.svelte';
import { startClientLiveSubscription } from './clientLive';

const HEARTBEAT_STALL_MS = 75_000;
const HEARTBEAT_WATCHDOG_MS = 15_000;
const CATCH_UP_RETRY_MS = 2_500;
const RESUBSCRIBE_BASE_MS = 1_000;
const RESUBSCRIBE_MAX_MS = 30_000;

class EventBusManager {
  // SvelteMap so getBus() is a reactive read — consumers like NotificationSync
  // re-run their $effect when a bus is started/stopped, which avoids a race
  // where the consumer mounts before startBus and never re-attaches.
  #buses = new SvelteMap<string, EventBus>();
  #subscriptions = new Map<string, { unsubscribe: () => void }>();
  #cleanups = new Map<string, () => void>();

  /**
   * Start an event bus for the given server. Creates the subscription and
   * stores the bus. If a bus already exists for this server, returns a
   * cleanup function without creating a duplicate.
   *
   * The bus stays intentionally small: it re-subscribes when the current live
   * WebSocket ends or when the server heartbeat goes silent while the tab is
   * visible. Consumers that own projected state can register `catchUpHandlers`
   * to refetch after those gaps instead of relying on subscription replay.
   *
   * @returns Cleanup function that stops the bus.
   */
  startBus(serverId: string, _gqlClient: GraphQLClient): () => void {
    if (this.#buses.has(serverId)) {
      // Already running — return a no-op cleanup (the real cleanup is from
      // the original startBus call)
      return () => {};
    }

    const handlers = new SvelteSet<EventHandler>();
    const catchUpHandlers = new SvelteSet<(reason: EventBusCatchUpReason) => void>();
    const bus: EventBus = { handlers, catchUpHandlers };
    let lastEventAt = Date.now();
    let heartbeatCount = 0;
    let dispatchedEventCount = 0;
    let resubscribeCount = 0;
    let subscriptionGeneration = 0;
    let catchUpRetryTimer: ReturnType<typeof setTimeout> | null = null;
    let resubscribeTimer: ReturnType<typeof setTimeout> | null = null;
    let reconnectAttempt = 0;
    let liveStreamActive = false;
    // Set while we're tearing down a subscription (either to replace it
    // or because the bus is stopping). Prevents `onEnd` from firing a
    // reentrant resubscribe in response to our own unsubscribe.
    let teardownInProgress = false;
    let stopped = false;

    const debugState = () => ({
      generation: subscriptionGeneration,
      handlers: handlers.size,
      events: dispatchedEventCount,
      heartbeats: heartbeatCount,
      resubscribes: resubscribeCount,
      lastEventAgeMs: Date.now() - lastEventAt
    });

    const dispatchEvent = (event: Parameters<EventHandler>[0], generation: number) => {
      lastEventAt = Date.now();
      if (event.event?.__typename === 'HeartbeatEvent') {
        heartbeatCount++;
        reconnectAttempt = 0;
        console.debug(`[eventBus:${serverId}] heartbeat received (total: ${heartbeatCount})`);
        return;
      }
      dispatchedEventCount++;
      reconnectAttempt = 0;
      console.debug(
        `[eventBus:${serverId}] event dispatched`,
        event.event?.__typename ?? '<unknown>',
        {
          generation,
          eventId: event.id,
          total: dispatchedEventCount
        }
      );
      // Run handlers in isolation: a throw from one handler must not
      // stop the others or tear down the subscription itself.
      for (const handler of handlers) {
        try {
          handler(event);
        } catch (err) {
          console.error(`[eventBus:${serverId}] handler threw`, err);
        }
      }
    };

    const subscribeOnce = (reason: string) => {
      subscriptionGeneration++;
      const generation = subscriptionGeneration;
      console.debug(`[eventBus:${serverId}] subscribing`, {
        reason,
        ...debugState()
      });
      const server = serverRegistry.getServer(serverId);
      if (!server?.live) {
        console.warn(`[eventBus:${serverId}] server does not advertise Chatto live metadata`);
        liveStreamActive = false;
        bus.request = undefined;
        return { unsubscribe: () => {} };
      }
      if (!server.token && !isSameOriginServerURL(server.url)) {
        console.warn(
          `[eventBus:${serverId}] Chatto live stream requires an authenticated server token`
        );
        liveStreamActive = false;
        bus.request = undefined;
        return { unsubscribe: () => {} };
      }

      console.debug(`[eventBus:${serverId}] using Chatto live stream`, {
        reason,
        url: server.live.url
      });
      const activeRequest = { current: undefined as EventBus['request'] };
      const subscription = startClientLiveSubscription({
        server,
        info: server.live,
        onReady: () => {
          if (generation !== subscriptionGeneration || stopped) return;
          liveStreamActive = true;
          reconnectAttempt = 0;
          console.debug(`[eventBus:${serverId}] Chatto live stream ready`, debugState());
        },
        onEvent: (event) => dispatchEvent(event, generation),
        onCatchUpNeeded: () => notifyCatchUpHandlers('protobuf-sparse-event'),
        onError: (err) => {
          console.error(`[eventBus:${serverId}] Chatto live stream failed`, err);
        },
        onEnd: () => {
          liveStreamActive = false;
          if (bus.request === activeRequest.current) {
            bus.request = undefined;
          }
          if (teardownInProgress || stopped) return;
          console.warn(`[eventBus:${serverId}] Chatto live stream ended`);
          scheduleResubscribe('Chatto live stream ended', 'subscription-ended');
        }
      });
      activeRequest.current = subscription.request;
      bus.request = subscription.request;
      return {
        unsubscribe: () => {
          if (bus.request === subscription.request) {
            bus.request = undefined;
          }
          subscription.unsubscribe();
        }
      };
    };

    const notifyCatchUpHandlers = (
      reason: EventBusCatchUpReason,
      phase: 'immediate' | 'projection-grace' = 'immediate'
    ) => {
      console.debug(`[eventBus:${serverId}] notifying catch-up handlers`, {
        reason,
        phase,
        catchUpHandlers: catchUpHandlers.size,
        ...debugState()
      });
      for (const handler of catchUpHandlers) {
        try {
          handler(reason);
        } catch (err) {
          console.error(`[eventBus:${serverId}] catch-up handler threw`, err);
        }
      }
    };

    const scheduleCatchUpRetry = (reason: EventBusCatchUpReason) => {
      if (catchUpRetryTimer) clearTimeout(catchUpRetryTimer);
      catchUpRetryTimer = setTimeout(() => {
        catchUpRetryTimer = null;
        if (stopped) return;
        console.debug(`[eventBus:${serverId}] retrying catch-up after projection grace period`, {
          reason,
          ...debugState()
        });
        notifyCatchUpHandlers(reason, 'projection-grace');
      }, CATCH_UP_RETRY_MS);
    };

    const resubscribe = (
      reason: string,
      catchUpReason: EventBusCatchUpReason,
      notifyCatchUp = true
    ) => {
      if (stopped) return;
      if (resubscribeTimer) {
        clearTimeout(resubscribeTimer);
        resubscribeTimer = null;
      }
      resubscribeCount++;
      console.debug(`[eventBus:${serverId}] resubscribe requested`, {
        reason,
        ...debugState()
      });
      console.warn(
        `[eventBus:${serverId}] re-subscribing (${reason}; total resubscribes: ${resubscribeCount}; lastEvent: ${Math.round((Date.now() - lastEventAt) / 1000)}s ago)`
      );
      teardownInProgress = true;
      this.#subscriptions.get(serverId)?.unsubscribe();
      teardownInProgress = false;
      lastEventAt = Date.now();
      this.#subscriptions.set(serverId, subscribeOnce(reason));
      if (notifyCatchUp) {
        notifyCatchUpHandlers(catchUpReason);
        scheduleCatchUpRetry(catchUpReason);
      }
    };

    const scheduleResubscribe = (reason: string, catchUpReason: EventBusCatchUpReason) => {
      if (stopped || resubscribeTimer) return;
      const delayMs = Math.min(
        RESUBSCRIBE_MAX_MS,
        RESUBSCRIBE_BASE_MS * 2 ** Math.min(reconnectAttempt, 5)
      );
      reconnectAttempt++;
      console.warn(
        `[eventBus:${serverId}] scheduling re-subscribe in ${delayMs}ms (${reason}; attempt: ${reconnectAttempt})`
      );
      resubscribeTimer = setTimeout(() => {
        resubscribeTimer = null;
        resubscribe(reason, catchUpReason, false);
      }, delayMs);
      notifyCatchUpHandlers(catchUpReason);
      scheduleCatchUpRetry(catchUpReason);
    };

    console.debug(`[eventBus:${serverId}] bus started`, debugState());
    this.#subscriptions.set(serverId, subscribeOnce('initial start'));

    const heartbeatWatchdog = setInterval(() => {
      if (stopped) return;
      if (!liveStreamActive) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      const ageMs = Date.now() - lastEventAt;
      if (ageMs < HEARTBEAT_STALL_MS) return;
      console.debug(`[eventBus:${serverId}] heartbeat watchdog detected stale stream`, {
        ageMs,
        ...debugState()
      });
      console.warn(
        `[eventBus:${serverId}] heartbeat stalled; re-subscribing (${Math.round(ageMs / 1000)}s since last event)`
      );
      resubscribe('heartbeat stalled', 'heartbeat-stalled');
    }, HEARTBEAT_WATCHDOG_MS);

    this.#cleanups.set(serverId, () => {
      // Flag the closure so the upcoming sub.unsubscribe() in stopBus
      // doesn't fire a reentrant resubscribe through onEnd.
      stopped = true;
      console.debug(`[eventBus:${serverId}] bus stopping`, debugState());
      if (catchUpRetryTimer) clearTimeout(catchUpRetryTimer);
      if (resubscribeTimer) clearTimeout(resubscribeTimer);
      clearInterval(heartbeatWatchdog);
    });

    this.#buses.set(serverId, bus);

    return () => this.stopBus(serverId);
  }

  /** Stop and remove the event bus for the given server. */
  stopBus(serverId: string): void {
    const cleanup = this.#cleanups.get(serverId);
    if (cleanup) {
      cleanup();
      this.#cleanups.delete(serverId);
    }
    const sub = this.#subscriptions.get(serverId);
    if (sub) {
      // Mark teardown so the `onEnd` callback inside the pipe doesn't
      // try to resubscribe a bus that's going away.
      sub.unsubscribe();
      this.#subscriptions.delete(serverId);
    }
    this.#buses.delete(serverId);
  }

  /** Get the event bus for a server, or undefined if not started. */
  getBus(serverId: string): EventBus | undefined {
    return this.#buses.get(serverId);
  }

  /** Stop all buses. Used during teardown (e.g., logout). */
  stopAll(): void {
    for (const serverId of [...this.#buses.keys()]) {
      this.stopBus(serverId);
    }
  }
}

export const eventBusManager = new EventBusManager();

function isSameOriginServerURL(rawURL: string): boolean {
  if (typeof window === 'undefined') return false;
  try {
    return new URL(rawURL, window.location.href).origin === window.location.origin;
  } catch {
    return false;
  }
}
