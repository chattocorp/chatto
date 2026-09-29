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
import { Code, ConnectError, type Client, type Interceptor } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { createChattoClient as createServiceClient, type ConnectAPIConfig } from './api/connect.js';
import { effect, effectRoot, signal, untrack } from './reactivity/index.js';
import { eventBusManager } from './server/realtimeTransport.js';
import { generateServerId, serverRegistry } from './server/registry.js';
import { startClientRuntime } from './server/runtime.js';
import type { ServerConnection } from './server/serverConnection.js';
import { serverConnectionManager } from './server/serverConnection.js';
import { emptyServerSession } from './server/sessions.js';
import { parseServerUrl } from './util/serverUrl.js';
import type { ServerStateStore } from './server/store.js';

/** Settings for {@link connectChatto}. */
export interface ConnectChattoOptions {
  /** HTTP or HTTPS origin of the Chatto server, without credentials. */
  serverUrl: string;
  /** Bearer token, for example a bot API key. It is kept only in memory. */
  apiKey: string;
}

/** A projection reset; see {@link ChattoConnection.onReset}. */
export interface ChattoReset {
  /** Events since the previous connection can be missing. */
  readonly gap: boolean;
}

/** Realtime status of a connection. */
export type ChattoConnectionStatus = ServerConnection['status'];

/** A connected Chatto server. Close it when the host no longer needs it. */
export interface ChattoConnection {
  /** Registry ID of the server. */
  readonly serverId: string;
  /** The server's reactive state store, as the frontend uses it. */
  readonly store: ServerStateStore;
  /**
   * The server's connection: endpoints, status, and API facades. Available
   * only while the connection is open.
   */
  readonly connection: ServerConnection;
  /** Realtime status of the open connection, or `disconnected` after close. Reactive. */
  readonly status: ChattoConnectionStatus;
  /**
   * Wait until the server accepted the token and the viewer loaded. Rejects
   * when the server rejects the token, when a viewer read fails (for example
   * because the server is unreachable or returned an error), when discovery
   * reports a server release that this client does not support, when the
   * connection closes, or when `signal` aborts.
   *
   * The connection retries a failed viewer read in the background, with a
   * backoff. A `ready()` call made between two attempts waits for the next
   * attempt and reports its result, so a host can call `ready()` again after
   * a rejection without a delay of its own.
   */
  ready(options?: { signal?: AbortSignal }): Promise<{ viewerId: string }>;
  /**
   * Whether the server ended the session, for example because it rejected or
   * revoked the token. The connection does not recover; close it. Reactive.
   */
  readonly sessionEnded: boolean;
  /** Whether {@link close} was called. Reactive. */
  readonly closed: boolean;
  /**
   * Create a typed Connect client for a public service, with this
   * connection's authentication. The fixed token always belongs to the same
   * account, so projection resets do not fail these requests. After
   * `close()`, clients send nothing, responses in flight fail, and this
   * method throws a `Canceled` `ConnectError`.
   */
  service<T extends ServiceType>(service: T): Client<T>;
  /**
   * Receive semantic realtime events after the store applied them. Listeners
   * run in order and must not throw; handle asynchronous work yourself.
   * Realtime can start before `ready()` resolves, so add listeners before
   * you wait for it. A listener error is logged and does not stop other
   * listeners. Returns a function that removes the listener.
   */
  onEvent(listener: (event: RealtimeEvent) => void): () => void;
  /**
   * Receive projection resets. The first reset delivers the initial snapshot.
   * `gap` is true when a later reset replaced a stream that the server could
   * not resume, so events can have been missed. A resync that publishes
   * several resets before the next connection reports one gap.
   */
  onReset(listener: (reset: ChattoReset) => void): () => void;
  /**
   * Stop realtime delivery and remove the server and its state. Requests in
   * flight through this connection then fail, even when the server applied
   * them. Use `createChattoApi` for work that can outlive the connection.
   */
  close(): void;
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
 * for example with new credentials. The connection runs its own client
 * runtime, so it throws in an application that already runs one.
 */
export function connectChatto(options: ConnectChattoOptions): ChattoConnection {
  const url = parseServerUrl(options.serverUrl);
  if (!options.apiKey) throw new Error('A Chatto API key is required');
  if (openConnection) throw new Error('Close the open Chatto connection before connecting again');
  // A browser page on the server's own origin uses its cookie session for
  // that server, never a fixed token.
  if (typeof window !== 'undefined' && window.location?.origin === url.origin) {
    throw new Error("connectChatto cannot connect the page's own origin");
  }
  const serverId = generateServerId(url.origin, [
    ...serverRegistry.servers.map((server) => server.id),
    ...usedServerIds
  ]);
  // Throws when an application runtime already runs in this process.
  const runtime = startClientRuntime();
  usedServerIds.add(serverId);
  try {
    serverRegistry.addServer(
      { id: serverId, url: url.origin, name: url.host, iconUrl: null, addedAt: Date.now() },
      { ...emptyServerSession(), token: options.apiKey },
      { fixedToken: true }
    );
  } catch (error) {
    runtime.stop();
    throw error;
  }
  runtime.setActiveServer(serverId);

  const eventListeners = new Set<(event: RealtimeEvent) => void>();
  const resetListeners = new Set<(reset: ChattoReset) => void>();
  let resets = 0;
  // Whether the stream was connected after the previous reset. A resync
  // publishes a reset and then a snapshot before the next connection.
  let connectedSinceReset = false;
  const closed = signal(false);
  /** Call each listener; one failing listener does not stop the others. */
  const notify = <T>(listeners: Set<(value: T) => void>, value: T) => {
    for (const listener of [...listeners]) {
      try {
        listener(value);
      } catch (error) {
        console.error('[chatto-client] a realtime listener failed', error);
      }
    }
  };

  /** Realtime status; never creates a connection for a server that close() removed. */
  const currentStatus = (): ChattoConnectionStatus => {
    if (closed.get() || !serverRegistry.tryGetStore(serverId)) return 'disconnected';
    return serverConnectionManager.getClient(serverId).status;
  };

  // The event bus starts when the server is authenticated and discovery
  // finished, which can be before the viewer loads. Subscribe whenever the
  // runtime creates (or replaces) this server's bus.
  const disposeBusSubscription = effectRoot(() => {
    effect(() => {
      if (currentStatus() === 'connected') connectedSinceReset = true;
    });
    effect(() => {
      const bus = eventBusManager.getBus(serverId);
      if (!bus) return;
      return untrack(() =>
        bus.subscribe((update) => {
          if (update.reset) {
            resets++;
            const reset: ChattoReset = { gap: resets > 1 && connectedSinceReset };
            connectedSinceReset = false;
            notify(resetListeners, reset);
          }
          const event = update.event;
          if (event) notify(eventListeners, event);
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
    get status(): ChattoConnectionStatus {
      return currentStatus();
    },
    get sessionEnded() {
      return (serverRegistry.getServer(serverId)?.reauthRequiredAt ?? null) !== null;
    },
    get closed() {
      return closed.get();
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
        // Report only failures of attempts that start after this call: each
        // attempt clears its error first. Between two recovery attempts, wait
        // for the next one.
        let discoveryAttempted = false;
        let viewerAttempted = false;
        stop = effectRoot(() => {
          effect(() => {
            if (closed.get()) {
              finish(() => reject(new Error('The Chatto connection is closed')));
              return;
            }
            const server = serverRegistry.getServer(serverId);
            const current = serverRegistry.tryGetStore(serverId);
            if (!server || !current) return;
            if (server.reauthRequiredAt !== null) {
              finish(() => reject(new Error('Chatto rejected the API key')));
              return;
            }
            const { serverInfo, currentUser } = current;
            const viewerId = current.accountId;
            if (viewerId) {
              // Discovery that fails does not block requests, but a server
              // release without a supported realtime projection sends no events.
              if (serverInfo.loading) return;
              if (serverInfo.error === null && !serverInfo.isSupportedVersion) {
                finish(() => reject(new Error('The Chatto server version is not supported')));
              } else {
                finish(() => resolve({ viewerId }));
              }
              return;
            }
            // The server is unreachable or failed. The store logs the error.
            const discoveryFailed = serverInfo.error !== null;
            const viewerFailed = currentUser.loadError !== null;
            discoveryAttempted ||= !discoveryFailed;
            viewerAttempted ||= !viewerFailed;
            if (viewerFailed && viewerAttempted) {
              finish(() => reject(new Error('Could not load the viewer from the Chatto server')));
            } else if (discoveryFailed && discoveryAttempted && !currentUser.loading) {
              // A viewer read in progress can still succeed without discovery.
              finish(() => reject(new Error('Could not reach the Chatto server')));
            }
          });
        });
      });
    },
    service<T extends ServiceType>(service: T): Client<T> {
      if (closed.peek()) throw new ConnectError('The Chatto connection is closed', Code.Canceled);
      const base = serverConnectionManager.getClient(serverId).apiConfig;
      // After close(), send nothing: the token must not outlive the connection.
      const refuseAfterClose: Interceptor = (next) => (request) => {
        if (closed.peek()) throw new ConnectError('The Chatto connection is closed', Code.Canceled);
        return next(request);
      };
      const send =
        base.transport ??
        ((interceptors: Interceptor[]) =>
          createConnectTransport({ baseUrl: base.baseUrl, useBinaryFormat: true, interceptors }));
      const config: ConnectAPIConfig = {
        ...base,
        // Fail responses that arrive after close(), not after a privacy
        // reset: the host, not a shared cache, receives these responses.
        dataGeneration: () => (closed.peek() ? 1 : 0),
        transport: (interceptors) => send([refuseAfterClose, ...interceptors])
      };
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
      if (closed.peek()) return;
      closed.set(true);
      if (openConnection === connection) openConnection = null;
      disposeBusSubscription();
      runtime.stop();
      serverRegistry.removeServer(serverId);
    }
  };
  openConnection = connection;
  return connection;
}
