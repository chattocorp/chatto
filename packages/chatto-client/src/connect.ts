/**
 * Connect one Chatto server with a fixed bearer token, such as a bot API key.
 *
 * This is the entry point for bots, integrations, and other headless hosts.
 * It uses the same registry, server store, realtime transport, and client
 * runtime as the bundled frontend. The server becomes the runtime's active
 * server, so its realtime projection stays live.
 *
 * The host must provide Fetch and WebSocket (Node 22 and later do). Requests
 * contact only the configured server, which receives the host's IP address,
 * the token, and the request data.
 */

import type { ServiceType } from '@bufbuild/protobuf';
import type { Client } from '@connectrpc/connect';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { createChattoClient as createServiceClient, type ConnectAPIConfig } from './api/connect.js';
import { effect, effectRoot, untrack } from './reactivity/index.js';
import type { RealtimeProjectionUpdate } from './realtime/eventBus.js';
import { eventBusManager } from './server/realtimeTransport.js';
import { generateServerId, serverRegistry } from './server/registry.js';
import { startClientRuntime } from './server/runtime.js';
import type { ServerConnection } from './server/serverConnection.js';
import { serverConnectionManager } from './server/serverConnection.js';
import { emptyServerSession } from './server/sessions.js';
import type { ServerStateStore } from './server/store.js';

/** Settings for {@link connectChatto}. */
export interface ConnectChattoOptions {
  /** HTTP or HTTPS origin of the Chatto server, without credentials. */
  serverUrl: string;
  /** Bearer token, for example a bot API key. It is kept only in memory. */
  apiKey: string;
}

/** Realtime status of a connection. */
export type ChattoConnectionStatus = ServerConnection['status'];

/** A connected Chatto server. Close it when the host no longer needs it. */
export interface ChattoConnection {
  /** Registry ID of the server. */
  readonly serverId: string;
  /** The server's reactive state store, as the frontend uses it. */
  readonly store: ServerStateStore;
  /** The server's connection: endpoints, status, and API facades. */
  readonly connection: ServerConnection;
  /**
   * Wait until the server accepted the token and the viewer loaded. Rejects
   * when the server rejects the token or when `signal` aborts.
   */
  ready(options?: { signal?: AbortSignal }): Promise<{ viewerId: string }>;
  /**
   * Create a typed Connect client for a public service, with this
   * connection's authentication and privacy fences.
   */
  service<T extends ServiceType>(service: T): Client<T>;
  /**
   * Receive semantic realtime events after the store applied them. Listeners
   * run in order and must not throw; handle asynchronous work yourself.
   * Returns a function that removes the listener.
   */
  onEvent(listener: (event: RealtimeEvent) => void): () => void;
  /**
   * Receive projection resets. A reset after the first one means that the
   * server could not resume the stream, so events can have been missed.
   */
  onReset(listener: (update: RealtimeProjectionUpdate) => void): () => void;
  /** Stop realtime delivery and remove the server and its state. */
  close(): void;
}

function parseServerUrl(value: string): URL {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) {
    throw new Error('Use an HTTP or HTTPS Chatto server URL without credentials');
  }
  return url;
}

/** The open connection. The client runtime keeps one server live at a time. */
let openConnection: ChattoConnection | null = null;

/**
 * Server IDs of earlier connections. A new connection never reuses one, so a
 * late request of a closed connection cannot affect a newer session.
 */
const usedServerIds = new Set<string>();

/**
 * Register a server with a fixed token and keep its realtime projection live.
 * A process can have one open connection; close it before connecting again,
 * for example with new credentials.
 */
export function connectChatto(options: ConnectChattoOptions): ChattoConnection {
  const url = parseServerUrl(options.serverUrl);
  if (!options.apiKey) throw new Error('A Chatto API key is required');
  if (openConnection) throw new Error('Close the open Chatto connection before connecting again');
  const serverId = generateServerId(url.origin, [
    ...serverRegistry.servers.map((server) => server.id),
    ...usedServerIds
  ]);
  usedServerIds.add(serverId);
  serverRegistry.addServer(
    { id: serverId, url: url.origin, name: url.host, iconUrl: null, addedAt: Date.now() },
    { ...emptyServerSession(), token: options.apiKey }
  );
  const runtime = startClientRuntime();
  runtime.setActiveServer(serverId);

  const eventListeners = new Set<(event: RealtimeEvent) => void>();
  const resetListeners = new Set<(update: RealtimeProjectionUpdate) => void>();
  let closed = false;

  // The event bus starts once the viewer loaded. Subscribe whenever the
  // runtime creates (or replaces) this server's bus.
  const disposeBusSubscription = effectRoot(() => {
    effect(() => {
      const bus = eventBusManager.getBus(serverId);
      if (!bus) return;
      return untrack(() =>
        bus.subscribe((update) => {
          if (update.reset) for (const listener of [...resetListeners]) listener(update);
          const event = update.event;
          if (event) for (const listener of [...eventListeners]) listener(event);
        })
      );
    });
  });

  const store = () => serverRegistry.getStore(serverId);

  const connection: ChattoConnection = {
    serverId,
    get store() {
      return store();
    },
    get connection() {
      return serverConnectionManager.getClient(serverId);
    },
    ready({ signal } = {}) {
      return new Promise((resolve, reject) => {
        signal?.throwIfAborted();
        let stop: (() => void) | undefined;
        const finish = (settle: () => void) => {
          queueMicrotask(() => stop?.());
          signal?.removeEventListener('abort', abort);
          settle();
        };
        const abort = () => finish(() => reject(signal!.reason));
        signal?.addEventListener('abort', abort, { once: true });
        stop = effectRoot(() => {
          effect(() => {
            if (closed) return;
            const server = serverRegistry.getServer(serverId);
            const current = serverRegistry.tryGetStore(serverId);
            if (!server || !current) return;
            if (server.reauthRequiredAt !== null) {
              finish(() => reject(new Error('Chatto rejected the API key')));
              return;
            }
            const viewerId = current.accountId;
            if (viewerId) finish(() => resolve({ viewerId }));
          });
        });
      });
    },
    service<T extends ServiceType>(service: T): Client<T> {
      const config: ConnectAPIConfig = serverConnectionManager.getClient(serverId).apiConfig;
      return createServiceClient(service, config);
    },
    onEvent(listener) {
      eventListeners.add(listener);
      return () => eventListeners.delete(listener);
    },
    onReset(listener) {
      resetListeners.add(listener);
      return () => resetListeners.delete(listener);
    },
    close() {
      if (closed) return;
      closed = true;
      if (openConnection === connection) openConnection = null;
      disposeBusSubscription();
      runtime.stop();
      serverRegistry.removeServer(serverId);
    }
  };
  openConnection = connection;
  return connection;
}
