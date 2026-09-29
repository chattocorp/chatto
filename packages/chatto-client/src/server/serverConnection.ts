import { isExplicitSignOutRedirectInProgress } from '../auth/signOut.js';
import { signal } from '../reactivity/index.js';
import { csrfFetch } from '../auth/csrf.js';
import { browserCookieAuthenticationHeaders } from '../auth/authenticationMode.js';
import type { ConnectAPIConfig } from '../api/connect.js';
import type { ServerRegistry } from './registry.js';
import { disposeUserStore, getUserStore } from './users.js';
import { debugLog } from '../util/debugLog.js';

export type ConnectionStatus = 'connected' | 'connecting' | 'dormant' | 'disconnected';

const HIDDEN_RECONNECT_AFTER_MS = 30_000;
const MAX_BROWSER_RENEWAL_TIMER_MS = 24 * 60 * 60 * 1000;
const BROWSER_RENEWAL_RETRY_MS = 60_000;
let nextQueryScope = 0;

export interface ServerConnectionConfig {
  /** Server base URL (relative for origin, absolute for remote). */
  serverUrl: string;
  /** Bearer token for Connect/realtime auth, or null for origin cookie auth. */
  token: string | null;
  /** Access-token expiry as Unix epoch milliseconds. */
  accessTokenExpiresAt?: number | null;
  /**
   * Whether the bearer token belongs to a renewable OAuth session. A fixed
   * token, such as a bot API key, is never renewed; the server's rejection
   * ends it.
   */
  renewable?: boolean;
  /** Registered server ID, used to clear stale credentials after auth failures */
  serverId?: string;
  /**
   * The registry that owns the server's session. It renews and ends the
   * session. Without it, the connection never renews its token.
   */
  registry?: ServerConnectionRegistry;
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

/** The session operations that a connection asks its registry for. */
export type ServerConnectionRegistry = Pick<
  ServerRegistry,
  | 'isOriginServer'
  | 'confirmAuthenticationRequired'
  | 'renewServerAuthentication'
  | 'handleAuthenticationRequired'
>;

/** Stands in for a registry when a connection has none, for example in stories. */
const detachedRegistry: ServerConnectionRegistry = {
  isOriginServer: () => false,
  confirmAuthenticationRequired: async () => true,
  renewServerAuthentication: async () => null,
  handleAuthenticationRequired: () => {}
};

export class ServerConnection {
  readonly #statusSignal = signal<ConnectionStatus>('connecting');
  get status(): ConnectionStatus {
    return this.#statusSignal.get();
  }
  set status(value: ConnectionStatus) {
    this.#statusSignal.set(value);
  }
  readonly #failedAttemptsSignal = signal(0);
  get #failedAttempts() {
    return this.#failedAttemptsSignal.get();
  }
  set #failedAttempts(value) {
    this.#failedAttemptsSignal.set(value);
  }
  /**
   * Whether the latest completed attempt failed. A new attempt does not change
   * it: only success clears it and only failure sets it. A forced reconnect
   * (tab wake, network recovery) starts fresh and clears it.
   */
  readonly #connectionFailedSignal = signal(false);
  get #connectionFailed() {
    return this.#connectionFailedSignal.get();
  }
  set #connectionFailed(value) {
    this.#connectionFailedSignal.set(value);
  }
  readonly #realtimeUnsupportedSignal = signal(false);
  /**
   * Whether the server closed realtime because it does not support this
   * client's protocol. The transport does not reconnect then. Reactive.
   */
  get realtimeUnsupported(): boolean {
    return this.#realtimeUnsupportedSignal.get();
  }
  /** Record that the server does not support this client's realtime protocol. */
  markRealtimeUnsupported(): void {
    this.#realtimeUnsupportedSignal.set(true);
  }
  #lastVisibleAt = Date.now();
  #visibilityHandler: (() => void) | null = null;
  #onlineHandler: (() => void) | null = null;
  #suspendDetectorInterval: ReturnType<typeof setInterval> | null = null;
  #host: string;
  #connectBaseUrl: string;
  #realtimeUrl: string;
  #token: string | null;
  #accessTokenExpiresAt: number | null;
  readonly #renewable: boolean;
  #renewalTimer: ReturnType<typeof setTimeout> | null = null;
  #browserRenewal: Promise<boolean> | null = null;
  #browserRenewalTimer: ReturnType<typeof setTimeout> | null = null;
  #browserRenewAfter: number | null = null;
  #serverId: string | undefined;
  #realtimeReconnect: ((reason: string) => void) | null = null;
  #pendingForcedReconnectReason: string | null = null;
  #apis = new WeakMap<object, unknown>();
  readonly #registry: ServerConnectionRegistry;
  readonly #queryScope = `connection-${++nextQueryScope}`;
  #dataGeneration = 0;

  /** Generation of private data owned by this connection, independent of navigation. */
  get dataGeneration(): number {
    return this.#dataGeneration;
  }

  /** Reject all older API responses before they reach caches or API side effects. */
  invalidatePrivateData(): void {
    this.#dataGeneration++;
    if (this.#serverId) getUserStore(this.#serverId, this.queryScope).clear();
  }

  get isConnected() {
    return this.status === 'connected';
  }

  /** Show the connection warning after a failed attempt until an attempt succeeds. */
  get showConnectionLostIcon() {
    return this.#connectionFailed;
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
      queryScope: this.queryScope,
      baseUrl: this.#connectBaseUrl,
      bearerToken: this.#token,
      dataGeneration: () => this.#dataGeneration,
      renewBearerToken:
        this.#serverId && this.#token && this.#renewable
          ? (force) => this.#registry.renewServerAuthentication(this.#serverId!, force)
          : undefined,
      onAuthenticationRequired: this.#serverId
        ? (source) => this.#reportAuthenticationRequired(this.#serverId!, source)
        : undefined
    };
  }

  /**
   * Ask the registry to confirm a rejected session. An explicit origin sign-out
   * already ends that session, so its rejected requests are expected.
   */
  #reportAuthenticationRequired(serverId: string, source: string): void {
    if (isExplicitSignOutRedirectInProgress() && this.#registry.isOriginServer(serverId)) return;
    this.#registry.confirmAuthenticationRequired(serverId, source).catch((error) => {
      console.warn('[auth] could not confirm the rejected session', { serverId, source }, error);
    });
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
    this.#connectionFailed = false;
    if (this.status === 'connecting') {
      this.#pendingForcedReconnectReason = reason;
      debugLog('[ws:%s] Force reconnect queued — already connecting: %s', this.#host, reason);
      return;
    }
    if (this.#realtimeReconnect) {
      this.#pendingForcedReconnectReason = null;
      debugLog(
        '[ws:%s] Force realtime reconnect: %s (status: %s)',
        this.#host,
        reason,
        this.status
      );
      this.#failedAttempts = 0;
      this.#realtimeReconnect(reason);
      return;
    }
    debugLog(
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
    const pendingReason = this.#pendingForcedReconnectReason;
    if (pendingReason && this.status !== 'connecting') {
      this.#pendingForcedReconnectReason = null;
      this.forceReconnect(pendingReason);
    }
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
      debugLog('[ws:%s] Connected', this.#host);
      this.status = 'connected';
      this.#failedAttempts = 0;
      this.#connectionFailed = false;
      const pendingReason = this.#pendingForcedReconnectReason;
      if (pendingReason && this.#realtimeReconnect) {
        this.#pendingForcedReconnectReason = null;
        this.forceReconnect(pendingReason);
      }
      return;
    }

    if (status === 'dormant') {
      this.status = 'dormant';
      this.#failedAttempts = 0;
      this.#connectionFailed = false;
      return;
    }

    this.status = 'disconnected';
    this.#failedAttempts = failedAttempts;
    this.#connectionFailed = true;
  }

  /**
   * Recover from a realtime authentication-required close. Resolves to true
   * when a renewed bearer session can reconnect at once, and to false when the
   * session needs a new sign-in. Rejects when recovery failed temporarily,
   * including when the viewer check accepts a rejected cookie session; the
   * caller then reconnects after a delay.
   */
  async handleAuthenticationRequired(): Promise<boolean> {
    if (this.#serverId) {
      if (isExplicitSignOutRedirectInProgress() && this.#registry.isOriginServer(this.#serverId)) {
        return false;
      }
      if (this.#token && this.#renewable) {
        return (await this.#registry.renewServerAuthentication(this.#serverId, true)) !== null;
      }
      const required = await this.#registry.confirmAuthenticationRequired(
        this.#serverId,
        'realtime close frame'
      );
      if (!required) throw new Error('The viewer check accepted the rejected session.');
    }
    return false;
  }

  /** Renew the origin's stable HttpOnly cookie before its current window ends. */
  renewBrowserSession(): Promise<boolean> {
    if (this.#token !== null || !this.#serverId || !this.#registry.isOriginServer(this.#serverId)) {
      return Promise.resolve(false);
    }
    if (this.#browserRenewal) return this.#browserRenewal;

    const renew = async () => {
      const response = await csrfFetch('/auth/browser/session/renew', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...browserCookieAuthenticationHeaders
        },
        body: '{}'
      });
      if (response.status === 401) {
        this.#registry.handleAuthenticationRequired(this.#serverId!);
        return false;
      }
      if (!response.ok) {
        throw new Error(`Browser session renewal failed (${response.status})`);
      }
      const body: Record<string, unknown> = await response.json().catch(() => ({}));
      const renewAfter =
        typeof body.renewAfter === 'string' ? Date.parse(body.renewAfter) : Number.NaN;
      this.#browserRenewAfter = Number.isFinite(renewAfter) ? renewAfter : null;
      this.#scheduleBrowserSessionMaintenance();
      return true;
    };
    const operation =
      typeof navigator !== 'undefined' && navigator.locks
        ? navigator.locks.request('chatto:origin-session-renewal', renew)
        : renew();
    const renewal = operation.finally(() => {
      if (this.#browserRenewal === renewal) this.#browserRenewal = null;
    });
    this.#browserRenewal = renewal;
    return renewal;
  }

  #scheduleBrowserSessionMaintenance(retryDelayMs?: number): void {
    if (this.#browserRenewalTimer !== null) {
      clearTimeout(this.#browserRenewalTimer);
      this.#browserRenewalTimer = null;
    }
    if (this.#token !== null || !this.#serverId || !this.#registry.isOriginServer(this.#serverId)) {
      return;
    }
    const remaining =
      this.#browserRenewAfter === null
        ? MAX_BROWSER_RENEWAL_TIMER_MS
        : this.#browserRenewAfter - Date.now();
    const delay = retryDelayMs ?? Math.min(MAX_BROWSER_RENEWAL_TIMER_MS, Math.max(0, remaining));
    this.#browserRenewalTimer = setTimeout(() => {
      this.#browserRenewalTimer = null;
      if (this.#browserRenewAfter !== null && Date.now() < this.#browserRenewAfter) {
        this.#scheduleBrowserSessionMaintenance();
        return;
      }
      void this.renewBrowserSession().catch((error) => {
        console.warn('[auth:%s] background browser-session renewal failed', this.#host, error);
        this.#scheduleBrowserSessionMaintenance(BROWSER_RENEWAL_RETRY_MS);
      });
    }, delay);
  }

  #maintainBrowserSessionIfDue(): void {
    if (this.#browserRenewAfter === null || Date.now() >= this.#browserRenewAfter) {
      void this.renewBrowserSession().catch((error) => {
        console.warn('[auth:%s] browser-session maintenance failed', this.#host, error);
        this.#scheduleBrowserSessionMaintenance(BROWSER_RENEWAL_RETRY_MS);
      });
    }
  }

  /** Start or resume automatic origin-cookie maintenance. */
  maintainBrowserSession(): void {
    this.#maintainBrowserSessionIfDue();
  }

  /** Adopt a rotated token without replacing the connection or query scope. */
  updateBearerSession(token: string | null, accessTokenExpiresAt: number | null): void {
    const changed = token !== this.#token;
    this.#token = token;
    this.#accessTokenExpiresAt = accessTokenExpiresAt;
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
    const remaining = this.#accessTokenExpiresAt - Date.now();
    const refreshLead = Math.min(60_000, Math.max(0, remaining / 5));
    const delay = retryDelayMs ?? Math.max(0, remaining - refreshLead);
    this.#renewalTimer = setTimeout(() => {
      this.#renewalTimer = null;
      void this.#registry.renewServerAuthentication(this.#serverId!, true).catch((error) => {
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
    const { serverUrl, token, accessTokenExpiresAt, serverId, renewable = true } = config;
    this.#registry = config.registry ?? detachedRegistry;
    this.#host = hostFromServerUrl(serverUrl);
    this.#connectBaseUrl = connectBaseUrlFromServerUrl(serverUrl);
    this.#realtimeUrl = realtimeUrlFromServerUrl(serverUrl);
    this.#token = token;
    this.#accessTokenExpiresAt = accessTokenExpiresAt ?? null;
    this.#renewable = renewable;
    this.#serverId = serverId;
    this.#scheduleRenewal();

    // A suspended browser can retain a locally "open" WebSocket long after the
    // server has dropped it. Replace the active transport after a meaningful
    // hidden interval so its retained projection resumes by cursor. If that
    // cursor expired, the server responds on the same stream with a snapshot;
    // no component-level reload is needed.
    if (typeof document !== 'undefined') {
      this.#visibilityHandler = () => {
        if (document.visibilityState === 'visible') {
          const hiddenDuration = Date.now() - this.#lastVisibleAt;

          debugLog(
            '[ws:%s] visibility=visible after %ds hidden, status=%s',
            this.#host,
            Math.round(hiddenDuration / 1000),
            this.status
          );

          this.#lastVisibleAt = Date.now();
          this.#maintainBrowserSessionIfDue();
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
          debugLog(
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
        debugLog('[ws:%s] online event fired', this.#host);
        this.#maintainBrowserSessionIfDue();
        this.forceReconnect('network came back online');
      };
      window.addEventListener('online', this.#onlineHandler);
    }
  }

  /** Clean up event listeners owned by the connection state object. */
  dispose() {
    if (this.#serverId) disposeUserStore(this.#serverId, this.queryScope);
    this.#apis = new WeakMap();
    this.#pendingForcedReconnectReason = null;
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
    if (this.#browserRenewalTimer !== null) {
      clearTimeout(this.#browserRenewalTimer);
      this.#browserRenewalTimer = null;
    }
  }
}

/**
 * Manages Connect/realtime connection state for multiple Chatto instances.
 * The origin connection is created eagerly; remote connections are created
 * lazily on first access.
 */
export class ServerConnectionManager {
  #clients = new Map<string, ServerConnection>();
  readonly #registry: () => ServerRegistry;

  /** `registry` returns the owning client's registry; it is created after the manager. */
  constructor(registry: () => ServerRegistry) {
    this.#registry = registry;
  }
  #originClient: ServerConnection | null = null;
  #originClientServerId: string | undefined;

  /** The origin ConnectRPC base URL without creating an authenticated connection. */
  get originConnectBaseUrl(): string {
    return connectBaseUrlFromServerUrl(ORIGIN_SERVER_URL);
  }

  /** The origin connection always uses the browser's same-origin cookie. */
  get originClient(): ServerConnection {
    const origin = this.#registry().originServer;
    const serverId = origin?.id;
    if (this.#originClient && this.#originClientServerId === serverId) {
      return this.#originClient;
    }

    this.#originClient?.dispose();
    this.#originClient = new ServerConnection({
      serverUrl: ORIGIN_SERVER_URL,
      token: null,
      accessTokenExpiresAt: null,
      serverId,
      registry: this.#registry()
    });
    this.#originClientServerId = serverId;
    return this.#originClient;
  }

  /** Get or create a connection for a registered instance. */
  getClient(serverId: string): ServerConnection {
    if (this.#registry().isOriginServer(serverId)) {
      return this.originClient;
    }

    const existing = this.#clients.get(serverId);
    if (existing) return existing;

    const server = this.#registry().getServer(serverId);
    if (!server) {
      throw new Error(`Server "${serverId}" not found in registry`);
    }

    const client = new ServerConnection({
      serverUrl: server.url,
      token: server.token,
      accessTokenExpiresAt: server.accessTokenExpiresAt,
      renewable: !this.#registry().hasFixedToken(serverId),
      serverId,
      registry: this.#registry()
    });

    this.#clients.set(serverId, client);
    return client;
  }

  /** Destroy and remove a client. */
  destroyClient(serverId: string): boolean {
    if (this.#registry().isOriginServer(serverId)) {
      if (!this.#originClient) return false;
      this.#originClient.dispose();
      this.#originClient = null;
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
    const server = this.#registry().getServer(serverId);
    if (!server) return;
    if (this.#registry().isOriginServer(serverId)) {
      this.#originClient?.updateBearerSession(null, null);
      return;
    }
    this.#clients
      .get(serverId)
      ?.updateBearerSession(server.token, server.accessTokenExpiresAt ?? null);
  }
}
