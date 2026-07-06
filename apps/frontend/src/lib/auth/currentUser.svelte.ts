import {
  getCurrentUserViaConnect,
  type CurrentUser,
  type ViewerAPIConfig
} from '$lib/api-client/viewer';
import { clearCachedUser } from './loadAuth';
import { csrfFetch } from './csrf';
import { isAuthenticationRequiredError } from './errors';

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
 * Authentication-required failures are reported to the owning registry/store.
 * The caller decides whether to prompt for reauthentication, revoke a session,
 * or clear local state.
 */
export class CurrentUserState {
  user = $state<CurrentUser | undefined>(undefined);
  loading = $state(true);
  #cookieAuth: boolean;
  #apiConfig?: ViewerAPIConfig;
  #loadCurrentUser: (config: ViewerAPIConfig) => Promise<CurrentUser>;
  #onAuthenticationRequired?: () => void;
  #isLoggingOut = false;

  constructor(
    cookieAuth: boolean = false,
    apiConfig?: ViewerAPIConfig,
    loadCurrentUser = getCurrentUserViaConnect,
    onAuthenticationRequired?: () => void
  ) {
    this.#cookieAuth = cookieAuth;
    this.#apiConfig = apiConfig;
    this.#loadCurrentUser = loadCurrentUser;
    this.#onAuthenticationRequired = onAuthenticationRequired;
  }

  async load() {
    try {
      if (!this.#apiConfig) {
        throw new Error('current user Connect API config is not configured');
      }
      this.user = await this.#loadCurrentUser(this.#apiConfig);
    } catch (err) {
      if (isAuthenticationRequiredError(err)) {
        this.#onAuthenticationRequired?.();
        this.loading = false;
        return;
      }
      // Surface network failures (CORS, DNS, server down) as a console
      // error so unreachable instances are visible in the dev console.
      // Don't throw — the caller treats this as a per-instance soft
      // failure, not a global crash.
      console.error('[auth] failed to load current user', err);
    }

    this.loading = false;
  }

  /**
   * Handle auth failure.
   * Explicit sign-out paths can request server-side session revocation.
   * Auth-expiry paths should use the registry's reauth-required state instead
   * so the app can keep the current shell visible.
   */
  async handleAuthFailure(options: AuthFailureOptions = {}) {
    if (this.#isLoggingOut) return;

    if (!this.#cookieAuth) {
      console.log('Remote server auth failure — marking reauthentication required');
      this.#onAuthenticationRequired?.();
      this.loading = false;
      return;
    }

    this.#isLoggingOut = true;

    if (options.revokeServerSession) {
      await csrfFetch('/auth/logout', { method: 'POST' }).catch(() => {});
      this.user = undefined;
      clearCachedUser();
      this.loading = false;
      this.#isLoggingOut = false;
      return;
    }

    console.warn('[auth] handleAuthFailure: marking reauthentication required');
    this.#onAuthenticationRequired?.();

    this.#isLoggingOut = false;
  }
}
