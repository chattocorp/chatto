import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
import { createContext } from 'svelte';
import type { Client } from '@urql/svelte';
import { LoadCurrentUserDocument, clearCachedUser, type CurrentUser } from './loadAuth';

export type { CurrentUser };

/**
 * Per-instance current user state. Tracks who the authenticated user is on a
 * given Chatto instance.
 *
 * Cookie-authenticated instances (origin) handle auth failure with a full
 * logout flow (clear cookie, redirect to login). Bearer-authenticated instances
 * (remotes) just clear the local user state.
 */
export class CurrentUserState {
  user = $state<CurrentUser | undefined>(undefined);
  loading = $state(true);
  #client: Client;
  #cookieAuth: boolean;
  #isLoggingOut = false;

  constructor(client: Client, cookieAuth: boolean = false) {
    this.#client = client;
    this.#cookieAuth = cookieAuth;
  }

  async load() {
    const resp = await this.#client.query(LoadCurrentUserDocument, {});

    if (resp.error) {
      // Surface network failures (CORS, DNS, server down) as a console
      // error so unreachable instances are visible in the dev console.
      // Don't throw — the caller treats this as a per-instance soft
      // failure, not a global crash.
      console.error('[auth] failed to load current user', resp.error);
    }

    if (resp.data?.me) {
      this.user = resp.data.me;
    }
    this.loading = false;
  }

  /**
   * Re-validate the session by checking Query.me.
   * If the session has expired, triggers logout and redirect (cookie auth)
   * or clears user state (bearer auth).
   */
  async validateSession() {
    if (this.loading || this.#isLoggingOut) return;
    if (!this.user) return;

    const resp = await this.#client.query(
      LoadCurrentUserDocument,
      {},
      { requestPolicy: 'network-only' }
    );

    // Network error (e.g., dead TCP connection after sleep) — don't treat as auth failure.
    if (resp.error?.networkError) {
      console.log('Session validation skipped — network error:', resp.error.networkError.message);
      return;
    }

    if (!resp.data?.me) {
      console.warn('[auth] validateSession: server returned me=null — triggering auth failure');
      this.handleAuthFailure();
    } else {
      this.user = resp.data.me;
    }
  }

  /**
   * Handle auth failure.
   * Cookie auth (origin): clears session and redirects to login.
   * Bearer auth (remote): clears user state (instance becomes unauthenticated).
   */
  async handleAuthFailure() {
    if (this.#isLoggingOut) return;

    if (!this.#cookieAuth) {
      console.log('Remote instance auth failure — clearing user');
      this.user = undefined;
      this.loading = false;
      return;
    }

    this.#isLoggingOut = true;

    console.warn('[auth] handleAuthFailure → /: clearing session and redirecting');
    this.user = undefined;

    clearCachedUser();

    sessionStorage.setItem('returnUrl', window.location.pathname + window.location.search);

    // Clear the session cookie by calling the logout endpoint. This is necessary
    // because with cookie-based sessions, the session data lives in the cookie itself.
    // When another tab/device triggers logout, this tab still has the old cookie.
    // Without clearing it, the server would still see a valid session on redirect.
    await fetch('/auth/logout', { method: 'POST' }).catch(() => {});

    // Redirect to / which handles both authenticated and unauthenticated users.
    // invalidateAll forces SvelteKit to re-run all load functions so the root
    // layout sees the cleared user state.
    goto(resolve('/'), { invalidateAll: true }).finally(() => {
      this.#isLoggingOut = false;
    });
  }
}

export const [getCurrentUser, setCurrentUser] = createContext<CurrentUserState>();

/**
 * Initialize an empty current user context. Use this at the root layout level
 * to make the context available throughout the app.
 *
 * This does NOT fetch the user - it just sets up an empty state. Routes that
 * require authentication (like /chat) should use initCurrentUserFromData()
 * to populate the user from their load function data.
 */
export function initCurrentUserContext(): CurrentUserState {
  const s = new CurrentUserState(graphqlClientManager.originClient.client, true);
  s.loading = false;
  setCurrentUser(s);
  return s;
}

export async function initCurrentUser() {
  const s = setCurrentUser(
    new CurrentUserState(graphqlClientManager.originClient.client, true)
  );
  await s.load();
  return s;
}

/**
 * Initialize the current user context synchronously from data loaded in a SvelteKit load function.
 *
 * Use this when the load function has already verified authentication and loaded the user.
 * This avoids the async loading state since we already have the user data.
 *
 * @param user - The user data from the load function
 * @returns The initialized CurrentUserState
 */
export function initCurrentUserFromData(user: CurrentUser): CurrentUserState {
  const s = new CurrentUserState(graphqlClientManager.originClient.client, true);
  s.user = user;
  s.loading = false;
  setCurrentUser(s);
  return s;
}
