/**
 * A Chatto client: an isolated set of servers with their registry, realtime
 * transports, and runtime.
 *
 * Create one client per independent host, such as a bot, or one per page in
 * an application with a UI. Clients in one process share nothing that one
 * of them could change for another: each server belongs to exactly one
 * client.
 */

import { parseServerUrl } from './util/serverUrl.js';
import type { ConnectOptions, Server } from './server/server.js';
import { EventBusManager, type LiveServers } from './server/realtimeTransport.js';
import { ServerRegistry } from './server/registry.js';
import { ServerConnectionManager } from './server/serverConnection.js';
import { startClientRuntime, type ClientRuntime } from './server/runtime.js';
import { emptyServerSession } from './server/sessions.js';

/** Settings for {@link createClient}. */
export interface ClientOptions {
  /**
   * Where the client keeps its server catalogue and renewable sessions.
   * `memory` (the default) keeps nothing between runs. `device` uses the
   * browser's local storage and restores the servers at creation; a process
   * can have one such client.
   */
  storage?: 'memory' | 'device';
  /**
   * Treat a server on the page's own origin as the origin server, which uses
   * the browser's cookie session. Default: false. A process can have one
   * such client.
   */
  originServer?: boolean;
  /**
   * Which servers keep a persistent WebSocket: `all` (the default), or only
   * the server that {@link ChattoClient.setActiveServer} selects. The others
   * catch up by polling.
   */
  liveServers?: LiveServers;
}

/** Clients that use device storage or the origin server; a process can have one of each. */
const exclusiveClients = new Map<'deviceStorage' | 'originServer', ChattoClient>();
/** Numbers the server IDs of `connect`, so that no ID is used twice in a process. */
let connectionCount = 0;

/** An isolated Chatto client; see the module documentation. */
export class ChattoClient {
  /** The client's servers, sessions, and stores. */
  readonly registry: ServerRegistry;
  /** The client's server connections: endpoints, tokens, and status. */
  readonly connections: ServerConnectionManager;
  /** The client's event buses and realtime transports. */
  readonly realtime: EventBusManager;
  /** Servers that {@link connect} added and that are not closed. */
  readonly #connected = new Set<Server>();
  #runtime: ClientRuntime | null = null;
  /** Calls of {@link start} without a matching {@link stop}. */
  #starts = 0;
  #activeServerId: string | null = null;
  #closed = false;

  /** Use {@link createClient}. */
  constructor(options: ClientOptions) {
    if (options.storage === 'device' && exclusiveClients.has('deviceStorage')) {
      throw new Error('A process can have one Chatto client with device storage');
    }
    if (options.originServer && exclusiveClients.has('originServer')) {
      throw new Error('A process can have one Chatto client for the origin server');
    }
    this.realtime = new EventBusManager({ liveServers: options.liveServers ?? 'all' });
    this.connections = new ServerConnectionManager(() => this.registry);
    this.registry = new ServerRegistry(
      { connections: this.connections, realtime: this.realtime },
      { deviceStorage: options.storage === 'device', originServer: options.originServer ?? false }
    );
    if (options.storage === 'device') exclusiveClients.set('deviceStorage', this);
    if (options.originServer) exclusiveClients.set('originServer', this);
  }

  /**
   * Start the runtime: recovery, realtime transports, and session-termination
   * handling. Applications with a UI call it when they mount. {@link connect}
   * starts it by itself. Each call needs a matching {@link stop}.
   */
  start(): void {
    if (this.#closed) throw new Error('The Chatto client is closed');
    this.#starts++;
    this.#ensureRuntime();
  }

  /**
   * End one {@link start}. The runtime stops when every start has ended and
   * no server that {@link connect} added is open.
   */
  stop(): void {
    this.#starts = Math.max(0, this.#starts - 1);
    this.#stopRuntimeWhenIdle();
  }

  /**
   * Select the server that keeps a persistent WebSocket when
   * `liveServers` is `selected`, or null for none.
   */
  setActiveServer(serverId: string | null): void {
    this.#activeServerId = serverId;
    this.#runtime?.setActiveServer(serverId);
  }

  /**
   * Add a server with a fixed bearer token, such as a bot API key, and keep
   * its realtime stream live. The token is never renewed or written to device
   * storage. A client can add a server several times, also with different
   * tokens; each call adds a separate server with its own ID. The server
   * keeps its realtime events for the first `consumeEvents` or `run` call.
   */
  connect(options: ConnectOptions): Server {
    if (this.#closed) throw new Error('The Chatto client is closed');
    const url = parseServerUrl(options.serverUrl);
    if (!options.apiKey) throw new Error('A Chatto API key is required');
    // Requests to the page's own origin also carry its cookie session, so a
    // request would have two credentials. That server uses the cookie session.
    if (typeof window !== 'undefined' && window.location?.origin === url.origin) {
      throw new Error('A browser page cannot connect its own origin with an API key');
    }
    // Never reuse an ID: a late request of a closed server cannot affect a newer one.
    const serverId = `${url.hostname.replace(/[^a-z0-9-]/gi, '-')}~${++connectionCount}`;
    this.registry.addServer(
      { id: serverId, url: url.origin, name: url.host, iconUrl: null, addedAt: Date.now() },
      { ...emptyServerSession(), token: options.apiKey },
      { fixedToken: true }
    );
    const server = this.registry.getStore(serverId);
    try {
      // Realtime can start before ready() resolves; keep events for the first consumer.
      server.retainEvents();
      // A server that a host handles events of stays live in every client.
      this.realtime.keepLive(serverId);
      this.#connected.add(server);
      server.onDispose(() => {
        this.realtime.keepLive(serverId, false);
        this.#connected.delete(server);
        // Without work left, no timer must keep a Node host alive.
        this.#stopRuntimeWhenIdle();
      });
      this.#ensureRuntime();
    } catch (error) {
      // Leave nothing behind: no server, live transport, or subscription.
      server.close();
      throw error;
    }
    return server;
  }

  /** A server of this client by ID, or undefined. */
  server(serverId: string): Server | undefined {
    return this.registry.tryGetStore(serverId);
  }

  /**
   * Close every server that {@link connect} added, stop the runtime and all
   * realtime transports, and release the servers. Device storage is left as
   * it is. The client cannot be used afterwards.
   */
  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    for (const server of [...this.#connected]) server.close();
    this.#stopRuntime();
    this.realtime.stopAll();
    this.registry.dispose();
    for (const [kind, owner] of exclusiveClients) {
      if (owner === this) exclusiveClients.delete(kind);
    }
  }

  #ensureRuntime(): void {
    if (this.#runtime) return;
    const runtime = startClientRuntime(this);
    runtime.setActiveServer(this.#activeServerId);
    this.#runtime = runtime;
  }

  #stopRuntime(): void {
    this.#runtime?.stop();
    this.#runtime = null;
  }

  /** Stop the runtime when no start and no connected server needs it; no timer then keeps Node alive. */
  #stopRuntimeWhenIdle(): void {
    if (this.#starts === 0 && this.#connected.size === 0) this.#stopRuntime();
  }
}

/**
 * Create an isolated Chatto client.
 *
 * ```ts
 * const client = createClient();
 * const server = client.connect({ serverUrl, apiKey });
 * await server.run((ctx) => ctx.reply(`Hello, ${ctx.message.authorId}`));
 * ```
 */
export function createClient(options: ClientOptions = {}): ChattoClient {
  return new ChattoClient(options);
}
