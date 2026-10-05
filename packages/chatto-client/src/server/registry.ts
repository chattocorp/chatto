import { ReactiveMap, batch, untrack } from '../reactivity/index.js';
import { clearUserStores } from './users.js';
import { Server } from './server.js';
import type { ServerConnectionManager } from './serverConnection.js';
import type { EventBusManager } from './realtimeTransport.js';
import {
  claimServerId,
  generateServerId,
  isServerIdClaimed,
  releaseServerId
} from './serverIds.js';
import { Codecs, globalSlot, serverSlot } from '../storage/slot.js';
import { getViewerStateViaConnect, type CurrentUser } from '../api/viewer.js';
import { connectEndpoint } from '../api/connect.js';
import { isAuthenticationRequiredError } from '../auth/errors.js';
import {
  ServerCatalog,
  type ServerRegistration,
  type ServerRegistrationMetadataPatch
} from './catalog.js';
import { emptyServerSession, ServerSessions, type ServerSession } from './sessions.js';
import {
  oauthBearerSession,
  persistedBearerSession,
  type NewBearerSession
} from '../auth/bearerSession.js';

export type { ServerRegistration } from './catalog.js';
export type { ServerSession } from './sessions.js';

/**
 * A registered Chatto server in the multi-server client.
 */
export interface RegisteredServer extends ServerRegistration, ServerSession {
  /** Bearer token for API auth, or null when unauthenticated/legacy cookie auth */
  token: string | null;
  /** Authenticated user ID on this server, or null if not yet authenticated */
  userId: string | null;
  /** Authenticated user's login on this server */
  userLogin: string | null;
  /** Authenticated user's display name on this server */
  userDisplayName: string | null;
  /** Authenticated user's avatar URL on this server */
  userAvatarUrl: string | null;
  /** Epoch ms when this server last rejected auth, or null when auth is usable */
  reauthRequiredAt: number | null;
}

export interface AuthenticatedUserSummary {
  id: string;
  login: string;
  displayName?: string | null;
  avatarUrl?: string | null;
}

// Storage key intentionally stays as 'instances' — renaming would lose users'
// multi-server registrations (including remote bearer tokens that can't be
// regenerated). The in-code rename is purely cosmetic.
type PersistedRegisteredServer = RegisteredServer & { source?: 'local' | 'synced' };

type PersistedServerAuthentication = {
  version: 1;
  token: string | null;
  refreshToken: string | null;
  accessTokenExpiresAt: number | null;
  refreshTokenExpiresAt: number | null;
  oauthClientId: string | null;
  refreshRequestId: string | null;
  reauthRequiredAt: number | null;
};

type ServerAuthentication = Omit<PersistedServerAuthentication, 'version'>;

function normalizeRegisteredServer(server: PersistedRegisteredServer): RegisteredServer {
  const { source: _retiredSource, ...local } = server;
  return {
    ...emptyServerSession(),
    ...local,
    iconUrl: server.iconUrl ?? null,
    reauthRequiredAt: server.reauthRequiredAt ?? null
  };
}

function isOptionalNullableString(value: unknown): boolean {
  return value === undefined || value === null || typeof value === 'string';
}

function isOptionalNullableNumber(value: unknown): boolean {
  return (
    value === undefined || value === null || (typeof value === 'number' && Number.isFinite(value))
  );
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === 'string';
}

function isNullableNumber(value: unknown): value is number | null {
  return value === null || (typeof value === 'number' && Number.isFinite(value));
}

function isPersistedServerAuthentication(value: unknown): value is PersistedServerAuthentication {
  if (typeof value !== 'object' || value === null) return false;
  const authentication = value as Record<string, unknown>;
  return (
    authentication.version === 1 &&
    isNullableString(authentication.token) &&
    isNullableString(authentication.refreshToken) &&
    isNullableNumber(authentication.accessTokenExpiresAt) &&
    isNullableNumber(authentication.refreshTokenExpiresAt) &&
    isNullableString(authentication.oauthClientId) &&
    isNullableString(authentication.refreshRequestId) &&
    isNullableNumber(authentication.reauthRequiredAt)
  );
}

function isPersistedServer(value: unknown): value is PersistedRegisteredServer {
  if (typeof value !== 'object' || value === null) return false;
  const server = value as Record<string, unknown>;
  if (
    typeof server.id !== 'string' ||
    server.id.length === 0 ||
    typeof server.url !== 'string' ||
    typeof server.name !== 'string' ||
    typeof server.addedAt !== 'number' ||
    !Number.isFinite(server.addedAt) ||
    !isOptionalNullableString(server.iconUrl) ||
    !isOptionalNullableString(server.token) ||
    !isOptionalNullableString(server.refreshToken) ||
    !isOptionalNullableNumber(server.accessTokenExpiresAt) ||
    !isOptionalNullableNumber(server.refreshTokenExpiresAt) ||
    !isOptionalNullableString(server.oauthClientId) ||
    !isOptionalNullableString(server.refreshRequestId) ||
    !isOptionalNullableString(server.userId) ||
    !isOptionalNullableString(server.userLogin) ||
    !isOptionalNullableString(server.userDisplayName) ||
    !isOptionalNullableString(server.userAvatarUrl) ||
    (server.reauthRequiredAt !== undefined &&
      server.reauthRequiredAt !== null &&
      (typeof server.reauthRequiredAt !== 'number' || !Number.isFinite(server.reauthRequiredAt))) ||
    (server.source !== undefined && server.source !== 'local' && server.source !== 'synced')
  ) {
    return false;
  }

  try {
    const url = new URL(server.url);
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
}

function isPersistedServerArray(value: unknown): value is PersistedRegisteredServer[] {
  if (!Array.isArray(value) || !value.every(isPersistedServer)) return false;
  return new Set(value.map((server) => server.id)).size === value.length;
}

function registrationFromServer(server: RegisteredServer): ServerRegistration {
  return {
    id: server.id,
    url: server.url,
    name: server.name,
    iconUrl: server.iconUrl,
    addedAt: server.addedAt
  };
}

function sessionFromServer(server: RegisteredServer): ServerSession {
  return {
    token: server.token,
    refreshToken: server.refreshToken ?? null,
    accessTokenExpiresAt: server.accessTokenExpiresAt ?? null,
    refreshTokenExpiresAt: server.refreshTokenExpiresAt ?? null,
    oauthClientId: server.oauthClientId ?? null,
    refreshRequestId: server.refreshRequestId ?? null,
    userId: server.userId,
    userLogin: server.userLogin,
    userDisplayName: server.userDisplayName,
    userAvatarUrl: server.userAvatarUrl,
    reauthRequiredAt: server.reauthRequiredAt
  };
}

function authenticationFromSession(session: ServerSession): ServerAuthentication {
  return {
    token: session.token,
    refreshToken: session.refreshToken ?? null,
    accessTokenExpiresAt: session.accessTokenExpiresAt ?? null,
    refreshTokenExpiresAt: session.refreshTokenExpiresAt ?? null,
    oauthClientId: session.oauthClientId ?? null,
    refreshRequestId: session.refreshRequestId ?? null,
    reauthRequiredAt: session.reauthRequiredAt
  };
}

function emptyServerAuthentication(): ServerAuthentication {
  return authenticationFromSession(emptyServerSession());
}

/** Split the legacy combined persistence shape into its runtime owners. */
export function splitPersistedServers(servers: PersistedRegisteredServer[]): {
  registrations: ServerRegistration[];
  sessions: Array<readonly [string, ServerSession]>;
} {
  const normalized = servers.map(normalizeRegisteredServer);
  return {
    registrations: normalized.map(registrationFromServer),
    sessions: normalized.map((server) => [server.id, sessionFromServer(server)] as const)
  };
}

const serversSlot = globalSlot(
  'instances',
  [] as PersistedRegisteredServer[],
  Codecs.json<PersistedRegisteredServer[]>(isPersistedServerArray)
);

const serverAuthenticationCodec = Codecs.json<PersistedServerAuthentication | null>(
  (value): value is PersistedServerAuthentication | null =>
    value === null || isPersistedServerAuthentication(value)
);

function authenticationSlot(serverId: string) {
  return serverSlot<PersistedServerAuthentication | null>(
    serverId,
    'authentication',
    null,
    serverAuthenticationCodec
  );
}

/**
 * Read a server's independently keyed authentication state. `undefined` means
 * the legacy combined record has not been migrated; `null` means a present
 * record was corrupt and must not fall back to possibly stale credentials.
 */
function readPersistedAuthentication(serverId: string): ServerAuthentication | null | undefined {
  const slot = authenticationSlot(serverId);
  if (typeof localStorage === 'undefined') return undefined;
  try {
    if (localStorage.getItem(slot.key) === null) return undefined;
  } catch {
    return null;
  }
  const stored = slot.get();
  if (!stored) return null;
  const { version: _version, ...authentication } = stored;
  return authentication;
}

function persistAuthentication(serverId: string, authentication: ServerAuthentication): boolean {
  const slot = authenticationSlot(serverId);
  slot.set({ version: 1, ...authentication });
  const stored = readPersistedAuthentication(serverId);
  return (
    stored !== undefined &&
    stored !== null &&
    stored.token === authentication.token &&
    stored.refreshToken === authentication.refreshToken &&
    stored.accessTokenExpiresAt === authentication.accessTokenExpiresAt &&
    stored.refreshTokenExpiresAt === authentication.refreshTokenExpiresAt &&
    stored.oauthClientId === authentication.oauthClientId &&
    stored.refreshRequestId === authentication.refreshRequestId &&
    stored.reauthRequiredAt === authentication.reauthRequiredAt
  );
}

const RETIRED_ACCOUNT_DATA_KEYS = [
  'chatto:account-data:authorization',
  'chatto:account-data:device-id',
  'chatto:account-data:tinybase'
];

function clearRetiredAccountDataStorage(): void {
  if (typeof localStorage === 'undefined') return;
  try {
    for (const key of RETIRED_ACCOUNT_DATA_KEYS) localStorage.removeItem(key);
  } catch {
    // Browser storage can be unavailable in privacy-restricted contexts.
  }
}

/** Read and split the legacy combined storage shape used at registry construction. */
export function restorePersistedServerState(): ReturnType<typeof splitPersistedServers> {
  const stored = serversSlot.get();
  const normalized = stored.map(normalizeRegisteredServer);
  if (stored.some((server) => server.source !== undefined)) {
    serversSlot.set(normalized);
  }
  const persisted = splitPersistedServers(normalized);
  for (const [serverId, session] of persisted.sessions) {
    const authentication = readPersistedAuthentication(serverId);
    if (authentication === undefined) {
      persistAuthentication(serverId, authenticationFromSession(session));
    } else {
      Object.assign(session, authentication ?? emptyServerAuthentication());
    }
  }
  clearRetiredAccountDataStorage();
  return persisted;
}

/** Where a registry keeps the server catalogue and authentication records. */
interface RegistryStorage {
  readAuthentication(serverId: string): ServerAuthentication | null | undefined;
  writeAuthentication(serverId: string, authentication: ServerAuthentication): boolean;
  writeServers(servers: PersistedRegisteredServer[]): void;
  /** Whether another tab can have changed the stored records. */
  readonly shared: boolean;
}

const deviceStorage: RegistryStorage = {
  readAuthentication: readPersistedAuthentication,
  writeAuthentication: persistAuthentication,
  writeServers: (servers) => serversSlot.set(servers),
  get shared() {
    return typeof localStorage !== 'undefined';
  }
};

/**
 * Keeps authentication records in memory only, for one registry. Renewal
 * reads its own writes back, as it does with device storage; no other tab
 * shares the records.
 */
function memoryStorage(): RegistryStorage {
  const records = new Map<string, ServerAuthentication>();
  return {
    readAuthentication: (serverId) => records.get(serverId),
    writeAuthentication: (serverId, authentication) => {
      // An empty record means that the server has no session; keep nothing.
      if (Object.values(authentication).every((value) => value === null)) records.delete(serverId);
      else records.set(serverId, { ...authentication });
      return true;
    },
    writeServers: () => {},
    shared: false
  };
}

/** Settings of a {@link ServerRegistry}; a client passes its own. */
export interface ServerRegistryOptions {
  /**
   * Keep the server catalogue and renewable sessions in device storage, and
   * restore them at construction. Only one client in a page should use it.
   */
  deviceStorage: boolean;
  /**
   * Treat a server on the page's own origin as the origin server, which uses
   * the browser's cookie session instead of a bearer token.
   */
  originServer: boolean;
}

/** The parts of a client that the registry uses. */
export interface ServerRegistryContext {
  readonly connections: ServerConnectionManager;
  readonly realtime: EventBusManager;
}

/**
 * Client-side registry of connected Chatto servers.
 * Owns both registration data and per-server state stores.
 *
 * Registration and store creation are atomic — when a server is added,
 * its store is created immediately. Methods that write several fields run in
 * one `batch`, so effects never observe a half-applied change. This eliminates race conditions where
 * computed values see a registered server but no store exists yet.
 *
 * The store map is a ReactiveMap, so getStore() lookups are reactive in
 * computed values and UI reactions.
 *
 * The registry does NOT track which server is "active".
 * An application selects the server that the user looks at, for example from
 * its URL, and reports it to the client runtime (see `startClientRuntime`).
 */
export class ServerRegistry {
  readonly catalog: ServerCatalog;
  readonly #context: ServerRegistryContext;
  readonly #options: ServerRegistryOptions;
  #storage: RegistryStorage;
  /** Set by {@link dispose}; late async work then changes nothing. */
  #disposed = false;
  readonly sessions: ServerSessions;
  #stores = new ReactiveMap<string, Server>();
  #renewalPromises = new Map<string, Promise<string | null>>();
  /** In-flight viewer checks that confirm a rejected origin cookie session. */
  #authenticationChecks = new Map<string, Promise<boolean>>();
  /** Stores whose discovery and viewer startup has been scheduled. */
  #startedServerNetwork = new Set<string>();
  /**
   * Servers whose bearer token is fixed, such as a bot API key. Their token is
   * never renewed or written to device storage, and the server's rejection
   * ends the session. See {@link addServer}.
   */
  readonly #fixedTokenServers = new Set<string>();
  /** Callbacks of {@link watchStores} for stores that the registry creates later. */
  readonly #storeWatchers = new Set<(store: Server) => void>();

  constructor(context: ServerRegistryContext, options: ServerRegistryOptions) {
    this.#context = context;
    this.#options = options;
    this.#storage = options.deviceStorage ? deviceStorage : memoryStorage();
    const persisted = options.deviceStorage
      ? restorePersistedServerState()
      : { registrations: [], sessions: new Map<string, ServerSession>() };
    // Check every ID before claiming one, so a failed construction claims nothing.
    const taken = persisted.registrations.find((registration) =>
      isServerIdClaimed(registration.id)
    );
    if (taken) throw new Error(`Server ID "${taken.id}" belongs to another Chatto client`);
    for (const registration of persisted.registrations) claimServerId(registration.id, this);
    this.catalog = new ServerCatalog(persisted.registrations);
    this.sessions = new ServerSessions(persisted.sessions);
  }

  /** Composed compatibility view for cross-server rendering and commands. */
  get servers(): RegisteredServer[] {
    return this.catalog.registrations.map((registration) => ({
      ...registration,
      ...(this.sessions.get(registration.id) ?? emptyServerSession())
    }));
  }

  /** Device-local public metadata for the servers known to this client. */
  get registrations(): ServerRegistration[] {
    return this.catalog.registrations;
  }

  /**
   * The origin server — the one serving the SPA.
   * Derived by matching registered server URLs against window.location.origin.
   * Returns undefined if the origin server isn't registered.
   */
  get originServer(): RegisteredServer | undefined {
    if (!this.#options.originServer || typeof window === 'undefined') return undefined;
    const origin = window.location.origin;
    return this.servers.find((s) => {
      try {
        return new URL(s.url).origin === origin;
      } catch {
        return false;
      }
    });
  }

  /**
   * The origin rejected its viewer and no loaded data remains to read. The
   * reauthentication notice then offers nothing, so the viewer must sign in.
   */
  get originSignInRequired(): boolean {
    const origin = this.originServer;
    if (!origin || origin.reauthRequiredAt == null) return false;
    const store = this.tryGetStore(origin.id);
    return !!store && !store.currentUser.user && !store.realtimeSync.hasDisplayableView;
  }

  /**
   * Check whether a registered server is the origin (the server serving the SPA).
   * Uses URL comparison — no stored flag needed.
   */
  isOriginServer(serverId: string): boolean {
    const server = this.getServer(serverId);
    if (!server || !this.#options.originServer || typeof window === 'undefined') return false;
    try {
      return new URL(server.url).origin === window.location.origin;
    } catch {
      return false;
    }
  }

  #registerOrigin(
    id: string,
    url: string,
    name: string,
    iconUrl: string | null,
    credentials: string | NewBearerSession | null = null,
    user: AuthenticatedUserSummary | null = null
  ): void {
    this.addServer(
      {
        id,
        url,
        name,
        iconUrl,
        addedAt: Date.now()
      },
      {
        ...(typeof credentials === 'string'
          ? { token: credentials }
          : credentials
            ? persistedBearerSession(credentials)
            : { token: null }),
        userId: user?.id ?? null,
        userLogin: user?.login ?? null,
        userDisplayName: user?.displayName ?? user?.login ?? null,
        userAvatarUrl: user?.avatarUrl ?? null,
        reauthRequiredAt: null
      }
    );
  }

  /** Install a verified origin viewer and discard any legacy origin bearer session. */
  authenticateOriginCookie(user: CurrentUser): void {
    batch(() => {
      if (typeof window === 'undefined') return;
      this.#installOriginCookie(user);
      const origin = this.originServer;
      if (origin) {
        this.getStore(origin.id).currentUser.accept(user);
        this.clearAuthenticationRequired(origin.id);
        this.#context.connections.originClient.maintainBrowserSession();
      }
    });
  }

  #installOriginCookie(user: CurrentUser): void {
    const origin = this.originServer;
    if (!origin) {
      const originUrl = window.location.origin;
      const id = generateServerId(
        originUrl,
        this.servers.map((s) => s.id)
      );
      this.#registerOrigin(id, originUrl, 'Chatto', null, null, user);
      return;
    }

    const cookieSession: ServerSession = {
      token: null,
      refreshToken: null,
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      oauthClientId: null,
      refreshRequestId: null,
      userId: user.id,
      userLogin: user.login,
      userDisplayName: user.displayName,
      userAvatarUrl: user.avatarUrl ?? null,
      reauthRequiredAt: null
    };
    // A new cookie viewer must not inherit the previous account's projection
    // or its realtime cursor, even though both accounts use cookie auth.
    const previousUserId = origin.userId ?? this.tryGetStore(origin.id)?.currentUser.user?.id;
    if (previousUserId && previousUserId !== user.id) {
      this.#replaceServerAuth(origin.id, cookieSession);
      return;
    }
    if (
      origin.token === null &&
      origin.refreshToken === null &&
      origin.accessTokenExpiresAt === null &&
      origin.refreshTokenExpiresAt === null &&
      origin.oauthClientId === null &&
      origin.refreshRequestId === null
    ) {
      this.sessions.replace(origin.id, cookieSession);
      this.#persistAuthentication(origin.id);
      this.#persist();
    } else {
      this.#replaceServerAuth(origin.id, cookieSession);
    }
  }

  /** Settle the origin cookie-auth store when root load found no user. */
  settleOriginUnauthenticated(): void {
    const origin = this.originServer;
    if (!origin) return;
    if (origin.token !== null) return;
    const store = this.tryGetStore(origin.id);
    if (!store) return;
    store.currentUser.reset();
  }

  clearServerAuthentication(id: string): void {
    batch(() => {
      const server = this.getServer(id);
      if (!server) return;
      this.#replaceServerAuth(id, {
        token: null,
        refreshToken: null,
        accessTokenExpiresAt: null,
        refreshTokenExpiresAt: null,
        oauthClientId: null,
        refreshRequestId: null,
        userId: null,
        userLogin: null,
        userDisplayName: null,
        userAvatarUrl: null,
        reauthRequiredAt: null
      });
      const store = this.tryGetStore(id);
      if (store) {
        store.currentUser.reset();
      }
    });
  }

  clearOriginAuthentication(): void {
    const origin = this.originServer;
    if (!origin) return;
    this.clearServerAuthentication(origin.id);
  }

  handleAuthenticationRequired(id: string): void {
    batch(() => {
      const session = this.sessions.get(id);
      if (!session || session.reauthRequiredAt !== null) return;

      this.#context.realtime.stopBus(id);
      if (this.tryGetStore(id)) this.getStore(id).endSession();
      else clearUserStores(id);
      this.sessions.update(id, { reauthRequiredAt: Date.now() });
      this.#persistAuthenticationPatch(id, {
        reauthRequiredAt: this.sessions.get(id)?.reauthRequiredAt ?? null
      });
      this.#persist();
      const store = this.tryGetStore(id);
      if (store) {
        store.currentUser.invalidateVerification();
        store.currentUser.loading = false;
      }
    });
  }

  /**
   * Report an `Unauthenticated` result from a request that did not itself read
   * the viewer. `source` names the request for diagnostics.
   *
   * A session with a token is marked at once: a renewable session reaches this
   * only after its refresh grant was rejected, and a fixed token cannot recover.
   * The origin cookie session is different. It cannot renew itself, so one
   * rejected request must not end it. The registry reads the viewer once and
   * marks the session only when that read is also rejected. Reports that arrive
   * during the read share it.
   *
   * Resolves to true when the session needs a new sign-in. Rejects when the
   * viewer read fails for another reason; the caller then retries later.
   */
  confirmAuthenticationRequired(id: string, source: string): Promise<boolean> {
    console.warn('[auth] request rejected as unauthenticated', { serverId: id, source });
    const session = this.sessions.get(id);
    const registration = this.catalog.get(id);
    if (!session || !registration) return Promise.resolve(false);
    if (session.reauthRequiredAt !== null) return Promise.resolve(true);
    if (session.token !== null || !this.isOriginServer(id)) {
      this.handleAuthenticationRequired(id);
      return Promise.resolve(true);
    }

    const existing = this.#authenticationChecks.get(id);
    if (existing) return existing;
    const check = this.#confirmCookieAuthenticationRequired(
      id,
      registration.url,
      session.userId
    ).finally(() => {
      if (this.#authenticationChecks.get(id) === check) this.#authenticationChecks.delete(id);
    });
    this.#authenticationChecks.set(id, check);
    return check;
  }

  async #confirmCookieAuthenticationRequired(
    id: string,
    url: string,
    userId: string | null
  ): Promise<boolean> {
    try {
      await getViewerStateViaConnect(
        { baseUrl: connectEndpoint(url), bearerToken: null },
        { timeoutMs: 10_000 }
      );
      console.warn('[auth] viewer check accepted the origin session; keeping it', {
        serverId: id
      });
      return false;
    } catch (error) {
      if (!isAuthenticationRequiredError(error)) throw error;
    }
    // A sign-in or sign-out during the check replaced the rejected session.
    const current = this.sessions.get(id);
    if (!current || current.token !== null || current.userId !== userId) return false;
    this.handleAuthenticationRequired(id);
    return true;
  }

  clearAuthenticationRequired(id: string): void {
    batch(() => {
      const session = this.sessions.get(id);
      if (!session || session.reauthRequiredAt === null) return;
      this.sessions.update(id, { reauthRequiredAt: null });
      this.#persistAuthenticationPatch(id, { reauthRequiredAt: null });
      this.#persist();
    });
  }

  /** Return a usable access token, rotating the persisted pair when needed. */
  renewServerAuthentication(id: string, force = false): Promise<string | null> {
    const existing = this.#renewalPromises.get(id);
    if (existing) {
      if (!force) return existing;
      const tokenBeforeWait = this.sessions.get(id)?.token ?? null;
      return existing.then((token) => {
        if (!token || token !== tokenBeforeWait) return token;
        return this.renewServerAuthentication(id, true);
      });
    }
    const renewal = this.#renewServerAuthentication(id, force).finally(() => {
      if (this.#renewalPromises.get(id) === renewal) this.#renewalPromises.delete(id);
    });
    this.#renewalPromises.set(id, renewal);
    return renewal;
  }

  async #renewServerAuthentication(id: string, force: boolean): Promise<string | null> {
    const originalToken = this.sessions.get(id)?.token ?? null;
    return this.#withRefreshLock(id, async () => {
      this.#adoptPersistedBearerSession(id);
      let session = this.sessions.get(id);
      if (!session?.token) return null;
      const registration = this.catalog.get(id);
      if (!registration || !session.refreshToken) {
        this.handleAuthenticationRequired(id);
        return null;
      }
      if (session.reauthRequiredAt !== null) return null;

      if (session.token !== originalToken) return session.token;
      if (!force && (session.accessTokenExpiresAt ?? 0) > Date.now()) {
        return session.token;
      }

      const requestId = session.refreshRequestId || crypto.randomUUID();
      if (session.refreshRequestId !== requestId) {
        this.sessions.update(id, { refreshRequestId: requestId });
        session = this.sessions.get(id);
      }
      if (!session?.refreshToken) {
        this.handleAuthenticationRequired(id);
        return null;
      }
      // Rotation is unsafe unless a lost response can be retried with the
      // exact same ID after a reload or in another tab. StorageSlot writes
      // are intentionally best-effort elsewhere, so verify this security-
      // sensitive write before sending the refresh credential.
      this.#persistAuthentication(id);
      this.#persist();
      const persistedRequestId = this.#storage.readAuthentication(id)?.refreshRequestId;
      if (persistedRequestId !== requestId) {
        throw new Error('Unable to persist bearer renewal state.');
      }

      const response = await fetch(new URL('/oauth/token', registration.url), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          grant_type: 'refresh_token',
          refresh_token: session.refreshToken,
          refresh_request_id: requestId,
          client_id: session.oauthClientId ?? ''
        }),
        signal: AbortSignal.timeout(10_000)
      });
      const body: Record<string, unknown> = await response.json().catch(() => ({}));
      // Sign-out or another tab's rotation can finish while this request is
      // in flight. A stale response must not change the new local session.
      const current = this.sessions.get(id);
      const persisted = this.#storage.readAuthentication(id);
      if (
        current?.refreshToken !== session.refreshToken ||
        current?.refreshRequestId !== requestId ||
        persisted?.refreshToken !== session.refreshToken ||
        persisted?.refreshRequestId !== requestId
      )
        return null;
      if (!response.ok) {
        if (response.status === 400 && body.error === 'invalid_grant') {
          this.handleAuthenticationRequired(id);
          return null;
        }
        throw new Error(
          typeof body.error_description === 'string'
            ? body.error_description
            : `Bearer session renewal failed (${response.status})`
        );
      }

      const credentials = oauthBearerSession(body, session.oauthClientId ?? null);
      if (!credentials) throw new Error('The server returned an invalid bearer session.');
      this.#updateBearerSessionInPlace(id, {
        ...persistedBearerSession(credentials),
        reauthRequiredAt: null
      });
      return credentials.token;
    });
  }

  async #withRefreshLock<T>(id: string, operation: () => Promise<T>): Promise<T> {
    if (typeof navigator !== 'undefined' && navigator.locks) {
      return navigator.locks.request(`chatto:bearer-refresh:${id}`, operation);
    }
    return operation();
  }

  #adoptPersistedBearerSession(id: string): void {
    const persisted = this.#storage.readAuthentication(id);
    // Without device storage (for example in Node), and for fixed tokens that
    // are never stored, the in-memory session is the only copy, and another
    // tab cannot have rotated it.
    if (this.#fixedTokenServers.has(id)) return;
    if (persisted === undefined && !this.#storage.shared) return;
    const current = this.sessions.get(id);
    if (!current) return;
    if (!persisted?.token) {
      if (current.token) this.clearServerAuthentication(id);
      return;
    }
    if (
      persisted.token === current.token &&
      persisted.refreshToken === current.refreshToken &&
      persisted.accessTokenExpiresAt === current.accessTokenExpiresAt &&
      persisted.refreshTokenExpiresAt === current.refreshTokenExpiresAt &&
      persisted.oauthClientId === current.oauthClientId &&
      persisted.refreshRequestId === current.refreshRequestId &&
      persisted.reauthRequiredAt === current.reauthRequiredAt
    ) {
      return;
    }
    this.#updateBearerSessionInPlace(
      id,
      {
        token: persisted.token,
        refreshToken: persisted.refreshToken,
        accessTokenExpiresAt: persisted.accessTokenExpiresAt,
        refreshTokenExpiresAt: persisted.refreshTokenExpiresAt,
        oauthClientId: persisted.oauthClientId,
        refreshRequestId: persisted.refreshRequestId,
        reauthRequiredAt: persisted.reauthRequiredAt
      },
      false
    );
  }

  #updateBearerSessionInPlace(id: string, data: Partial<ServerSession>, persist = true): void {
    batch(() => {
      if (!this.sessions.update(id, data)) return;
      if (persist) this.#persistAuthentication(id);
      this.#persist();
      this.#context.connections.updateBearerSession(id);
    });
  }

  /**
   * Bootstrap the registry: create stores for all registered servers.
   * Call once at application startup, before any computed value or UI reads stores.
   */
  init(): void {
    for (const registration of this.registrations) {
      if (!this.#stores.has(registration.id)) this.#createStore(registration.id);
    }
  }

  /** Start discovery and remote viewer recovery for a store once. */
  #startServerNetwork(serverId: string): void {
    const store = this.#stores.get(serverId);
    if (!store || this.#startedServerNetwork.has(serverId)) return;
    this.#startedServerNetwork.add(serverId);
    void store.serverInfo.init().catch(() => {
      // Recovery observes the discovery error on the store.
    });

    const session = this.sessions.ensure(serverId);
    if (this.isOriginServer(serverId)) return;
    if (session.token === null) {
      store.currentUser.reset();
      return;
    }
    if (!store.currentUser.user) void store.currentUser.load();
  }

  /** Add a server and create its retained state store. Transport ownership is centralized. */
  addServer(
    registration: ServerRegistration | RegisteredServer,
    session?: ServerSession,
    options: {
      /** The session's bearer token is fixed and must never be renewed. */
      fixedToken?: boolean;
    } = {}
  ): void {
    if (this.#disposed) return;
    batch(() => {
      const publicRegistration: ServerRegistration = {
        id: registration.id,
        url: registration.url,
        name: registration.name,
        iconUrl: registration.iconUrl,
        addedAt: registration.addedAt
      };
      const localSession =
        session ??
        ('token' in registration ? sessionFromServer(registration) : emptyServerSession());
      claimServerId(registration.id, this);
      if (!this.catalog.add(publicRegistration)) return;
      if (options.fixedToken) this.#fixedTokenServers.add(registration.id);
      this.sessions.replace(registration.id, localSession);
      this.#persistAuthentication(registration.id);
      this.#persist();
      try {
        this.#createStore(registration.id);
      } catch (error) {
        // A server without a store must not stay registered.
        this.removeServer(registration.id);
        throw error;
      }
    });
  }

  /** Remove a server by ID. Disposes its event bus, store, and connection state. */
  removeServer(id: string): boolean {
    return batch(() => {
      const server = this.servers.find((s) => s.id === id);
      if (!server) {
        return false;
      }
      // Stop event bus subscription
      this.#context.realtime.stopBus(id);

      // Dispose state store
      this.#stores.get(id)?.dispose();
      this.#stores.delete(id);
      this.#startedServerNetwork.delete(id);
      // A fixed token was never written to device storage; nothing to clear.
      const fixedToken = this.#fixedTokenServers.delete(id);

      // Dispose connection state
      this.#context.connections.destroyClient(id);

      this.sessions.remove(id);
      this.catalog.remove(id);
      releaseServerId(id, this);
      if (!fixedToken) this.#storage.writeAuthentication(id, emptyServerAuthentication());
      this.#persist();
      return true;
    });
  }

  /** Remove all local registrations and sessions without synchronizing deletions. */
  removeAll(): void {
    batch(() => {
      const ids = this.servers.map((server) => server.id);
      const persistedIds = ids.filter((id) => !this.#fixedTokenServers.has(id));
      this.#disposeServers(ids);
      for (const id of persistedIds)
        this.#storage.writeAuthentication(id, emptyServerAuthentication());
      this.sessions.clear();
      this.catalog.reset();
      this.#persist();
    });
  }

  /** Clear every session and remote registration while retaining the configured origin. */
  resetToOrigin(): void {
    batch(() => {
      const origin = this.originServer;
      const ids = this.servers.map((server) => server.id);
      const persistedIds = ids.filter((id) => !this.#fixedTokenServers.has(id));
      this.#disposeServers(ids);
      for (const id of persistedIds)
        this.#storage.writeAuthentication(id, emptyServerAuthentication());
      this.sessions.clear();
      this.catalog.reset(origin ? [registrationFromServer(origin)] : []);
      if (origin) {
        claimServerId(origin.id, this);
        this.sessions.ensure(origin.id);
        this.#persistAuthentication(origin.id);
        this.#createStore(origin.id);
        this.settleOriginUnauthenticated();
      }
      this.#persist();
    });
  }

  /**
   * Dispose every store, connection, and event bus and release the server
   * IDs. Device storage is left as it is, so a later client restores it.
   */
  dispose(): void {
    batch(() => {
      this.#disposeServers(this.servers.map((server) => server.id));
      this.sessions.clear();
      this.catalog.reset();
    });
    // Work that is still running, such as an origin probe or a renewal, must
    // not register servers or write the device storage of a later client.
    this.#disposed = true;
    this.#storage = memoryStorage();
  }

  #disposeServers(ids: string[]): void {
    for (const id of ids) {
      this.#context.realtime.stopBus(id);
      this.#stores.get(id)?.dispose();
      this.#stores.delete(id);
      this.#startedServerNetwork.delete(id);
      this.#fixedTokenServers.delete(id);
      this.#context.connections.destroyClient(id);
      releaseServerId(id, this);
    }
  }

  /** Update device-local public metadata without touching the local session. */
  updateRegistration(id: string, data: ServerRegistrationMetadataPatch): boolean {
    if (!this.catalog.update(id, data)) return false;
    this.#persist();
    return true;
  }

  replaceServerAuthentication(
    id: string,
    data: Pick<
      RegisteredServer,
      | 'token'
      | 'refreshToken'
      | 'accessTokenExpiresAt'
      | 'refreshTokenExpiresAt'
      | 'oauthClientId'
      | 'refreshRequestId'
      | 'userId'
      | 'userLogin'
      | 'userDisplayName'
      | 'userAvatarUrl'
      | 'reauthRequiredAt'
    >
  ): boolean {
    return this.#replaceServerAuth(id, data);
  }

  #replaceServerAuth(
    id: string,
    data: Pick<
      RegisteredServer,
      | 'token'
      | 'refreshToken'
      | 'accessTokenExpiresAt'
      | 'refreshTokenExpiresAt'
      | 'oauthClientId'
      | 'refreshRequestId'
      | 'userId'
      | 'userLogin'
      | 'userDisplayName'
      | 'userAvatarUrl'
      | 'reauthRequiredAt'
    >,
    startNetwork = true
  ): boolean {
    return batch(() => {
      if (!this.catalog.get(id) || !this.sessions.get(id)) return false;
      this.#context.realtime.stopBus(id);
      this.#stores.get(id)?.dispose();
      this.#stores.delete(id);
      this.#startedServerNetwork.delete(id);
      this.#context.connections.destroyClient(id);

      this.sessions.replace(id, data);
      this.#persistAuthentication(id);
      this.#persist();
      this.#createStore(id, startNetwork);
      return true;
    });
  }

  #persist(): void {
    // The combined record remains a migration/compatibility adapter. Merge
    // independently persisted authentication at write time so a stale tab's
    // metadata snapshot can never put old rotated credentials back into it.
    this.#storage.writeServers(
      // Fixed tokens, such as bot API keys, stay in memory only.
      this.servers
        .filter((server) => !this.#fixedTokenServers.has(server.id))
        .map((server) => {
          const persisted = this.#storage.readAuthentication(server.id);
          return {
            ...server,
            ...(persisted === undefined
              ? authenticationFromSession(server)
              : (persisted ?? emptyServerAuthentication()))
          };
        })
    );
  }

  #persistAuthentication(id: string): boolean {
    const session = this.sessions.get(id);
    if (!session) return false;
    if (this.#fixedTokenServers.has(id)) return true;
    return this.#storage.writeAuthentication(id, authenticationFromSession(session));
  }

  #persistAuthenticationPatch(id: string, patch: Partial<ServerAuthentication>): boolean {
    const session = this.sessions.get(id);
    if (!session) return false;
    if (this.#fixedTokenServers.has(id)) return true;
    const stored = this.#storage.readAuthentication(id);
    const current = stored ?? authenticationFromSession(session);
    return this.#storage.writeAuthentication(id, { ...current, ...patch });
  }

  /** Whether the server's bearer token is fixed; see {@link addServer}. */
  hasFixedToken(id: string): boolean {
    return this.#fixedTokenServers.has(id);
  }

  /** Get a server by ID. */
  getServer(id: string): RegisteredServer | undefined {
    return this.servers.find((s) => s.id === id);
  }

  /**
   * Get the state store for a registered server.
   * Safe in computed values — stores are created atomically with registration,
   * so every registered server always has a store.
   */
  getStore(serverId: string): Server {
    const store = this.#stores.get(serverId);
    if (!store) {
      throw new Error(
        `No store for server "${serverId}". Is it registered? ` +
          `Call registry.init() before accessing stores.`
      );
    }
    return store;
  }

  /**
   * Get the state store for a registered server, or undefined if not found.
   * Use when the server may not be registered (e.g., unresolved URL segments).
   */
  tryGetStore(serverId: string): Server | undefined {
    return this.#stores.get(serverId);
  }

  /** Whether discovery or a retained bearer session still needs recovery. */
  needsRecovery(id: string): boolean {
    const store = this.#stores.get(id);
    const session = this.sessions.get(id);
    if (!store || !session) return false;
    return (
      store.serverInfo.error !== null ||
      Boolean(
        session.token &&
        session.reauthRequiredAt === null &&
        (!store.currentUser.user || store.currentUser.verifiedUserId === null) &&
        !store.currentUser.loading
      )
    );
  }

  /**
   * Retry discovery that failed or found a server this client cannot use, then
   * restore the viewer without starting an OAuth flow.
   */
  async recoverServer(id: string): Promise<void> {
    const store = this.#stores.get(id);
    if (!store) return;
    this.#startServerNetwork(id);
    if (store.serverInfo.compatibilityProblem !== null) await store.serverInfo.init();
    if (this.#stores.get(id) !== store || store.serverInfo.error !== null) return;
    const session = this.sessions.get(id);
    if (
      !session ||
      (!this.isOriginServer(id) && !session.token) ||
      session.reauthRequiredAt !== null ||
      (store.currentUser.user && store.currentUser.verifiedUserId !== null)
    )
      return;
    await store.currentUser.load();
  }

  /** Check the private-data boundary before publishing a complete account response. */
  #acceptViewer(id: string, owner: Server, user: CurrentUser): void {
    batch(() => {
      if (this.#stores.get(id) !== owner) return;
      if (this.isOriginServer(id)) {
        this.authenticateOriginCookie(user);
      } else {
        const session = this.sessions.get(id);
        if (!session || session.reauthRequiredAt !== null) return;
        if (session.userId && session.userId !== user.id) {
          this.#replaceServerAuth(
            id,
            {
              ...session,
              userId: user.id,
              userLogin: user.login,
              userDisplayName: user.displayName,
              userAvatarUrl: user.avatarUrl ?? null
            },
            false
          );
        }
        const store = this.#stores.get(id);
        if (!store) return;
        store.currentUser.accept(user);
        this.#startServerNetwork(id);
        this.sessions.update(id, {
          userId: user.id,
          userLogin: user.login,
          userDisplayName: user.displayName,
          userAvatarUrl: user.avatarUrl
        });
        this.#persist();
      }
    });
  }

  /** Create a state store for a server and wire up remote user sync. */
  #createStore(serverId: string, startNetwork = true): Server {
    const registration = this.catalog.get(serverId);
    if (!registration) throw new Error(`Server "${serverId}" not found in catalogue`);
    this.sessions.ensure(serverId);
    const serverConnection = this.#context.connections.getClient(serverId);
    const store = new Server(
      registration,
      () => this.sessions.ensure(serverId),
      this.isOriginServer(serverId),
      serverConnection,
      {
        realtime: this.#context.realtime,
        remove: () => this.removeServer(serverId),
        fixedToken: this.#fixedTokenServers.has(serverId)
      },
      undefined,
      () => {
        if (this.isOriginServer(serverId) && !store.currentUser.user) {
          this.clearOriginAuthentication();
        } else this.handleAuthenticationRequired(serverId);
      },
      (user) => this.#acceptViewer(serverId, store, user)
    );
    this.#stores.set(serverId, store);
    for (const watcher of [...this.#storeWatchers]) watcher(store);
    if (startNetwork) this.#startServerNetwork(serverId);

    return store;
  }

  /**
   * Call `setup` for every server store of this registry: for the current
   * stores now, and for each new store when the registry creates it, before
   * the store receives realtime data. Use it to subscribe to store events,
   * such as `onReset`. `setup` can return a cleanup function. It runs when the
   * store is disposed or when the returned function stops the watch. A
   * failing `setup` is logged and does not stop the others.
   */
  watchStores(setup: (store: Server) => (() => void) | void): () => void {
    const detachers = new Map<Server, () => void>();
    const attach = (store: Server) => {
      let cleanup: (() => void) | void = undefined;
      try {
        cleanup = untrack(() => setup(store));
      } catch (error) {
        console.error('[chatto-client] a store watcher failed', error);
      }
      let stopDisposeListener = () => {};
      const detach = () => {
        stopDisposeListener();
        detachers.delete(store);
        cleanup?.();
      };
      stopDisposeListener = store.onDispose(detach);
      detachers.set(store, detach);
    };
    for (const store of untrack(() => [...this.#stores.values()])) attach(store);
    this.#storeWatchers.add(attach);
    return () => {
      this.#storeWatchers.delete(attach);
      for (const detach of [...detachers.values()]) detach();
    };
  }

  /** Whether the server has an authenticated user. False if not registered. */
  isAuthenticated(serverId: string): boolean {
    return this.tryGetStore(serverId)?.isAuthenticated ?? false;
  }
}
