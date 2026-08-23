import { isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
import type { ConnectAPIConfig } from '$lib/api-client/connect';
import { serverRegistry } from './registry.svelte';

export type ConnectionStatus = 'connected' | 'connecting' | 'dormant' | 'disconnected';

const HIDDEN_RECONNECT_AFTER_MS = 30_000;
let nextQueryScope = 0;

export interface ServerConnectionConfig {
  /** Server base URL (relative for origin, absolute for remote). */
  serverUrl: string;
  /** Bearer token for Connect/realtime auth, or null for origin cookie auth. */
  token: string | null;
  /** Access-token expiry as Unix epoch milliseconds. */
  accessTokenExpiresAt?: number | null;
  /** Absolute renewable-session expiry as Unix epoch milliseconds. */
  refreshTokenExpiresAt?: number | null;
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
  #failedAttempts = $state(0);
  #lastVisibleAt = Date.now();
  #visibilityHandler: (() => void) | null = null;
  #onlineHandler: (() => void) | null = null;
  #suspendDetectorInterval: ReturnType<typeof setInterval> | null = null;
  #host: string;
  #connectBaseUrl: string;
  #realtimeUrl: string;
  #token: string | null;
  #accessTokenExpiresAt: number | null;
  #refreshTokenExpiresAt: number | null;
  #renewalTimer: ReturnType<typeof setTimeout> | null = null;
  #serverId: string | undefined;
  #realtimeReconnect: ((reason: string) => void) | null = null;
  #apis = new WeakMap<object, unknown>();
  readonly #queryScope = `connection-${++nextQueryScope}`;

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

  /** Opaque cache scope that changes whenever credentials or transport are replaced. */
  get queryScope(): string {
    return this.#queryScope;
  }

  /** ConnectRPC configuration for helpers that are not API factories. */
  get apiConfig(): ConnectAPIConfig {
    return {
      serverId: this.#serverId,
      baseUrl: this.#connectBaseUrl,
      bearerToken: this.#token,
      renewBearerToken:
        this.#serverId && this.#token
          ? (force) => serverRegistry.renewServerAuthentication(this.#serverId!, force)
          : undefined
    };
  }

  /** Return one API facade per factory for this connection's lifetime. */
  getAPI<T>(factory: (config: ConnectAPIConfig) => T): T {
    if (this.#apis.has(factory)) return this.#apis.get(factory) as T;
    const api = factory(this.apiConfig);
    this.#apis.set(factory, api);
    return api;
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
      this.status = 'connecting';
      this.#failedAttempts = failedAttempts;
      return;
    }

    if (status === 'connected') {
      console.log('[ws:%s] Connected', this.#host);
      this.status = 'connected';
      this.#failedAttempts = 0;
      return;
    }

    if (status === 'dormant') {
      this.status = 'dormant';
      this.#failedAttempts = 0;
      return;
    }

    this.status = 'disconnected';
    this.#failedAttempts = failedAttempts;
  }

  async handleAuthenticationRequired(): Promise<boolean> {
    if (this.#serverId) {
      if (isExplicitSignOutRedirectInProgress() && serverRegistry.isOriginServer(this.#serverId)) {
        return false;
      }
      if (this.#token) {
        return (await serverRegistry.renewServerAuthentication(this.#serverId, true)) !== null;
      }
      serverRegistry.handleAuthenticationRequired(this.#serverId);
    }
    return false;
  }

  /** Adopt a rotated token without replacing the connection or query scope. */
  updateBearerSession(
    token: string | null,
    accessTokenExpiresAt: number | null,
    refreshTokenExpiresAt: number | null
  ): void {
    const changed = token !== this.#token;
    this.#token = token;
    this.#accessTokenExpiresAt = accessTokenExpiresAt;
    this.#refreshTokenExpiresAt = refreshTokenExpiresAt;
    this.#scheduleRenewal();
    if (changed && this.status === 'connected') {
      this.forceReconnect('access token rotated');
    }
  }

  #scheduleRenewal(retryDelayMs?: number): void {
    if (this.#renewalTimer !== null) {
      clearTimeout(this.#renewalTimer);
      this.#renewalTimer = null;
    }
    if (!this.#serverId || !this.#token || !this.#accessTokenExpiresAt) return;
    if (
      this.#refreshTokenExpiresAt !== null &&
      this.#accessTokenExpiresAt >= this.#refreshTokenExpiresAt
    ) {
      return;
    }
    const remaining = this.#accessTokenExpiresAt - Date.now();
    const refreshLead = Math.min(60_000, Math.max(0, remaining / 5));
    const delay = retryDelayMs ?? Math.max(0, remaining - refreshLead);
    this.#renewalTimer = setTimeout(() => {
      this.#renewalTimer = null;
      void serverRegistry.renewServerAuthentication(this.#serverId!, true).catch((error) => {
        console.warn('[auth:%s] background bearer renewal failed', this.#host, error);
        const retryRemaining = this.#accessTokenExpiresAt
          ? this.#accessTokenExpiresAt - Date.now()
          : 0;
        const retryDelay =
          retryRemaining > 0 ? Math.min(30_000, Math.max(250, retryRemaining / 2)) : 30_000;
        this.#scheduleRenewal(retryDelay);
      });
    }, delay);
  }

  constructor(config: ServerConnectionConfig) {
    const { serverUrl, token, accessTokenExpiresAt, refreshTokenExpiresAt, serverId } = config;
    this.#host = hostFromServerUrl(serverUrl);
    this.#connectBaseUrl = connectBaseUrlFromServerUrl(serverUrl);
    this.#realtimeUrl = realtimeUrlFromServerUrl(serverUrl);
    this.#token = token;
    this.#accessTokenExpiresAt = accessTokenExpiresAt ?? null;
    this.#refreshTokenExpiresAt = refreshTokenExpiresAt ?? null;
    this.#serverId = serverId;
    this.#scheduleRenewal();

    // A suspended browser can retain a locally "open" WebSocket long after the
    // server has dropped it. Replace the active transport after a meaningful
    // hidden interval so its retained projection resumes by cursor. If that
    // cursor expired, the server responds on the same stream with a compacted
    // reset; no component-level reload is needed.
    if (typeof document !== 'undefined') {
      this.#visibilityHandler = () => {
        if (document.visibilityState === 'visible') {
          const hiddenDuration = Date.now() - this.#lastVisibleAt;

          console.debug(
            '[ws:%s] visibility=visible after %ds hidden, status=%s',
            this.#host,
            Math.round(hiddenDuration / 1000),
            this.status
          );

          this.#lastVisibleAt = Date.now();
          if (hiddenDuration >= HIDDEN_RECONNECT_AFTER_MS) {
            this.forceReconnect(`tab visible after ${Math.round(hiddenDuration / 1000)}s hidden`);
          }
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
    // meaningful while the tab is visible. If the socket still reports
    // connected, the heartbeat watchdog owns silent-dead detection.
    if (typeof window !== 'undefined') {
      let lastTick = Date.now();
      this.#suspendDetectorInterval = setInterval(() => {
        const now = Date.now();
        const gap = now - lastTick;
        lastTick = now;
        if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
        if (gap > 30_000 && this.status !== 'connected') {
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
    this.#apis = new WeakMap();
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
    if (this.#renewalTimer !== null) {
      clearTimeout(this.#renewalTimer);
      this.#renewalTimer = null;
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
      accessTokenExpiresAt: origin?.accessTokenExpiresAt,
      refreshTokenExpiresAt: origin?.refreshTokenExpiresAt,
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
      accessTokenExpiresAt: server.accessTokenExpiresAt,
      refreshTokenExpiresAt: server.refreshTokenExpiresAt,
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

  /** Push persisted bearer rotation into an existing connection in place. */
  updateBearerSession(serverId: string): void {
    const server = serverRegistry.getServer(serverId);
    if (!server) return;
    if (serverRegistry.isOriginServer(serverId)) {
      this.#originClient?.updateBearerSession(
        server.token,
        server.accessTokenExpiresAt ?? null,
        server.refreshTokenExpiresAt ?? null
      );
      this.#originClientToken = server.token;
      return;
    }
    this.#clients
      .get(serverId)
      ?.updateBearerSession(
        server.token,
        server.accessTokenExpiresAt ?? null,
        server.refreshTokenExpiresAt ?? null
      );
  }
}

export const serverConnectionManager = new ServerConnectionManager();
