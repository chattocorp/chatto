/**
 * Manages per-server event buses backed by the protobuf wire connection.
 *
 * The public bus shape intentionally stays compatible with the older
 * earlier event bus while the transport underneath is now
 * `chatto.wire.v1.StreamEvent` over `/api/wire`.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
  streamEventToEventEnvelope,
  type EventBus,
  type EventBusCatchUpReason,
  type EventHandler
} from '$lib/eventBus.svelte';
import type { StreamEvent } from '$lib/pb/chatto/wire/v1/protocol_pb';
import type { ServerConnection } from './serverConnection.svelte';
import { wireEventBusManager } from './wireEventBus.svelte';

const HEARTBEAT_STALL_MS = 75_000;
const HEARTBEAT_WATCHDOG_MS = 15_000;
const CATCH_UP_RETRY_MS = 2_500;

class EventBusManager {
  // SvelteMap so getBus() is a reactive read — consumers like NotificationSync
  // re-run their $effect when a bus is started/stopped, which avoids a race
  // where the consumer mounts before startBus and never re-attaches.
  #buses = new SvelteMap<string, EventBus>();
  #cleanups = new Map<string, () => void>();

  /**
   * Start an event bus for the given server. If a bus already exists for this
   * server, returns a no-op cleanup without creating a duplicate.
   */
  startBus(serverId: string, connection: ServerConnection): () => void {
    if (this.#buses.has(serverId)) return () => {};

    const handlers = new SvelteSet<EventHandler>();
    const catchUpHandlers = new SvelteSet<(reason: EventBusCatchUpReason) => void>();
    const bus: EventBus = { handlers, catchUpHandlers };

    let lastEventAt = Date.now();
    let heartbeatCount = 0;
    let dispatchedEventCount = 0;
    let catchUpRetryTimer: ReturnType<typeof setTimeout> | null = null;
    let stopped = false;

    const debugState = () => ({
      handlers: handlers.size,
      events: dispatchedEventCount,
      heartbeats: heartbeatCount,
      lastEventAgeMs: Date.now() - lastEventAt
    });

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
        notifyCatchUpHandlers(reason, 'projection-grace');
      }, CATCH_UP_RETRY_MS);
    };

    const dispatchWireEvent = (streamEvent: StreamEvent) => {
      lastEventAt = Date.now();
      const event = streamEventToEventEnvelope(streamEvent);
      if (!event) return;

      if (event.event?.__typename === 'HeartbeatEvent') {
        heartbeatCount++;
        console.debug(`[eventBus:${serverId}] heartbeat received (total: ${heartbeatCount})`);
        return;
      }

      dispatchedEventCount++;
      console.debug(
        `[eventBus:${serverId}] event dispatched`,
        event.event?.__typename ?? '<unknown>',
        {
          eventId: event.id,
          total: dispatchedEventCount
        }
      );

      for (const handler of handlers) {
        try {
          handler(event);
        } catch (err) {
          console.error(`[eventBus:${serverId}] handler threw`, err);
        }
      }
    };

    this.#buses.set(serverId, bus);
    wireEventBusManager.startBus(serverId, connection);
    wireEventBusManager.getBus(serverId)?.handlers.add(dispatchWireEvent);

    const heartbeatWatchdog = setInterval(() => {
      if (stopped) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      const ageMs = Date.now() - lastEventAt;
      if (ageMs < HEARTBEAT_STALL_MS) return;
      console.debug(`[eventBus:${serverId}] heartbeat watchdog detected stale wire stream`, {
        ageMs,
        ...debugState()
      });
      console.warn(
        `[eventBus:${serverId}] heartbeat stalled; requesting projected-state catch-up (${Math.round(ageMs / 1000)}s since last event)`
      );
      lastEventAt = Date.now();
      notifyCatchUpHandlers('heartbeat-stalled');
      scheduleCatchUpRetry('heartbeat-stalled');
    }, HEARTBEAT_WATCHDOG_MS);

    this.#cleanups.set(serverId, () => {
      stopped = true;
      console.debug(`[eventBus:${serverId}] bus stopping`, debugState());
      if (catchUpRetryTimer) clearTimeout(catchUpRetryTimer);
      clearInterval(heartbeatWatchdog);
      wireEventBusManager.getBus(serverId)?.handlers.delete(dispatchWireEvent);
    });

    console.debug(`[eventBus:${serverId}] wire-backed bus started`, debugState());

    return () => this.stopBus(serverId);
  }

  /** Stop and remove the event bus for the given server. */
  stopBus(serverId: string): void {
    const cleanup = this.#cleanups.get(serverId);
    if (cleanup) {
      cleanup();
      this.#cleanups.delete(serverId);
    }
    wireEventBusManager.stopBus(serverId);
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
