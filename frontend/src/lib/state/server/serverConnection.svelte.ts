import { serverRegistry } from './registry.svelte';
import {
  wireEventBusManager,
  type ServerConnectionStatus,
  type WireConnectionConfig
} from './wireEventBus.svelte';

export type ConnectionStatus = ServerConnectionStatus;

export interface ServerConnectionConfig extends WireConnectionConfig {
  /** Registered server ID, used for bus status and manual reconnects. */
  serverId?: string;
  /** Human-readable endpoint used in debug logs. */
  label: string;
}

export function httpToWireUrl(httpUrl: string): string {
  const trimmed = httpUrl.replace(/\/$/, '');
  if (trimmed.endsWith('/api/wire')) return trimmed;
  if (trimmed.endsWith('/api/graphql')) {
    return `${trimmed.slice(0, -'/api/graphql'.length)}/api/wire`;
  }
  return `${trimmed}/api/wire`;
}

const HOME_WIRE_URL = '/api/wire';

export class ServerConnection {
  readonly wireUrl: string;
  readonly token: string | null;
  readonly serverId?: string;
  readonly #label: string;
  #lastVisibleAt = Date.now();
  #visibilityHandler: (() => void) | null = null;
  #onlineHandler: (() => void) | null = null;
  #suspendDetectorInterval: ReturnType<typeof setInterval> | null = null;

  constructor(config: ServerConnectionConfig) {
    this.wireUrl = config.wireUrl;
    this.token = config.token;
    this.serverId = config.serverId;
    this.#label = config.label;
    this.#installReconnectTriggers();
  }

  get status(): ConnectionStatus {
    return this.#state?.status ?? 'connecting';
  }

  get reconnectCount(): number {
    return this.#state?.reconnectCount ?? 0;
  }

  get isConnected(): boolean {
    return this.status === 'connected';
  }

  get showConnectionLostIcon(): boolean {
    return this.status === 'disconnected';
  }

  get showConnectionLostBanner(): boolean {
    return (this.#state?.failedAttempts ?? 0) >= 6;
  }

  forceReconnect(reason: string): void {
    if (!this.serverId) return;
    if (this.status === 'connecting') {
      console.debug('[wire:%s] force reconnect skipped; already connecting: %s', this.#label, reason);
      return;
    }
    console.debug('[wire:%s] force reconnect: %s', this.#label, reason);
    wireEventBusManager.reconnect(this.serverId, reason);
  }

  retry(): void {
    this.forceReconnect('user-initiated retry');
  }

  dispose(): void {
    if (this.#visibilityHandler && typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.#visibilityHandler);
      this.#visibilityHandler = null;
    }
    if (this.#onlineHandler && typeof window !== 'undefined') {
      window.removeEventListener('online', this.#onlineHandler);
      this.#onlineHandler = null;
    }
    if (this.#suspendDetectorInterval !== null) {
      clearInterval(this.#suspendDetectorInterval);
      this.#suspendDetectorInterval = null;
    }
  }

  get #state() {
    return this.serverId ? wireEventBusManager.getState(this.serverId) : undefined;
  }

  #installReconnectTriggers(): void {
    if (typeof document !== 'undefined') {
      this.#visibilityHandler = () => {
        if (document.visibilityState === 'visible') {
          const hiddenDuration = Date.now() - this.#lastVisibleAt;
          if (this.status === 'disconnected' || hiddenDuration > 30_000) {
            this.forceReconnect(`tab visible after ${Math.round(hiddenDuration / 1000)}s hidden`);
          }
          this.#lastVisibleAt = Date.now();
        } else {
          this.#lastVisibleAt = Date.now();
        }
      };
      document.addEventListener('visibilitychange', this.#visibilityHandler);
    }

    if (typeof window !== 'undefined') {
      let lastTick = Date.now();
      this.#suspendDetectorInterval = setInterval(() => {
        const now = Date.now();
        const gap = now - lastTick;
        lastTick = now;
        if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
        if (gap > 30_000) {
          this.forceReconnect(`suspend detected (timer gap: ${Math.round(gap / 1000)}s)`);
        }
      }, 10_000);

      this.#onlineHandler = () => {
        this.forceReconnect('network came back online');
      };
      window.addEventListener('online', this.#onlineHandler);
    }
  }
}

class ServerConnectionManager {
  #clients = new Map<string, ServerConnection>();
  #originClient: ServerConnection | null = null;
  #originClientToken: string | null = null;
  #originClientServerId: string | undefined;

  get originClient(): ServerConnection {
    const origin = serverRegistry.originServer;
    const token = origin?.token ?? null;
    const serverId = origin?.id;
    if (
      this.#originClient &&
      this.#originClientToken === token &&
      this.#originClientServerId === serverId
    ) {
      return this.#originClient;
    }

    this.#originClient?.dispose();
    this.#originClient = new ServerConnection({
      wireUrl: HOME_WIRE_URL,
      token,
      serverId,
      label: 'origin'
    });
    this.#originClientToken = token;
    this.#originClientServerId = serverId;
    return this.#originClient;
  }

  getClient(serverId: string): ServerConnection {
    if (serverRegistry.isOriginServer(serverId)) {
      return this.originClient;
    }

    const existing = this.#clients.get(serverId);
    if (existing) return existing;

    const server = serverRegistry.getServer(serverId);
    if (!server) {
      throw new Error(`Server "${serverId}" not found in registry`);
    }

    const client = new ServerConnection({
      wireUrl: httpToWireUrl(server.url),
      token: server.token,
      serverId,
      label: server.url
    });

    this.#clients.set(serverId, client);
    return client;
  }

  destroyClient(serverId: string): boolean {
    if (serverRegistry.isOriginServer(serverId)) {
      if (!this.#originClient) return false;
      this.#originClient.dispose();
      this.#originClient = null;
      this.#originClientToken = null;
      this.#originClientServerId = undefined;
      return true;
    }

    const client = this.#clients.get(serverId);
    if (!client) return false;

    client.dispose();
    this.#clients.delete(serverId);
    return true;
  }
}

export const serverConnectionManager = new ServerConnectionManager();
