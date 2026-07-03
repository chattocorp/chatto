import { isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
import { serverRegistry } from './registry.svelte';

export type ConnectionStatus = 'connected' | 'connecting' | 'disconnected';

export interface ServerConnectionConfig {
  /** Server base URL (relative for origin, absolute for remote). */
  serverUrl: string;
  /** Bearer token for Connect/realtime auth, or null for origin cookie auth. */
  token: string | null;
  /** Registered server ID, used to clear stale credentials after auth failures */
  serverId?: string;
}

/** Construct a WebSocket URL from an HTTP URL (http→ws, https→wss). */
export function httpToWsUrl(httpUrl: string): string {
  return httpUrl.replace(/^http/, 'ws');
}

function hostFromServerUrl(url: string): string {
  if (url.startsWith('/')) {
    return typeof window !== 'undefined' ? window.location.host : 'localhost';
  }
  return url.match(/^[a-z][a-z0-9+.-]*:\/\/([^/?#]+)/i)?.[1] ?? url;
}

function originFromServerUrl(url: string): string {
  if (url.startsWith('/')) {
    return typeof window !== 'undefined' ? window.location.origin : 'http://localhost';
  }
  return new URL(url).origin;
}

function connectBaseUrlFromServerUrl(url: string): string {
  return new URL('/api/connect', originFromServerUrl(url)).toString();
}

function realtimeUrlFromServerUrl(url: string): string {
  return httpToWsUrl(new URL('/api/realtime', originFromServerUrl(url)).toString());
}

const ORIGIN_SERVER_URL = '/';

export class ServerConnection {
  status = $state<ConnectionStatus>('connecting');
  reconnectCount = $state(0);
  #failedAttempts = $state(0);
  #wasDisconnected = false;
  #lastVisibleAt = Date.now();
  #visibilityHandler: (() => void) | null = null;
  #onlineHandler: (() => void) | null = null;
  #suspendDetectorInterval: ReturnType<typeof setInterval> | null = null;
  #host: string;
  #connectBaseUrl: string;
  #realtimeUrl: string;
  #token: string | null;
  #serverId: string | undefined;
  #realtimeReconnect: ((reason: string) => void) | null = null;

  get isConnected() {
    return this.status === 'connected';
  }

  /** Show disconnection icon immediately when WebSocket is not connected */
  get showConnectionLostIcon() {
    return this.status === 'disconnected';
  }

  /** Show urgent (orange) disconnection indicator after 6 failed reconnection attempts (~30+ seconds) */
  get showConnectionLostBanner() {
    return this.#failedAttempts >= 6;
  }

  get connectBaseUrl(): string {
    return this.#connectBaseUrl;
  }

  get realtimeUrl(): string {
    return this.#realtimeUrl;
  }

  get bearerToken(): string | null {
    return this.#token;
  }

  get serverId(): string | undefined {
    return this.#serverId;
  }

  /** Force-terminate and immediately reconnect the WebSocket. */
  forceReconnect(reason: string) {
    if (this.#realtimeReconnect) {
      if (this.status === 'connecting') {
        console.log('[ws:%s] Force reconnect skipped — already connecting: %s', this.#host, reason);
        return;
      }
      console.log(
        '[ws:%s] Force realtime reconnect: %s (status: %s)',
        this.#host,
        reason,
        this.status
      );
      this.#failedAttempts = 0;
      this.#realtimeReconnect(reason);
      return;
    }

    if (this.status === 'connecting') {
      console.log('[ws:%s] Force reconnect skipped — already connecting: %s', this.#host, reason);
      return;
    }
    console.log(
      '[ws:%s] Force realtime reconnect skipped — no realtime stream is registered: %s',
      this.#host,
      reason
    );
  }

  /** Explicit user-initiated retry; equivalent to forceReconnect. */
  retry() {
    this.forceReconnect('user-initiated retry');
  }

  registerRealtimeReconnect(handler: (reason: string) => void): () => void {
    this.#realtimeReconnect = handler;
    return () => {
      if (this.#realtimeReconnect === handler) {
        this.#realtimeReconnect = null;
      }
    };
  }

  setRealtimeConnectionStatus(status: ConnectionStatus, failedAttempts = 0): void {
    if (status === 'connecting') {
      if (this.status === 'disconnected') {
        this.#wasDisconnected = true;
      }
      this.status = 'connecting';
      this.#failedAttempts = failedAttempts;
      return;
    }

    if (status === 'connected') {
      console.log(
        '[ws:%s] Connected (prev status: %s, wasDisconnected: %s)',
        this.#host,
        this.status,
        this.#wasDisconnected
      );
      if (this.#wasDisconnected) {
        this.#wasDisconnected = false;
        this.reconnectCount++;
        console.log('[ws:%s] Reconnected (count: %d)', this.#host, this.reconnectCount);
      }
      this.status = 'connected';
      this.#failedAttempts = 0;
      return;
    }

    if (this.status === 'connected') {
      this.#wasDisconnected = true;
    }
    this.status = 'disconnected';
    this.#failedAttempts = failedAttempts;
  }

  handleAuthenticationRequired(): void {
    if (this.#serverId) {
      if (
        isExplicitSignOutRedirectInProgress() &&
        serverRegistry.isOriginServer(this.#serverId)
      ) {
        return;
      }
      serverRegistry.handleAuthenticationRequired(this.#serverId);
    }
  }

  constructor(config: ServerConnectionConfig) {
    const { serverUrl, token, serverId } = config;
    this.#host = hostFromServerUrl(serverUrl);
    this.#connectBaseUrl = connectBaseUrlFromServerUrl(serverUrl);
    this.#realtimeUrl = realtimeUrlFromServerUrl(serverUrl);
    this.#token = token;
    this.#serverId = serverId;

    // Reconnect when tab becomes visible after being backgrounded.
    // If the tab was hidden for >30s, force-terminate the WebSocket regardless of
    // reported status. This catches silently-dead connections where the OS killed
    // the socket during sleep but the client never received a close event.
    if (typeof document !== 'undefined') {
      this.#visibilityHandler = () => {
        if (document.visibilityState === 'visible') {
          const hiddenDuration = Date.now() - this.#lastVisibleAt;

          if (this.status === 'disconnected' || hiddenDuration > 30_000) {
            console.debug(
              '[ws:%s] visibility=visible after %ds hidden, status=%s → forceReconnect',
              this.#host,
              Math.round(hiddenDuration / 1000),
              this.status
            );
            this.forceReconnect(`tab visible after ${Math.round(hiddenDuration / 1000)}s hidden`);
          } else {
            console.debug(
              '[ws:%s] visibility=visible after %ds hidden, status=%s → no reconnect',
              this.#host,
              Math.round(hiddenDuration / 1000),
              this.status
            );
          }

          this.#lastVisibleAt = Date.now();
        } else {
          this.#lastVisibleAt = Date.now();
        }
      };
      document.addEventListener('visibilitychange', this.#visibilityHandler);
    }

    // Detect wake from OS-level sleep/suspend via timer gap. When the JS
    // event loop is frozen (lid close, phone lock), setInterval callbacks
    // don't fire. On wake the first callback fires with a large actual gap.
    //
    // Background-tab throttling produces the same signal (Chrome/Firefox
    // throttle setInterval to ~1/min in hidden tabs), so the gap is only
    // meaningful while the tab is visible. The visibility handler covers
    // the hidden case on resume.
    if (typeof window !== 'undefined') {
      let lastTick = Date.now();
      this.#suspendDetectorInterval = setInterval(() => {
        const now = Date.now();
        const gap = now - lastTick;
        lastTick = now;
        if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
        if (gap > 30_000) {
          console.debug(
            '[ws:%s] Suspend detector fired (timer gap %ds)',
            this.#host,
            Math.round(gap / 1000)
          );
          this.forceReconnect(`suspend detected (timer gap: ${Math.round(gap / 1000)}s)`);
        }
      }, 10_000);

      // Reconnect when network comes back online (e.g., after airplane mode
      // or Wi-Fi re-association following sleep).
      this.#onlineHandler = () => {
        console.debug('[ws:%s] online event fired', this.#host);
        this.forceReconnect('network came back online');
      };
      window.addEventListener('online', this.#onlineHandler);
    }
  }

  /** Clean up event listeners owned by the connection state object. */
  dispose() {
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
}

/**
 * Manages Connect/realtime connection state for multiple Chatto instances.
 * The origin connection is created eagerly; remote connections are created
 * lazily on first access.
 */
class ServerConnectionManager {
  #clients = new Map<string, ServerConnection>();
  #originClient: ServerConnection | null = null;
  #originClientToken: string | null = null;
  #originClientServerId: string | undefined;

  /** The origin instance connection (serves the SPA, prefers bearer auth when available). */
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
      serverUrl: ORIGIN_SERVER_URL,
      token,
      serverId
    });
    this.#originClientToken = token;
    this.#originClientServerId = serverId;
    return this.#originClient;
  }

  /** Get or create a connection for a registered instance. */
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
      serverUrl: server.url,
      token: server.token,
      serverId
    });

    this.#clients.set(serverId, client);
    return client;
  }

  /** Destroy and remove a client. */
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
