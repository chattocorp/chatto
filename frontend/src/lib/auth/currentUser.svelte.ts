import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import type { Client } from '@urql/svelte';
import { LoadCurrentUserDocument, clearCachedUser, type CurrentUser } from './loadAuth';
import { csrfFetch } from './csrf';

export type { CurrentUser };

/**
 * Per-server current-user state. One instance per registered server,
 * owned by `ServerStateStore`. Consumers read the active server's
 * instance via `serverRegistry.getStore(getServerId()).currentUser`, the
 * same way they reach every other per-server store.
 *
 * Cookie-authenticated instances (origin) handle auth failure with a full
 * logout flow (clear cookie, redirect to login). Bearer-authenticated
 * instances (remotes) just clear the local user state.
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

    const fetched = resp.data?.viewer?.user;
    if (fetched) {
      this.user = fetched;
    }
    this.loading = false;
  }

  /**
   * Re-validate the session by checking Query.viewer.
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

    const fetched = resp.data?.viewer?.user;
    if (!fetched) {
      console.warn('[auth] validateSession: server returned viewer=null — triggering auth failure');
      this.handleAuthFailure();
    } else {
      this.user = fetched;
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
    await csrfFetch('/auth/logout', { method: 'POST' }).catch(() => {});

    // Redirect to / which handles both authenticated and unauthenticated users.
    // invalidateAll forces SvelteKit to re-run all load functions so the root
    // layout sees the cleared user state.
    goto(resolve('/'), { invalidateAll: true }).finally(() => {
      this.#isLoggingOut = false;
    });
  }
}
