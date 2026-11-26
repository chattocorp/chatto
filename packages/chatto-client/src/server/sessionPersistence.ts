/** Persists client sessions and checks whether stored credentials belong to the current sign-in. */
import { sha256 } from 'js-sha256';
import { Codecs, globalSlot, serverSlot } from '../storage/slot.js';
import type { ServerRegistration } from './catalog.js';
import { emptyServerSession, type ServerSession, type RegisteredServer } from './sessions.js';

// Storage key intentionally stays as 'instances' — renaming would lose users'
// multi-server registrations (including remote bearer tokens that can't be
// regenerated). The in-code rename is purely cosmetic.
type PersistedRegisteredServer = Omit<RegisteredServer, 'bearerSessionId'> & {
  source?: 'local' | 'synced';
  /** Sign-in that supplied the cached profile. Older tabs omit this when they write the catalogue. */
  profileSignInId?: string | null;
};

type PersistedServerAuthentication = {
  version: 1;
  /** Local sign-in ID shared by all rotations of this credential pair. */
  bearerSessionId?: string | null;
  token: string | null;
  refreshToken: string | null;
  accessTokenExpiresAt: number | null;
  refreshTokenExpiresAt: number | null;
  oauthClientId: string | null;
  refreshRequestId: string | null;
  reauthRequiredAt: number | null;
};

/** Durable credential fields; hosts must never log their values. */
export type ServerAuthentication = Omit<PersistedServerAuthentication, 'version'>;

function normalizeRegisteredServer(server: PersistedRegisteredServer): PersistedRegisteredServer {
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
    isOptionalNullableString(authentication.bearerSessionId) &&
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
    !isOptionalNullableString(server.profileSignInId) ||
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

/** Extract catalogue fields from a combined registration and session. */
export function registrationFromServer(server: RegisteredServer): ServerRegistration {
  return {
    id: server.id,
    url: server.url,
    name: server.name,
    iconUrl: server.iconUrl,
    addedAt: server.addedAt
  };
}

/** Extract the session fields from a combined registration and session. */
export function sessionFromServer(server: RegisteredServer): ServerSession {
  return {
    ...authenticationFromSession(server),
    userId: server.userId,
    userLogin: server.userLogin,
    userDisplayName: server.userDisplayName,
    userAvatarUrl: server.userAvatarUrl
  };
}

function authenticationFromSession(
  session: ServerSession | ServerAuthentication
): ServerAuthentication {
  return {
    bearerSessionId: session.bearerSessionId ?? null,
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

/** Tabs that hold the same pre-upgrade pair must agree on its ID before either rotates it. */
export function legacyBearerSessionId(access: string, refresh: string): string {
  return 'legacy:' + sha256(JSON.stringify([access, refresh]));
}

/** Adopt a rotation only when the sign-in ID or the original credential pair proves continuity. */
function sameBearerSession(session: ServerSession, stored: ServerAuthentication): boolean {
  if (session.bearerSessionId && stored.bearerSessionId)
    return session.bearerSessionId === stored.bearerSessionId;
  if (session.token === stored.token && (session.refreshToken ?? null) === stored.refreshToken)
    return true;
  return !!(
    !session.bearerSessionId &&
    stored.bearerSessionId?.startsWith('legacy:') &&
    session.token &&
    session.refreshToken &&
    legacyBearerSessionId(session.token, session.refreshToken) === stored.bearerSessionId
  );
}

/** Keep cached profile fields only when the credentials still identify the same session. */
function mergeAuthentication<T extends ServerSession>(
  session: T,
  stored: ServerAuthentication | null
): T {
  const auth = authenticationFromSession(stored ?? emptyServerSession());
  const clearProfile = (session.token || auth.token) && !sameBearerSession(session, auth);
  return {
    ...session,
    ...(clearProfile ? emptyServerSession() : {}),
    ...auth
  };
}

/** Split the legacy combined persistence shape into its runtime owners. */
export function splitPersistedServers(servers: PersistedRegisteredServer[]): {
  registrations: ServerRegistration[];
  sessions: Array<readonly [string, ServerSession]>;
} {
  const normalized = servers.map(normalizeRegisteredServer);
  return {
    registrations: normalized.map(registrationFromServer),
    sessions: normalized.map((server) => {
      const session = sessionFromServer(server);
      session.bearerSessionId = server.profileSignInId ?? null;
      // Older tabs could put their stale viewer beside newer credentials.
      if (session.refreshToken && !session.bearerSessionId)
        return [
          server.id,
          { ...emptyServerSession(), ...authenticationFromSession(session) }
        ] as const;
      return [server.id, session] as const;
    })
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
  return authenticationFromSession(stored);
}

function persistAuthentication(serverId: string, authentication: ServerAuthentication): boolean {
  const slot = authenticationSlot(serverId);
  slot.set({ version: 1, ...authentication });
  const stored = readPersistedAuthentication(serverId);
  return !!stored && sameAuthentication(stored, authentication);
}

function sameAuthentication(
  stored: ServerAuthentication,
  authentication: ServerAuthentication
): boolean {
  return (
    (stored.bearerSessionId ?? null) === (authentication.bearerSessionId ?? null) &&
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
  if (stored.some((server) => server.source !== undefined)) {
    serversSlot.set(stored.map(normalizeRegisteredServer));
  }
  const state = restoreSessions(stored, deviceStorage);
  clearRetiredAccountDataStorage();
  return state;
}

function restoreSessions(servers: PersistedRegisteredServer[], storage: RegistryStorage) {
  const state = splitPersistedServers(servers);
  for (const [id, session] of state.sessions) {
    const auth = storage.readAuthentication(id);
    if (auth !== undefined) Object.assign(session, mergeAuthentication(session, auth));
    else if (storage === deviceStorage)
      storage.writeAuthentication(id, authenticationFromSession(session));
  }
  return state;
}

/** Where a registry keeps the server catalogue and authentication records. */
interface RegistryStorage {
  readServers?(): RegisteredServer[];
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

/**
 * Synchronous durable persistence supplied by a native host.
 * Writes must be committed before returning; renewal reads back its request ID
 * before sending. Hosts own encryption, exclusive access, validation and cleanup.
 * Authentication records override catalogue snapshots when restoring after a crash.
 */
export interface ServerRegistryPersistence extends RegistryStorage {
  /** Read the saved combined catalogue; latest authentication is read separately. */
  readServers(): RegisteredServer[];
}

/** Owns one client's storage backend. The registry owns the lifetime of live sessions and viewers. */
export class SessionPersistence {
  readonly #storage: RegistryStorage;

  constructor(options: { deviceStorage: boolean; persistence?: ServerRegistryPersistence }) {
    if (options.deviceStorage && options.persistence)
      throw new Error('Choose browser device storage or host persistence, not both.');
    this.#storage =
      options.persistence ?? (options.deviceStorage ? deviceStorage : memoryStorage());
  }

  /** Restore the catalogue once at startup. Only browser storage needs legacy writeback. */
  restore(): ReturnType<typeof splitPersistedServers> {
    if (this.#storage === deviceStorage) return restorePersistedServerState();
    if (!this.#storage.readServers) return { registrations: [], sessions: [] };
    const saved = this.#storage.readServers();
    if (!isPersistedServerArray(saved)) throw new Error('Invalid persisted server catalogue.');
    return restoreSessions(saved, this.#storage);
  }

  /** Read current credentials to fence an in-flight rotation against newer writes. */
  readAuthentication(id: string) {
    return this.#storage.readAuthentication(id);
  }

  /** Save the credential fields. The result reports whether storage accepted the write. */
  writeSession(id: string, session: ServerSession): boolean {
    return this.#storage.writeAuthentication(id, authenticationFromSession(session));
  }

  /** Persist a sign-out marker so older catalogue credentials cannot be restored. */
  clearAuthentication(id: string): void {
    this.#storage.writeAuthentication(id, emptyServerAuthentication());
  }

  /** Apply a partial update only to the sign-in that owns the stored credentials. */
  patchAuthentication(
    id: string,
    session: ServerSession,
    patch: Partial<ServerAuthentication>
  ): boolean {
    const stored = this.readAuthentication(id);
    if (stored && (session.token || stored.token) && !sameBearerSession(session, stored))
      return false;
    return this.#storage.writeAuthentication(id, {
      ...(stored ?? authenticationFromSession(session)),
      ...patch
    });
  }

  /** Save public metadata with the latest credentials and their matching cached profile. */
  writeServers(servers: RegisteredServer[]): void {
    this.#storage.writeServers(
      servers.map((server) => {
        const stored = this.readAuthentication(server.id);
        const merged = stored === undefined ? server : mergeAuthentication(server, stored);
        const { bearerSessionId, ...snapshot } = merged;
        return { ...snapshot, profileSignInId: bearerSessionId ?? null };
      })
    );
  }

  /** Decide what a live tab may adopt. Missing private storage leaves the in-memory session intact. */
  bearerSessionChange(
    id: string,
    current: ServerSession
  ):
    | { action: 'keep' }
    | { action: 'discard' }
    | { action: 'adopt'; credentials: ServerAuthentication } {
    const stored = this.readAuthentication(id);
    if (stored === undefined && !this.#storage.shared) return { action: 'keep' };
    if (!stored?.token || !sameBearerSession(current, stored))
      return { action: current.token ? 'discard' : 'keep' };
    if (sameAuthentication(stored, authenticationFromSession(current))) return { action: 'keep' };
    return { action: 'adopt', credentials: authenticationFromSession(stored) };
  }

  /** Check a pending viewer response against any sign-in change made by another tab. */
  viewerSessionChanged(id: string, session: ServerSession): boolean {
    if (!session.token || !this.#storage.shared) return false;
    const stored = this.readAuthentication(id);
    return !stored || !sameBearerSession(session, stored);
  }
}
