import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { WireClient, WireProtocolError, type WireConnectionStatus } from '$lib/wire/client';
import type { StreamEvent } from '$lib/pb/chatto/wire/v1/protocol_pb';

const RECONNECT_MS = 2_500;

export type WireEventHandler = (event: StreamEvent) => void;

export type ServerConnectionStatus = 'connected' | 'connecting' | 'disconnected';

export interface WireConnectionConfig {
  wireUrl: string;
  token: string | null;
}

export interface WireEventBus {
  handlers: SvelteSet<WireEventHandler>;
}

export class WireConnectionState {
  status = $state<ServerConnectionStatus>('connecting');
  reconnectCount = $state(0);
  failedAttempts = $state(0);
}

interface WireConnection {
  client: WireClient;
  state: WireConnectionState;
  reconnect(reason: string): void;
  stop(): void;
}

class WireEventBusManager {
  #buses = new SvelteMap<string, WireEventBus>();
  #connections = new Map<string, WireConnection>();

  startBus(serverId: string, config: WireConnectionConfig): () => void {
    if (this.#buses.has(serverId)) return () => {};

    const bus: WireEventBus = { handlers: new SvelteSet<WireEventHandler>() };
    const connection = startWireConnection(serverId, config, bus);
    this.#buses.set(serverId, bus);
    this.#connections.set(serverId, connection);
    return () => this.stopBus(serverId);
  }

  getBus(serverId: string): WireEventBus | undefined {
    return this.#buses.get(serverId);
  }

  getClient(serverId: string): WireClient | undefined {
    return this.#connections.get(serverId)?.client;
  }

  getState(serverId: string): WireConnectionState | undefined {
    return this.#connections.get(serverId)?.state;
  }

  reconnect(serverId: string, reason: string): void {
    this.#connections.get(serverId)?.reconnect(reason);
  }

  stopBus(serverId: string): void {
    this.#connections.get(serverId)?.stop();
    this.#connections.delete(serverId);
    this.#buses.delete(serverId);
  }
}

function startWireConnection(
  serverId: string,
  config: WireConnectionConfig,
  bus: WireEventBus
): WireConnection {
  const client = new WireClient({
    url: config.wireUrl,
    token: config.token
  });
  const state = new WireConnectionState();

  let stopped = false;
  let hasConnected = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  const clearReconnectTimer = () => {
    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = null;
  };

  const scheduleReconnect = (reason: string) => {
    if (stopped || reconnectTimer) return;
    state.status = 'disconnected';
    console.warn(`[wire:${serverId}] reconnecting after ${reason}`);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect(reason);
    }, RECONNECT_MS);
  };

  const connect = (reason: string) => {
    if (stopped) return;
    state.status = 'connecting';
    try {
      void client
        .connect({ resumeAfter: client.lastDeliveryCursor || undefined })
        .then((hello) => {
          if (hasConnected) state.reconnectCount++;
          hasConnected = true;
          state.status = 'connected';
          state.failedAttempts = 0;
          console.log(`[wire:${serverId}] connected`, {
            reason,
            protocolVersion: hello.protocolVersion,
            serverVersion: hello.serverVersion,
            methods: hello.methods,
            features: hello.features
          });
        })
        .catch((error: unknown) => {
          if (stopped || client.status === 'closed') return;
          state.failedAttempts++;
          state.status = 'disconnected';
          console.error(`[wire:${serverId}] connect failed`, wireErrorLogPayload(error));
          scheduleReconnect('connect failure');
        });
    } catch (error: unknown) {
      state.failedAttempts++;
      state.status = 'disconnected';
      console.error(`[wire:${serverId}] connect failed`, wireErrorLogPayload(error));
      scheduleReconnect('connect exception');
    }
  };

  const stopEventListener = client.onEvent((event) => {
    console.log(`[wire:${serverId}] event`, wireEventLogPayload(event));
    for (const handler of bus.handlers) {
      try {
        handler(event);
      } catch (err) {
        console.error(`[wire:${serverId}] handler threw`, err);
      }
    }
    try {
      client.ack(event);
    } catch (err) {
      console.debug(`[wire:${serverId}] event ack skipped`, wireErrorLogPayload(err));
    }
  });

  const stopErrorListener = client.onError((error) => {
    console.error(`[wire:${serverId}] error`, wireErrorLogPayload(error));
  });

  const stopCloseListener = client.onClose(() => {
    if (!stopped) {
      if (wireStatusToServerStatus(client.status) === 'disconnected') {
        state.status = 'disconnected';
      }
      scheduleReconnect('socket close');
    }
  });

  connect('initial start');

  return {
    client,
    state,
    reconnect(reason: string) {
      if (stopped) return;
      clearReconnectTimer();
      state.failedAttempts = 0;
      state.status = 'connecting';
      client.dispose();
      connect(reason);
    },
    stop() {
      stopped = true;
      clearReconnectTimer();
      stopEventListener();
      stopErrorListener();
      stopCloseListener();
      client.dispose();
      state.status = 'disconnected';
    }
  };
}

function wireStatusToServerStatus(status: WireConnectionStatus): ServerConnectionStatus {
  if (status === 'connected') return 'connected';
  if (status === 'connecting' || status === 'idle') return 'connecting';
  return 'disconnected';
}

function wireEventLogPayload(event: StreamEvent) {
  return {
    eventId: event.eventId,
    eventType: event.eventType,
    deliveryCursor: cursorDebug(event.deliveryCursor),
    payload: event.payload.case ?? null,
    invalidates: event.invalidates.map((hint) => ({
      kind: hint.kind,
      id: hint.id
    })),
    event
  };
}

function wireErrorLogPayload(error: unknown) {
  if (error instanceof WireProtocolError) {
    return {
      message: error.message,
      code: error.wireError?.code,
      requestId: error.wireError?.requestId,
      retryable: error.wireError?.retryable
    };
  }
  if (error instanceof Error) {
    return { message: error.message };
  }
  return error;
}

function cursorDebug(cursor: string) {
  if (!cursor) return { present: false };
  return {
    present: true,
    length: cursor.length,
    suffix: cursor.slice(-8)
  };
}

export const wireEventBusManager = new WireEventBusManager();
