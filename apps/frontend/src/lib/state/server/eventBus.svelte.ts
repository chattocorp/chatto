/**
 * Manages per-server realtime event streams. One `/api/realtime` WebSocket
 * per registered server feeds the local event bus; consumers receive the
 * existing EventEnvelope shape with the decoded protobuf envelope attached for
 * paths that are ready to use the public realtime payload directly.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type {
  EventBusCatchUpReason,
  EventBusCatchUpPhase,
  EventBusCatchUpSignal,
  EventBusCatchUpHandler,
  EventHandler,
  EventBus,
  EventEnvelope
} from '$lib/eventBus.svelte';
import { attachRealtimeEventEnvelope } from '$lib/eventBus.svelte';
import { roomEventKind } from '$lib/render/eventKinds';
import { realtimeEventToEventEnvelope } from '$lib/realtimeEventMapper';
import {
  RealtimeClientFrame,
  RealtimeClientHello,
  RealtimeServerFrame,
  RealtimeSubscribeEvents
} from '@chatto/api-types/realtime/v1/realtime_pb';
import type { ServerConnection } from './serverConnection.svelte';

const DEFAULT_HEARTBEAT_STALL_MS = 75_000;
const MIN_HEARTBEAT_STALL_MS = 30_000;
const MISSED_HEARTBEATS_BEFORE_STALL = 3;
const HEARTBEAT_WATCHDOG_MS = 15_000;
const CATCH_UP_RETRY_MS = 2_500;
const RECONNECT_WAIT_MS = 5_000;

type RealtimeMessageEvent = { data: ArrayBuffer | Blob | Uint8Array };
type RealtimeCloseEvent = { code?: number; reason?: string };
type RealtimeSocket = {
  binaryType: BinaryType;
  readyState: number;
  onopen: (() => void) | null;
  onmessage: ((event: RealtimeMessageEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  onclose: ((event: RealtimeCloseEvent) => void) | null;
  send(data: Uint8Array): void;
  close(code?: number, reason?: string): void;
};
type RealtimeSocketFactory = (url: string) => RealtimeSocket;

let realtimeSocketFactory: RealtimeSocketFactory = (url) => new WebSocket(url) as RealtimeSocket;

export function setRealtimeSocketFactoryForTests(factory: RealtimeSocketFactory | null): void {
  realtimeSocketFactory = factory ?? ((url) => new WebSocket(url) as RealtimeSocket);
}

async function messageDataToBytes(data: RealtimeMessageEvent['data']): Promise<Uint8Array> {
  if (data instanceof Uint8Array) return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  return new Uint8Array(await data.arrayBuffer());
}

function clientHelloFrame(token: string | null): Uint8Array {
  return new RealtimeClientFrame({
    frame: {
      case: 'hello',
      value: new RealtimeClientHello({
        protocolVersion: 1,
        bearerToken: token ?? undefined
      })
    }
  }).toBinary();
}

function subscribeEventsFrame(): Uint8Array {
  return new RealtimeClientFrame({
    frame: {
      case: 'subscribeEvents',
      value: new RealtimeSubscribeEvents()
    }
  }).toBinary();
}

function heartbeatStallMsForInterval(seconds: number): number {
  if (seconds <= 0) return DEFAULT_HEARTBEAT_STALL_MS;
  return Math.max(MIN_HEARTBEAT_STALL_MS, seconds * MISSED_HEARTBEATS_BEFORE_STALL * 1000);
}

class EventBusManager {
  // SvelteMap so getBus() is a reactive read — consumers like NotificationSync
  // re-run their $effect when a bus is started/stopped, avoiding mount races.
  #buses = new SvelteMap<string, EventBus>();
  #subscriptions = new Map<string, { unsubscribe: () => void }>();
  #cleanups = new Map<string, () => void>();

  /**
   * Start an event bus for the given server. Creates the realtime socket and
   * stores the bus. If a bus already exists for this server, returns a no-op.
   */
  startBus(serverId: string, serverConnection: ServerConnection): () => void {
    if (this.#buses.has(serverId)) return () => {};

    const handlers = new SvelteSet<EventHandler>();
    const catchUpHandlers = new SvelteSet<EventBusCatchUpHandler>();
    const bus: EventBus = { handlers, catchUpHandlers };
    let lastEventAt = Date.now();
    let heartbeatStallMs = DEFAULT_HEARTBEAT_STALL_MS;
    let heartbeatCount = 0;
    let dispatchedEventCount = 0;
    let reconnectCount = 0;
    let reconnectAttempts = 0;
    let generation = 0;
    let socket: RealtimeSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let catchUpRetryTimer: ReturnType<typeof setTimeout> | null = null;
    let stopped = false;

    const debugState = () => ({
      generation,
      handlers: handlers.size,
      events: dispatchedEventCount,
      heartbeats: heartbeatCount,
      reconnects: reconnectCount,
      heartbeatStallMs,
      lastEventAgeMs: Date.now() - lastEventAt
    });

    const notifyCatchUpHandlers = (
      reason: EventBusCatchUpReason,
      phase: EventBusCatchUpPhase = 'immediate'
    ) => {
      const signal: EventBusCatchUpSignal = { reason, phase };
      console.debug(`[eventBus:${serverId}] notifying catch-up handlers`, {
        reason,
        phase,
        catchUpHandlers: catchUpHandlers.size,
        ...debugState()
      });
      for (const handler of catchUpHandlers) {
        try {
          handler(signal);
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

    const clearReconnectTimer = () => {
      if (!reconnectTimer) return;
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    };

    const detachSocket = (close = true) => {
      const current = socket;
      socket = null;
      if (!current) return;
      current.onopen = null;
      current.onmessage = null;
      current.onerror = null;
      current.onclose = null;
      if (close) current.close(1000, 'replaced');
    };

    const stopForAuthenticationRequired = (current: RealtimeSocket, reason: string) => {
      console.warn(`[eventBus:${serverId}] realtime authentication required`, {
        reason,
        ...debugState()
      });
      stopped = true;
      current.onclose = null;
      if (socket === current) socket = null;
      serverConnection.setRealtimeConnectionStatus('disconnected', reconnectAttempts);
      current.close(1000, 'authentication_required');
      serverConnection.handleAuthenticationRequired();
    };

    const dispatchEvent = (event: EventEnvelope) => {
      dispatchedEventCount++;
      console.debug(
        `[eventBus:${serverId}] event dispatched`,
        roomEventKind(event.event) ?? '<unknown>',
        { eventId: event.id, total: dispatchedEventCount, ...debugState() }
      );
      for (const handler of handlers) {
        try {
          handler(event);
        } catch (err) {
          console.error(`[eventBus:${serverId}] handler threw`, err);
        }
      }
    };

    const connect = (reason: string) => {
      if (stopped) return;
      clearReconnectTimer();
      generation++;
      const socketGeneration = generation;
      lastEventAt = Date.now();
      serverConnection.setRealtimeConnectionStatus('connecting', reconnectAttempts);
      console.debug(`[eventBus:${serverId}] opening realtime socket`, {
        reason,
        url: serverConnection.realtimeUrl,
        ...debugState()
      });

      const nextSocket = realtimeSocketFactory(serverConnection.realtimeUrl);
      nextSocket.binaryType = 'arraybuffer';
      socket = nextSocket;

      nextSocket.onopen = () => {
        if (stopped || socket !== nextSocket) return;
        console.debug(`[eventBus:${serverId}] realtime socket opened`, debugState());
        nextSocket.send(clientHelloFrame(serverConnection.bearerToken));
      };

      nextSocket.onmessage = (message) => {
        void (async () => {
          if (stopped || socket !== nextSocket) return;
          let frame: RealtimeServerFrame;
          try {
            frame = RealtimeServerFrame.fromBinary(await messageDataToBytes(message.data));
          } catch (err) {
            console.error(`[eventBus:${serverId}] failed to decode realtime frame`, err);
            return;
          }

          lastEventAt = Date.now();
          switch (frame.frame.case) {
            case 'hello':
              heartbeatStallMs = heartbeatStallMsForInterval(
                frame.frame.value.heartbeatIntervalSeconds
              );
              nextSocket.send(subscribeEventsFrame());
              return;
            case 'subscribed':
              reconnectAttempts = 0;
              serverConnection.setRealtimeConnectionStatus('connected');
              console.debug(`[eventBus:${serverId}] realtime stream subscribed`, {
                generation: socketGeneration
              });
              return;
            case 'heartbeat':
              heartbeatCount++;
              console.debug(`[eventBus:${serverId}] heartbeat received`, {
                total: heartbeatCount
              });
              return;
            case 'event': {
              const event = realtimeEventToEventEnvelope(frame.frame.value);
              if (event) dispatchEvent(attachRealtimeEventEnvelope(event, frame.frame.value));
              return;
            }
            case 'error':
              console.error(`[eventBus:${serverId}] realtime error`, {
                code: frame.frame.value.code,
                message: frame.frame.value.message,
                fatal: frame.frame.value.fatal
              });
              if (frame.frame.value.code === 'authentication_required') {
                stopForAuthenticationRequired(nextSocket, 'error frame');
                return;
              }
              if (frame.frame.value.fatal) {
                nextSocket.close(1011, frame.frame.value.code || 'fatal realtime error');
              }
              return;
            case 'close':
              if (frame.frame.value.code === 'authentication_required') {
                stopForAuthenticationRequired(nextSocket, 'close frame');
                return;
              }
              nextSocket.onclose = null;
              if (socket === nextSocket) socket = null;
              nextSocket.close(1000, frame.frame.value.message || frame.frame.value.code);
              if (frame.frame.value.reconnect) {
                scheduleReconnect(
                  'server requested close',
                  'subscription-ended',
                  frame.frame.value.retryAfterMs
                );
              } else {
                serverConnection.setRealtimeConnectionStatus('disconnected', reconnectAttempts);
              }
              return;
            case 'pong':
            case undefined:
              return;
          }
        })();
      };

      nextSocket.onerror = (event) => {
        console.error(`[eventBus:${serverId}] realtime socket error`, event);
      };

      nextSocket.onclose = (event) => {
        if (stopped || socket !== nextSocket) return;
        socket = null;
        console.warn(`[eventBus:${serverId}] realtime socket closed`, {
          code: event.code,
          reason: event.reason,
          ...debugState()
        });
        scheduleReconnect('socket closed', 'subscription-ended');
      };
    };

    const scheduleReconnect = (
      reason: string,
      catchUpReason: EventBusCatchUpReason,
      delayMs?: number
    ) => {
      if (stopped) return;
      clearReconnectTimer();
      reconnectCount++;
      reconnectAttempts++;
      serverConnection.setRealtimeConnectionStatus('disconnected', reconnectAttempts);
      notifyCatchUpHandlers(catchUpReason);
      scheduleCatchUpRetry(catchUpReason);
      const wait = delayMs ?? (reconnectAttempts <= 1 ? 0 : RECONNECT_WAIT_MS);
      console.warn(`[eventBus:${serverId}] reconnecting realtime stream`, {
        reason,
        wait,
        attempt: reconnectAttempts,
        ...debugState()
      });
      reconnectTimer = setTimeout(() => connect(reason), wait);
    };

    const reconnectNow = (reason: string, catchUpReason: EventBusCatchUpReason) => {
      if (stopped) return;
      detachSocket(true);
      reconnectAttempts = 0;
      scheduleReconnect(reason, catchUpReason, 0);
    };

    const unregisterReconnect = serverConnection.registerRealtimeReconnect((reason) => {
      reconnectNow(reason, 'ws-reconnected');
    });

    console.debug(`[eventBus:${serverId}] bus started`, debugState());
    this.#subscriptions.set(serverId, { unsubscribe: () => detachSocket(true) });
    connect('initial start');

    const heartbeatWatchdog = setInterval(() => {
      if (stopped) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      const ageMs = Date.now() - lastEventAt;
      if (ageMs < heartbeatStallMs) return;
      console.warn(`[eventBus:${serverId}] heartbeat stalled`, {
        ageMs,
        ...debugState()
      });
      reconnectNow('heartbeat stalled', 'heartbeat-stalled');
    }, HEARTBEAT_WATCHDOG_MS);

    this.#cleanups.set(serverId, () => {
      stopped = true;
      console.debug(`[eventBus:${serverId}] bus stopping`, debugState());
      if (catchUpRetryTimer) clearTimeout(catchUpRetryTimer);
      clearReconnectTimer();
      clearInterval(heartbeatWatchdog);
      unregisterReconnect();
      detachSocket(true);
      serverConnection.setRealtimeConnectionStatus('disconnected');
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
