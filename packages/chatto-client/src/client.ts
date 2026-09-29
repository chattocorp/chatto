/**
 * A Chatto client: an isolated set of servers with their registry, stores,
 * connections, realtime transports, and runtime.
 *
 * Create one client per independent host, such as a bot, or one per page in
 * an application with a UI. Clients in one process share nothing that one
 * of them could change for another: each server belongs to exactly one
 * client.
 */

import { parseServerUrl } from './util/serverUrl.js';
import { Connection, type ConnectOptions } from './connection.js';
import { EventBusManager, type LiveServers } from './server/realtimeTransport.js';
import { ServerRegistry } from './server/registry.js';
import { ServerConnectionManager } from './server/serverConnection.js';
import { startClientRuntime, type ClientRuntime } from './server/runtime.js';
import { emptyServerSession } from './server/sessions.js';
import { detachedVoiceCallFactory, type VoiceCallFactory } from './server/voiceCall.js';

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
  /** Creates the voice-call controller of each server store. Default: no call media. */
  voiceCall?: VoiceCallFactory;
}

/** Clients that use device storage or the origin server; a process can have one of each. */
const exclusiveClients = new Map<'deviceStorage' | 'originServer', ChattoClient>();
/** Numbers the server IDs of connections, so that no ID is used twice in a process. */
let connectionCount = 0;

/** An isolated Chatto client; see the module documentation. */
export class ChattoClient {
  /** The client's servers, sessions, and stores. */
  readonly registry: ServerRegistry;
  /** The client's server connections: endpoints, tokens, and status. */
  readonly connections: ServerConnectionManager;
  /** The client's event buses and realtime transports. */
  readonly realtime: EventBusManager;
  readonly #options: ClientOptions;
  readonly #openConnections = new Set<Connection>();
  #runtime: ClientRuntime | null = null;
  /** Whether the application started the runtime; see {@link start}. */
  #startedExplicitly = false;
  #activeServerId: string | null = null;
  #closed = false;

  /** Use {@link createClient}. */
  constructor(options: ClientOptions = {}) {
    if (options.storage === 'device' && exclusiveClients.has('deviceStorage')) {
      throw new Error('A process can have one Chatto client with device storage');
    }
    if (options.originServer && exclusiveClients.has('originServer')) {
      throw new Error('A process can have one Chatto client for the origin server');
    }
    this.#options = options;
    this.realtime = new EventBusManager({ liveServers: options.liveServers ?? 'all' });
    this.connections = new ServerConnectionManager(() => this.registry);
    this.registry = new ServerRegistry(
      {
        connections: this.connections,
        realtime: this.realtime,
        voiceCall: options.voiceCall ?? detachedVoiceCallFactory
      },
      { deviceStorage: options.storage === 'device', originServer: options.originServer ?? false }
    );
    if (options.storage === 'device') exclusiveClients.set('deviceStorage', this);
    if (options.originServer) exclusiveClients.set('originServer', this);
  }

  /**
   * Start the runtime: recovery, realtime transports, and session-termination
   * handling. Applications with a UI call it once. {@link connect} starts it
   * by itself and stops it when its last connection closes.
   */
  start(): void {
    this.#startedExplicitly = true;
    this.#ensureRuntime();
  }

  /**
   * Stop the runtime that {@link start} started. The runtime keeps running
   * while connections are open; it stops when the last one closes.
   */
  stop(): void {
    this.#startedExplicitly = false;
    if (this.#openConnections.size === 0) this.#stopRuntime();
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
   * Connect a server with a fixed bearer token, such as a bot API key. The
   * token is never renewed or written to device storage. A client can hold
   * several connections, also to the same server with different tokens.
   */
  connect(options: ConnectOptions): Connection {
    if (this.#closed) throw new Error('The Chatto client is closed');
    const url = parseServerUrl(options.serverUrl);
    if (!options.apiKey) throw new Error('A Chatto API key is required');
    // The origin server uses the page's cookie session, never a fixed token.
    if (
      this.#options.originServer &&
      typeof window !== 'undefined' &&
      window.location?.origin === url.origin
    ) {
      throw new Error("A client for the origin server cannot connect the page's own origin");
    }
    // Never reuse an ID: a late request of a closed connection cannot affect a newer one.
    const serverId = `${url.hostname.replace(/[^a-z0-9-]/gi, '-')}~${++connectionCount}`;
    this.registry.addServer(
      { id: serverId, url: url.origin, name: url.host, iconUrl: null, addedAt: Date.now() },
      { ...emptyServerSession(), token: options.apiKey },
      { fixedToken: true }
    );
    let connection: Connection;
    try {
      connection = new Connection(serverId, url.origin, {
        registry: this.registry,
        connections: this.connections,
        realtime: this.realtime,
        onClose: (closed) => this.#connectionClosed(closed)
      });
      this.#openConnections.add(connection);
      this.#ensureRuntime();
    } catch (error) {
      this.registry.removeServer(serverId);
      throw error;
    }
    return connection;
  }

  /**
   * Close every connection, stop the runtime and all realtime transports, and
   * release the servers. Device storage is left as it is. The client cannot
   * be used afterwards.
   */
  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    for (const connection of [...this.#openConnections]) connection.close();
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

  #connectionClosed(connection: Connection): void {
    this.#openConnections.delete(connection);
    // Without work left, no timer must keep a Node host alive.
    if (!this.#startedExplicitly && this.#openConnections.size === 0) this.#stopRuntime();
  }
}

/**
 * Create an isolated Chatto client.
 *
 * ```ts
 * const client = createClient();
 * const connection = client.connect({ serverUrl, apiKey });
 * await connection.run((ctx) => ctx.reply(`Hello, ${ctx.message.authorId}`));
 * ```
 */
export function createClient(options: ClientOptions = {}): ChattoClient {
  return new ChattoClient(options);
}
