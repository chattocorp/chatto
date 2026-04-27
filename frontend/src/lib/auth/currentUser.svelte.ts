import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import {
  graphqlClientManager,
  setAuthFailureHandler,
  setSessionValidationHandler
} from '$lib/state/instance/graphqlClient.svelte';
import { createContext } from 'svelte';
import type { Client } from '@urql/svelte';
import { LoadCurrentUserDocument, clearCachedUser, type CurrentUser } from './loadAuth';

export type { CurrentUser };

/**
 * Per-instance current user state. Tracks who the authenticated user is on a
 * given Chatto instance.
 *
 * For the origin instance (isOrigin=true), auth failure redirects to login.
 * For remote instances, auth failure just clears the user state.
 */
export class CurrentUserState {
  user = $state<CurrentUser | undefined>(undefined);
  loading = $state(true);
  #client: Client;
  #isOrigin: boolean;
  #isLoggingOut = false;

  constructor(client: Client, isOrigin: boolean = false) {
    this.#client = client;
    this.#isOrigin = isOrigin;
  }

  async load() {
    const resp = await this.#client.query(LoadCurrentUserDocument, {});

    if (resp.data?.me) {
      this.user = resp.data.me;
    }
    this.loading = false;
  }

  /**
   * Re-validate the session by checking Query.me.
   * If the session has expired, triggers logout and redirect (origin)
   * or clears user state (remote).
   */
  async validateSession() {
    // Don't validate during initial load or if already logging out
    if (this.loading || this.#isLoggingOut) return;

    // Only validate if we think we're logged in
    if (!this.user) return;

    const resp = await this.#client.query(
      LoadCurrentUserDocument,
      {},
      { requestPolicy: 'network-only' } // Always fetch fresh, bypass any cache
    );

    // Network error (e.g., dead TCP connection after sleep) — don't treat as auth failure.
    // The WebSocket reconnect handler will call triggerSessionValidation() again once
    // connectivity is restored.
    if (resp.error?.networkError) {
      console.log('Session validation skipped — network error:', resp.error.networkError.message);
      return;
    }

    // Server responded but me is null — session has genuinely expired
    if (!resp.data?.me) {
      console.warn('[auth] validateSession: server returned me=null — triggering auth failure');
      this.handleAuthFailure();
    } else {
      // Update user data in case it changed (e.g., avatar, display name)
      this.user = resp.data.me;
    }
  }

  /**
   * Handle auth failure.
   * Origin instance: clears session and redirects to login.
   * Remote instance: clears user state (instance becomes unauthenticated).
   */
  async handleAuthFailure() {
    if (this.#isLoggingOut) return;

    if (!this.#isOrigin) {
      // Remote instance: just clear user state, no redirect
      console.log('Remote instance auth failure — clearing user');
      this.user = undefined;
      this.loading = false;
      return;
    }

    // Origin instance: full logout flow
    this.#isLoggingOut = true;

    console.warn('[auth] handleAuthFailure → /: clearing session and redirecting');
    this.user = undefined;

    // Clear the cached user in loadAuth so the next navigation will re-fetch
    clearCachedUser();

    // Store current URL for redirect after login
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
  s.loading = false; // Not loading - we're not fetching
  setCurrentUser(s);
  return s;
}

export async function initCurrentUser() {
  const s = setCurrentUser(
    new CurrentUserState(graphqlClientManager.originClient.client, true)
  );
  await s.load();

  // Register handlers for auth events from GraphQL client
  setAuthFailureHandler(() => s.handleAuthFailure());
  setSessionValidationHandler(() => s.validateSession());

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
 *
 * @example
 * // In +layout.svelte
 * import { initCurrentUserFromData } from '$lib/auth/currentUser.svelte';
 * let { data } = $props();
 * initCurrentUserFromData(data.user);
 */
export function initCurrentUserFromData(user: CurrentUser): CurrentUserState {
  const s = new CurrentUserState(graphqlClientManager.originClient.client, true);
  s.user = user;
  s.loading = false;
  setCurrentUser(s);

  // Register handlers for auth events from GraphQL client
  setAuthFailureHandler(() => s.handleAuthFailure());
  setSessionValidationHandler(() => s.validateSession());

  return s;
}
