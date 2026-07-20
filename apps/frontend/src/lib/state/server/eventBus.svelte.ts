/**
 * Owns the session-long event bus for every authenticated server and assigns
 * its transport one of three modes: live, polling, or dormant. Only the
 * URL-active server keeps a persistent WebSocket. Inactive projections catch
 * up through serialized, short-lived connections to the same event stream.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type {
  EventHandler,
  ProjectionHandler,
  EventBus,
  EventEnvelope
} from '$lib/eventBus.svelte';
import { roomEventKind } from '$lib/render/eventKinds';
import { realtimeEventToEventEnvelope } from '$lib/realtimeEventMapper';
import {
  RealtimeClientFrame,
  RealtimeClientHello,
  RealtimeHydrateRoom,
  RealtimeServerFrame,
  RealtimeSubscribeEvents,
  type RealtimeProjectionEvent
} from '@chatto/api-types/realtime/v1/realtime_pb';
import type { ConnectionStatus, ServerConnection } from './serverConnection.svelte';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';

const DEFAULT_HEARTBEAT_STALL_MS = 75_000;
const MIN_HEARTBEAT_STALL_MS = 30_000;
const MISSED_HEARTBEATS_BEFORE_STALL = 3;
const HEARTBEAT_WATCHDOG_MS = 15_000;
const RECONNECT_WAIT_MS = 5_000;
const INACTIVE_POLL_INTERVAL_MS = 60_000;
const INACTIVE_POLL_JITTER_MS = 10_000;
const INACTIVE_POLL_TIMEOUT_MS = 30_000;
const HYDRATION_RETRY_FALLBACK_MS = 1_000;

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
type TransportMode = 'dormant' | 'polling' | 'live';

export type RealtimeServerRegistration = {
  serverId: string;
  connection: ServerConnection;
  projectionSupported: boolean;
  sync: RealtimeProjectionSyncState;
};

type TransportController = {
  readonly sync: RealtimeProjectionSyncState;
  readonly projectionSupported: boolean;
  update(projectionSupported: boolean): void;
  setMode(mode: 'dormant' | 'live'): void;
  pollOnce(): Promise<boolean>;
  hydrateRoom(roomId: string): void;
  cleanup(): void;
};

let realtimeSocketFactory: RealtimeSocketFactory = (url) => new WebSocket(url) as RealtimeSocket;
let pollRandom = Math.random;

export function setRealtimeSocketFactoryForTests(factory: RealtimeSocketFactory | null): void {
  realtimeSocketFactory = factory ?? ((url) => new WebSocket(url) as RealtimeSocket);
}

export function setRealtimePollRandomForTests(random: (() => number) | null): void {
  pollRandom = random ?? Math.random;
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
        protocolVersion: 2,
        bearerToken: token ?? undefined
      })
    }
  }).toBinary();
}

function subscribeEventsFrame(resumeCursor: string | null, retainedRoomIds: string[]): Uint8Array {
  return new RealtimeClientFrame({
    frame: {
      case: 'subscribeEvents',
      value: new RealtimeSubscribeEvents({
        resumeCursor: resumeCursor ?? undefined,
        retainedRoomIds
      })
    }
  }).toBinary();
}

function hydrateRoomFrame(roomId: string): Uint8Array {
  return new RealtimeClientFrame({
    frame: {
      case: 'hydrateRoom',
      value: new RealtimeHydrateRoom({ roomId })
    }
  }).toBinary();
}

function heartbeatStallMsForInterval(seconds: number): number {
  if (seconds <= 0) return DEFAULT_HEARTBEAT_STALL_MS;
  return Math.max(MIN_HEARTBEAT_STALL_MS, seconds * MISSED_HEARTBEATS_BEFORE_STALL * 1000);
}

function projectionResets(event: RealtimeProjectionEvent): boolean {
  return event.operations.some((operation) => operation.operation.case === 'reset');
}

class EventBusManager {
  // Reactive so context consumers can attach after a server becomes authenticated.
  #buses = new SvelteMap<string, EventBus>();
  #controllers = new Map<string, TransportController>();
  #managedServerIds = new Set<string>();
  #activeServerId: string | null = null;
  #pollCycleRunning = false;
  #pollTimer: ReturnType<typeof setTimeout> | null = null;

  /**
   * Compatibility entry point for direct consumers and focused tests. New app
   * ownership should use synchronizeAuthenticatedServers().
   */
  startBus(
    serverId: string,
    serverConnection: ServerConnection,
    realtimeProjectionSupported = true,
    sync = new RealtimeProjectionSyncState()
  ): () => void {
    const controller = this.ensureBus(
      serverId,
      serverConnection,
      realtimeProjectionSupported,
      sync
    );
    if (realtimeProjectionSupported) controller.setMode('live');
    return () => this.stopBus(serverId);
  }

  /** Register the stable bus/reducer surface without necessarily opening a socket. */
  ensureBus(
    serverId: string,
    serverConnection: ServerConnection,
    realtimeProjectionSupported = true,
    sync = new RealtimeProjectionSyncState()
  ): TransportController {
    const existing = this.#controllers.get(serverId);
    if (existing) {
      existing.update(realtimeProjectionSupported);
      return existing;
    }

    const handlers = new SvelteSet<EventHandler>();
    const projectionHandlers = new SvelteSet<ProjectionHandler>();
    const bus: EventBus = { handlers, projectionHandlers };
    let projectionSupported = realtimeProjectionSupported;
    let mode: TransportMode = 'dormant';
    let lastEventAt = Date.now();
    let heartbeatStallMs = DEFAULT_HEARTBEAT_STALL_MS;
    let heartbeatCount = 0;
    let dispatchedEventCount = 0;
    let reconnectCount = 0;
    let reconnectAttempts = 0;
    let generation = 0;
    let socket: RealtimeSocket | null = null;
    let socketSubscribed = false;
    let requestedRoomIds = new SvelteSet<string>();
    let pendingHydrationRoomId: string | null = null;
    let hydrationRetryTimer: ReturnType<typeof setTimeout> | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let pollResolution: ((caughtUp: boolean) => void) | null = null;
    let pollTimeout: ReturnType<typeof setTimeout> | null = null;
    let stopped = false;

    const debugState = () => ({
      mode,
      generation,
      handlers: handlers.size,
      events: dispatchedEventCount,
      heartbeats: heartbeatCount,
      reconnects: reconnectCount,
      heartbeatStallMs,
      lastEventAgeMs: Date.now() - lastEventAt
    });

    const clearReconnectTimer = () => {
      if (!reconnectTimer) return;
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    };

    const clearHydrationRetryTimer = () => {
      if (!hydrationRetryTimer) return;
      clearTimeout(hydrationRetryTimer);
      hydrationRetryTimer = null;
    };

    const sendNextRoomHydration = () => {
      if (!socketSubscribed || !socket || pendingHydrationRoomId || hydrationRetryTimer) return;
      for (const roomId of sync.desiredRoomIds) {
        if (requestedRoomIds.has(roomId)) continue;
        requestedRoomIds.add(roomId);
        pendingHydrationRoomId = roomId;
        socket.send(hydrateRoomFrame(roomId));
        return;
      }
    };

    const finishRoomHydrationRequest = (roomId: string) => {
      if (pendingHydrationRoomId !== roomId) return;
      pendingHydrationRoomId = null;
      clearHydrationRetryTimer();
      sendNextRoomHydration();
    };

    const retryRoomHydration = (roomId: string, retryAfterMs: number | undefined) => {
      if (pendingHydrationRoomId === roomId) pendingHydrationRoomId = null;
      requestedRoomIds.delete(roomId);
      clearHydrationRetryTimer();
      hydrationRetryTimer = setTimeout(() => {
        hydrationRetryTimer = null;
        sendNextRoomHydration();
      }, Math.max(1, retryAfterMs ?? HYDRATION_RETRY_FALLBACK_MS));
    };

    const resolvePoll = (caughtUp: boolean) => {
      if (pollTimeout) {
        clearTimeout(pollTimeout);
        pollTimeout = null;
      }
      const resolve = pollResolution;
      pollResolution = null;
      resolve?.(caughtUp);
    };

    const detachSocket = (close = true, reason = 'replaced') => {
      const current = socket;
      socket = null;
      socketSubscribed = false;
      requestedRoomIds = new SvelteSet<string>();
      pendingHydrationRoomId = null;
      clearHydrationRetryTimer();
      if (!current) return;
      current.onopen = null;
      current.onmessage = null;
      current.onerror = null;
      current.onclose = null;
      if (close) current.close(1000, reason);
    };

    const becomeDormant = (markStale: boolean, status: ConnectionStatus = 'dormant') => {
      const wasPolling = mode === 'polling';
      mode = 'dormant';
      clearReconnectTimer();
      detachSocket(true, 'dormant');
      if (markStale) sync.markStale();
      if (wasPolling) resolvePoll(false);
      serverConnection.setRealtimeConnectionStatus(status);
    };

    const stopForAuthenticationRequired = (current: RealtimeSocket, reason: string) => {
      console.warn(`[eventBus:${serverId}] realtime authentication required`, {
        reason,
        ...debugState()
      });
      mode = 'dormant';
      current.onclose = null;
      if (socket === current) socket = null;
      serverConnection.setRealtimeConnectionStatus('disconnected', reconnectAttempts);
      current.close(1000, 'authentication_required');
      resolvePoll(false);
      serverConnection.handleAuthenticationRequired();
    };

    const stopForUnsupportedProtocol = (current: RealtimeSocket) => {
      console.warn(`[eventBus:${serverId}] realtime projection protocol is unsupported`, {
        ...debugState()
      });
      projectionSupported = false;
      mode = 'dormant';
      current.onclose = null;
      if (socket === current) socket = null;
      serverConnection.setRealtimeConnectionStatus('disconnected', reconnectAttempts);
      current.close(1000, 'unsupported_protocol');
      resolvePoll(false);
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
        } catch (error) {
          console.error(`[eventBus:${serverId}] handler threw`, error);
        }
      }
    };

    const dispatchProjectionEvent = (event: RealtimeProjectionEvent) => {
      if (projectionHandlers.size === 0) {
        throw new Error('projection event received before reducer registration');
      }
      for (const handler of projectionHandlers) handler(event);
    };

    const connect = (reason: string) => {
      if (stopped || !projectionSupported || mode === 'dormant' || socket) return;
      clearReconnectTimer();
      generation++;
      const socketGeneration = generation;
      lastEventAt = Date.now();
      sync.beginCatchUp();
      if (mode === 'live') {
        serverConnection.setRealtimeConnectionStatus('connecting', reconnectAttempts);
      }
      console.debug(`[eventBus:${serverId}] opening realtime socket`, {
        reason,
        url: serverConnection.realtimeUrl,
        ...debugState()
      });

      const nextSocket = realtimeSocketFactory(serverConnection.realtimeUrl);
      socketSubscribed = false;
      nextSocket.binaryType = 'arraybuffer';
      socket = nextSocket;

      nextSocket.onopen = () => {
        if (stopped || socket !== nextSocket) return;
        nextSocket.send(clientHelloFrame(serverConnection.bearerToken));
      };

      nextSocket.onmessage = (message) => {
        void (async () => {
          if (stopped || socket !== nextSocket) return;
          let frame: RealtimeServerFrame;
          try {
            frame = RealtimeServerFrame.fromBinary(await messageDataToBytes(message.data));
          } catch (error) {
            console.error(`[eventBus:${serverId}] failed to decode realtime frame`, error);
            // Never continue past a frame we could not understand: a later
            // caught_up boundary would otherwise make the missing mutation
            // permanent in the retained projection.
            nextSocket.close(1003, 'invalid realtime frame');
            return;
          }

          lastEventAt = Date.now();
          switch (frame.frame.case) {
            case 'hello':
              sync.takeTransportEvictions();
              heartbeatStallMs = heartbeatStallMsForInterval(
                frame.frame.value.heartbeatIntervalSeconds
              );
              requestedRoomIds = new SvelteSet(sync.retainedRoomIds);
              nextSocket.send(subscribeEventsFrame(sync.resumeCursor, [...requestedRoomIds]));
              return;
            case 'subscribed':
              socketSubscribed = true;
              reconnectAttempts = 0;
              sendNextRoomHydration();
              if (mode === 'live') serverConnection.setRealtimeConnectionStatus('connected');
              console.debug(`[eventBus:${serverId}] realtime stream subscribed`, {
                generation: socketGeneration,
                mode
              });
              return;
            case 'heartbeat':
              heartbeatCount++;
              return;
            case 'event': {
              const event = realtimeEventToEventEnvelope(frame.frame.value);
              if (event) dispatchEvent(event);
              return;
            }
            case 'projectionEvent':
              try {
                dispatchProjectionEvent(frame.frame.value);
              } catch (error) {
                console.error(`[eventBus:${serverId}] projection reducer failed`, error);
                nextSocket.close(1011, 'projection reducer failed');
                return;
              }
              sync.acceptProjectionEvent(
                frame.frame.value.resumeCursor,
                projectionResets(frame.frame.value)
              );
              for (const operation of frame.frame.value.operations) {
                if (operation.operation.case === 'roomTimelineReplace') {
                  const roomId = operation.operation.value.roomId;
                  sync.confirmRoom(roomId);
                  finishRoomHydrationRequest(roomId);
                }
              }
              return;
            case 'caughtUp': {
              sync.markCaughtUp(frame.frame.value.cursor);
              const completedPoll = pollResolution !== null;
              resolvePoll(true);
              if (mode === 'polling') {
                mode = 'dormant';
                detachSocket(true, 'caught_up');
                // The projection is usable, but the closed transport means
                // absence stops being authoritative immediately.
                sync.markStale();
                serverConnection.setRealtimeConnectionStatus('dormant');
              } else if (completedPoll && mode === 'live') {
                serverConnection.setRealtimeConnectionStatus('connected');
              }
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
              if (frame.frame.value.code === 'unsupported_protocol') {
                stopForUnsupportedProtocol(nextSocket);
                return;
              }
              if (frame.frame.value.code.startsWith('room_hydration_')) {
                const roomId = frame.frame.value.roomId ?? pendingHydrationRoomId;
                if (roomId) retryRoomHydration(roomId, frame.frame.value.retryAfterMs);
                return;
              }
              if (
                frame.frame.value.code === 'room_unavailable' ||
                frame.frame.value.code === 'too_many_retained_rooms'
              ) {
                const roomId = frame.frame.value.roomId ?? pendingHydrationRoomId;
                if (roomId) finishRoomHydrationRequest(roomId);
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
              // The close frame bypasses the socket's normal onclose handler.
              // Release socket-scoped hydration state before reconnecting so a
              // request whose response was lost can be sent on the replacement.
              socketSubscribed = false;
              pendingHydrationRoomId = null;
              clearHydrationRetryTimer();
              nextSocket.close(1000, frame.frame.value.message || frame.frame.value.code);
              if (mode === 'live' && frame.frame.value.reconnect) {
                scheduleReconnect('server requested close', frame.frame.value.retryAfterMs);
              } else {
                resolvePoll(false);
                if (mode === 'polling') {
                  mode = 'dormant';
                  serverConnection.setRealtimeConnectionStatus('disconnected');
                }
              }
              return;
            case 'pong':
              return;
            case undefined:
              console.error(`[eventBus:${serverId}] unsupported realtime server frame`);
              nextSocket.close(1003, 'unsupported realtime frame');
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
        socketSubscribed = false;
        pendingHydrationRoomId = null;
        clearHydrationRetryTimer();
        console.warn(`[eventBus:${serverId}] realtime socket closed`, {
          code: event.code,
          reason: event.reason,
          ...debugState()
        });
        if (mode === 'live') {
          scheduleReconnect('socket closed');
        } else {
          mode = 'dormant';
          resolvePoll(false);
          serverConnection.setRealtimeConnectionStatus('disconnected');
        }
      };
    };

    function scheduleReconnect(reason: string, delayMs?: number): void {
      if (stopped || mode !== 'live' || !projectionSupported) return;
      clearReconnectTimer();
      reconnectCount++;
      reconnectAttempts++;
      sync.markStale();
      serverConnection.setRealtimeConnectionStatus('disconnected', reconnectAttempts);
      const wait = delayMs ?? (reconnectAttempts <= 1 ? 0 : RECONNECT_WAIT_MS);
      reconnectTimer = setTimeout(() => connect(reason), wait);
    }

    const reconnectNow = (reason: string) => {
      if (stopped || mode !== 'live' || !projectionSupported) return;
      detachSocket(true);
      reconnectAttempts = 0;
      scheduleReconnect(reason, 0);
    };

    const unregisterReconnect = serverConnection.registerRealtimeReconnect((reason) => {
      reconnectNow(reason);
    });

    const heartbeatWatchdog = setInterval(() => {
      if (stopped || mode !== 'live' || !projectionSupported) return;
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
      const ageMs = Date.now() - lastEventAt;
      if (ageMs >= heartbeatStallMs) reconnectNow('heartbeat stalled');
    }, HEARTBEAT_WATCHDOG_MS);

    const controller: TransportController = {
      sync,
      get projectionSupported() {
        return projectionSupported;
      },
      update(supported) {
        projectionSupported = supported;
        if (supported && mode === 'live') connect('projection capability confirmed');
        if (!supported && mode !== 'dormant') becomeDormant(false);
      },
      setMode(nextMode) {
        if (stopped) return;
        if (nextMode === 'dormant') {
          // A normal reconciliation keeps an already-running background
          // catch-up alive until it reaches caught_up or its timeout.
          if (mode === 'polling') return;
          becomeDormant(true);
          return;
        }
        if (mode === 'live') return;
        // Promotion transfers the existing socket to live ownership. Settle
        // the old polling promise and cancel its timeout so it cannot later
        // demote this active controller or close a replacement socket.
        resolvePoll(false);
        mode = 'live';
        serverConnection.setRealtimeConnectionStatus(socketSubscribed ? 'connected' : 'connecting');
        connect('server became active');
      },
      pollOnce() {
        if (stopped || !projectionSupported || mode === 'live') {
          return Promise.resolve(false);
        }
        if (pollResolution) return Promise.resolve(false);
        mode = 'polling';
        return new Promise<boolean>((resolve) => {
          pollResolution = resolve;
          pollTimeout = setTimeout(
            () => becomeDormant(true, 'disconnected'),
            INACTIVE_POLL_TIMEOUT_MS
          );
          connect('inactive server catch-up');
        });
      },
      hydrateRoom(roomId) {
        if (!roomId || stopped || !projectionSupported) return;
        sync.retainRoom(roomId);
        const evictedRoomIds = sync.takeTransportEvictions();
        for (const evictedRoomId of evictedRoomIds) requestedRoomIds.delete(evictedRoomId);
        if (evictedRoomIds.length > 0 && socket) {
          detachSocket(true, 'room retention rollover');
          connect('room retention rollover');
          return;
        }
        if (socketSubscribed && socket && !requestedRoomIds.has(roomId)) {
          sendNextRoomHydration();
        }
      },
      cleanup() {
        stopped = true;
        mode = 'dormant';
        clearReconnectTimer();
        clearInterval(heartbeatWatchdog);
        unregisterReconnect();
        detachSocket(true, 'stopped');
        resolvePoll(false);
        serverConnection.setRealtimeConnectionStatus('disconnected');
      }
    };

    this.#buses.set(serverId, bus);
    this.#controllers.set(serverId, controller);
    return controller;
  }

  /** Materialise one room timeline on the server's existing projection stream. */
  hydrateRoom(serverId: string, roomId: string): void {
    this.#controllers.get(serverId)?.hydrateRoom(roomId);
  }

  /**
   * Reconcile all authenticated projections against the URL-active server.
   * This is the only application-level transport ownership entry point.
   */
  synchronizeAuthenticatedServers(
    registrations: RealtimeServerRegistration[],
    activeServerId: string | null
  ): void {
    const nextIds = new Set(registrations.map((registration) => registration.serverId));
    for (const serverId of this.#managedServerIds) {
      if (!nextIds.has(serverId)) this.stopBus(serverId);
    }
    this.#managedServerIds = nextIds;
    this.#activeServerId = nextIds.has(activeServerId ?? '') ? activeServerId : null;

    for (const registration of registrations) {
      this.ensureBus(
        registration.serverId,
        registration.connection,
        registration.projectionSupported,
        registration.sync
      );
    }
    // Close the previous live transport before opening the next one so a
    // route change never leaves two persistent sockets, even momentarily.
    for (const registration of registrations) {
      if (registration.serverId !== this.#activeServerId) {
        this.#controllers.get(registration.serverId)?.setMode('dormant');
      }
    }
    if (this.#activeServerId) {
      this.#controllers.get(this.#activeServerId)?.setMode('live');
    }

    void this.#runPollCycle(true);
    this.#scheduleNextPoll();
  }

  /** Stop and remove the event bus and its projection session transport. */
  stopBus(serverId: string): void {
    this.#controllers.get(serverId)?.cleanup();
    this.#controllers.delete(serverId);
    this.#buses.delete(serverId);
    this.#managedServerIds.delete(serverId);
    if (this.#activeServerId === serverId) this.#activeServerId = null;
  }

  getBus(serverId: string): EventBus | undefined {
    return this.#buses.get(serverId);
  }

  stopAll(): void {
    this.#clearPollTimer();
    for (const serverId of [...this.#controllers.keys()]) this.stopBus(serverId);
  }

  async #runPollCycle(onlyEmpty: boolean): Promise<void> {
    if (this.#pollCycleRunning) return;
    this.#pollCycleRunning = true;
    try {
      for (const serverId of this.#managedServerIds) {
        if (serverId === this.#activeServerId) continue;
        const controller = this.#controllers.get(serverId);
        if (!controller?.projectionSupported) continue;
        if (onlyEmpty && controller.sync.phase !== 'empty') continue;
        await controller.pollOnce();
      }
    } finally {
      this.#pollCycleRunning = false;
    }
  }
  #scheduleNextPoll(): void {
    this.#clearPollTimer();
    if (this.#managedServerIds.size === 0) return;
    const jitter = (pollRandom() * 2 - 1) * INACTIVE_POLL_JITTER_MS;
    this.#pollTimer = setTimeout(() => {
      this.#pollTimer = null;
      void this.#runPollCycle(false).finally(() => this.#scheduleNextPoll());
    }, INACTIVE_POLL_INTERVAL_MS + jitter);
  }

  #clearPollTimer(): void {
    if (!this.#pollTimer) return;
    clearTimeout(this.#pollTimer);
    this.#pollTimer = null;
  }
}

export const eventBusManager = new EventBusManager();
