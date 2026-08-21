import { SvelteMap } from 'svelte/reactivity';

/** Device-local authentication state for one known Chatto server. */
export interface ServerSession {
  token: string | null;
  /** Rotating renewal credential; absent on pre-0.5 persisted sessions. */
  refreshToken?: string | null;
  /** Access credential expiry as Unix epoch milliseconds. */
  accessTokenExpiresAt?: number | null;
  /** Absolute renewable-session expiry as Unix epoch milliseconds. */
  refreshTokenExpiresAt?: number | null;
  /** Public OAuth client identifier, or null for direct origin login. */
  oauthClientId?: string | null;
  /** Persisted idempotency key for an in-flight refresh rotation. */
  refreshRequestId?: string | null;
  userId: string | null;
  userLogin: string | null;
  userDisplayName: string | null;
  userAvatarUrl: string | null;
  reauthRequiredAt: number | null;
}

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
  #sessions = new SvelteMap<string, ServerSession>();

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
