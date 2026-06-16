import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import type { Client } from '@urql/svelte';
import { LoadCurrentUserDocument, clearCachedUser, type CurrentUser } from './loadAuth';
import { csrfFetch } from './csrf';

export type { CurrentUser };

interface AuthFailureOptions {
  revokeServerSession?: boolean;
}

/**
 * Per-server current-user state. One instance per registered server,
 * owned by `ServerStateStore`. Consumers read the active server's
 * instance via `serverRegistry.getStore(getServerId()).currentUser`, the
 * same way they reach every other per-server store.
 *
 * Cookie-authenticated instances (origin) handle auth failure by clearing
 * local state and redirecting. Bearer-authenticated
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
   * Handle auth failure.
   * Cookie auth (origin): clears local state and redirects to login. Explicit
   * sign-out paths can request server-side session revocation.
   * Bearer auth (remote): clears user state (instance becomes unauthenticated).
   */
  async handleAuthFailure(options: AuthFailureOptions = {}) {
    if (this.#isLoggingOut) return;

    if (!this.#cookieAuth) {
      console.log('Remote instance auth failure — clearing user');
      this.user = undefined;
      this.loading = false;
      return;
    }

    this.#isLoggingOut = true;

    console.warn('[auth] handleAuthFailure -> /: clearing local session state and redirecting');
    this.user = undefined;

    clearCachedUser();

    sessionStorage.setItem('returnUrl', window.location.pathname + window.location.search);

    if (options.revokeServerSession) {
      await csrfFetch('/auth/logout', { method: 'POST' }).catch(() => {});
    }

    // Redirect to / which handles both authenticated and unauthenticated users.
    // invalidateAll forces SvelteKit to re-run all load functions so the root
    // layout sees the cleared user state.
    goto(resolve('/'), { invalidateAll: true }).finally(() => {
      this.#isLoggingOut = false;
    });
  }
}
