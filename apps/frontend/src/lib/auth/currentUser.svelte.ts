import {
  getCurrentUserViaConnect,
  type CurrentUser,
  type ViewerAPIConfig
} from '$lib/api-client/viewer';
import { browserCookieAuthenticationHeaders } from './authenticationMode';
import { csrfFetch } from './csrf';
import { isAuthenticationRequiredError } from './errors';
import { getOriginViewer } from './originViewer';
import { isExplicitSignOutRedirectInProgress } from './signOut';

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
  /** Identity confirmed by the latest successful viewer request, excluding a disk view. */
  verifiedUserId = $state<string | null>(null);
  #cookieAuth: boolean;
  #apiConfig?: ViewerAPIConfig;
  #loadCurrentUser: (config: ViewerAPIConfig) => Promise<CurrentUser>;
  #onAuthenticationRequired?: () => void;
  #loadPromise: Promise<void> | null = null;
  #generation = 0;
  #onLoaded?: (user: CurrentUser) => void;
  #isLoggingOut = false;

  constructor(
    cookieAuth: boolean = false,
    apiConfig?: ViewerAPIConfig,
    loadCurrentUser = cookieAuth ? getOriginViewer : getCurrentUserViaConnect,
    onAuthenticationRequired?: () => void,
    onLoaded?: (user: CurrentUser) => void
  ) {
    this.#cookieAuth = cookieAuth;
    this.#apiConfig = apiConfig;
    this.#loadCurrentUser = loadCurrentUser;
    this.#onAuthenticationRequired = onAuthenticationRequired;
    this.#onLoaded = onLoaded;
  }

  /** Load the viewer once, sharing an in-flight request between route and store owners. */
  load(): Promise<void> {
    if (this.#loadPromise) return this.#loadPromise;

    this.loading = true;
    const promise = this.#loadViewer(this.#generation).finally(() => {
      if (this.#loadPromise === promise) this.#loadPromise = null;
    });
    this.#loadPromise = promise;
    return promise;
  }

  /** Apply live account data through the registry's identity boundary. False if this owner was replaced. */
  apply(user: CurrentUser): boolean {
    if (this.#onLoaded) this.#onLoaded(user);
    else this.accept(user);
    return this.user?.id === user.id && this.verifiedUserId === user.id;
  }

  /** Publish complete account data after the registry has checked the account boundary. */
  accept(user: CurrentUser): void {
    this.#generation++;
    this.user = user;
    this.verifiedUserId = user.id;
    this.loading = false;
  }

  /** Clear account data and fence requests from a retired session or store. */
  reset(): void {
    this.invalidateVerification();
    this.user = undefined;
  }

  /** Retain display data after auth loss, but reject requests from the previous session. */
  invalidateVerification(): void {
    this.#generation++;
    this.#loadPromise = null;
    this.verifiedUserId = null;
    this.loading = false;
  }

  async #loadViewer(generation: number): Promise<void> {
    const isCurrent = () =>
      generation === this.#generation &&
      !(this.#cookieAuth && isExplicitSignOutRedirectInProgress());
    try {
      if (!this.#apiConfig) {
        throw new Error('current user Connect API config is not configured');
      }
      const user = await this.#loadCurrentUser(this.#apiConfig);
      if (!isCurrent()) return;
      this.apply(user);
    } catch (err) {
      if (!isCurrent()) return;
      if (isAuthenticationRequiredError(err)) {
        this.invalidateVerification();
        // The bearer interceptor already tried a refresh. If that grant was
        // rejected, the registry marked reauthentication required. A 401
        // after a successful refresh is not proof that the session was revoked.
        if (this.#cookieAuth || !this.#apiConfig?.renewBearerToken)
          this.#onAuthenticationRequired?.();
        return;
      }
      // Surface network failures (CORS, DNS, server down) as a console
      // error so unreachable instances are visible in the dev console.
      // Don't throw — the caller treats this as a per-instance soft
      // failure, not a global crash.
      console.error('[auth] failed to load current user', err);
    } finally {
      if (generation === this.#generation) this.loading = false;
    }
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
      console.warn('Remote server auth failure — marking reauthentication required');
      this.invalidateVerification();
      this.#onAuthenticationRequired?.();
      this.loading = false;
      return;
    }

    this.#isLoggingOut = true;

    if (options.revokeServerSession) {
      this.reset();
      await csrfFetch('/auth/browser/logout', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...browserCookieAuthenticationHeaders
        },
        body: '{}'
      }).catch(() => {});
      this.#isLoggingOut = false;
      return;
    }

    console.warn('[auth] handleAuthFailure: marking reauthentication required');
    this.invalidateVerification();
    this.#onAuthenticationRequired?.();

    this.#isLoggingOut = false;
  }
}
