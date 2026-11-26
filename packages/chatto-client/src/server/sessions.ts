import type { ServerRegistration } from './catalog.js';
import { ReactiveMap } from '../reactivity/index.js';

/** Device-local authentication state for one known Chatto server. */
export interface ServerSession {
  /** Bearer token for API auth, or null when unauthenticated/legacy cookie auth */
  token: string | null;
  /** Device-local ID for one sign-in. It stays the same across token rotations. */
  bearerSessionId?: string | null;
  /** Rotating renewal credential; absent on pre-0.5 persisted sessions. */
  refreshToken?: string | null;
  /** Access credential expiry as Unix epoch milliseconds. */
  accessTokenExpiresAt?: number | null;
  /** Expiry of the current renewable-session window as Unix epoch milliseconds. */
  refreshTokenExpiresAt?: number | null;
  /** Public OAuth client identifier, or null for direct origin login. */
  oauthClientId?: string | null;
  /** Persisted idempotency key for an in-flight refresh rotation. */
  refreshRequestId?: string | null;
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

/** Public catalogue metadata and the device-local session for one registered server. */
export interface RegisteredServer extends ServerRegistration, ServerSession {}

export function emptyServerSession(): ServerSession {
  return {
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
  };
}

/** Owns credentials and local user summaries without server catalogue data. */
export class ServerSessions {
  #sessions = new ReactiveMap<string, ServerSession>();

  constructor(initial: Iterable<readonly [string, ServerSession]> = []) {
    for (const [id, session] of initial) {
      this.#sessions.set(id, { ...emptyServerSession(), ...session });
    }
  }

  get(id: string): ServerSession | undefined {
    return this.#sessions.get(id);
  }

  ensure(id: string): ServerSession {
    let session = this.#sessions.get(id);
    if (!session) {
      session = emptyServerSession();
      this.#sessions.set(id, session);
    }
    return session;
  }

  replace(id: string, session: ServerSession): ServerSession {
    const replacement = { ...emptyServerSession(), ...session };
    this.#sessions.set(id, replacement);
    return replacement;
  }

  update(id: string, data: Partial<ServerSession>): boolean {
    const session = this.#sessions.get(id);
    if (!session) return false;
    this.#sessions.set(id, { ...session, ...data });
    return true;
  }

  remove(id: string): boolean {
    return this.#sessions.delete(id);
  }

  clear(): void {
    this.#sessions.clear();
  }
}
